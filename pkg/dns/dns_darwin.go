//go:build darwin

package dns

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConfigureDNS configures DNS server addresses and search suffixes on macOS using scutil
// and /etc/resolver entries. This provides the macOS equivalent of Windows NRPT split-DNS.
func ConfigureDNS(interfaceAlias string, servers []net.IP, suffixes []string) error {
	if len(servers) == 0 && len(suffixes) == 0 {
		return nil
	}

	var serverStrs []string
	for _, ip := range servers {
		serverStrs = append(serverStrs, ip.String())
	}

	// 1. Configure dynamic store via scutil for the interface
	serviceKey := fmt.Sprintf("State:/Network/Service/PoseaVPN_%s/DNS", interfaceAlias)
	ip4Key := fmt.Sprintf("State:/Network/Service/PoseaVPN_%s/IPv4", interfaceAlias)

	var scutilScript bytes.Buffer
	scutilScript.WriteString("open\n")
	scutilScript.WriteString("d.init\n")

	if len(serverStrs) > 0 {
		scutilScript.WriteString(fmt.Sprintf("d.add ServerAddresses * %s\n", strings.Join(serverStrs, " ")))
	}
	if len(suffixes) > 0 {
		cleanSuffixes := make([]string, len(suffixes))
		for i, s := range suffixes {
			cleanSuffixes[i] = strings.TrimPrefix(s, ".")
		}
		scutilScript.WriteString(fmt.Sprintf("d.add SupplementalMatchDomains * %s\n", strings.Join(cleanSuffixes, " ")))
		scutilScript.WriteString(fmt.Sprintf("d.add SearchDomains * %s\n", strings.Join(cleanSuffixes, " ")))
		scutilScript.WriteString("d.add SupplementalMatchOrders * 100000\n")
	}
	scutilScript.WriteString(fmt.Sprintf("set %s\n", serviceKey))

	scutilScript.WriteString("d.init\n")
	scutilScript.WriteString(fmt.Sprintf("d.add InterfaceName %s\n", interfaceAlias))
	scutilScript.WriteString(fmt.Sprintf("set %s\n", ip4Key))
	scutilScript.WriteString("quit\n")

	cmd := exec.Command("scutil")
	cmd.Stdin = &scutilScript
	if out, err := cmd.CombinedOutput(); err != nil {
		// Log but don't fail immediately, try /etc/resolver
		_ = out
	}

	// 2. Set up /etc/resolver/<suffix> entries for domain routing if we have write access
	if len(servers) > 0 && len(suffixes) > 0 {
		_ = os.MkdirAll("/etc/resolver", 0o755)
		for _, s := range suffixes {
			domain := strings.TrimPrefix(s, ".")
			if domain == "" {
				continue
			}
			resPath := filepath.Join("/etc/resolver", domain)
			var content strings.Builder
			content.WriteString(fmt.Sprintf("# PoseaVPN:%s\n", interfaceAlias))
			for _, srv := range serverStrs {
				content.WriteString(fmt.Sprintf("nameserver %s\n", srv))
			}
			content.WriteString(fmt.Sprintf("search %s\n", domain))
			_ = os.WriteFile(resPath, []byte(content.String()), 0o644)
		}
	}

	return nil
}

// CleanUpNRPT cleans up DNS settings created by PoseaVPN for this interface on macOS.
func CleanUpNRPT(interfaceAlias string) error {
	serviceKey := fmt.Sprintf("State:/Network/Service/PoseaVPN_%s/DNS", interfaceAlias)
	ip4Key := fmt.Sprintf("State:/Network/Service/PoseaVPN_%s/IPv4", interfaceAlias)

	var scutilScript bytes.Buffer
	scutilScript.WriteString("open\n")
	scutilScript.WriteString(fmt.Sprintf("remove %s\n", serviceKey))
	scutilScript.WriteString(fmt.Sprintf("remove %s\n", ip4Key))
	scutilScript.WriteString("quit\n")

	cmd := exec.Command("scutil")
	cmd.Stdin = &scutilScript
	_ = cmd.Run()

	// Clean up matching /etc/resolver files
	tag := fmt.Sprintf("# PoseaVPN:%s", interfaceAlias)
	if entries, err := os.ReadDir("/etc/resolver"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			filePath := filepath.Join("/etc/resolver", e.Name())
			if data, err := os.ReadFile(filePath); err == nil {
				if strings.Contains(string(data), tag) {
					_ = os.Remove(filePath)
				}
			}
		}
	}

	return nil
}
