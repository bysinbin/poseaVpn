package tunnel

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"

	"poseavpn/pkg/config"
	"poseavpn/pkg/dnsproxy"
)

// Options carries information about the tunnels that are already running, so a
// new tunnel can avoid clashing with them.
type Options struct {
	// OtherReach is every prefix the other running tunnels really claim in Windows
	// (after their own address mapping, so a remapped network counts as its virtual prefix).
	OtherReach []netip.Prefix
}

// ---------------------------------------------------------------- loopback addresses

// Each DNS proxy listens on its own private loopback address 127.77.<n>.1.
var (
	loopMu   sync.Mutex
	loopUsed = map[int]bool{}
)

func allocLoopback() (netip.Addr, func(), error) {
	loopMu.Lock()
	defer loopMu.Unlock()
	for n := 1; n <= 250; n++ {
		if loopUsed[n] {
			continue
		}
		loopUsed[n] = true
		addr := netip.AddrFrom4([4]byte{127, 77, byte(n), 1})
		return addr, func() {
			loopMu.Lock()
			delete(loopUsed, n)
			loopMu.Unlock()
		}, nil
	}
	return netip.Addr{}, nil, fmt.Errorf("no free loopback address for the DNS proxy")
}

// ---------------------------------------------------------------- conflict detection

// reachPrefixes lists every prefix a tunnel makes reachable: its own on-link
// subnets and all AllowedIPs.
func reachPrefixes(cfg *config.Config) []netip.Prefix {
	var out []netip.Prefix
	for _, a := range cfg.Interface.Addresses {
		out = append(out, a.Masked())
	}
	for _, p := range cfg.Peers {
		for _, ip := range p.AllowedIPs {
			out = append(out, ip.Masked())
		}
	}
	return out
}

// bestMatch returns the length of the longest prefix containing ip (-1 = none).
func bestMatch(ip netip.Addr, prefixes []netip.Prefix) int {
	best := -1
	for _, p := range prefixes {
		if p.Contains(ip) && p.Bits() > best {
			best = p.Bits()
		}
	}
	return best
}

// dnsNeedsProxy reports whether Windows would send a query for ip to another,
// already running tunnel instead of this one. Windows uses the longest matching
// prefix and, on a tie, the tunnel that was connected first.
func dnsNeedsProxy(ip netip.Addr, mine *config.Config, otherReach []netip.Prefix) bool {
	other := bestMatch(ip, otherReach)
	if other < 0 {
		return false // nobody else claims this address
	}
	return other >= bestMatch(ip, reachPrefixes(mine))
}

// virtualOf returns the virtual address that stands for a real address of a remapped network.
func virtualOf(a netip.Addr, maps []Mapping) (netip.Addr, bool) {
	if !a.Is4() {
		return a, false
	}
	t, ok := translate(a.As4(), maps, false) // real -> virtual
	if !ok {
		return a, false
	}
	return netip.AddrFrom4(t), true
}

// setupDNSProxies starts a forced-interface DNS proxy for every DNS server of this
// tunnel that Windows could not reach by routing (it overlaps another tunnel) or
// that lives in a remapped network (its answers must be translated), and returns
// the server list to configure on the adapter.
func (t *Tunnel) setupDNSProxies(servers []net.IP, ifIndex uint32, otherReach []netip.Prefix, maps []Mapping) []net.IP {
	if (len(otherReach) == 0 && len(maps) == 0) || len(servers) == 0 {
		return servers
	}

	var src netip.Addr
	for _, a := range t.Config.Interface.Addresses {
		if a.Addr().Is4() {
			src = a.Addr()
			break
		}
	}
	if !src.IsValid() {
		return servers
	}

	var mapA func(netip.Addr) (netip.Addr, bool)
	if len(maps) > 0 {
		mapA = func(a netip.Addr) (netip.Addr, bool) { return virtualOf(a, maps) }
	}

	out := make([]net.IP, len(servers))
	copy(out, servers)
	for i, s := range servers {
		up, ok := netip.AddrFromSlice(s)
		if !ok {
			continue
		}
		up = up.Unmap()
		if !up.Is4() {
			continue
		}
		dial := up
		if v, mapped := virtualOf(up, maps); mapped {
			// The real address is not routed to this adapter (only its virtual
			// prefix is), so the proxy talks to the virtual address and the
			// translation layer turns it into the real one.
			dial = v
		} else if !dnsNeedsProxy(up, t.Config, otherReach) && mapA == nil {
			continue
		}

		loop, release, err := allocLoopback()
		if err != nil {
			log.Printf("[PoseaVPN] %s: %v", t.Name, err)
			continue
		}
		p, err := dnsproxy.Start(loop, dial, src, ifIndex, mapA)
		if err != nil {
			release()
			log.Printf("[PoseaVPN] %s: DNS proxy for %s failed: %v (using the address directly)", t.Name, up, err)
			continue
		}
		t.dnsProxies = append(t.dnsProxies, p)
		t.freeLoop = append(t.freeLoop, release)
		t.DNSProxies = append(t.DNSProxies, fmt.Sprintf("%s → %s", up, loop))
		out[i] = net.IP(loop.AsSlice())
		log.Printf("[PoseaVPN] %s: DNS %s uses proxy %s", t.Name, up, loop)
	}
	return out
}

func (t *Tunnel) stopDNSProxies() {
	for _, p := range t.dnsProxies {
		p.Close()
	}
	for _, f := range t.freeLoop {
		f()
	}
	t.dnsProxies, t.freeLoop = nil, nil
}
