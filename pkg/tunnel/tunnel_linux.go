//go:build linux

package tunnel

import (
	"fmt"
	"log"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"

	"golang.zx2c4.com/wireguard/tun"
)

func createTUN(adapterName string, mtu int) (tun.Device, string, error) {
	tunDev, err := tun.CreateTUN(adapterName, mtu)
	if err != nil {
		return nil, "", err
	}
	actualName, err := tunDev.Name()
	if err != nil {
		_ = tunDev.Close()
		return nil, "", fmt.Errorf("failed to retrieve tun interface name: %w", err)
	}
	return tunDev, actualName, nil
}

func configureOSNetwork(t *Tunnel, opts Options) error {
	var reach []netip.Prefix
	for _, a := range t.Config.Interface.Addresses {
		if a.Addr().Is4() {
			if a.Bits() < 32 && overlapsAny(a.Masked(), opts.OtherReach) {
				log.Printf("[PoseaVPN] %s: subnet %s overlaps another tunnel; using %s/32 only", t.Name, a.Masked(), a.Addr())
				a = netip.PrefixFrom(a.Addr(), 32)
			}
			cidr := a.String()
			cmd := exec.Command("ip", "addr", "add", cidr, "dev", t.ActualName)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to add IP %s on %s: %w (%s)", cidr, t.ActualName, err, strings.TrimSpace(string(out)))
			}
		} else if a.Addr().Is6() {
			cmd := exec.Command("ip", "-6", "addr", "add", a.String(), "dev", t.ActualName)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to add IPv6 %s on %s: %w (%s)", a, t.ActualName, err, strings.TrimSpace(string(out)))
			}
		}
		reach = append(reach, a.Masked())
	}

	// Set MTU and bring up
	if t.Config.Interface.MTU > 0 {
		_ = exec.Command("ip", "link", "set", "dev", t.ActualName, "mtu", strconv.Itoa(t.Config.Interface.MTU), "up").Run()
	} else {
		_ = exec.Command("ip", "link", "set", "dev", t.ActualName, "up").Run()
	}

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
				cmd := exec.Command("ip", "route", "add", route.String(), "dev", t.ActualName)
				_ = cmd.Run()
			} else if route.Addr().Is6() {
				cmd := exec.Command("ip", "-6", "route", "add", route.String(), "dev", t.ActualName)
				_ = cmd.Run()
			}

			reach = append(reach, route)
			log.Printf("[PoseaVPN] Route added: %s via %s", route, t.ActualName)
		}
	}
	t.reach = reach
	return nil
}

func teardownOSNetwork(t *Tunnel) {}
