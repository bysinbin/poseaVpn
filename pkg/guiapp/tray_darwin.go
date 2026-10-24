//go:build darwin

package guiapp

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const traySocketPath = "/tmp/poseavpn-tray.sock"

func trayRunning() bool {
	conn, err := net.DialTimeout("unix", traySocketPath, 200*time.Millisecond)
	if err == nil {
		conn.Close()
		return true
	}
	// Fallback check with pgrep
	cmd := exec.Command("pgrep", "-x", "poseavpn-tray")
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func ensureTray() {
	if trayRunning() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(dir, "poseavpn-tray"),
		filepath.Join(dir, "bin", "poseavpn-tray"),
		filepath.Join(dir, "..", "MacOS", "poseavpn-tray"),
		filepath.Join(dir, "PoseaVPN Tray.app", "Contents", "MacOS", "poseavpn-tray"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			_ = exec.Command(c).Start()
			return
		}
	}
}
