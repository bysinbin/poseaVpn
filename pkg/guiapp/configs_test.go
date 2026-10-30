package guiapp

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const validConf = `[Interface]
PrivateKey = aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
Address = 10.0.0.2/24

[Peer]
PublicKey = bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb=
Endpoint = 203.0.113.1:51820
AllowedIPs = 10.0.0.0/24
`

// isolate the per-user data dir (os.UserConfigDir uses %APPDATA% on Windows, $HOME on macOS/Linux)
func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
}

func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "configs.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

func TestImportZip(t *testing.T) {
	isolate(t)
	a := New(ModeMulti)

	zp := makeZip(t, map[string]string{
		"a.conf":               validConf,
		"nested/dir/b.v2.conf": validConf, // dotted name + subfolder
		"bom.conf":             "\ufeff" + validConf,
		"readme.txt":           "not a config",
		"bad.conf":             "this is not wireguard",
		"__MACOSX/._a.conf":    "junk",
		"../../evil.conf":      validConf, // path tricks must not escape the library
	})

	res := &ImportResult{Imported: []string{}, Errors: []string{}}
	a.importZip(zp, res)

	got := append([]string{}, res.Imported...)
	sort.Strings(got)
	want := []string{"a", "b.v2", "bom", "evil"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("imported = %v, want %v (errors: %v)", got, want, res.Errors)
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "bad.conf") {
		t.Fatalf("expected exactly one error for bad.conf, got %v", res.Errors)
	}

	// everything must have landed inside the profile library
	entries, _ := os.ReadDir(profilesDir())
	if len(entries) != 4 {
		t.Fatalf("library has %d files, want 4", len(entries))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(profilesDir()), "..", "evil.conf")); err == nil {
		t.Fatal("zip entry escaped the profile library")
	}
}

func TestImportZipEmptyAndDuplicates(t *testing.T) {
	isolate(t)
	a := New(ModeMulti)

	empty := makeZip(t, map[string]string{"readme.txt": "x"})
	res := &ImportResult{Imported: []string{}, Errors: []string{}}
	a.importZip(empty, res)
	if len(res.Imported) != 0 || len(res.Errors) != 1 {
		t.Fatalf("empty zip: imported=%v errors=%v", res.Imported, res.Errors)
	}

	// importing the same zip twice must not overwrite: names get -2
	zp := makeZip(t, map[string]string{"a.conf": validConf})
	res = &ImportResult{Imported: []string{}, Errors: []string{}}
	a.importZip(zp, res)
	a.importZip(zp, res)
	if strings.Join(res.Imported, ",") != "a,a-2" {
		t.Fatalf("duplicate import = %v", res.Imported)
	}
}

func TestEditRenameAndValidate(t *testing.T) {
	isolate(t)
	a := New(ModeMulti)

	name, err := a.ImportText("sirket.vpn", validConf)
	if err != nil || name != "sirket.vpn" {
		t.Fatalf("ImportText = %q, %v (dotted names must survive)", name, err)
	}

	text, err := a.GetProfileText("sirket.vpn")
	if err != nil || !strings.Contains(text, "[Interface]") {
		t.Fatalf("GetProfileText = %v", err)
	}

	if msg := a.ValidateConfig("garbage"); msg == "" {
		t.Fatal("ValidateConfig accepted garbage")
	}
	if msg := a.ValidateConfig(validConf); msg != "" {
		t.Fatalf("ValidateConfig rejected a valid config: %s", msg)
	}

	// edit + rename
	edited := strings.Replace(text, "10.0.0.2/24", "10.0.0.9/24", 1)
	final, err := a.SaveProfileText("sirket.vpn", "yeni-ad", edited)
	if err != nil || final != "yeni-ad" {
		t.Fatalf("SaveProfileText = %q, %v", final, err)
	}
	if _, err := os.Stat(filepath.Join(profilesDir(), "sirket.vpn.conf")); err == nil {
		t.Fatal("old file still exists after rename")
	}
	got, _ := a.GetProfileText("yeni-ad")
	if !strings.Contains(got, "10.0.0.9/24") {
		t.Fatal("edit was not saved")
	}

	// invalid edit must be rejected and must not damage the file
	if _, err := a.SaveProfileText("yeni-ad", "yeni-ad", "garbage"); err == nil {
		t.Fatal("invalid config was saved")
	}
	still, _ := a.GetProfileText("yeni-ad")
	if still != got {
		t.Fatal("file changed after a rejected save")
	}

	// renaming onto an existing profile is refused
	if _, err := a.ImportText("diger", validConf); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveProfileText("yeni-ad", "diger", got); err == nil {
		t.Fatal("rename onto an existing profile was allowed")
	}
}

func TestOpenVPNProfileHandling(t *testing.T) {
	isolate(t)
	a := New(ModeMulti)

	ovpnText := `dev tun
proto udp4
remote 203.0.113.10 1194
auth-user-pass
dhcp-option DNS 10.0.0.1
route 192.168.10.0 255.255.255.0
`
	name, err := a.ImportText("sample_ovpn", ovpnText)
	if err != nil {
		t.Fatalf("ImportText failed: %v", err)
	}
	if name != "sample_ovpn" {
		t.Fatalf("unexpected profile name: %s", name)
	}

	st := a.GetState()
	if len(st.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(st.Profiles))
	}
	p := st.Profiles[0]
	if p.Proto != "openvpn" {
		t.Errorf("expected proto openvpn, got %s", p.Proto)
	}
	if !p.NeedsAuth {
		t.Errorf("expected NeedsAuth=true")
	}
	if p.Endpoint != "203.0.113.10:1194" {
		t.Errorf("expected endpoint 203.0.113.10:1194, got %s", p.Endpoint)
	}

	// Test credentials
	_ = a.SaveCredentials(name, "testuser", "testpass")
	cred := a.GetCredentials(name)
	if cred.Username != "testuser" || cred.Password != "testpass" {
		t.Errorf("credentials not remembered correctly: %+v", cred)
	}

	// Test validate
	if msg := a.ValidateConfig(ovpnText); msg != "" {
		t.Errorf("ValidateConfig rejected valid ovpn: %s", msg)
	}
}


func TestKeyGenerationAndCreateTunnel(t *testing.T) {
	isolate(t)
	a := New(ModeMulti)

	kp, err := a.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	if len(kp.PrivateKey) != 44 || len(kp.PublicKey) != 44 {
		t.Fatalf("unexpected key lengths: %+v", kp)
	}

	derivedPub, err := a.PublicKeyFromPrivate(kp.PrivateKey)
	if err != nil {
		t.Fatalf("PublicKeyFromPrivate failed: %v", err)
	}
	if derivedPub != kp.PublicKey {
		t.Errorf("derived public key mismatch: %s != %s", derivedPub, kp.PublicKey)
	}

	conf := `[Interface]
PrivateKey = ` + kp.PrivateKey + `
Address = 10.0.0.5/24

[Peer]
PublicKey = ` + kp.PublicKey + `
Endpoint = 198.51.100.1:51820
AllowedIPs = 0.0.0.0/0
`
	prof, err := a.CreateTunnel("new-wireguard-tunnel", conf)
	if err != nil {
		t.Fatalf("CreateTunnel failed: %v", err)
	}
	if prof.Name != "new-wireguard-tunnel" {
		t.Errorf("unexpected profile name: %s", prof.Name)
	}
	if prof.Proto != "wireguard" {
		t.Errorf("unexpected proto: %s", prof.Proto)
	}
	if prof.Endpoint != "198.51.100.1:51820" {
		t.Errorf("unexpected endpoint: %s", prof.Endpoint)
	}
}
