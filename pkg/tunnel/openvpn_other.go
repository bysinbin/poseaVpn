//go:build !windows && !darwin

package tunnel

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
)

func OpenVPNLogPath(name string) string {
	return filepath.Join("/tmp", cleanAdapterName(name)+".log")
}

func FindOpenVPNBinary() (string, error) {
	if p, err := exec.LookPath("openvpn"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("openvpn not found in PATH")
}

func prepareOpenVPN(name, openvpnExe, confPath, authFile string, mgmtPort int) (*exec.Cmd, func(), error) {
	args := []string{
		"--config", confPath,
		"--dev", "tun",
		"--management", "127.0.0.1", strconv.Itoa(mgmtPort),
		"--verb", "3",
		"--suppress-timestamps",
	}
	if authFile != "" {
		args = append(args, "--auth-user-pass", authFile)
	}
	cmd := exec.Command(openvpnExe, args...)
	return cmd, func() {}, nil
}

func configurePrivateNetworkForIP(name, ip string) {}
