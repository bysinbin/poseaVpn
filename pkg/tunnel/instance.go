package tunnel

import (
	"net/netip"
	"time"
)

// StatusInfo represents the current state of one active tunnel.
type StatusInfo struct {
	Active        bool     `json:"active"`
	TunnelName    string   `json:"tunnel_name,omitempty"`
	Addresses     []string `json:"addresses,omitempty"`
	Endpoints     []string `json:"endpoints,omitempty"`
	AllowedIPs    []string `json:"allowed_ips,omitempty"`
	Transport     string   `json:"transport,omitempty"`
	ConnectedAt   int64    `json:"connected_at,omitempty"`   // unix seconds
	LastHandshake int64    `json:"last_handshake,omitempty"` // unix seconds, 0 = none yet
	RxBytes       uint64   `json:"rx_bytes,omitempty"`
	TxBytes       uint64   `json:"tx_bytes,omitempty"`
	Proto         string   `json:"proto,omitempty"` // "wireguard" or "openvpn"
	DNSProxies    []string `json:"dns_proxies,omitempty"`
	Mappings      []string `json:"mappings,omitempty"`
}

// Instance is the common interface implemented by both WireGuard and OpenVPN tunnels.
type Instance interface {
	TunnelName() string
	Proto() string
	IsRunning() bool
	Stop() error
	Status() StatusInfo
	Reach() []netip.Prefix
	StartedTime() time.Time
}
