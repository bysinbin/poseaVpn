package guiapp

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"poseavpn/pkg/config"
	"poseavpn/pkg/ovpn"
)

var (
	importFilter = runtime.FileFilter{DisplayName: "VPN yapılandırmaları (*.conf, *.ovpn, *.zip)", Pattern: "*.conf;*.ovpn;*.zip"}
	ovpnFilter   = runtime.FileFilter{DisplayName: "OpenVPN (*.ovpn)", Pattern: "*.ovpn"}
	zipFilter    = runtime.FileFilter{DisplayName: "Zip arşivi (*.zip)", Pattern: "*.zip"}
)

const (
	maxConfSize   = 1024 << 10 // 1 MB is plenty for both WireGuard and OpenVPN configs with embedded certs
	maxZipEntries = 2000
)

// importZip imports every .conf inside a zip archive. Entries are only read in
// memory (never extracted to disk), so path tricks inside the archive cannot
// write anywhere; the profile name is derived from the entry's file name.
func (a *App) importZip(zipPath string, res *ImportResult) {
	base := filepath.Base(zipPath)

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		res.Errors = append(res.Errors, base+": zip açılamadı: "+err.Error())
		return
	}
	defer r.Close()

	if len(r.File) > maxZipEntries {
		res.Errors = append(res.Errors, fmt.Sprintf("%s: arşivde çok fazla dosya var (%d)", base, len(r.File)))
		return
	}

	found := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		ext := path.Ext(f.Name)
		if !strings.EqualFold(ext, ".conf") && !strings.EqualFold(ext, ".ovpn") {
			continue
		}
		found++
		label := base + "!" + f.Name

		if f.UncompressedSize64 > maxConfSize {
			res.Errors = append(res.Errors, label+": dosya çok büyük")
			continue
		}
		rc, oerr := f.Open()
		if oerr != nil {
			res.Errors = append(res.Errors, label+": "+oerr.Error())
			continue
		}
		data, rerr := io.ReadAll(io.LimitReader(rc, maxConfSize+1))
		rc.Close()
		if rerr != nil {
			res.Errors = append(res.Errors, label+": "+rerr.Error())
			continue
		}
		if len(data) > maxConfSize {
			res.Errors = append(res.Errors, label+": dosya çok büyük")
			continue
		}

		name, serr := a.saveProfile(cleanName(path.Base(f.Name)), string(data))
		if serr != nil {
			res.Errors = append(res.Errors, label+": "+serr.Error())
			continue
		}
		res.Imported = append(res.Imported, name)
	}

	if found == 0 {
		res.Errors = append(res.Errors, base+": arşivde .conf veya .ovpn dosyası yok")
	}
}

// ---------------------------------------------------------------- editing

// profilePath resolves a profile name to its file: the library first, then the
// config remembered by the single app.
func profilePath(name string) (string, error) {
	c := filepath.Join(profilesDir(), cleanName(name)+".conf")
	if _, err := os.Stat(c); err == nil {
		return c, nil
	}
	o := filepath.Join(profilesDir(), cleanName(name)+".ovpn")
	if _, err := os.Stat(o); err == nil {
		return o, nil
	}
	if last := loadSettings().LastConfig; last != "" && strings.EqualFold(cleanName(last), cleanName(name)) {
		if _, err := os.Stat(last); err == nil {
			return last, nil
		}
	}
	return "", fmt.Errorf("'%s' profili bulunamadı", name)
}

// tunnelActive reports whether a tunnel with this name is currently up.
func (a *App) tunnelActive(name string) bool {
	list, err := a.client.List()
	if err != nil {
		return false
	}
	for _, t := range list {
		if strings.EqualFold(t.TunnelName, name) {
			return true
		}
	}
	return false
}

// GetProfileText returns the raw .conf text of a profile for the editor.
func (a *App) GetProfileText(name string) (string, error) {
	p, err := profilePath(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(string(data), "\ufeff"), nil
}

// ValidateConfig returns "" when the text is a valid config, else a readable error.
func (a *App) ValidateConfig(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	if ovpn.IsOpenVPN(text) {
		if _, err := ovpn.Parse(strings.NewReader(text)); err != nil {
			return err.Error()
		}
		return ""
	}
	if _, err := config.Parse(strings.NewReader(text)); err != nil {
		return err.Error()
	}
	return ""
}

// SaveProfileText validates and saves an edited config. newName may rename a
// library profile. Editing is refused while the tunnel is up (the change would
// not apply until it reconnects). It returns the final profile name.
func (a *App) SaveProfileText(oldName, newName, text string) (string, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	isOvpn := ovpn.IsOpenVPN(text)
	if isOvpn {
		if _, err := ovpn.Parse(strings.NewReader(text)); err != nil {
			return "", fmt.Errorf("OpenVPN yapılandırması geçersiz: %w", err)
		}
	} else {
		if _, err := config.Parse(strings.NewReader(text)); err != nil {
			return "", fmt.Errorf("yapılandırma geçersiz: %w", err)
		}
	}

	if a.tunnelActive(oldName) {
		return "", fmt.Errorf("önce '%s' tünelini kapatın, sonra kaydedin", oldName)
	}

	src, err := profilePath(oldName)
	if err != nil {
		return "", err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		newName = cleanName(oldName)
	} else {
		newName = cleanName(newName)
	}
	renamed := !strings.EqualFold(newName, cleanName(oldName))

	ext := filepath.Ext(src)
	if ext == "" {
		if isOvpn {
			ext = ".ovpn"
		} else {
			ext = ".conf"
		}
	}

	// the config remembered by the single app lives outside the library: edit it in place
	if !inLibrary(src) {
		if renamed {
			return "", fmt.Errorf("kitaplık dışındaki dosya yeniden adlandırılamaz; önce içe aktarın")
		}
		if err := os.WriteFile(src, []byte(text), 0o600); err != nil {
			return "", err
		}
		return cleanName(oldName), nil
	}

	dst := filepath.Join(profilesDir(), newName+ext)
	if renamed {
		if _, err := os.Stat(dst); err == nil {
			return "", fmt.Errorf("'%s' adında bir profil zaten var", newName)
		}
	}
	if err := os.WriteFile(dst, []byte(text), 0o600); err != nil {
		return "", err
	}
	if renamed {
		_ = os.Remove(src)
	}
	return newName, nil
}

// GenerateKeyPair creates a new WireGuard private/public key pair.
func (a *App) GenerateKeyPair() (config.KeyPair, error) {
	return config.GenerateKeyPair()
}

// PublicKeyFromPrivate derives a WireGuard public key from a base64-encoded private key.
func (a *App) PublicKeyFromPrivate(privKey string) (string, error) {
	return config.PublicKeyFromPrivate(privKey)
}

// GeneratePresharedKey generates a 32-byte pre-shared key for WireGuard.
func (a *App) GeneratePresharedKey() (string, error) {
	return config.GeneratePresharedKey()
}

// SetClipboard copies the given text to the system clipboard.
func (a *App) SetClipboard(text string) error {
	if a.ctx != nil {
		return runtime.ClipboardSetText(a.ctx, text)
	}
	return nil
}

// CreateTunnel creates a new WireGuard or OpenVPN profile from text, saves it,
// and selects it immediately.
func (a *App) CreateTunnel(name, text string) (ProfileInfo, error) {
	savedName, err := a.saveProfile(cleanName(name), text)
	if err != nil {
		return ProfileInfo{}, err
	}

	path, err := profilePath(savedName)
	if err != nil {
		return ProfileInfo{}, err
	}

	if a.mode == ModeSingle {
		s := loadSettings()
		s.LastConfig = path
		saveSettings(s)
	}

	active := map[string]bool{}
	if list, err := a.client.List(); err == nil {
		for _, t := range list {
			active[strings.ToLower(t.TunnelName)] = true
		}
	}

	return describe(path, active), nil
}

// ExportProfile lets the user choose a location and saves a single profile file.
func (a *App) ExportProfile(name string) (string, error) {
	src, err := profilePath(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("profil okunamadı: %w", err)
	}

	ext := filepath.Ext(src)
	filter := confFilter
	if strings.EqualFold(ext, ".ovpn") {
		filter = ovpnFilter
	}

	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Profili dışa aktar",
		DefaultFilename: filepath.Base(src),
		Filters:         []runtime.FileFilter{filter},
	})
	if err != nil || dest == "" {
		return "", err
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("dosya yazılamadı: %w", err)
	}
	return filepath.Base(dest), nil
}

// ExportAllZip packages all library profiles into a single .zip file.
func (a *App) ExportAllZip() (string, error) {
	entries, err := os.ReadDir(profilesDir())
	if err != nil {
		return "", fmt.Errorf("profiller okunamadı: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if strings.EqualFold(ext, ".conf") || strings.EqualFold(ext, ".ovpn") {
			files = append(files, filepath.Join(profilesDir(), e.Name()))
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("dışa aktarılacak profil bulunamadı")
	}

	timestamp := time.Now().Format("20060102-150405")
	defaultName := fmt.Sprintf("poseavpn-profiller-%s.zip", timestamp)

	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Tüm profilleri zip olarak dışa aktar",
		DefaultFilename: defaultName,
		Filters:         []runtime.FileFilter{zipFilter},
	})
	if err != nil || dest == "" {
		return "", err
	}

	f, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("zip oluşturulamadı: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		w, err := zw.Create(filepath.Base(p))
		if err != nil {
			_ = zw.Close()
			return "", err
		}
		if _, err := w.Write(data); err != nil {
			_ = zw.Close()
			return "", err
		}
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("zip kapatılamadı: %w", err)
	}

	return fmt.Sprintf("%d profil '%s' dosyasına aktarıldı", len(files), filepath.Base(dest)), nil
}
