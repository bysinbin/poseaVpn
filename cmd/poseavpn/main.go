package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"poseavpn/pkg/config"
	"poseavpn/pkg/ipc"
	"poseavpn/pkg/ovpn"
	"poseavpn/pkg/transport"
	"poseavpn/pkg/tunnel"
)

const version = "2.0.0"

func printBanner() {
	fmt.Printf("=====================================================\n")
	fmt.Printf(" 🛡️  PoseaVPN v%s - AD Uyumlu VPN İstemcisi (WG & OVPN)\n", version)
	fmt.Printf("=====================================================\n\n")
}

func printUsage() {
	printBanner()
	fmt.Printf("Kullanim:\n")
	fmt.Printf("  poseavpn connect <ayar_dosyasi.(conf|ovpn)>  Tüneli baslatir (Domain User yetkisiyle calisir)\n")
	fmt.Printf("  poseavpn disconnect [ad]                    Tüneli sonlandirir (ad verilmezse hepsini)\n")
	fmt.Printf("  poseavpn status                             Tüm aktif tünellerin durumunu görüntüler\n")
	fmt.Printf("  poseavpn run <ayar_dosyasi.conf>            Servissiz dogrudan konsolda calistirir (Admin gerekir)\n")
	fmt.Printf("  poseavpn fallback-server                    Sunucu icin WebSocket proxy ag gecidini baslatir\n\n")
	fmt.Printf("Parametreler (fallback-server icin):\n")
	fmt.Printf("  --listen <adres:port>   (Varsayilan: :8443)\n")
	fmt.Printf("  --target <adres:port>   (Varsayilan: 127.0.0.1:51820)\n")
}

func handleConnect(confPath string) {
	if confPath == "" {
		fmt.Println("Hata: Baglanilacak .conf veya .ovpn dosyasinin yolu belirtilmelidir.")
		fmt.Println("Ornek: poseavpn connect C:\\kurumsal\\work.conf")
		os.Exit(1)
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		fmt.Printf("Hata: Ayar dosyasi okunamadi: %v\n", err)
		os.Exit(1)
	}

	isOvpn := strings.EqualFold(filepath.Ext(confPath), ".ovpn") || ovpn.IsOpenVPN(string(data))
	var username, password string
	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	if name == "" {
		name = "PoseaVPN"
	}

	if isOvpn {
		sum, err := ovpn.Parse(strings.NewReader(string(data)))
		if err != nil {
			fmt.Printf("Hata: OpenVPN ayar dosyasi gecersiz: %v\n", err)
			os.Exit(1)
		}
		if sum.AuthUserPass {
			fmt.Printf("[*] OpenVPN kimlik dogrulamasi gerekiyor:\n")
			fmt.Printf("Kullanici Adi: ")
			_, _ = fmt.Scanln(&username)
			fmt.Printf("Parola: ")
			_, _ = fmt.Scanln(&password)
		}
	} else {
		// Validate config syntax locally before sending
		_, err = config.Parse(strings.NewReader(string(data)))
		if err != nil {
			fmt.Printf("Hata: Ayar dosyasi formati gecersiz: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("[+] Arka plan servisine baglanti talebi iletiliyor...\n")
	client := ipc.NewClient()
	var resp *ipc.Response
	if isOvpn {
		resp, err = client.ConnectWithAuth(name, string(data), "openvpn", username, password)
	} else {
		resp, err = client.Connect(name, string(data))
	}
	if err != nil {
		fmt.Printf("[-] Baglanti hatasi: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("[-] Servis reddetti: %s\n", resp.Message)
		os.Exit(1)
	}

	fmt.Printf("[✓] %s\n", resp.Message)
	if resp.Status != nil {
		fmt.Printf("    * Adaptör:  %s\n", resp.Status.TunnelName)
		fmt.Printf("    * IP(ler):  %s\n", strings.Join(resp.Status.Addresses, ", "))
		fmt.Printf("    * Sunucu:   %s\n", strings.Join(resp.Status.Endpoints, ", "))
		fmt.Printf("    * Iletim:   %s\n", resp.Status.Transport)
		fmt.Printf("    * Ag Sinifi: Private (Active Directory Firewall korumasi aktif)\n")
	}
}

func handleDisconnect(name string) {
	client := ipc.NewClient()
	var resp *ipc.Response
	var err error
	if name == "" {
		resp, err = client.Disconnect()
	} else {
		resp, err = client.DisconnectTunnel(name)
	}
	if err != nil {
		fmt.Printf("[-] Hata: %v\n", err)
		os.Exit(1)
	}

	if resp.Success {
		fmt.Printf("[✓] %s\n", resp.Message)
	} else {
		fmt.Printf("[-] Servis yaniti: %s\n", resp.Message)
	}
}

func handleStatus() {
	client := ipc.NewClient()
	list, err := client.List()
	if err != nil {
		fmt.Printf("[-] Durum alinamadi: %v\n", err)
		os.Exit(1)
	}

	if len(list) == 0 {
		fmt.Println("[i] Aktif bir WireGuard tüneli bulunmuyor.")
		return
	}

	fmt.Printf("[✓] Aktif tünel sayisi: %d\n", len(list))
	for _, status := range list {
		fmt.Printf("\n  ● %s\n", status.TunnelName)
		fmt.Printf("    * IP(ler):  %s\n", strings.Join(status.Addresses, ", "))
		fmt.Printf("    * Sunucu:   %s\n", strings.Join(status.Endpoints, ", "))
		fmt.Printf("    * Rotalar:  %s\n", strings.Join(status.AllowedIPs, ", "))
		fmt.Printf("    * Iletim:   %s\n", status.Transport)
		for _, m := range status.Mappings {
			fmt.Printf("    * Adres esleme: %s (bu aga sanal adreslerle ulasin)\n", m)
		}
		for _, p := range status.DNSProxies {
			fmt.Printf("    * DNS vekili: %s (DNS adresi cakistigi veya esleme nedeniyle)\n", p)
		}
		if status.LastHandshake > 0 {
			fmt.Printf("    * El sikisma: %s once\n", time.Since(time.Unix(status.LastHandshake, 0)).Round(time.Second))
		} else {
			fmt.Printf("    * El sikisma: henuz yok\n")
		}
		fmt.Printf("    * Trafik:   %d B alindi / %d B gonderildi\n", status.RxBytes, status.TxBytes)
	}
}

func handleRunDirect(confPath string) {
	if confPath == "" {
		fmt.Println("Hata: Calistirilacak .conf dosyasi belirtilmelidir.")
		os.Exit(1)
	}

	cfg, err := config.ParseFile(confPath)
	if err != nil {
		fmt.Printf("Hata: Ayar dosyasi hatali: %v\n", err)
		os.Exit(1)
	}

	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	fmt.Printf("[+] Doğrudan modda tünel '%s' başlatılıyor...\n", name)

	t, err := tunnel.Start(name, cfg)
	if err != nil {
		fmt.Printf("[-] Tunel baslatilamadi: %v\n", err)
		os.Exit(1)
	}
	defer t.Stop()

	fmt.Printf("[✓] Tünel aktif! Durdurmak için Ctrl+C tuşlarına basın.\n")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Printf("\n[+] Tünel kapatılıyor...\n")
}

func handleFallbackServer(listenAddr, targetAddr string) {
	fmt.Printf("=====================================================\n")
	fmt.Printf(" 🌐 PoseaVPN WebSocket Fallback Ağ Geçidi\n")
	fmt.Printf(" Dinleme Adresi: %s\n", listenAddr)
	fmt.Printf(" Hedef WireGuard UDP: %s\n", targetAddr)
	fmt.Printf("=====================================================\n")

	server := transport.NewFallbackServer(listenAddr, targetAddr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Ağ geçidi hatası: %v", err)
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := strings.ToLower(os.Args[1])

	switch command {
	case "connect":
		confPath := ""
		if len(os.Args) > 2 {
			confPath = os.Args[2]
		}
		handleConnect(confPath)

	case "disconnect":
		name := ""
		if len(os.Args) > 2 {
			name = os.Args[2]
		}
		handleDisconnect(name)

	case "status":
		handleStatus()

	case "run":
		confPath := ""
		if len(os.Args) > 2 {
			confPath = os.Args[2]
		}
		handleRunDirect(confPath)

	case "fallback-server":
		fs := flag.NewFlagSet("fallback-server", flag.ExitOnError)
		listenAddr := fs.String("listen", ":8443", "HTTP/WebSocket dinleme adresi")
		targetAddr := fs.String("target", "127.0.0.1:51820", "Hedef WireGuard UDP adresi")
		_ = fs.Parse(os.Args[2:])
		handleFallbackServer(*listenAddr, *targetAddr)

	case "-v", "--version", "version":
		fmt.Printf("PoseaVPN version %s\n", version)

	default:
		printUsage()
	}
}
