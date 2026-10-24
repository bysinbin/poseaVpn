# 🛡️ PoseaVPN (v1.1)

Active Directory (AD) ve kurumsal ağlar için özel olarak geliştirilmiş, **Windows ve macOS** üzerinde **WireGuard ve OpenVPN** protokollerini aynı anda destekleyen modern, hafif ve çoklu tünel uyumlu kurumsal VPN istemcisi.

---

## 🎯 Kurumsal Ortamlardaki Sorunlar ve Çözümleri

Standart VPN istemcileri kurumsal etki alanı veya kısıtlı kullanıcı ortamlarında şu sorunları yaşatır:
1. **Yönetici Yetkisi Gereksinimi (Non-Admin / Domain Users)**  
   Standart etki alanı kullanıcılarının (Domain Users) yerel admin yetkisi yoktur. Windows'ta Named Pipe, macOS'te sistem ağ ayarları erişimi engellenir ve tünel açılamaz.
2. **Genel Ağ (Public Network) & Güvenlik Duvarı Engeli**  
   Windows sanal adaptörü varsayılan olarak "Public" yapar ve Kerberos, LDAP, SMB trafiğini keser; etki alanı bağlantısı kopar.
3. **AD DNS & NetBIOS / Suffix Kaybı**  
   Split-tunneling yapılmadığında veya DNS suffix'leri iletilmediğinde `\\dosyasunucusu\paylasim` ve intranet kaynakları çözülemez.
4. **Farklı VPN Protokolleri (WireGuard & OpenVPN İhtiyacı)**  
   Kurumsal altyapılarda hem `.conf` (WireGuard) hem de `.ovpn` (OpenVPN auth-user-pass) sunucuları bulunur; kullanıcıların birden çok farklı istemci kurması gerekir.

### 💡 PoseaVPN v1.1 ile Neler Çözüldü?
- **Çift Protokol Desteği (WireGuard & OpenVPN):** Hem modern WireGuard `.conf` hem de OpenVPN `.ovpn` profillerini aynı kitaplıkta yönetebilir ve aynı anda çalıştırabilirsiniz.
- **Platformlar Arası Tam Destek (Windows & macOS):**
  - **Windows:** Wintun sürücüsü, Windows Service mimarisi, Named Pipe DACL, NRPT split-DNS ve Network Profile (Private) otomasyonu.
  - **macOS:** Yerel `utun` çekirdek sürücüsü (harici kext gerektirmez), `launchd` LaunchDaemon mimarisi, `scutil` / `/etc/resolver/` split-DNS, menü çubuğu (menu bar) tepsisi ve Unix Domain Socket (`/var/run/poseavpn.sock`).
- **Kimlik Bilgisi (auth-user-pass) Yönetimi:** OpenVPN kullanıcı adı ve şifreleri tek seferde kaydedilebilir; sonrasında tek tıkla bağlantı kurulur.
- **Standart Kullanıcı Yetkisiyle Yönetim:** Arka plan servisi `SYSTEM` (Windows) veya `root` (macOS LaunchDaemon) olarak çalışır; son kullanıcılar şifresiz tünel açıp kapatabilir.
- **1:1 IP NAT & Adres Eşleme (Conflicting Subnet Mapping):** Çakışan alt ağlara sahip birden fazla VPN tüneli aynı anda açıldığında, paket seviyesinde şeffaf adres eşleme (`198.18.0.0/15`) yaparak her iki ağa da erişim sağlar.
- **Dahili DNS Vekili:** Çakışan DNS sunucularını interface-bound soketler (`IP_UNICAST_IF` / `IP_BOUND_IF`) üzerinden yönlendirir.

---

## 📂 Dizin Yapısı

```
poseavpn/
├── bin/                          # Derlenmiş çalıştırılabilir dosyalar
│   ├── openvpn/                  # Bağımsız OpenVPN motoru ve DLL'leri (Windows)
│   ├── poseavpn                  # Kullanıcı CLI istemcisi (macOS / Linux)
│   ├── poseavpn-service          # Arka Plan Servisi (macOS LaunchDaemon)
│   ├── poseavpn-tray             # Menü çubuğu uygulaması (macOS NSStatusItem)
│   ├── poseavpn-single           # Tekli Wails arayüzü (macOS)
│   ├── poseavpn-multi            # Çoklu Wails arayüzü (macOS)
│   ├── poseavpn.exe              # Kullanıcı CLI istemcisi (Windows)
│   ├── poseavpn-service.exe      # Windows Arka Plan Servisi (LocalSystem)
│   ├── poseavpn-tray.exe         # Windows Sistem Tepsisi uygulaması
│   ├── poseavpn-single.exe       # Windows Tekli arayüzü
│   ├── poseavpn-multi.exe        # Windows Çoklu arayüzü
│   └── wintun.dll               # Resmi imzalı Wintun 64-bit sürücüsü
├── cmd/
│   ├── poseavpn-tray/            # Sistem tepsisi / menü çubuğu kaynak kodları
│   ├── poseavpn-single/          # Tekli arayüz (Wails)
│   ├── poseavpn-multi/           # Çoklu arayüz (Wails)
│   ├── poseavpn/                 # İstemci CLI kaynak kodları
│   └── poseavpn-service/         # Servis / Daemon kaynak kodları
├── ui/common/                   # İki arayüzün ortak CSS / JS dosyaları
├── pkg/
│   ├── guiapp/                  # Arayüzlerin ortak arka ucu (WG + OVPN)
│   ├── config/                  # Standart WireGuard .conf ayrıştırıcı
│   ├── ovpn/                    # OpenVPN .ovpn ayrıştırıcı
│   ├── dns/                     # Windows NRPT & macOS scutil Split-DNS yöneticisi
│   ├── ipc/                     # Windows Named Pipe & macOS Unix Socket protokolü
│   ├── netprofile/              # Ağ profili (Private) ayarlayıcısı
│   ├── transport/               # İletim katmanı (UDP & WebSocket Fallback)
│   └── tunnel/                  # Wintun / utun + WireGuard-Go + OpenVPN Runner
├── build_mac.sh                 # macOS ikili derleme betiği
├── install_service_mac.sh       # macOS LaunchDaemon kurulum betiği (sudo gerektirir)
├── uninstall_service_mac.sh     # macOS LaunchDaemon kaldırma betiği
├── build.ps1                    # Windows PowerShell derleme betiği
├── install_service.bat          # Windows Servis kurulum betiği (Admin gerektirir)
├── uninstall_service.bat        # Windows Servis kaldırma betiği
└── installer/                   # WiX v5 tabanlı MSI kurulum paketi oluşturucu
```

---

## 🍏 macOS Kurulum ve Kullanım

### 1. macOS İkililerini Derleme
Tüm CLI, servis, tepsi ve masaüstü arayüzlerini derlemek için:
```bash
./build_mac.sh
```

### 2. Arka Plan Servisini Başlatma (Yalnızca bir kez - root/sudo ile)
```bash
sudo ./install_service_mac.sh
```
*(Veya doğrudan: `sudo bin/poseavpn-service --install`)*

Servis `/Library/LaunchDaemons/com.poseavpn.service.plist` olarak kaydedilir, otomatik başlatılır ve makine her açıldığında hazır bekler.

### 3. İstemciyi Çalıştırma (Admin / Sudo Şifresi Gerekmez!)

#### A) Masaüstü Uygulamaları ve Menü Çubuğu (Önerilen):
- **Menü Çubuğu İkonu (`bin/poseavpn-tray`):** Ekranın üstündeki menü çubuğuna yerleşir. Sol tık arayüzü açar, sağ tık menüsünden tünelleri tek tıkla bağlanıp kesebilirsiniz. İlk açılışta oturum açılışına (LaunchAgent) kendini ekler.
- **Tekli Arayüz (`bin/poseavpn-single`):** Sade, odaklanmış tek tünel arayüzü.
- **Çoklu Arayüz (`bin/poseavpn-multi`):** Profil kitaplığı, içe aktarma ve aynı anda birden fazla tünel yönetimi.

#### B) Komut Satırı (CLI) ile:
```bash
# Tüneli başlatma
bin/poseavpn connect ~/vpn/ayar.conf
# veya OpenVPN için
bin/poseavpn connect ~/vpn/sirket.ovpn

# Aktif tünellerin durumunu görme
bin/poseavpn status

# Tüneli sonlandırma
bin/poseavpn disconnect [tünel_adı]
```

---

## 🪟 Windows Kurulum ve Kullanım

### 1. Servisi Kurma (Yönetici ile)
```cmd
install_service.bat
```
*(Veya doğrudan: `bin\poseavpn-service.exe --install`)*

### 2. İstemciyi Çalıştırma
- **Sistem Tepsisi (`bin\poseavpn-tray.exe`):** Saatin yanında çalışır, başlangıca eklenir.
- **Tekli (`bin\poseavpn-single.exe`) & Çoklu (`bin\poseavpn-multi.exe`)** masaüstü uygulamaları.
- **CLI (`bin\poseavpn.exe`):**
  ```cmd
  bin\poseavpn.exe connect C:\yol\ayar.conf
  bin\poseavpn.exe status
  bin\poseavpn.exe disconnect
  ```

---

## 🌐 UDP Engelli Ağlar İçin WebSocket Fallback Ağ Geçidi

Kurumsal ağınız UDP paketlerini engelliyorsa:
```bash
# Sunucuda:
poseavpn fallback-server --listen :8443 --target 127.0.0.1:51820
```

İstemci `.conf` dosyasında `[Interface]` altına ekleyin:
```ini
Transport = wss
FallbackURL = wss://vpn.sirket.com:8443/wg
```

---

## 🔀 Çakışan Alt Ağlar İçin 1:1 IP NAT (Adres Eşleme)

İki farklı VPN sunucusu aynı iç IP aralığını kullanıyorsa (örn. her ikisi de `192.168.0.0/22`), PoseaVPN sonra bağlanan tünel için `198.18.0.0/15` havuzundan sanal bir aralık eşler. İki ağa da çakışma olmadan eşzamanlı erişebilirsiniz.
