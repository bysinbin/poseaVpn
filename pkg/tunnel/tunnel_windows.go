//go:build windows

package tunnel

import (
	"fmt"
	"log"
	"net/netip"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"poseavpn/pkg/dns"
	"poseavpn/pkg/netprofile"
)

func createTUN(adapterName string, mtu int) (tun.Device, string, error) {
	tunDev, err := tun.CreateTUN(adapterName, mtu)
	if err != nil {
		return nil, "", err
	}
	return tunDev, adapterName, nil
}

func configureOSNetwork(t *Tunnel, opts Options) error {
	nativeTun, ok := t.tunDev.(*tun.NativeTun)
	if !ok {
		return fmt.Errorf("expected native tun device")
	}

	luid := winipcfg.LUID(nativeTun.LUID())

	// Set IP addresses. Subnet overlap with another tunnel uses /32
	addrs := make([]netip.Prefix, 0, len(t.Config.Interface.Addresses))
	var reach []netip.Prefix
	for _, a := range t.Config.Interface.Addresses {
		if a.Addr().Is4() && a.Bits() < 32 && overlapsAny(a.Masked(), opts.OtherReach) {
			log.Printf("[PoseaVPN] %s: subnet %s overlaps another tunnel; using %s/32 only", t.Name, a.Masked(), a.Addr())
			a = netip.PrefixFrom(a.Addr(), 32)
		}
		addrs = append(addrs, a)
		reach = append(reach, a.Masked())
	}
	if err := luid.SetIPAddresses(addrs); err != nil {
		return fmt.Errorf("failed to set IP addresses: %w", err)
	}

	// Configure Routes for AllowedIPs
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
			nextHop := netip.IPv4Unspecified()
			if route.Addr().Is6() {
				nextHop = netip.IPv6Unspecified()
			}
			if err := luid.AddRoute(route, nextHop, 0); err != nil && err != windows.ERROR_OBJECT_ALREADY_EXISTS {
				return fmt.Errorf("failed to add route %s: %w", route, err)
			}
			reach = append(reach, route)
			log.Printf("[PoseaVPN] Route added: %s", route)
		}
	}
	t.reach = reach

	// Lower interface metric so tunnel routes win over overlapping local routes
	if ipif, err := luid.IPInterface(windows.AF_INET); err == nil {
		ipif.UseAutomaticMetric = false
		ipif.Metric = 5
		ipif.RouterDiscoveryBehavior = winipcfg.RouterDiscoveryDisabled
		ipif.DadTransmits = 0
		_ = ipif.Set()
	}

	// Configure DNS & Active Directory NRPT rules
	if len(t.Config.Interface.DNS) > 0 || len(t.Config.Interface.DNSSearches) > 0 {
		servers := t.Config.Interface.DNS
		if ipif, err := luid.IPInterface(windows.AF_INET); err == nil {
			servers = t.setupDNSProxies(servers, ipif.InterfaceIndex, opts.OtherReach, t.Mappings)
		}
		_ = dns.ConfigureDNS(t.Name, servers, t.Config.Interface.DNSSearches)
	}

	// Active Directory Fix: Set Network Profile to Private
	go func() {
		time.Sleep(1 * time.Second)
		_ = netprofile.SetInterfaceCategory(t.Name, netprofile.CategoryPrivate)
	}()

	return nil
}

func teardownOSNetwork(t *Tunnel) {
	_ = dns.CleanUpNRPT(t.Name)
}
