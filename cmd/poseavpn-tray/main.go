// poseavpn-tray is the system tray companion of the PoseaVPN desktop apps.
// It runs as the normal (non-admin) user, talks to the SYSTEM service through
// the same backend as the GUIs and keeps running when the GUI window is closed.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"poseavpn/pkg/guiapp"
)

const maxSlots = 20

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
	unlock, alreadyRunning, err := singleInstanceLock()
	if alreadyRunning {
		logf("another tray instance is already running, exiting")
		return
	}
	if err != nil {
		logf("single instance error: %v", err)
		return
	}
	defer unlock()

	// Wait for taskbar / UI environment if needed
	if waitForTaskbar(45 * time.Second) {
		logf("taskbar is ready")
	} else {
		logf("taskbar not found after waiting; trying anyway")
	}

	systray.Run(onReady, func() { logf("systray exited") })
	logf("systray.Run returned")
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
	setupTrayClickBehavior()

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
	autoItem = systray.AddMenuItemCheckbox(autostartMenuTitle, "", autostartEnabled())
	quitItem := systray.AddMenuItem("Çıkış", "Tepsiyi ve açık arayüzleri kapatır (tüneller açık kalır)")

	go func() {
		for {
			select {
			case <-disconnItem.ClickedCh:
				go run(func() (string, error) { return app.Disconnect("") })
			case <-openMultiItem.ClickedCh:
				launchMulti()
			case <-openSingleItem.ClickedCh:
				launchSingle()
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
		showError("PoseaVPN", msg)
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
		showError("PoseaVPN", err.Error())
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

// openWindow (left click on the icon) starts the multi app when profiles exist,
// otherwise the single app.
func openWindow() {
	if len(app.GetState().Profiles) > 0 {
		launchMulti()
		return
	}
	launchSingle()
}
