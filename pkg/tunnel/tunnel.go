package tunnel

import (
	"fmt"
	"log"
	"net/netip"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"

	"poseavpn/pkg/config"
	"poseavpn/pkg/dnsproxy"
	"poseavpn/pkg/transport"
)

// Tunnel represents an active WireGuard tunnel instance.
type Tunnel struct {
	Name           string
	ActualName     string    // OS interface name (e.g. utun3 on Darwin, adapterName on Windows)
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
	reach          []netip.Prefix // what this tunnel really claims in the OS
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

	// 3. Create TUN device
	tunDev, actualName, err := createTUN(adapterName, cfg.Interface.MTU)
	if err != nil {
		t.releaseVirtual()
		if fallbackClient != nil {
			fallbackClient.Close()
		}
		return nil, fmt.Errorf("failed to create network adapter '%s': %w", adapterName, err)
	}
	t.tunDev = tunDev
	t.ActualName = actualName

	// 4. Create wireguard device. With address mapping, a translating layer sits
	// between TUN and WireGuard.
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

	// 6. Configure OS IP addresses, routing, and DNS
	if err := configureOSNetwork(t, opts); err != nil {
		t.Stop()
		return nil, err
	}

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

	// Clean up DNS and OS network rules
	teardownOSNetwork(t)
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
