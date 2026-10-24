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

func configureOSNetwork(t *Tunnel, opts Options) error {
	var ifIndex uint32
	if iface, err := net.InterfaceByName(t.ActualName); err == nil {
		ifIndex = uint32(iface.Index)
	}

	// 1. Assign IP addresses to utun interface
	var reach []netip.Prefix
	firstV4 := true
	for _, a := range t.Config.Interface.Addresses {
		if a.Addr().Is4() {
			if a.Bits() < 32 && overlapsAny(a.Masked(), opts.OtherReach) {
				log.Printf("[PoseaVPN] %s: subnet %s overlaps another tunnel; using %s/32 only", t.Name, a.Masked(), a.Addr())
				a = netip.PrefixFrom(a.Addr(), 32)
			}
			ipStr := a.Addr().String()
			var cmd *exec.Cmd
			if firstV4 {
				cmd = exec.Command("ifconfig", t.ActualName, "inet", ipStr, ipStr, "netmask", "255.255.255.255")
				firstV4 = false
			} else {
				cmd = exec.Command("ifconfig", t.ActualName, "alias", ipStr, ipStr, "netmask", "255.255.255.255")
			}
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to set IP %s on %s: %w (%s)", ipStr, t.ActualName, err, strings.TrimSpace(string(out)))
			}
		} else if a.Addr().Is6() {
			ipStr := a.Addr().String()
			cmd := exec.Command("ifconfig", t.ActualName, "inet6", ipStr, "prefixlen", strconv.Itoa(a.Bits()))
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

	// 2. Configure routing for AllowedIPs
	virtualOfReal := make(map[netip.Prefix]netip.Prefix, len(t.Mappings))
	for _, m := range t.Mappings {
		virtualOfReal[m.Real] = m.Virtual
	}

	for _, peer := range t.Config.Peers {
		for _, allowed := range peer.AllowedIPs {
			route := allowed.Masked()
			if v, ok := virtualOfReal[route]; ok {
				route = v
			}

			if route.Addr().Is4() {
				if route.Bits() == 0 {
					_ = exec.Command("route", "-q", "-n", "add", "-inet", "0.0.0.0/1", "-interface", t.ActualName).Run()
					_ = exec.Command("route", "-q", "-n", "add", "-inet", "128.0.0.0/1", "-interface", t.ActualName).Run()
				} else if route.Bits() == 32 {
					_ = exec.Command("route", "-q", "-n", "add", "-host", route.Addr().String(), "-interface", t.ActualName).Run()
				} else {
					_ = exec.Command("route", "-q", "-n", "add", "-net", route.String(), "-interface", t.ActualName).Run()
				}
			} else if route.Addr().Is6() {
				if route.Bits() == 0 {
					_ = exec.Command("route", "-q", "-n", "add", "-inet6", "-net", "::/1", "-interface", t.ActualName).Run()
					_ = exec.Command("route", "-q", "-n", "add", "-inet6", "-net", "8000::/1", "-interface", t.ActualName).Run()
				} else {
					_ = exec.Command("route", "-q", "-n", "add", "-inet6", "-net", route.String(), "-interface", t.ActualName).Run()
				}
			}

			reach = append(reach, route)
			log.Printf("[PoseaVPN] Route added: %s via %s", route, t.ActualName)
		}
	}
	t.reach = reach

	// 3. Configure DNS & search domains
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
	_ = dns.CleanUpNRPT(t.ActualName)
}
