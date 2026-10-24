//go:build windows

package tunnel

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.zx2c4.com/wintun"
)

// OpenVPNLogPath returns the path to the persistent log file for a tunnel.
func OpenVPNLogPath(name string) string {
	baseDir := filepath.Join(os.Getenv("ProgramData"), "PoseaVPN", "logs")
	if os.Getenv("ProgramData") == "" {
		baseDir = filepath.Join(os.TempDir(), "poseavpn-logs")
	}
	_ = os.MkdirAll(baseDir, 0o755)
	return filepath.Join(baseDir, cleanAdapterName(name)+".log")
}

// FindOpenVPNBinary searches for the bundled openvpn.exe on Windows.
func FindOpenVPNBinary() (string, error) {
	self, err := os.Executable()
	if err == nil {
		selfDir := filepath.Dir(self)
		candidates := []string{
			filepath.Join(selfDir, "openvpn", "openvpn.exe"),
			filepath.Join(selfDir, "bin", "openvpn", "openvpn.exe"),
			filepath.Join(selfDir, "..", "bin", "openvpn", "openvpn.exe"),
			filepath.Join(selfDir, "..", "openvpn", "openvpn.exe"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return filepath.Abs(c)
			}
		}
	}

	// Working directory fallback
	candidates := []string{
		`bin\openvpn\openvpn.exe`,
		`openvpn\openvpn.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return filepath.Abs(c)
		}
	}

	return "", fmt.Errorf("bundled openvpn.exe not found in bin/openvpn/")
}

func createOrOpenWintunAdapter(name string) (*wintun.Adapter, error) {
	if existing, err := wintun.OpenAdapter(name); err == nil && existing != nil {
		_ = existing.Close()
		time.Sleep(300 * time.Millisecond)
	}

	adapter, err := wintun.CreateAdapter(name, "Wintun", nil)
	if err != nil {
		if existing, oerr := wintun.OpenAdapter(name); oerr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("failed to create Wintun adapter '%s': %w", name, err)
	}
	return adapter, nil
}

func prepareOpenVPN(name, openvpnExe, confPath, authFile string, mgmtPort int) (*exec.Cmd, func(), error) {
	adapterName := "PoseaVPN-OVPN-" + cleanAdapterName(name)
	wAdapter, err := createOrOpenWintunAdapter(adapterName)
	if err != nil {
		return nil, nil, err
	}

	args := []string{
		"--config", confPath,
		"--windows-driver", "wintun",
		"--dev-node", adapterName,
		"--management", "127.0.0.1", strconv.Itoa(mgmtPort),
		"--verb", "3",
		"--suppress-timestamps",
	}

	if authFile != "" {
		args = append(args, "--auth-user-pass", authFile)
	}

	cmd := exec.Command(openvpnExe, args...)
	cmd.Dir = filepath.Dir(openvpnExe) // so it finds wintun.dll and ssl dlls
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	cleanup := func() {
		if wAdapter != nil {
			_ = wAdapter.Close()
		}
	}

	return cmd, cleanup, nil
}

func configurePrivateNetworkForIP(name, ip string) {
	if ip != "" {
		psCmd := fmt.Sprintf(`$a = Get-NetIPAddress -IPAddress "%s" -ErrorAction SilentlyContinue; if ($a) { Set-NetConnectionProfile -InterfaceIndex $a.InterfaceIndex -NetworkCategory Private }`, ip)
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[OpenVPN %s] Set Private by IP %s warning: %v (%s)", name, ip, err, strings.TrimSpace(string(out)))
		} else {
			log.Printf("[OpenVPN %s] Network profile for IP %s set to Private successfully", name, ip)
		}
	}
}
