//go:build darwin

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	autostartMenuTitle = "macOS ile başlat"
	traySocket         = "/tmp/poseavpn-tray.sock"
)

func singleInstanceLock() (func(), bool, error) {
	// Probe existing socket
	if conn, err := net.DialTimeout("unix", traySocket, 200*time.Millisecond); err == nil {
		conn.Close()
		return nil, true, nil
	}

	_ = os.Remove(traySocket)
	l, err := net.Listen("unix", traySocket)
	if err != nil {
		return nil, false, err
	}

	unlock := func() {
		_ = l.Close()
		_ = os.Remove(traySocket)
	}

	return unlock, false, nil
}

func waitForTaskbar(timeout time.Duration) bool {
	return true
}

func closeGUIs() {
	_ = exec.Command("pkill", "-f", "poseavpn-single").Run()
	_ = exec.Command("pkill", "-f", "poseavpn-multi").Run()
	_ = exec.Command("pkill", "-f", "PoseaVPN").Run()
	logf("closed desktop apps on exit")
}

func findAppBinary(name string) string {
	self, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(self)
		candidates := []string{
			filepath.Join(dir, name),
			filepath.Join(dir, "bin", name),
			filepath.Join(dir, "..", "MacOS", name),
			filepath.Join(dir, "..", "Helpers", name),
			filepath.Join(dir, "Helpers", name),
			filepath.Join(dir, name+".app", "Contents", "MacOS", name),
			filepath.Join("/Applications/PoseaVPN.app/Contents/MacOS", name),
			filepath.Join("/Applications/PoseaVPN.app/Contents/Helpers", name),
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				return c
			}
		}
	}
	return name
}

func launch(exeName string) {
	bin := findAppBinary(exeName)
	if _, err := os.Stat(bin); err != nil {
		messageBox("PoseaVPN", exeName+" bulunamadı", 0)
		return
	}
	_ = exec.Command(bin).Start()
}

func launchMulti() {
	// Check if PoseaVPN (main app bundle) is present
	p := findAppBinary("PoseaVPN")
	if _, err := os.Stat(p); err == nil {
		_ = exec.Command(p).Start()
		return
	}
	launch("poseavpn-multi")
}

func launchSingle() {
	launch("poseavpn-single")
}

func messageBox(title, text string, flags uint32) {
	script := fmt.Sprintf(`display alert "%s" message "%s" as informational`, strings.ReplaceAll(title, `"`, `\"`), strings.ReplaceAll(text, `"`, `\"`))
	_ = exec.Command("osascript", "-e", script).Run()
}

func showError(title, text string) {
	script := fmt.Sprintf(`display alert "%s" message "%s" as critical`, strings.ReplaceAll(title, `"`, `\"`), strings.ReplaceAll(text, `"`, `\"`))
	_ = exec.Command("osascript", "-e", script).Run()
}

func trayPlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.poseavpn.tray.plist")
}

func enableAutostartOnFirstRun() {
	p := trayPlistPath()
	if p == "" {
		return
	}
	if _, err := os.Stat(p); err == nil {
		return // already setup
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}
	absExe, _ := filepath.Abs(exe)

	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.poseavpn.tray</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, absExe)
	_ = os.WriteFile(p, []byte(content), 0o644)
	logf("autostart: created launch agent %s for %s", p, absExe)
}

func autostartEnabled() bool {
	p := trayPlistPath()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func toggleAutostart() {
	p := trayPlistPath()
	if p == "" {
		return
	}

	if autostartEnabled() {
		_ = os.Remove(p)
		autoItem.Uncheck()
		return
	}

	exe, _ := os.Executable()
	absExe, _ := filepath.Abs(exe)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.poseavpn.tray</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, absExe)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		messageBox("PoseaVPN", "Başlangıç ayarı değiştirilemedi: "+err.Error(), 0)
		return
	}
	autoItem.Check()
}
