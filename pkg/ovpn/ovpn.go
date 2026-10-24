package ovpn

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Summary contains high-level metadata extracted from an OpenVPN configuration.
type Summary struct {
	Remote          string   // e.g. "203.0.113.10:1194"
	Proto           string   // "udp", "tcp", "udp4", etc.
	Dev             string   // "tun" or "tap"
	AuthUserPass    bool     // true if directive 'auth-user-pass' exists
	DNS             []string // extracted from 'dhcp-option DNS ...'
	Routes          []string // extracted from 'route ...'
	VerifyX509Name  string
	HasInlineCert   bool
}

// Parse parses an OpenVPN configuration stream and extracts summary metadata.
func Parse(r io.Reader) (*Summary, error) {
	s := &Summary{
		Dev:   "tun",
		Proto: "udp",
	}

	scanner := bufio.NewScanner(r)
	inBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Handle XML-like inline certificate tags <ca>, <cert>, <key>, etc.
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			tag := strings.Trim(line, "<>/")
			if strings.HasPrefix(line, "</") {
				inBlock = false
			} else {
				inBlock = true
				if tag == "cert" || tag == "ca" || tag == "key" {
					s.HasInlineCert = true
				}
			}
			continue
		}
		if inBlock {
			continue
		}

		// Ignore comments
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		cmd := strings.ToLower(fields[0])
		switch cmd {
		case "remote":
			if len(fields) >= 2 {
				host := fields[1]
				port := "1194"
				if len(fields) >= 3 {
					port = fields[2]
				}
				s.Remote = host + ":" + port
				if len(fields) >= 4 {
					s.Proto = strings.ToLower(fields[3])
				}
			}
		case "proto":
			if len(fields) >= 2 {
				s.Proto = strings.ToLower(fields[1])
			}
		case "dev":
			if len(fields) >= 2 {
				s.Dev = strings.ToLower(fields[1])
			}
		case "auth-user-pass":
			s.AuthUserPass = true
		case "verify-x509-name":
			if len(fields) >= 2 {
				s.VerifyX509Name = strings.Trim(fields[1], `"'`)
			}
		case "dhcp-option":
			if len(fields) >= 3 && strings.EqualFold(fields[1], "DNS") {
				s.DNS = append(s.DNS, fields[2])
			}
		case "route":
			if len(fields) >= 2 {
				rStr := fields[1]
				if len(fields) >= 3 {
					rStr += "/" + fields[2]
				}
				s.Routes = append(s.Routes, rStr)
			}
		}
	}

	return s, scanner.Err()
}

// ParseFile reads and parses an OpenVPN configuration file.
func ParseFile(filePath string) (*Summary, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// IsOpenVPN checks if the data looks like an OpenVPN configuration.
func IsOpenVPN(text string) bool {
	// Look for distinctive OpenVPN directives
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";") {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) == 0 {
			continue
		}
		cmd := strings.ToLower(fields[0])
		if cmd == "client" || cmd == "tls-client" || cmd == "auth-user-pass" ||
			cmd == "dev" || (cmd == "remote" && len(fields) >= 2) {
			return true
		}
	}
	return false
}
