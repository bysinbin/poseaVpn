package config

import (
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	rawConf := `
[Interface]
PrivateKey = aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
Address = 10.10.10.5/24, 192.168.100.2
DNS = 10.10.10.1, corp.local
MTU = 1400
Transport = auto
FallbackURL = wss://vpn.example.com/wg

[Peer]
PublicKey = bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb=
Endpoint = 203.0.113.1:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

	cfg, err := Parse(strings.NewReader(rawConf))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.Interface.MTU != 1400 {
		t.Errorf("Expected MTU 1400, got %d", cfg.Interface.MTU)
	}
	if len(cfg.Interface.Addresses) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(cfg.Interface.Addresses))
	}
	if len(cfg.Interface.DNS) != 1 {
		t.Errorf("Expected 1 IP DNS, got %d", len(cfg.Interface.DNS))
	}
	if len(cfg.Interface.DNSSearches) != 1 || cfg.Interface.DNSSearches[0] != "corp.local" {
		t.Errorf("Expected corp.local search suffix, got %v", cfg.Interface.DNSSearches)
	}
	if cfg.Interface.Transport != TransportFallback {
		t.Errorf("Expected TransportFallback, got %s", cfg.Interface.Transport)
	}
	if len(cfg.Peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(cfg.Peers))
	}
	if cfg.Peers[0].PersistentKeepalive != 25 {
		t.Errorf("Expected keepalive 25, got %d", cfg.Peers[0].PersistentKeepalive)
	}
}
