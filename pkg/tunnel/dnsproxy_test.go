package tunnel

import (
	"net/netip"
	"strings"
	"testing"

	"poseavpn/pkg/config"
)

func cfgOf(addr string, routes ...string) *config.Config {
	c := &config.Config{}
	c.Interface.Addresses = []netip.Prefix{netip.MustParsePrefix(addr)}
	p := config.PeerConfig{}
	for _, r := range routes {
		p.AllowedIPs = append(p.AllowedIPs, netip.MustParsePrefix(r))
	}
	c.Peers = []config.PeerConfig{p}
	return c
}

func reachOfAll(cfgs []*config.Config) []netip.Prefix {
	var out []netip.Prefix
	for _, c := range cfgs {
		out = append(out, reachPrefixes(c)...)
	}
	return out
}

func TestDNSNeedsProxy(t *testing.T) {
	// sample simulated configs
	tunnelAlpha := cfgOf("10.0.0.3/24", "10.0.0.0/24", "192.168.0.0/22")
	tunnelBeta := cfgOf("10.0.0.2/24", "10.0.0.0/24", "192.168.230.0/24")
	tunnelGamma := cfgOf("20.0.0.2/24", "20.0.0.0/24", "192.168.0.0/22")
	tunnelDelta := cfgOf("10.0.60.2/24", "10.0.60.0/24", "192.168.30.0/24")

	cases := []struct {
		name   string
		dns    string
		mine   *config.Config
		others []*config.Config
		want   bool
	}{
		{"tunnelBeta DNS 10.0.0.1 while tunnelAlpha (same VPN subnet) is up", "10.0.0.1", tunnelBeta, []*config.Config{tunnelAlpha}, true},
		{"tunnelGamma DNS 192.168.1.254 inside tunnelAlpha LAN range", "192.168.1.254", tunnelGamma, []*config.Config{tunnelAlpha}, true},
		{"no tunnel running", "10.0.0.1", tunnelBeta, nil, false},
		{"tunnelDelta DNS 10.0.60.1 does not overlap tunnelAlpha", "10.0.60.1", tunnelDelta, []*config.Config{tunnelAlpha}, false},
		{"tunnelAlpha DNS 192.168.2.222 with only tunnelDelta running", "192.168.2.222", tunnelAlpha, []*config.Config{tunnelDelta}, false},
		{"two other tunnels, one overlaps", "10.0.0.1", tunnelBeta, []*config.Config{tunnelDelta, tunnelAlpha}, true},
		{
			// this tunnel has a more specific route for its DNS: it wins by longest prefix
			"more specific own route wins", "192.168.1.254",
			cfgOf("20.0.0.2/24", "20.0.0.0/24", "192.168.1.254/32"), []*config.Config{tunnelAlpha}, false,
		},
	}
	for _, c := range cases {
		got := dnsNeedsProxy(netip.MustParseAddr(c.dns), c.mine, reachOfAll(c.others))
		if got != c.want {
			t.Errorf("%s: dnsNeedsProxy(%s) = %v, want %v", c.name, c.dns, got, c.want)
		}
	}
}

func TestLoopbackAllocator(t *testing.T) {
	a1, free1, err := allocLoopback()
	if err != nil {
		t.Fatal(err)
	}
	a2, free2, err := allocLoopback()
	if err != nil {
		t.Fatal(err)
	}
	if a1 == a2 {
		t.Fatalf("two proxies got the same loopback address %s", a1)
	}
	if !strings.HasPrefix(a1.String(), "127.77.") || !a1.IsLoopback() {
		t.Fatalf("unexpected loopback address %s", a1)
	}
	free1()
	a3, free3, _ := allocLoopback()
	if a3 != a1 {
		t.Fatalf("freed address %s was not reused (got %s)", a1, a3)
	}
	free2()
	free3()
}
