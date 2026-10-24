//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	autostartMenuTitle = "Windows ile başlat"
	runKeyPath         = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName       = "PoseaVPNTray"
)

func singleInstanceLock() (func(), bool, error) {
	name, _ := windows.UTF16PtrFromString(`Local\PoseaVPNTray`)
	h, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { _ = windows.CloseHandle(h) }, false, nil
}

func waitForTaskbar(timeout time.Duration) bool {
	findWindow := windows.NewLazySystemDLL("user32.dll").NewProc("FindWindowW")
	cls, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	deadline := time.Now().Add(timeout)
	for {
		if hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(cls)), 0); hwnd != 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func closeGUIs() {
	names := []string{"poseavpn-single.exe", "poseavpn-multi.exe"}

	taskkill := func(force bool, name string) {
		args := []string{"/IM", name}
		if force {
			args = append([]string{"/F"}, args...)
		}
		cmd := exec.Command("taskkill", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
	}

	for _, n := range names {
		taskkill(false, n)
	}
	time.Sleep(2 * time.Second)
	for _, n := range names {
		taskkill(true, n)
	}
	logf("closed desktop apps on exit")
}

func launch(exeName string) {
	self, _ := os.Executable()
	path := filepath.Join(filepath.Dir(self), exeName)
	if _, err := os.Stat(path); err != nil {
		messageBox("PoseaVPN", exeName+" bulunamadı:\n"+filepath.Dir(self), windows.MB_OK|windows.MB_ICONWARNING)
		return
	}
	_ = exec.Command(path).Start()
}

func launchMulti() {
	launch("poseavpn-multi.exe")
}

func launchSingle() {
	launch("poseavpn-single.exe")
}

func messageBox(title, text string, flags uint32) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	_, _ = windows.MessageBox(0, m, t, flags|windows.MB_SETFOREGROUND)
}

func showError(title, text string) {
	messageBox(title, text, windows.MB_OK|windows.MB_ICONERROR)
}

func enableAutostartOnFirstRun() {
	const flagKey = `Software\PoseaVPN`
	const flagName = "TrayAutostartInitialized"

	fk, _, err := registry.CreateKey(registry.CURRENT_USER, flagKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		logf("autostart: cannot open flag key: %v", err)
		return
	}
	defer fk.Close()
	if v, _, err := fk.GetIntegerValue(flagName); err == nil && v == 1 {
		logf("autostart: already initialised")
		return
	}

	if !autostartEnabled() {
		rk, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
		if err != nil {
			logf("autostart: cannot open Run key: %v", err)
			return
		}
		exe, _ := os.Executable()
		if err := rk.SetStringValue(runValueName, `"`+exe+`"`); err != nil {
			logf("autostart: cannot write Run value: %v", err)
		} else {
			logf("autostart: enabled for %s", exe)
		}
		rk.Close()
	}
	_ = fk.SetDWordValue(flagName, 1)
}

func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValueName)
	return err == nil
}

func toggleAutostart() {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		messageBox("PoseaVPN", "Başlangıç ayarı değiştirilemedi: "+err.Error(), windows.MB_OK|windows.MB_ICONERROR)
		return
	}
	defer k.Close()

	if autostartEnabled() {
		_ = k.DeleteValue(runValueName)
		autoItem.Uncheck()
		return
	}
	exe, _ := os.Executable()
	if err := k.SetStringValue(runValueName, `"`+exe+`"`); err != nil {
		messageBox("PoseaVPN", "Başlangıç ayarı değiştirilemedi: "+err.Error(), windows.MB_OK|windows.MB_ICONERROR)
		return
	}
	autoItem.Check()
}
