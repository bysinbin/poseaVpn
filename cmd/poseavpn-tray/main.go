// poseavpn-tray is the system tray companion of the PoseaVPN desktop apps.
// It runs as the normal (non-admin) user, talks to the SYSTEM service through
// the same backend as the GUIs and keeps running when the GUI window is closed.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"poseavpn/pkg/guiapp"
)

const (
	maxSlots     = 20
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "PoseaVPNTray"
)

// slot is one reusable profile entry in the tray menu.
type slot struct {
	item *systray.MenuItem
	name string // tunnel / profile name currently shown in this slot
	last bool   // true: remembered single-mode config (connect via ConnectLast)
	on   bool
}

var (
	app   = guiapp.New(guiapp.ModeMulti)
	slots [maxSlots]*slot
	mu    sync.Mutex

	headerItem  *systray.MenuItem
	emptyItem   *systray.MenuItem
	disconnItem *systray.MenuItem
	svcItem     *systray.MenuItem
	autoItem    *systray.MenuItem

	lastSig string

	// refreshMu serialises menu updates (ticker + click handlers) and guards lastSig.
	refreshMu sync.Mutex
)

// invalidate forces the next refresh to rebuild the menu.
func invalidate() {
	refreshMu.Lock()
	lastSig = ""
	refreshMu.Unlock()
}

func main() {
	initLog()
	logf("starting (pid %d)", os.Getpid())

	// single instance
	name, _ := windows.UTF16PtrFromString(`Local\PoseaVPNTray`)
	h, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		logf("another tray instance is already running, exiting")
		return
	}
	if err != nil {
		logf("mutex error: %v", err)
		return
	}
	defer windows.CloseHandle(h)

	// When started from the Run key right after login, Explorer's taskbar may not
	// exist yet and Shell_NotifyIcon(NIM_ADD) would fail. Wait for it first.
	if waitForTaskbar(45 * time.Second) {
		logf("taskbar is ready")
	} else {
		logf("taskbar not found after waiting; trying anyway")
	}

	systray.Run(onReady, func() { logf("systray exited") })
	logf("systray.Run returned")
}

// waitForTaskbar polls for Explorer's Shell_TrayWnd window.
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

// initLog writes a small diagnostic log to %LOCALAPPDATA%\PoseaVPN\tray.log.
func initLog() {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "PoseaVPN")
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	path := filepath.Join(dir, "tray.log")
	// keep the file small: start over when it grows beyond 256 KB
	if st, err := os.Stat(path); err == nil && st.Size() > 256*1024 {
		_ = os.Remove(path)
	}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(f)
		log.SetFlags(log.LstdFlags)
	}
}

func logf(format string, args ...interface{}) { log.Printf(format, args...) }

func onReady() {
	logf("onReady")
	enableAutostartOnFirstRun()

	systray.SetIcon(iconOff)
	systray.SetTitle("PoseaVPN")
	systray.SetTooltip("PoseaVPN")
	systray.SetOnTapped(openWindow)

	headerItem = systray.AddMenuItem("PoseaVPN", "")
	headerItem.Disable()
	systray.AddSeparator()

	for i := range slots {
		it := systray.AddMenuItemCheckbox("", "", false)
		it.Hide()
		s := &slot{item: it}
		slots[i] = s
		go func() {
			for range it.ClickedCh {
				toggleSlot(s)
			}
		}()
	}

	emptyItem = systray.AddMenuItem("Profil yok — pencereyi açıp içe aktarın", "")
	emptyItem.Hide()
	go func() {
		for range emptyItem.ClickedCh {
			openWindow()
		}
	}()

	systray.AddSeparator()
	disconnItem = systray.AddMenuItem("Tümünü kes", "Tüm aktif tünelleri kapat")
	disconnItem.Hide()
	openMultiItem := systray.AddMenuItem("Çoklu arayüzü aç", "Birden fazla profil ve tünel yönetimi")
	openSingleItem := systray.AddMenuItem("Tekli arayüzü aç", "Tek yapılandırma ile basit arayüz")
	svcItem = systray.AddMenuItem("Servisi başlat", "Arka plan servisini başlat (yönetici onayı gerekir)")
	svcItem.Hide()
	systray.AddSeparator()
	autoItem = systray.AddMenuItemCheckbox("Windows ile başlat", "", autostartEnabled())
	quitItem := systray.AddMenuItem("Çıkış", "Tepsiyi ve açık arayüzleri kapatır (tüneller açık kalır)")

	go func() {
		for {
			select {
			case <-disconnItem.ClickedCh:
				go run(func() (string, error) { return app.Disconnect("") })
			case <-openMultiItem.ClickedCh:
				launch("poseavpn-multi.exe")
			case <-openSingleItem.ClickedCh:
				launch("poseavpn-single.exe")
			case <-svcItem.ClickedCh:
				go serviceAction()
			case <-autoItem.ClickedCh:
				toggleAutostart()
			case <-quitItem.ClickedCh:
				closeGUIs()
				systray.Quit()
				return
			}
		}
	}()

	refresh()
	go func() {
		for range time.Tick(3 * time.Second) {
			refresh()
		}
	}()
}

// refresh reads the backend state and updates icon, tooltip and menu.
func refresh() {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	st := app.GetState()

	active := map[string]bool{}
	for _, t := range st.Tunnels {
		active[strings.ToLower(t.TunnelName)] = true
	}

	type entry struct {
		name string
		last bool
		on   bool
	}
	var entries []entry
	seen := map[string]bool{}
	for _, p := range st.Profiles {
		if p.Error != "" {
			continue
		}
		entries = append(entries, entry{name: p.Name, on: active[strings.ToLower(p.Name)]})
		seen[strings.ToLower(p.Name)] = true
	}
	// remembered single-mode config that is not part of the profile library
	if st.LastConfig != nil && st.LastConfig.Error == "" && !seen[strings.ToLower(st.LastConfig.Name)] {
		entries = append(entries, entry{name: st.LastConfig.Name, last: true, on: active[strings.ToLower(st.LastConfig.Name)]})
	}
	// tunnels started elsewhere (CLI) that have no profile entry
	for _, t := range st.Tunnels {
		if !seen[strings.ToLower(t.TunnelName)] && (st.LastConfig == nil || !strings.EqualFold(st.LastConfig.Name, t.TunnelName)) {
			entries = append(entries, entry{name: t.TunnelName, on: true})
		}
	}
	if len(entries) > maxSlots {
		entries = entries[:maxSlots]
	}

	// signature to avoid needless menu churn
	var sig strings.Builder
	sig.WriteString(st.Service)
	for _, e := range entries {
		fmt.Fprintf(&sig, "|%s:%t:%t", e.name, e.last, e.on)
	}
	n := len(st.Tunnels)
	fmt.Fprintf(&sig, "|n=%d", n)
	if sig.String() == lastSig {
		return
	}
	lastSig = sig.String()

	mu.Lock()
	for i, s := range slots {
		if i < len(entries) {
			e := entries[i]
			s.name, s.last, s.on = e.name, e.last, e.on
			s.item.SetTitle(e.name)
			if e.on {
				s.item.Check()
			} else {
				s.item.Uncheck()
			}
			if st.Service == "running" {
				s.item.Enable()
			} else {
				s.item.Disable()
			}
			s.item.Show()
		} else {
			s.name = ""
			s.item.Hide()
		}
	}
	mu.Unlock()

	if len(entries) == 0 {
		emptyItem.Show()
	} else {
		emptyItem.Hide()
	}
	if n > 0 {
		disconnItem.Show()
	} else {
		disconnItem.Hide()
	}

	switch st.Service {
	case "running":
		svcItem.Hide()
	case "stopped":
		svcItem.SetTitle("Servisi başlat")
		svcItem.Show()
	default:
		svcItem.SetTitle("Servisi kur ve başlat")
		svcItem.Show()
	}

	switch {
	case st.Service != "running":
		systray.SetIcon(iconWarn)
		systray.SetTooltip("PoseaVPN — arka plan servisi çalışmıyor")
		headerItem.SetTitle("Servis çalışmıyor")
	case n > 0:
		systray.SetIcon(iconOn)
		tip := fmt.Sprintf("PoseaVPN — %d aktif tünel", n)
		systray.SetTooltip(tip)
		headerItem.SetTitle(tip[len("PoseaVPN — "):])
	default:
		systray.SetIcon(iconOff)
		systray.SetTooltip("PoseaVPN — bağlı değil")
		headerItem.SetTitle("Bağlı değil")
	}
}

func toggleSlot(s *slot) {
	mu.Lock()
	name, last, on := s.name, s.last, s.on
	mu.Unlock()
	if name == "" {
		return
	}
	switch {
	case on:
		go run(func() (string, error) { return app.Disconnect(name) })
	case last:
		go run(func() (string, error) { return app.ConnectLast() })
	default:
		go run(func() (string, error) { return app.ConnectProfile(name) })
	}
}

// run executes a backend action, reports failures and refreshes the menu.
func run(fn func() (string, error)) {
	headerItem.SetTitle("Çalışıyor…")
	_, err := fn()
	invalidate() // force menu rebuild
	refresh()
	if err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "AUTH_REQUIRED:") {
			msg = "Bu OpenVPN profili kullanıcı adı ve şifre gerektiriyor.\nLütfen önce PoseaVPN uygulamasını açıp bağlanarak bilgilerinizi girin/kaydedin."
		}
		messageBox("PoseaVPN", msg, windows.MB_OK|windows.MB_ICONERROR)
	}
}

func serviceAction() {
	st := app.GetState()
	var err error
	if st.Service == "notinstalled" {
		err = app.ServiceInstall()
	} else {
		err = app.ServiceStart()
	}
	if err != nil {
		messageBox("PoseaVPN", err.Error(), windows.MB_OK|windows.MB_ICONERROR)
		return
	}
	// the elevated helper needs a moment
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second)
		invalidate()
		refresh()
		if app.GetState().Service == "running" {
			return
		}
	}
}

// closeGUIs closes the single and multi apps when the tray exits. It first asks
// politely (WM_CLOSE, so the apps can shut down cleanly) and force-kills whatever
// is still alive after a short grace period. Tunnels are left running.
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
		taskkill(true, n) // no-op when the app already closed
	}
	logf("closed desktop apps on exit")
}

// launch starts one of the desktop apps that sits next to the tray executable.
// A second launch just brings the already open window forward.
func launch(exeName string) {
	self, _ := os.Executable()
	path := filepath.Join(filepath.Dir(self), exeName)
	if _, err := os.Stat(path); err != nil {
		messageBox("PoseaVPN", exeName+" bulunamadı:\n"+filepath.Dir(self), windows.MB_OK|windows.MB_ICONWARNING)
		return
	}
	_ = exec.Command(path).Start()
}

// openWindow (left click on the icon) starts the multi app when profiles exist,
// otherwise the single app.
func openWindow() {
	if len(app.GetState().Profiles) > 0 {
		launch("poseavpn-multi.exe")
		return
	}
	launch("poseavpn-single.exe")
}

func messageBox(title, text string, flags uint32) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	_, _ = windows.MessageBox(0, m, t, flags|windows.MB_SETFOREGROUND)
}

// ---------------------------------------------------------------- autostart

// enableAutostartOnFirstRun registers the tray to start with Windows the very first
// time it runs, so it no longer has to be opened by hand. A flag in HKCU remembers
// that this already happened, so a user who later turns "Windows ile başlat" off
// is not overridden.
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
