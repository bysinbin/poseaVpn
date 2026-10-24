package guiapp

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// trayMutexName must match the mutex created by cmd/poseavpn-tray.
const trayMutexName = `Local\PoseaVPNTray`

// trayRunning reports whether poseavpn-tray.exe currently holds its single-instance mutex.
func trayRunning() bool {
	name, err := windows.UTF16PtrFromString(trayMutexName)
	if err != nil {
		return false
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(h)
	return true
}

// ensureTray starts the tray app (next to this executable) when it is not running,
// so the tray icon exists whenever one of the desktop apps is opened.
func ensureTray() {
	if trayRunning() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	tray := filepath.Join(filepath.Dir(exe), "poseavpn-tray.exe")
	if _, err := os.Stat(tray); err != nil {
		return
	}
	_ = exec.Command(tray).Start()
}
