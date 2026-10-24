package ovpn

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	raw := `
dev tun
proto udp4
remote 203.0.113.10 1194
auth-user-pass
dhcp-option DNS 10.0.0.1
dhcp-option DNS 1.1.1.1
route 192.168.10.0 255.255.255.0
<ca>
dummy ca
</ca>
`
	s, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.Remote != "203.0.113.10:1194" {
		t.Errorf("expected remote 203.0.113.10:1194, got %s", s.Remote)
	}
	if !s.AuthUserPass {
		t.Errorf("expected AuthUserPass to be true")
	}
	if len(s.DNS) != 2 || s.DNS[0] != "10.0.0.1" || s.DNS[1] != "1.1.1.1" {
		t.Errorf("unexpected DNS servers: %v", s.DNS)
	}
	if len(s.Routes) != 1 || s.Routes[0] != "192.168.10.0/255.255.255.0" {
		t.Errorf("unexpected routes: %v", s.Routes)
	}
	if !IsOpenVPN(raw) {
		t.Errorf("expected IsOpenVPN to be true")
	}
}
