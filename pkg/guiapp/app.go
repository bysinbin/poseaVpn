// Package guiapp is the shared backend of the PoseaVPN desktop apps
// (single-config and multi-config). Its exported methods are bound to the
// Wails frontend. It runs as the normal, non-admin user and talks to the
// SYSTEM service over the named pipe.
package guiapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"poseavpn/pkg/config"
	"poseavpn/pkg/ipc"
	"poseavpn/pkg/ovpn"
	"poseavpn/pkg/tunnel"
)

const (
	ModeSingle = "single"
	ModeMulti  = "multi"

	serviceName = "PoseaVPN"
	Version     = "1.3.0"
)

// ProfileInfo is a parsed summary of a .conf or .ovpn file (private keys are never exposed).
type ProfileInfo struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Addresses  []string `json:"addresses"`
	Endpoint   string   `json:"endpoint"`
	AllowedIPs []string `json:"allowedIps"`
	DNS        []string `json:"dns"`
	Transport  string   `json:"transport"`
	Proto      string   `json:"proto"` // "wireguard" or "openvpn"
	NeedsAuth  bool     `json:"needsAuth"`
	Active     bool     `json:"active"`
	// External is true for a config that lives outside the profile library, e.g. the
	// file remembered by the single app. It is listed too, but never copied or deleted.
	External bool   `json:"external"`
	Error    string `json:"error,omitempty"`
}

// Credential holds username and password for OpenVPN auth-user-pass.
type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// State is everything the UI needs for one refresh.
type State struct {
	Mode       string           `json:"mode"`
	Version    string           `json:"version"`
	Service    string           `json:"service"` // running | stopped | notinstalled | unknown
	Tunnels    []ipc.StatusInfo `json:"tunnels"`
	Profiles   []ProfileInfo    `json:"profiles"`
	LastConfig *ProfileInfo     `json:"lastConfig"`
}

// ImportResult summarises an import action.
type ImportResult struct {
	Imported []string `json:"imported"`
	Errors   []string `json:"errors"`
}

type settings struct {
	LastConfig  string                `json:"last_config"`
	Credentials map[string]Credential `json:"credentials,omitempty"`
}

// App is the Wails-bound application object.
type App struct {
	ctx    context.Context
	mode   string
	client *ipc.Client
	mu     sync.Mutex
}

// New creates the backend for the given mode (ModeSingle or ModeMulti).
func New(mode string) *App {
	return &App{mode: mode, client: ipc.NewClient()}
}

// Startup is the Wails OnStartup hook.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	go ensureTray()
}

// Shutdown is the Wails OnShutdown hook (tunnels are left running on purpose).
func (a *App) Shutdown(ctx context.Context) {}

// ---------------------------------------------------------------- paths

func dataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "PoseaVPN")
}

func profilesDir() string { return filepath.Join(dataDir(), "profiles") }

func settingsPath() string { return filepath.Join(dataDir(), "settings.json") }

func loadSettings() settings {
	var s settings
	if data, err := os.ReadFile(settingsPath()); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func saveSettings(s settings) {
	if err := os.MkdirAll(dataDir(), 0o700); err != nil {
		return
	}
	if data, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(settingsPath(), data, 0o600)
	}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

// cleanName makes a safe adapter / file name. Only a ".conf" or ".ovpn" extension is removed,
// so names that contain dots (e.g. "sirket.v2") stay intact.
func cleanName(s string) string {
	s = filepath.Base(s)
	if strings.EqualFold(filepath.Ext(s), ".conf") {
		s = s[:len(s)-len(".conf")]
	} else if strings.EqualFold(filepath.Ext(s), ".ovpn") {
		s = s[:len(s)-len(".ovpn")]
	}
	s = strings.TrimSpace(unsafeName.ReplaceAllString(s, "_"))
	if s == "" {
		s = "profile"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

// ---------------------------------------------------------------- state

// GetState returns the current service, tunnel and profile state.
func (a *App) GetState() State {
	st := State{
		Mode:     a.mode,
		Version:  Version,
		Service:  queryServiceState(),
		Tunnels:  []ipc.StatusInfo{},
		Profiles: []ProfileInfo{},
	}

	active := map[string]bool{}
	if st.Service == "running" {
		if list, err := a.client.List(); err == nil && list != nil {
			st.Tunnels = list
			for _, t := range list {
				active[strings.ToLower(t.TunnelName)] = true
			}
		}
	}

	if a.mode == ModeMulti {
		st.Profiles = listProfiles(active)
	}
	if last := loadSettings().LastConfig; last != "" {
		if _, err := os.Stat(last); err == nil {
			p := describe(last, active)
			st.LastConfig = &p
			// The multi app also lists the file the single app remembered,
			// otherwise the two apps (and the tray) would disagree.
			if a.mode == ModeMulti && p.External && !hasProfile(st.Profiles, p.Name) {
				st.Profiles = append(st.Profiles, p)
				sortProfiles(st.Profiles)
			}
		}
	}
	return st
}

func hasProfile(list []ProfileInfo, name string) bool {
	for _, p := range list {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

func sortProfiles(list []ProfileInfo) {
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
}

// inLibrary reports whether path is a file inside the profile library.
func inLibrary(path string) bool {
	return strings.EqualFold(filepath.Clean(filepath.Dir(path)), filepath.Clean(profilesDir()))
}

func describe(path string, active map[string]bool) ProfileInfo {
	p := ProfileInfo{
		Name:       cleanName(path),
		Path:       path,
		Addresses:  []string{},
		AllowedIPs: []string{},
		DNS:        []string{},
		External:   !inLibrary(path),
	}
	p.Active = active[strings.ToLower(p.Name)]

	if strings.EqualFold(filepath.Ext(path), ".ovpn") {
		p.Proto = "openvpn"
		sum, err := ovpn.ParseFile(path)
		if err != nil {
			p.Error = err.Error()
			return p
		}
		p.NeedsAuth = sum.AuthUserPass
		p.Endpoint = sum.Remote
		p.DNS = sum.DNS
		p.AllowedIPs = sum.Routes
		protoStr := "udp"
		if sum.Proto != "" {
			protoStr = sum.Proto
		}
		p.Transport = "OpenVPN (" + protoStr + ")"
		return p
	}

	p.Proto = "wireguard"
	cfg, err := config.ParseFile(path)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	for _, ad := range cfg.Interface.Addresses {
		p.Addresses = append(p.Addresses, ad.String())
	}
	for _, d := range cfg.Interface.DNS {
		p.DNS = append(p.DNS, d.String())
	}
	p.DNS = append(p.DNS, cfg.Interface.DNSSearches...)
	p.Transport = string(cfg.Interface.Transport)
	if len(cfg.Peers) > 0 {
		p.Endpoint = cfg.Peers[0].Endpoint
		for _, pr := range cfg.Peers {
			for _, ip := range pr.AllowedIPs {
				p.AllowedIPs = append(p.AllowedIPs, ip.String())
			}
		}
	}
	return p
}

func listProfiles(active map[string]bool) []ProfileInfo {
	out := []ProfileInfo{}
	entries, err := os.ReadDir(profilesDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if !strings.EqualFold(ext, ".conf") && !strings.EqualFold(ext, ".ovpn") {
			continue
		}
		out = append(out, describe(filepath.Join(profilesDir(), e.Name()), active))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ---------------------------------------------------------------- tunnels

func (a *App) connectPathWithAuth(path, username, password string, rememberAuth bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("dosya okunamadı: %w", err)
	}

	name := cleanName(path)
	isOvpn := strings.EqualFold(filepath.Ext(path), ".ovpn") || ovpn.IsOpenVPN(string(data))

	if isOvpn {
		sum, err := ovpn.Parse(strings.NewReader(string(data)))
		if err != nil {
			return "", fmt.Errorf("OpenVPN yapılandırması geçersiz: %w", err)
		}
		if sum.AuthUserPass {
			if username == "" {
				cred := a.GetCredentials(name)
				username = cred.Username
				password = cred.Password
			}
			if username == "" {
				return "", fmt.Errorf("AUTH_REQUIRED: '%s' profili kullanıcı adı ve şifre gerektiriyor", name)
			}
		}
		resp, err := a.client.ConnectWithAuth(name, string(data), "openvpn", username, password)
		if err != nil {
			return "", err
		}
		if !resp.Success {
			if strings.Contains(resp.Message, "AUTH_FAILED") {
				a.deleteCredentials(name)
			}
			return "", fmt.Errorf("%s", resp.Message)
		}
		if sum.AuthUserPass && rememberAuth {
			_ = a.SaveCredentials(name, username, password)
		}
		if !inLibrary(path) {
			s := loadSettings()
			s.LastConfig = path
			saveSettings(s)
		}
		return resp.Message, nil
	}

	if _, err := config.Parse(strings.NewReader(string(data))); err != nil {
		return "", fmt.Errorf("yapılandırma geçersiz: %w", err)
	}
	resp, err := a.client.Connect(name, string(data))
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "", fmt.Errorf("%s", resp.Message)
	}
	// Only a config outside the library is "remembered"; connecting a library
	// profile must not replace the file the single app remembers.
	if !inLibrary(path) {
		s := loadSettings()
		s.LastConfig = path
		saveSettings(s)
	}
	// the service reports address mapping in English; show it in Turkish
	return strings.Replace(resp.Message, "address mapping:", "Adres eşleme:", 1), nil
}

func (a *App) connectPath(path string) (string, error) {
	return a.connectPathWithAuth(path, "", "", false)
}

// ConnectProfile connects a profile by name: a library profile, or the config
// remembered by the single app when it has that name (multi mode).
func (a *App) ConnectProfile(name string) (string, error) {
	path, err := profilePath(name)
	if err != nil {
		return "", err
	}
	return a.connectPath(path)
}

// ConnectProfileWithAuth connects an OpenVPN profile with explicit credentials.
func (a *App) ConnectProfileWithAuth(name, username, password string, remember bool) (string, error) {
	path, err := profilePath(name)
	if err != nil {
		return "", err
	}
	return a.connectPathWithAuth(path, username, password, remember)
}

// ConnectLast connects the remembered config file (single mode).
func (a *App) ConnectLast() (string, error) {
	last := loadSettings().LastConfig
	if last == "" {
		return "", fmt.Errorf("önce bir yapılandırma dosyası seçin")
	}
	return a.connectPath(last)
}

// Disconnect stops one tunnel; an empty name stops all of them.
func (a *App) Disconnect(name string) (string, error) {
	resp, err := a.client.DisconnectTunnel(name)
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "", fmt.Errorf("%s", resp.Message)
	}
	return resp.Message, nil
}

// GetProfileLog returns the latest log output for a profile or tunnel.
func (a *App) GetProfileLog(name string) (string, error) {
	logData, err := a.client.GetLog(name)
	if err == nil && logData != "" {
		return logData, nil
	}
	// Also check directly on disk if service is stopped
	p := tunnel.OpenVPNLogPath(name)
	if data, rerr := os.ReadFile(p); rerr == nil && len(data) > 0 {
		return string(data), nil
	}
	if err != nil {
		return "", err
	}
	return "Bu profil için henüz bir bağlantı günlüğü kaydedilmemiş.", nil
}

// ---------------------------------------------------------------- credentials

// SaveCredentials stores username and password for a profile.
func (a *App) SaveCredentials(name, username, password string) error {
	s := loadSettings()
	if s.Credentials == nil {
		s.Credentials = make(map[string]Credential)
	}
	s.Credentials[strings.ToLower(cleanName(name))] = Credential{
		Username: username,
		Password: password,
	}
	saveSettings(s)
	return nil
}

// GetCredentials returns saved credentials for a profile.
func (a *App) GetCredentials(name string) Credential {
	s := loadSettings()
	if s.Credentials != nil {
		if cred, ok := s.Credentials[strings.ToLower(cleanName(name))]; ok {
			return cred
		}
	}
	return Credential{}
}

func (a *App) deleteCredentials(name string) {
	s := loadSettings()
	if s.Credentials != nil {
		delete(s.Credentials, strings.ToLower(cleanName(name)))
		saveSettings(s)
	}
}

// ---------------------------------------------------------------- configs

var confFilter = runtime.FileFilter{DisplayName: "VPN yapılandırmaları (*.conf, *.ovpn)", Pattern: "*.conf;*.ovpn"}

// ChooseConfig opens a file dialog and remembers the selection (single mode).
func (a *App) ChooseConfig() (*ProfileInfo, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "VPN yapılandırma dosyası seç (.conf veya .ovpn)",
		Filters: []runtime.FileFilter{confFilter, importFilter},
	})
	if err != nil || path == "" {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".ovpn") {
		if _, perr := ovpn.ParseFile(path); perr != nil {
			return nil, fmt.Errorf("OpenVPN yapılandırması geçersiz: %w", perr)
		}
	} else {
		if _, perr := config.ParseFile(path); perr != nil {
			return nil, fmt.Errorf("yapılandırma geçersiz: %w", perr)
		}
	}
	s := loadSettings()
	s.LastConfig = path
	saveSettings(s)
	p := describe(path, nil)
	return &p, nil
}

// ImportConfigs lets the user pick .conf / .ovpn files and/or .zip archives that contain
// them, and copies every valid config into the library.
func (a *App) ImportConfigs() (*ImportResult, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "VPN yapılandırmalarını içe aktar (.conf, .ovpn veya .zip)",
		Filters: []runtime.FileFilter{importFilter, confFilter, zipFilter},
	})
	if err != nil || len(paths) == 0 {
		return &ImportResult{Imported: []string{}, Errors: []string{}}, err
	}
	res := &ImportResult{Imported: []string{}, Errors: []string{}}
	for _, p := range paths {
		if strings.EqualFold(filepath.Ext(p), ".zip") {
			a.importZip(p, res)
			continue
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			res.Errors = append(res.Errors, filepath.Base(p)+": "+rerr.Error())
			continue
		}
		name, ierr := a.saveProfile(cleanName(p), string(data))
		if ierr != nil {
			res.Errors = append(res.Errors, filepath.Base(p)+": "+ierr.Error())
			continue
		}
		res.Imported = append(res.Imported, name)
	}
	return res, nil
}

// ImportText stores pasted configuration text as a new profile.
func (a *App) ImportText(name, text string) (string, error) {
	return a.saveProfile(cleanName(name), text)
}

// saveProfile validates and writes a profile, avoiding name collisions.
func (a *App) saveProfile(name, text string) (string, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	ext := ".conf"
	if ovpn.IsOpenVPN(text) {
		ext = ".ovpn"
		if _, err := ovpn.Parse(strings.NewReader(text)); err != nil {
			return "", fmt.Errorf("OpenVPN yapılandırması geçersiz: %w", err)
		}
	} else {
		if _, err := config.Parse(strings.NewReader(text)); err != nil {
			return "", fmt.Errorf("yapılandırma geçersiz: %w", err)
		}
	}

	if err := os.MkdirAll(profilesDir(), 0o700); err != nil {
		return "", err
	}
	final := name
	for i := 2; ; i++ {
		cPath := filepath.Join(profilesDir(), final+".conf")
		oPath := filepath.Join(profilesDir(), final+".ovpn")
		_, cErr := os.Stat(cPath)
		_, oErr := os.Stat(oPath)
		if os.IsNotExist(cErr) && os.IsNotExist(oErr) {
			break
		}
		final = fmt.Sprintf("%s-%d", name, i)
	}
	if err := os.WriteFile(filepath.Join(profilesDir(), final+ext), []byte(text), 0o600); err != nil {
		return "", err
	}
	return final, nil
}

// DeleteProfile removes a library profile, or, for the config remembered by the
// single app, only forgets it (the file itself is never touched).
// Active tunnels must be stopped first.
func (a *App) DeleteProfile(name string) error {
	if list, err := a.client.List(); err == nil {
		for _, t := range list {
			if strings.EqualFold(t.TunnelName, name) {
				return fmt.Errorf("önce '%s' tünelini kapatın", name)
			}
		}
	}
	p, err := profilePath(name)
	if err != nil {
		return err
	}
	if !inLibrary(p) {
		if last := loadSettings().LastConfig; last != "" && strings.EqualFold(cleanName(last), cleanName(name)) {
			s := loadSettings()
			s.LastConfig = ""
			saveSettings(s)
			return nil
		}
	}
	a.deleteCredentials(name)
	return os.Remove(p)
}

// ---------------------------------------------------------------- service

// ServiceStop stops the service gracefully via IPC.
func (a *App) ServiceStop() error {
	return a.client.StopService()
}
