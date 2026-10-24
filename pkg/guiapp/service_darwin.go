//go:build darwin

package guiapp

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"poseavpn/pkg/ipc"
)

const (
	launchDaemonPlist = "/Library/LaunchDaemons/com.poseavpn.service.plist"
	launchDaemonLabel = "com.poseavpn.service"
)

func queryServiceState() string {
	// 1. If IPC socket connects, service is running
	conn, err := net.DialTimeout("unix", ipc.SocketPath, 300*time.Millisecond)
	if err == nil {
		conn.Close()
		return "running"
	}
	conn2, err2 := net.DialTimeout("unix", ipc.FallbackSocketPath, 300*time.Millisecond)
	if err2 == nil {
		conn2.Close()
		return "running"
	}

	// 2. Check launchctl list
	cmd := exec.Command("launchctl", "list", launchDaemonLabel)
	if out, err := cmd.CombinedOutput(); err == nil && !strings.Contains(string(out), "Could not find") {
		return "running"
	}

	// 3. Check if plist is installed
	if _, err := os.Stat(launchDaemonPlist); err == nil {
		return "stopped"
	}

	return "notinstalled"
}

func serviceExe() string {
	exe, err := os.Executable()
	if err != nil {
		return "poseavpn-service"
	}
	dir := filepath.Dir(exe)
	for _, c := range []string{
		filepath.Join(dir, "poseavpn-service"),
		filepath.Join(dir, "bin", "poseavpn-service"),
		filepath.Join(dir, "..", "MacOS", "poseavpn-service"),
		filepath.Join(dir, "..", "Helpers", "poseavpn-service"),
		filepath.Join(dir, "Helpers", "poseavpn-service"),
		"/Applications/PoseaVPN.app/Contents/Helpers/poseavpn-service",
		"/Applications/PoseaVPN.app/Contents/MacOS/poseavpn-service",
		"/usr/local/bin/poseavpn-service",
	} {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return filepath.Join(dir, "poseavpn-service")
}

// runElevated executes a shell script with macOS administrator privileges via osascript.
func runElevated(shellScript string) error {
	escaped := strings.ReplaceAll(shellScript, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	appleScript := fmt.Sprintf(`do shell script "%s" with administrator privileges`, escaped)

	cmd := exec.Command("osascript", "-e", appleScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yönetici onayı verilmedi veya iptal edildi: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ServiceInstall installs and starts the launchd daemon (shows administrator prompt).
func (a *App) ServiceInstall() error {
	exe := serviceExe()
	absExe, err := filepath.Abs(exe)
	if err != nil {
		absExe = exe
	}

	cliLink := ""
	cliCandidates := []string{
		filepath.Join(filepath.Dir(absExe), "poseavpn"),
		filepath.Join(filepath.Dir(absExe), "..", "Helpers", "poseavpn"),
		"/Applications/PoseaVPN.app/Contents/Helpers/poseavpn",
	}
	for _, cand := range cliCandidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			absCand, _ := filepath.Abs(cand)
			cliLink = fmt.Sprintf("mkdir -p /usr/local/bin && ln -sf '%s' /usr/local/bin/poseavpn && ln -sf '%s' /usr/local/bin/poseavpn-service ; ", absCand, absExe)
			break
		}
	}

	script := fmt.Sprintf(`%smkdir -p /var/log && cat <<'EOF' > %s
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/poseavpn.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/poseavpn.log</string>
</dict>
</plist>
EOF
chmod 644 %s && chown root:wheel %s && launchctl load -w %s`, cliLink, launchDaemonPlist, launchDaemonLabel, absExe, launchDaemonPlist, launchDaemonPlist, launchDaemonPlist)

	if err := runElevated(script); err != nil {
		return err
	}

	go ensureTray()
	return nil
}

// ServiceStart starts the launchd daemon (shows administrator prompt).
func (a *App) ServiceStart() error {
	return runElevated(fmt.Sprintf("launchctl load -w %s || launchctl start %s", launchDaemonPlist, launchDaemonLabel))
}

// ServiceRestart restarts the launchd daemon (shows administrator prompt if IPC fails).
func (a *App) ServiceRestart() error {
	c := ipc.NewClient()
	if resp, err := c.SendRequest(ipc.Request{Action: "restart"}); err == nil && resp.Success {
		return nil
	}
	return runElevated(fmt.Sprintf("launchctl kickstart -k system/%s || (launchctl unload %s && launchctl load -w %s)", launchDaemonLabel, launchDaemonPlist, launchDaemonPlist))
}

// ServiceUninstall stops and removes the launchd daemon (shows administrator prompt).
func (a *App) ServiceUninstall() error {
	return runElevated(fmt.Sprintf("launchctl unload -w %s; rm -f %s", launchDaemonPlist, launchDaemonPlist))
}
