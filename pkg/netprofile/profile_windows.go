//go:build windows

package netprofile

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// NetworkCategory defines the Windows firewall network profile category.
type NetworkCategory string

const (
	CategoryPublic  NetworkCategory = "Public"
	CategoryPrivate NetworkCategory = "Private"
	CategoryDomain  NetworkCategory = "DomainAuthenticated"
)

// SetInterfaceCategory sets the network profile category for a given network interface alias.
// This is critical on Active Directory domain-joined machines to prevent Windows Firewall
// from blocking LDAP, Kerberos, SMB, and RPC traffic on the newly created tunnel.
func SetInterfaceCategory(interfaceAlias string, category NetworkCategory) error {
	// PowerShell command to set the connection profile
	psCmd := fmt.Sprintf(
		`$profile = Get-NetConnectionProfile -InterfaceAlias "%s" -ErrorAction SilentlyContinue; if ($profile) { Set-NetConnectionProfile -InterfaceAlias "%s" -NetworkCategory %s }`,
		interfaceAlias, interfaceAlias, category,
	)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set network category for %s: %w (output: %s)", interfaceAlias, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// GetInterfaceCategory retrieves the current network category for the interface.
func GetInterfaceCategory(interfaceAlias string) (string, error) {
	psCmd := fmt.Sprintf(`(Get-NetConnectionProfile -InterfaceAlias "%s" -ErrorAction SilentlyContinue).NetworkCategory`, interfaceAlias)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get network category: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
