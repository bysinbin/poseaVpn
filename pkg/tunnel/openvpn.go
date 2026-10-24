package tunnel

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"poseavpn/pkg/ovpn"
)

// OpenVPNTunnel manages an active OpenVPN process and its network adapter.
type OpenVPNTunnel struct {
	name           string
	startedAt      time.Time
	cmd            *exec.Cmd
	mgmtPort       int
	mgmtConn       net.Conn
	summary        *ovpn.Summary
	tempDir        string
	logPath        string
	cleanupAdapter func()

	mu        sync.Mutex
	running   bool
	rxBytes   uint64
	txBytes   uint64
	lastState string
	addresses []string
	stopOnce  sync.Once
	stopCh    chan struct{}
}

func cleanAdapterName(name string) string {
	s := strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	res := b.String()
	if len(res) > 20 {
		res = res[:20]
	}
	return res
}

// StartOpenVPN launches an OpenVPN tunnel instance.
func StartOpenVPN(name, configData, username, password string) (*OpenVPNTunnel, error) {
	openvpnExe, err := FindOpenVPNBinary()
	if err != nil {
		return nil, err
	}

	summary, err := ovpn.Parse(strings.NewReader(configData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenVPN config: %w", err)
	}

	if summary.AuthUserPass && username == "" {
		return nil, fmt.Errorf("AUTH_REQUIRED: '%s' profili kullanıcı adı ve parola gerektiriyor", name)
	}

	tempDir, err := os.MkdirTemp("", "poseavpn-ovpn-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	confPath := filepath.Join(tempDir, "tunnel.ovpn")
	if err := os.WriteFile(confPath, []byte(configData), 0o600); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	// Pick a free TCP port for management
	mgmtListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to allocate management port: %w", err)
	}
	mgmtPort := mgmtListener.Addr().(*net.TCPAddr).Port
	_ = mgmtListener.Close()

	var authFile string
	if summary.AuthUserPass && username != "" {
		authFile = filepath.Join(tempDir, "auth.txt")
		content := username + "\n" + password + "\n"
		if err := os.WriteFile(authFile, []byte(content), 0o600); err != nil {
			_ = os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to write auth file: %w", err)
		}
	}

	cmd, cleanupAdapter, err := prepareOpenVPN(name, openvpnExe, confPath, authFile, mgmtPort)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}

	// Capture log to persistent path
	logPath := OpenVPNLogPath(name)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		if cleanupAdapter != nil {
			cleanupAdapter()
		}
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to start openvpn process: %w", err)
	}

	t := &OpenVPNTunnel{
		name:           name,
		startedAt:      time.Now(),
		cmd:            cmd,
		mgmtPort:       mgmtPort,
		summary:        summary,
		tempDir:        tempDir,
		logPath:        logPath,
		cleanupAdapter: cleanupAdapter,
		running:        true,
		stopCh:         make(chan struct{}),
	}

	// Wait asynchronously for OpenVPN process exit
	go func() {
		_ = cmd.Wait()
		if logFile != nil {
			logFile.Close()
		}
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	// Connect to management interface
	go t.monitorManagement()

	// Wait up to 15 seconds for initial connection or early failure
	waitErr := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if !t.IsRunning() {
				waitErr <- extractOpenVPNError(logPath)
				return
			}
			t.mu.Lock()
			st := t.lastState
			t.mu.Unlock()
			if st == "CONNECTED" {
				waitErr <- nil
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !t.IsRunning() {
			waitErr <- extractOpenVPNError(logPath)
			return
		}
		waitErr <- fmt.Errorf("OpenVPN sunucusuna bağlanma zaman aşımına uğradı (sunucu yanıt vermiyor olabilir)")
	}()

	if err := <-waitErr; err != nil {
		_ = t.Stop()
		return nil, err
	}

	return t, nil
}

func extractOpenVPNError(logPath string) error {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("OpenVPN işlemi başlatılamadı")
	}
	content := string(data)
	if strings.Contains(content, "AUTH_FAILED") {
		return fmt.Errorf("Kimlik doğrulama başarısız (AUTH_FAILED). Kullanıcı adı veya parola hatalı.")
	}
	if strings.Contains(content, "Enter Auth Username:") {
		return fmt.Errorf("AUTH_REQUIRED: OpenVPN kullanıcı adı ve parola gerektiriyor")
	}
	if strings.Contains(content, "TLS Error") {
		return fmt.Errorf("TLS hatası: Sunucu yanıt vermiyor veya sertifika geçersiz.")
	}
	if strings.Contains(content, "Cannot resolve host") {
		return fmt.Errorf("Sunucu alan adı çözümlenemedi (DNS hatası).")
	}

	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.Contains(l, "Options error:") || strings.Contains(l, "ERROR:") || strings.Contains(l, "Fatal") {
			return fmt.Errorf("%s", l)
		}
	}

	return fmt.Errorf("OpenVPN işlemi kapandı (kullanıcı bilgileri veya sunucu ayarları geçersiz olabilir)")
}



func (t *OpenVPNTunnel) monitorManagement() {
	deadline := time.Now().Add(10 * time.Second)
	var conn net.Conn
	var err error
	for time.Now().Before(deadline) {
		conn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", t.mgmtPort), 500*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if conn == nil {
		log.Printf("[OpenVPN %s] Could not connect to management interface: %v", t.name, err)
		return
	}

	t.mu.Lock()
	t.mgmtConn = conn
	t.mu.Unlock()

	defer func() {
		_ = conn.Close()
	}()

	// Ask OpenVPN for byte counts every 1s and state updates
	_, _ = fmt.Fprintf(conn, "bytecount 1\nstate on\nstate\n")

	scanner := bufio.NewScanner(conn)
	privateNetConfigured := false

	for scanner.Scan() {
		select {
		case <-t.stopCh:
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, ">BYTECOUNT:") {
			parts := strings.Split(strings.TrimPrefix(line, ">BYTECOUNT:"), ",")
			if len(parts) >= 2 {
				rx, _ := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
				tx, _ := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
				t.mu.Lock()
				t.rxBytes = rx
				t.txBytes = tx
				t.mu.Unlock()
			}
			continue
		}

		var stateLine string
		if strings.HasPrefix(line, ">STATE:") {
			stateLine = strings.TrimPrefix(line, ">STATE:")
		} else if strings.Count(line, ",") >= 2 && !strings.HasPrefix(line, ">") && !strings.HasPrefix(line, "INFO:") && !strings.HasPrefix(line, "SUCCESS:") {
			stateLine = line
		}

		if stateLine != "" {
			parts := strings.Split(stateLine, ",")
			if len(parts) >= 2 {
				st := strings.TrimSpace(parts[1])
				t.mu.Lock()
				t.lastState = st
				if len(parts) >= 4 && strings.TrimSpace(parts[3]) != "" {
					t.addresses = []string{strings.TrimSpace(parts[3])}
				}
				t.mu.Unlock()

				if (st == "CONNECTED" || st == "ASSIGN_IP") && !privateNetConfigured {
					privateNetConfigured = true
					// Apply Windows Private network category to allow Active Directory SMB/Kerberos
					go t.configurePrivateNetwork()
				}
			}
		}
	}
}

func (t *OpenVPNTunnel) configurePrivateNetwork() {
	time.Sleep(1500 * time.Millisecond)
	t.mu.Lock()
	var ip string
	if len(t.addresses) > 0 {
		ip = t.addresses[0]
	}
	t.mu.Unlock()

	configurePrivateNetworkForIP(t.name, ip)
}

// Log returns the OpenVPN log output.
func (t *OpenVPNTunnel) Log() string {
	if t.logPath != "" {
		data, err := os.ReadFile(t.logPath)
		if err == nil {
			return string(data)
		}
	}
	return ""
}

// TunnelName returns the tunnel name.
func (t *OpenVPNTunnel) TunnelName() string { return t.name }

// Proto returns "openvpn".
func (t *OpenVPNTunnel) Proto() string { return "openvpn" }

// StartedTime returns when the tunnel was started.
func (t *OpenVPNTunnel) StartedTime() time.Time { return t.startedAt }

// IsRunning reports whether the OpenVPN process is still active.
func (t *OpenVPNTunnel) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// Reach returns prefixes claimed by this tunnel.
func (t *OpenVPNTunnel) Reach() []netip.Prefix {
	var list []netip.Prefix
	if t.summary != nil {
		for _, r := range t.summary.Routes {
			// Format e.g. "192.168.10.0/255.255.255.0"
			parts := strings.Split(r, "/")
			if len(parts) == 2 {
				ip := parts[0]
				mask := parts[1]
				// Calculate CIDR prefix
				if p, err := netip.ParsePrefix(ip + "/" + maskToCIDR(mask)); err == nil {
					list = append(list, p)
				}
			}
		}
	}
	return list
}

func maskToCIDR(maskStr string) string {
	ip := net.ParseIP(maskStr).To4()
	if ip == nil {
		return maskStr // might already be prefix length like "24"
	}
	ones, _ := net.IPMask(ip).Size()
	return strconv.Itoa(ones)
}

// Status returns the current StatusInfo.
func (t *OpenVPNTunnel) Status() StatusInfo {
	t.mu.Lock()
	rx := t.rxBytes
	tx := t.txBytes
	st := t.lastState
	t.mu.Unlock()

	transport := "udp"
	var endpoints []string
	var dnsServers []string
	if t.summary != nil {
		if t.summary.Proto != "" {
			transport = t.summary.Proto
		}
		if t.summary.Remote != "" {
			endpoints = append(endpoints, t.summary.Remote)
		}
		dnsServers = t.summary.DNS
	}

	info := StatusInfo{
		Active:      t.IsRunning(),
		TunnelName:  t.name,
		Addresses:   t.addresses,
		Transport:   "OpenVPN (" + transport + ")",
		Endpoints:   endpoints,
		ConnectedAt: t.startedAt.Unix(),
		RxBytes:     rx,
		TxBytes:     tx,
		Proto:       "openvpn",
	}

	if st != "" && st != "CONNECTED" {
		info.Transport += " [" + st + "]"
	}

	if len(dnsServers) > 0 {
		info.DNSProxies = dnsServers
	}

	return info
}

// Stop cleanly terminates the OpenVPN process.
func (t *OpenVPNTunnel) Stop() error {
	t.stopOnce.Do(func() {
		close(t.stopCh)
		t.mu.Lock()
		t.running = false
		conn := t.mgmtConn
		cmd := t.cmd
		t.mu.Unlock()

		if conn != nil {
			_, _ = fmt.Fprintf(conn, "signal SIGTERM\n")
			_ = conn.Close()
		}

		if cmd != nil && cmd.Process != nil {
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				_ = cmd.Process.Kill()
			}
		}

		if t.tempDir != "" {
			_ = os.RemoveAll(t.tempDir)
		}

		if t.cleanupAdapter != nil {
			t.cleanupAdapter()
			t.cleanupAdapter = nil
		}

		log.Printf("[OpenVPN %s] Tunnel stopped.", t.name)
	})
	return nil
}
