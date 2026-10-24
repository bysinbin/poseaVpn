//go:build darwin

package tunnel

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"

	"golang.zx2c4.com/wireguard/tun"

	"poseavpn/pkg/dns"
)

func createTUN(adapterName string, mtu int) (tun.Device, string, error) {
	// On macOS, WireGuard creates a native utun interface
	tunDev, err := tun.CreateTUN("utun", mtu)
	if err != nil {
		return nil, "", err
	}
	actualName, err := tunDev.Name()
	if err != nil {
		_ = tunDev.Close()
		return nil, "", fmt.Errorf("failed to retrieve utun interface name: %w", err)
	}
	return tunDev, actualName, nil
}

// getDefaultGateways returns default IPv4 and IPv6 gateways and their network interfaces.
func getDefaultGateways() (gw4, iface4, gw6, iface6 string) {
	if out, err := exec.Command("netstat", "-nr", "-f", "inet").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == "default" {
				gw := fields[1]
				if !strings.HasPrefix(gw, "link#") {
					gw4 = gw
				}
				iface4 = fields[3]
				break
			}
		}
	}
	if out, err := exec.Command("netstat", "-nr", "-f", "inet6").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == "default" {
				gw := fields[1]
				if !strings.HasPrefix(gw, "link#") {
					gw6 = gw
				}
				iface6 = fields[3]
				break
			}
		}
	}
	return
}

func extractEndpointIPs(endpoint string) []net.IP {
	if endpoint == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	return ips
}

func configureOSNetwork(t *Tunnel, opts Options) error {
	var ifIndex uint32
	if iface, err := net.InterfaceByName(t.ActualName); err == nil {
		ifIndex = uint32(iface.Index)
	}

	// 1. Assign IP addresses to utun interface
	var reach []netip.Prefix
	for _, a := range t.Config.Interface.Addresses {
		if a.Addr().Is4() {
			if a.Bits() < 32 && overlapsAny(a.Masked(), opts.OtherReach) {
				log.Printf("[PoseaVPN] %s: subnet %s overlaps another tunnel; using %s/32 only", t.Name, a.Masked(), a.Addr())
				a = netip.PrefixFrom(a.Addr(), 32)
			}
			ipStr := a.Addr().String()
			cidrStr := a.String()
			// Darwin utun address assignment syntax: ifconfig <utun> inet <cidr> <ip> alias
			cmd := exec.Command("ifconfig", t.ActualName, "inet", cidrStr, ipStr, "alias")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to set IP %s on %s: %w (%s)", cidrStr, t.ActualName, err, strings.TrimSpace(string(out)))
			}
		} else if a.Addr().Is6() {
			cmd := exec.Command("ifconfig", t.ActualName, "inet6", a.String(), "alias")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to set IPv6 %s on %s: %w (%s)", a, t.ActualName, err, strings.TrimSpace(string(out)))
			}
		}
		reach = append(reach, a.Masked())
	}

	// Bring interface up and set MTU
	if t.Config.Interface.MTU > 0 {
		_ = exec.Command("ifconfig", t.ActualName, "mtu", strconv.Itoa(t.Config.Interface.MTU), "up").Run()
	} else {
		_ = exec.Command("ifconfig", t.ActualName, "up").Run()
	}

	// 2. Identify if default routing (0.0.0.0/0 or ::/0) is requested
	virtualOfReal := make(map[netip.Prefix]netip.Prefix, len(t.Mappings))
	for _, m := range t.Mappings {
		virtualOfReal[m.Real] = m.Virtual
	}

	hasDefaultV4 := false
	hasDefaultV6 := false
	for _, peer := range t.Config.Peers {
		for _, allowed := range peer.AllowedIPs {
			route := allowed.Masked()
			if v, ok := virtualOfReal[route]; ok {
				route = v
			}
			if route.Addr().Is4() && route.Bits() == 0 {
				hasDefaultV4 = true
			} else if route.Addr().Is6() && route.Bits() == 0 {
				hasDefaultV6 = true
			}
		}
	}

	// 3. For full tunnel routing, add direct host routes to peer endpoints via physical gateway
	// to prevent recursive routing loops (which freeze traffic and peg CPU at 100%).
	gw4, iface4, gw6, iface6 := getDefaultGateways()
	if hasDefaultV4 || hasDefaultV6 {
		for _, peer := range t.Config.Peers {
			for _, epIP := range extractEndpointIPs(peer.Endpoint) {
				epStr := epIP.String()
				if epIP.To4() != nil && hasDefaultV4 {
					var cmd *exec.Cmd
					if gw4 != "" {
						cmd = exec.Command("route", "-q", "-n", "add", "-host", epStr, gw4)
					} else if iface4 != "" {
						cmd = exec.Command("route", "-q", "-n", "add", "-host", epStr, "-interface", iface4)
					}
					if cmd != nil {
						if out, err := cmd.CombinedOutput(); err != nil {
							log.Printf("[PoseaVPN] Warning: host route for endpoint %s failed: %v (%s)", epStr, err, strings.TrimSpace(string(out)))
						} else {
							log.Printf("[PoseaVPN] Direct route added for endpoint: %s via %s (%s)", epStr, gw4, iface4)
							t.endpointRoutes = append(t.endpointRoutes, epStr)
						}
					}
				} else if epIP.To4() == nil && hasDefaultV6 {
					var cmd *exec.Cmd
					if gw6 != "" {
						cmd = exec.Command("route", "-q", "-n", "add", "-inet6", "-host", epStr, gw6)
					} else if iface6 != "" {
						cmd = exec.Command("route", "-q", "-n", "add", "-inet6", "-host", epStr, "-interface", iface6)
					}
					if cmd != nil {
						if out, err := cmd.CombinedOutput(); err != nil {
							log.Printf("[PoseaVPN] Warning: host route for IPv6 endpoint %s failed: %v (%s)", epStr, err, strings.TrimSpace(string(out)))
						} else {
							log.Printf("[PoseaVPN] Direct route added for IPv6 endpoint: %s via %s (%s)", epStr, gw6, iface6)
							t.endpointRoutes = append(t.endpointRoutes, epStr)
						}
					}
				}
			}
		}
	}

	// 4. Configure routing for AllowedIPs
	for _, peer := range t.Config.Peers {
		for _, allowed := range peer.AllowedIPs {
			route := allowed.Masked()
			if v, ok := virtualOfReal[route]; ok {
				route = v
			}

			if route.Addr().Is4() {
				if route.Bits() == 0 {
					for _, half := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
						cmd := exec.Command("route", "-q", "-n", "add", "-inet", half, "-interface", t.ActualName)
						if out, err := cmd.CombinedOutput(); err != nil {
							log.Printf("[PoseaVPN] Warning: route add %s via %s failed: %v (%s)", half, t.ActualName, err, strings.TrimSpace(string(out)))
						} else {
							t.addedRoutes = append(t.addedRoutes, half)
						}
					}
				} else {
					cmd := exec.Command("route", "-q", "-n", "add", "-inet", route.String(), "-interface", t.ActualName)
					if out, err := cmd.CombinedOutput(); err != nil {
						log.Printf("[PoseaVPN] Warning: route add %s via %s failed: %v (%s)", route, t.ActualName, err, strings.TrimSpace(string(out)))
					} else {
						t.addedRoutes = append(t.addedRoutes, route.String())
					}
				}
			} else if route.Addr().Is6() {
				if route.Bits() == 0 {
					for _, half := range []string{"::/1", "8000::/1"} {
						cmd := exec.Command("route", "-q", "-n", "add", "-inet6", half, "-interface", t.ActualName)
						if out, err := cmd.CombinedOutput(); err != nil {
							log.Printf("[PoseaVPN] Warning: route add %s via %s failed: %v (%s)", half, t.ActualName, err, strings.TrimSpace(string(out)))
						} else {
							t.addedRoutes = append(t.addedRoutes, half)
						}
					}
				} else {
					cmd := exec.Command("route", "-q", "-n", "add", "-inet6", route.String(), "-interface", t.ActualName)
					if out, err := cmd.CombinedOutput(); err != nil {
						log.Printf("[PoseaVPN] Warning: route add %s via %s failed: %v (%s)", route, t.ActualName, err, strings.TrimSpace(string(out)))
					} else {
						t.addedRoutes = append(t.addedRoutes, route.String())
					}
				}
			}

			reach = append(reach, route)
			log.Printf("[PoseaVPN] Route added: %s via %s", route, t.ActualName)
		}
	}
	t.reach = reach

	// 5. Configure DNS & search domains
	if len(t.Config.Interface.DNS) > 0 || len(t.Config.Interface.DNSSearches) > 0 {
		servers := t.Config.Interface.DNS
		if ifIndex > 0 {
			servers = t.setupDNSProxies(servers, ifIndex, opts.OtherReach, t.Mappings)
		}
		_ = dns.ConfigureDNS(t.ActualName, servers, t.Config.Interface.DNSSearches)
	}

	return nil
}

func teardownOSNetwork(t *Tunnel) {
	// 1. Delete explicit host routes for endpoints
	for _, ep := range t.endpointRoutes {
		cmd := exec.Command("route", "-q", "-n", "delete", "-host", ep)
		_ = cmd.Run()
		log.Printf("[PoseaVPN] Deleted endpoint route: %s", ep)
	}
	t.endpointRoutes = nil

	// 2. Delete added tunnel routes
	for _, r := range t.addedRoutes {
		var cmd *exec.Cmd
		if strings.Contains(r, ":") {
			cmd = exec.Command("route", "-q", "-n", "delete", "-inet6", r, "-interface", t.ActualName)
		} else {
			cmd = exec.Command("route", "-q", "-n", "delete", "-inet", r, "-interface", t.ActualName)
		}
		_ = cmd.Run()
	}
	t.addedRoutes = nil

	// 3. Clean up DNS
	_ = dns.CleanUpNRPT(t.ActualName)
}
