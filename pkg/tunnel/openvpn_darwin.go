//go:build darwin

package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// OpenVPNLogPath returns the path to the persistent log file for a tunnel on macOS.
func OpenVPNLogPath(name string) string {
	baseDir := "/var/log/poseavpn"
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		baseDir = filepath.Join(os.TempDir(), "poseavpn-logs")
		_ = os.MkdirAll(baseDir, 0o755)
	}
	return filepath.Join(baseDir, cleanAdapterName(name)+".log")
}

// FindOpenVPNBinary searches for the openvpn binary on macOS (bundled in Helpers, MacOS, Resources, Homebrew, MacPorts, PATH).
func FindOpenVPNBinary() (string, error) {
	self, err := os.Executable()
	if err == nil {
		selfDir := filepath.Dir(self)
		candidates := []string{
			filepath.Join(selfDir, "openvpn"),
			filepath.Join(selfDir, "openvpn", "openvpn"),
			filepath.Join(selfDir, "bin", "openvpn"),
			filepath.Join(selfDir, "bin", "openvpn", "openvpn"),
			filepath.Join(selfDir, "..", "bin", "openvpn"),
			filepath.Join(selfDir, "..", "MacOS", "openvpn"),
			filepath.Join(selfDir, "..", "Helpers", "openvpn"),
			filepath.Join(selfDir, "..", "Resources", "openvpn"),
			"/Applications/PoseaVPN.app/Contents/Helpers/openvpn",
			"/Applications/PoseaVPN.app/Contents/MacOS/openvpn",
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				return filepath.Abs(c)
			}
		}
	}

	// Homebrew, MacPorts, Tunnelblick, and standard paths
	home, _ := os.UserHomeDir()
	commonPaths := []string{
		"/Applications/PoseaVPN.app/Contents/Helpers/openvpn",
		"/opt/homebrew/sbin/openvpn",
		"/opt/homebrew/bin/openvpn",
		filepath.Join(home, "homebrew", "sbin", "openvpn"),
		filepath.Join(home, "homebrew", "bin", "openvpn"),
		"/usr/local/sbin/openvpn",
		"/usr/local/bin/openvpn",
		"/opt/local/sbin/openvpn",
		"/opt/local/bin/openvpn",
		"/usr/sbin/openvpn",
		"/usr/bin/openvpn",
		"/Applications/Tunnelblick.app/Contents/Resources/openvpn/openvpn-2.5.4-openssl-1.1.1l/openvpn",
		"/Applications/Tunnelblick.app/Contents/Resources/openvpn/openvpn-2.4.11-openssl-1.1.1l/openvpn",
	}
	for _, p := range commonPaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}

	// PATH lookup
	if path, err := exec.LookPath("openvpn"); err == nil {
		return filepath.Abs(path)
	}

	return "", fmt.Errorf("openvpn binary not found. Please install via 'brew install openvpn' or place openvpn in bin/")
}

func prepareOpenVPN(name, openvpnExe, confPath, authFile string, mgmtPort int) (*exec.Cmd, func(), error) {
	// macOS kernel provides utun natively; no driver installation needed
	args := []string{
		"--config", confPath,
		"--dev", "utun",
		"--management", "127.0.0.1", strconv.Itoa(mgmtPort),
		"--verb", "3",
		"--suppress-timestamps",
	}

	if authFile != "" {
		args = append(args, "--auth-user-pass", authFile)
	}

	cmd := exec.Command(openvpnExe, args...)
	cmd.Dir = filepath.Dir(openvpnExe)

	cleanup := func() {}
	return cmd, cleanup, nil
}

func configurePrivateNetworkForIP(name, ip string) {
	// No-op on macOS (no Windows Firewall network categories)
}
