//go:build windows

package guiapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func queryServiceState() string {
	cmd := exec.Command("sc.exe", "query", serviceName)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	s := string(out)
	switch {
	case err != nil && strings.Contains(s, "1060"):
		return "notinstalled"
	case strings.Contains(s, "RUNNING"):
		return "running"
	case strings.Contains(s, "STOPPED"):
		return "stopped"
	}
	return "unknown"
}

func serviceExe() string {
	exe, err := os.Executable()
	if err != nil {
		return "poseavpn-service.exe"
	}
	dir := filepath.Dir(exe)
	for _, c := range []string{
		filepath.Join(dir, "poseavpn-service.exe"),
		filepath.Join(dir, "bin", "poseavpn-service.exe"),
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return filepath.Join(dir, "poseavpn-service.exe")
}

func runElevated(exe, args string) error {
	err := windows.ShellExecute(0,
		windows.StringToUTF16Ptr("runas"),
		windows.StringToUTF16Ptr(exe),
		windows.StringToUTF16Ptr(args),
		windows.StringToUTF16Ptr(filepath.Dir(exe)),
		windows.SW_HIDE)
	if err != nil {
		return fmt.Errorf("yönetici onayı verilmedi veya iptal edildi")
	}
	return nil
}

// ServiceInstall installs and starts the service (shows the UAC prompt).
func (a *App) ServiceInstall() error {
	exe := serviceExe()
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("servis dosyası bulunamadı: %s", exe)
	}
	return runElevated(exe, "--install")
}

// ServiceStart starts an installed but stopped service (shows the UAC prompt).
func (a *App) ServiceStart() error {
	return runElevated("sc.exe", "start "+serviceName)
}

// ServiceRestart restarts the background service (shows the UAC prompt).
func (a *App) ServiceRestart() error {
	return runElevated("powershell.exe", "-NoProfile -ExecutionPolicy Bypass -Command Restart-Service "+serviceName)
}

// ServiceUninstall stops and removes the service (shows the UAC prompt).
func (a *App) ServiceUninstall() error {
	exe := serviceExe()
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("servis dosyası bulunamadı: %s", exe)
	}
	return runElevated(exe, "--uninstall")
}
