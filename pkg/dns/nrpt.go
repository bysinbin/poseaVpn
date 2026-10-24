package dns

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
)

// nrptComment tags NRPT rules per tunnel so several tunnels can coexist.
func nrptComment(interfaceAlias string) string {
	return "PoseaVPN:" + interfaceAlias
}

// ConfigureDNS configures DNS server addresses and search suffixes on the tunnel interface.
func ConfigureDNS(interfaceAlias string, servers []net.IP, suffixes []string) error {
	if len(servers) == 0 && len(suffixes) == 0 {
		return nil
	}

	var serverStrs []string
	for _, ip := range servers {
		serverStrs = append(serverStrs, fmt.Sprintf(`"%s"`, ip.String()))
	}

	var psCommands []string

	if len(serverStrs) > 0 {
		serversArg := strings.Join(serverStrs, ", ")
		psCommands = append(psCommands, fmt.Sprintf(
			`Set-DnsClientServerAddress -InterfaceAlias "%s" -ServerAddresses @(%s)`,
			interfaceAlias, serversArg,
		))
	}

	if len(suffixes) > 0 {
		primarySuffix := suffixes[0]
		psCommands = append(psCommands, fmt.Sprintf(
			`Set-DnsClient -InterfaceAlias "%s" -ConnectionSpecificSuffix "%s" -RegisterThisConnectionsAddress $false`,
			interfaceAlias, primarySuffix,
		))

		// Also configure NRPT (Name Resolution Policy Table) for each suffix so Windows directs
		// Active Directory SRV and host queries specifically to the VPN DNS servers
		if len(serverStrs) > 0 {
			serversArg := strings.Join(serverStrs, ", ")
			for _, suffix := range suffixes {
				ruleNamespace := "." + strings.TrimPrefix(suffix, ".")
				psCommands = append(psCommands, fmt.Sprintf(
					`Add-DnsClientNrptRule -Namespace "%s" -Nameservers @(%s) -Comment "%s" -ErrorAction SilentlyContinue`,
					ruleNamespace, serversArg, nrptComment(interfaceAlias),
				))
			}
		}
	}

	fullScript := strings.Join(psCommands, "; ")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", fullScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to configure DNS for %s: %w (output: %s)", interfaceAlias, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// CleanUpNRPT removes the NRPT rules added by PoseaVPN for one tunnel.
func CleanUpNRPT(interfaceAlias string) error {
	psCmd := fmt.Sprintf(`Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Comment -eq "%s" } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue`, nrptComment(interfaceAlias))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	_ = cmd.Run()
	return nil
}
