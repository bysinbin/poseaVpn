package config

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// TransportMode defines whether WireGuard packets flow over raw UDP or a fallback tunnel (WSS/TLS).
type TransportMode string

const (
	TransportUDP      TransportMode = "udp"
	TransportWSS      TransportMode = "wss"
	TransportFallback TransportMode = "auto" // Try UDP first, fallback to WSS if blocked
)

// Config represents a parsed WireGuard configuration file.
type Config struct {
	Interface InterfaceConfig `json:"interface"`
	Peers     []PeerConfig    `json:"peers"`
}

// InterfaceConfig holds the local tunnel interface configuration.
type InterfaceConfig struct {
	PrivateKey  string         `json:"private_key"`
	Addresses   []netip.Prefix `json:"addresses"`
	DNS         []net.IP       `json:"dns"`
	DNSSearches []string       `json:"dns_searches"`
	MTU         int            `json:"mtu"`
	ListenPort  int            `json:"listen_port"`
	Transport   TransportMode  `json:"transport"`
	FallbackURL string         `json:"fallback_url,omitempty"`
}

// PeerConfig holds remote WireGuard peer configuration.
type PeerConfig struct {
	PublicKey           string         `json:"public_key"`
	PresharedKey        string         `json:"preshared_key,omitempty"`
	Endpoint            string         `json:"endpoint"`
	AllowedIPs          []netip.Prefix `json:"allowed_ips"`
	PersistentKeepalive int            `json:"persistent_keepalive,omitempty"`
}

// ParseFile parses a standard or extended WireGuard .conf file.
func ParseFile(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	return Parse(file)
}

// Parse reads from an io.Reader and parses the WireGuard configuration.
func Parse(r io.Reader) (*Config, error) {
	// Editors on Windows often save with a UTF-8 BOM; it would hide the first "[Interface]".
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	r = strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff"))

	cfg := &Config{
		Interface: InterfaceConfig{
			MTU:       1420,
			Transport: TransportUDP,
		},
		Peers: []PeerConfig{},
	}

	scanner := bufio.NewScanner(r)
	currentSection := ""
	var currentPeer *PeerConfig

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Handle section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.Trim(line, "[] \t"))
			if section == "interface" {
				currentSection = "interface"
			} else if section == "peer" {
				currentSection = "peer"
				if currentPeer != nil {
					cfg.Peers = append(cfg.Peers, *currentPeer)
				}
				currentPeer = &PeerConfig{}
			} else {
				currentSection = ""
			}
			continue
		}

		// Handle key = value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		// Strip inline comments if present
		if idx := strings.IndexAny(val, "#;"); idx != -1 {
			val = strings.TrimSpace(val[:idx])
		}

		switch currentSection {
		case "interface":
			switch key {
			case "privatekey":
				cfg.Interface.PrivateKey = val
			case "address":
				for _, addrStr := range splitAndTrim(val, ",") {
					prefix, err := netip.ParsePrefix(addrStr)
					if err != nil {
						// Try parsing single IP as /32 or /128
						ip, parseErr := netip.ParseAddr(addrStr)
						if parseErr != nil {
							return nil, fmt.Errorf("line %d: invalid address '%s'", lineNum, addrStr)
						}
						bits := 32
						if ip.Is6() {
							bits = 128
						}
						prefix = netip.PrefixFrom(ip, bits)
					}
					cfg.Interface.Addresses = append(cfg.Interface.Addresses, prefix)
				}
			case "dns":
				for _, dnsStr := range splitAndTrim(val, ",") {
					ip := net.ParseIP(dnsStr)
					if ip != nil {
						cfg.Interface.DNS = append(cfg.Interface.DNS, ip)
					} else {
						// Likely a domain search suffix, like corp.local
						cfg.Interface.DNSSearches = append(cfg.Interface.DNSSearches, dnsStr)
					}
				}
			case "mtu":
				mtu, err := strconv.Atoi(val)
				if err == nil && mtu > 0 {
					cfg.Interface.MTU = mtu
				}
			case "listenport":
				port, err := strconv.Atoi(val)
				if err == nil {
					cfg.Interface.ListenPort = port
				}
			case "transport":
				switch strings.ToLower(val) {
				case "wss", "websocket":
					cfg.Interface.Transport = TransportWSS
				case "auto", "fallback":
					cfg.Interface.Transport = TransportFallback
				default:
					cfg.Interface.Transport = TransportUDP
				}
			case "fallbackurl":
				cfg.Interface.FallbackURL = val
			}

		case "peer":
			if currentPeer == nil {
				continue
			}
			switch key {
			case "publickey":
				currentPeer.PublicKey = val
			case "presharedkey":
				currentPeer.PresharedKey = val
			case "endpoint":
				currentPeer.Endpoint = val
			case "allowedips":
				for _, ipStr := range splitAndTrim(val, ",") {
					prefix, err := netip.ParsePrefix(ipStr)
					if err != nil {
						ip, parseErr := netip.ParseAddr(ipStr)
						if parseErr != nil {
							return nil, fmt.Errorf("line %d: invalid allowed IP '%s'", lineNum, ipStr)
						}
						bits := 32
						if ip.Is6() {
							bits = 128
						}
						prefix = netip.PrefixFrom(ip, bits)
					}
					currentPeer.AllowedIPs = append(currentPeer.AllowedIPs, prefix)
				}
			case "persistentkeepalive":
				ka, err := strconv.Atoi(val)
				if err == nil {
					currentPeer.PersistentKeepalive = ka
				}
			}
		}
	}

	if currentPeer != nil {
		cfg.Peers = append(cfg.Peers, *currentPeer)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	// Validate configuration
	if cfg.Interface.PrivateKey == "" {
		return nil, fmt.Errorf("interface PrivateKey is required")
	}
	if len(cfg.Interface.Addresses) == 0 {
		return nil, fmt.Errorf("interface Address is required")
	}

	return cfg, nil
}

// ToUAPI generates standard WireGuard UAPI format string for IPC device configuration.
func (c *Config) ToUAPI() (string, error) {
	var b strings.Builder

	// Decode private key from base64 to hex
	privKeyBytes, err := base64.StdEncoding.DecodeString(c.Interface.PrivateKey)
	if err != nil || len(privKeyBytes) != 32 {
		return "", fmt.Errorf("invalid base64 private key: %v", err)
	}
	fmt.Fprintf(&b, "private_key=%x\n", privKeyBytes)

	// 0 means "pick a random port"; written explicitly so a retry can override a busy fixed port.
	fmt.Fprintf(&b, "listen_port=%d\n", c.Interface.ListenPort)

	fmt.Fprintf(&b, "replace_peers=true\n")

	for _, peer := range c.Peers {
		pubKeyBytes, err := base64.StdEncoding.DecodeString(peer.PublicKey)
		if err != nil || len(pubKeyBytes) != 32 {
			return "", fmt.Errorf("invalid peer public key '%s': %v", peer.PublicKey, err)
		}
		fmt.Fprintf(&b, "public_key=%x\n", pubKeyBytes)

		if peer.PresharedKey != "" {
			pskBytes, err := base64.StdEncoding.DecodeString(peer.PresharedKey)
			if err == nil && len(pskBytes) == 32 {
				fmt.Fprintf(&b, "preshared_key=%x\n", pskBytes)
			}
		}

		if peer.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", peer.Endpoint)
		}

		keepalive := peer.PersistentKeepalive
		if keepalive == 0 {
			keepalive = 25
		}
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", keepalive)

		fmt.Fprintf(&b, "replace_allowed_ips=true\n")
		for _, allowed := range peer.AllowedIPs {
			fmt.Fprintf(&b, "allowed_ip=%s\n", allowed.Masked().String())
		}
	}

	return b.String(), nil
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
