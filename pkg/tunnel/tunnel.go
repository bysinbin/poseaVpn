package tunnel

import (
	"fmt"
	"log"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"poseavpn/pkg/config"
	"poseavpn/pkg/dns"
	"poseavpn/pkg/dnsproxy"
	"poseavpn/pkg/netprofile"
	"poseavpn/pkg/transport"
)

// Tunnel represents an active WireGuard tunnel instance.
type Tunnel struct {
	Name           string
	Config         *config.Config
	StartedAt      time.Time
	DNSProxies     []string  // "real DNS → loopback proxy" for DNS servers that overlap another tunnel
	Mappings       []Mapping // real network → virtual prefix, for networks that overlap another tunnel
	device         *device.Device
	tunDev         tun.Device
	fallbackClient *transport.FallbackClient
	dnsProxies     []*dnsproxy.Proxy
	freeLoop       []func()
	releaseVirt    []func()
	reach          []netip.Prefix // what this tunnel really claims in Windows
	mu             sync.Mutex
	running        bool
}

// Reach returns every prefix this tunnel claims in Windows' routing table. A
// remapped network counts as its virtual prefix, not its real one.
func (t *Tunnel) Reach() []netip.Prefix { return t.reach }

func (t *Tunnel) releaseVirtual() {
	for _, f := range t.releaseVirt {
		f()
	}
	t.releaseVirt = nil
}

// Start creates and activates the WireGuard tunnel on Windows.
func Start(adapterName string, cfg *config.Config) (*Tunnel, error) {
	return StartWithOptions(adapterName, cfg, Options{})
}

// StartWithOptions is Start with knowledge of the tunnels that are already running.
func StartWithOptions(adapterName string, cfg *config.Config, opts Options) (*Tunnel, error) {
	t := &Tunnel{
		Name:   adapterName,
		Config: cfg,
	}

	// 1. Check if fallback transport is needed
	var fallbackClient *transport.FallbackClient
	if (cfg.Interface.Transport == config.TransportWSS || cfg.Interface.Transport == config.TransportFallback) && cfg.Interface.FallbackURL != "" {
		fb, err := transport.NewFallbackClient(cfg.Interface.FallbackURL, true)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize fallback client: %w", err)
		}
		if err := fb.Start(); err != nil {
			fb.Close()
			return nil, fmt.Errorf("failed to connect fallback transport: %w", err)
		}
		fallbackClient = fb
		t.fallbackClient = fb

		// Rewrite peer endpoints to point to the local fallback UDP loopback
		for i := range cfg.Peers {
			cfg.Peers[i].Endpoint = fb.LocalEndpoint()
		}
	}

	// 2. Networks that overlap another running tunnel get a virtual prefix
	t.Mappings, t.releaseVirt = planMappings(cfg, opts.OtherReach)
	for _, m := range t.Mappings {
		log.Printf("[PoseaVPN] %s: %s overlaps another tunnel; reach it as %s", adapterName, m.Real, m.Virtual)
	}

	// 3. Create Wintun TUN device
	tunDev, err := tun.CreateTUN(adapterName, cfg.Interface.MTU)
	if err != nil {
		t.releaseVirtual()
		if fallbackClient != nil {
			fallbackClient.Close()
		}
		return nil, fmt.Errorf("failed to create Wintun adapter '%s': %w", adapterName, err)
	}
	t.tunDev = tunDev

	// 4. Create wireguard device. With address mapping, a translating layer sits
	// between Wintun and WireGuard.
	logger := device.NewLogger(device.LogLevelSilent, fmt.Sprintf("[%s] ", adapterName))
	bind := conn.NewDefaultBind()
	var wgTun tun.Device = tunDev
	if len(t.Mappings) > 0 {
		wgTun = &natDevice{Device: tunDev, maps: t.Mappings}
	}
	dev := device.NewDevice(wgTun, bind, logger)
	t.device = dev

	// 5. Generate and apply UAPI configuration
	// A fixed ListenPort may already be taken by another tunnel (multi-tunnel use).
	// A client does not need a fixed port, so fall back to a random one when it is busy.
	if cfg.Interface.ListenPort > 0 && !udpPortFree(cfg.Interface.ListenPort) {
		log.Printf("[PoseaVPN] ListenPort %d is busy for '%s'; using a random port", cfg.Interface.ListenPort, adapterName)
		cfg.Interface.ListenPort = 0
	}

	uapiConf, err := cfg.ToUAPI()
	if err != nil {
		t.Stop()
		return nil, fmt.Errorf("failed to build UAPI config: %w", err)
	}

	if err := dev.IpcSet(uapiConf); err != nil {
		t.Stop()
		return nil, fmt.Errorf("failed to apply UAPI configuration to device: %w", err)
	}

	if err := dev.Up(); err != nil {
		t.Stop()
		return nil, fmt.Errorf("failed to bring device up: %w", err)
	}

	// 5. Configure Windows IP addresses and Routing via winipcfg
	nativeTun, ok := tunDev.(*tun.NativeTun)
	if !ok {
		t.Stop()
		return nil, fmt.Errorf("expected native tun device")
	}

	luid := winipcfg.LUID(nativeTun.LUID())

	// Set IP addresses. When this tunnel's own subnet overlaps another tunnel, the
	// address is added as a host address (/32) so Windows does not also create an
	// on-link route for the whole subnet on this adapter.
	addrs := make([]netip.Prefix, 0, len(cfg.Interface.Addresses))
	var reach []netip.Prefix
	for _, a := range cfg.Interface.Addresses {
		if a.Addr().Is4() && a.Bits() < 32 && overlapsAny(a.Masked(), opts.OtherReach) {
			log.Printf("[PoseaVPN] %s: subnet %s overlaps another tunnel; using %s/32 only", adapterName, a.Masked(), a.Addr())
			a = netip.PrefixFrom(a.Addr(), 32)
		}
		addrs = append(addrs, a)
		reach = append(reach, a.Masked())
	}
	if err := luid.SetIPAddresses(addrs); err != nil {
		t.Stop()
		return nil, fmt.Errorf("failed to set IP addresses: %w", err)
	}

	// Configure Routes for AllowedIPs. A remapped network is routed as its virtual
	// prefix; the real prefix belongs to the tunnel that connected first.
	// Routes are added one by one (no flush) so a failure on an unrelated address
	// family cannot silently skip the remaining routes.
	virtualOfReal := make(map[netip.Prefix]netip.Prefix, len(t.Mappings))
	for _, m := range t.Mappings {
		virtualOfReal[m.Real] = m.Virtual
	}
	for _, peer := range cfg.Peers {
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
				t.Stop()
				return nil, fmt.Errorf("failed to add route %s: %w", route, err)
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

	// 6. Configure DNS & Active Directory NRPT rules.
	// A DNS server that overlaps another running tunnel would be reached through
	// that other tunnel by Windows, and a server inside a remapped network answers
	// with real addresses. Both go through a per-tunnel proxy that is forced onto
	// this tunnel's adapter and translates the answers.
	if len(cfg.Interface.DNS) > 0 || len(cfg.Interface.DNSSearches) > 0 {
		servers := cfg.Interface.DNS
		if ipif, err := luid.IPInterface(windows.AF_INET); err == nil {
			servers = t.setupDNSProxies(servers, ipif.InterfaceIndex, opts.OtherReach, t.Mappings)
		}
		_ = dns.ConfigureDNS(adapterName, servers, cfg.Interface.DNSSearches)
	}

	// 7. Active Directory Fix: Set Network Profile to Private
	// Wait a tiny moment for Windows Network List Manager to index the new adapter
	go func() {
		time.Sleep(1 * time.Second)
		_ = netprofile.SetInterfaceCategory(adapterName, netprofile.CategoryPrivate)
	}()

	t.StartedAt = time.Now()
	t.running = true
	log.Printf("[PoseaVPN] Tunnel '%s' successfully established.", adapterName)

	return t, nil
}

// Stop shuts down the tunnel, removes routes and restores network configuration.
func (t *Tunnel) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running && t.device == nil && t.tunDev == nil {
		return nil
	}
	t.running = false

	// Clean up AD NRPT rules of this tunnel only
	_ = dns.CleanUpNRPT(t.Name)
	t.stopDNSProxies()
	t.releaseVirtual()

	// Close device
	if t.device != nil {
		t.device.Close()
		t.device = nil
	}

	// Close TUN
	if t.tunDev != nil {
		_ = t.tunDev.Close()
		t.tunDev = nil
	}

	// Close Fallback client if running
	if t.fallbackClient != nil {
		_ = t.fallbackClient.Close()
		t.fallbackClient = nil
	}

	log.Printf("[PoseaVPN] Tunnel '%s' stopped.", t.Name)
	return nil
}

// IsRunning reports whether the tunnel is active.
func (t *Tunnel) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// TunnelName returns the tunnel adapter name.
func (t *Tunnel) TunnelName() string { return t.Name }

// Proto returns "wireguard".
func (t *Tunnel) Proto() string { return "wireguard" }

// StartedTime returns when the tunnel was started.
func (t *Tunnel) StartedTime() time.Time { return t.StartedAt }

// Status returns the current StatusInfo for this WireGuard tunnel.
func (t *Tunnel) Status() StatusInfo {
	info := StatusInfo{
		Active:      t.IsRunning(),
		TunnelName:  t.Name,
		Transport:   string(t.Config.Interface.Transport),
		ConnectedAt: t.StartedAt.Unix(),
		DNSProxies:  t.DNSProxies,
		Proto:       "wireguard",
	}
	for _, m := range t.Mappings {
		info.Mappings = append(info.Mappings, m.String())
	}
	for _, a := range t.Config.Interface.Addresses {
		info.Addresses = append(info.Addresses, a.String())
	}
	for _, p := range t.Config.Peers {
		if p.Endpoint != "" {
			info.Endpoints = append(info.Endpoints, p.Endpoint)
		}
		for _, ip := range p.AllowedIPs {
			info.AllowedIPs = append(info.AllowedIPs, ip.String())
		}
	}
	stats := t.Stats()
	info.RxBytes = stats.RxBytes
	info.TxBytes = stats.TxBytes
	if !stats.LastHandshake.IsZero() {
		info.LastHandshake = stats.LastHandshake.Unix()
	}
	return info
}
