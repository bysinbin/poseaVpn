# 🛡️ PoseaVPN

Active Directory (AD) etki alanına dahil edilmiş (domain-joined) Windows bilgisayarlar için özel olarak geliştirilmiş, **WireGuard ve OpenVPN** protokollerini aynı anda destekleyen kurumsal VPN istemcisi.

---

## 🎯 Active Directory Ortamındaki Sorunlar ve Çözümleri

Standart VPN istemcileri AD ortamında şu hataları verir:
1. **"WireGuard / OpenVPN may only be used by users who are a member of the Builtin Administrators group"**  
   Standart etki alanı kullanıcılarının (Domain Users) yerel admin yetkisi yoktur. Named Pipe erişimi engellenir ve tünel açılamaz.
2. **Genel Ağ (Public Network) & Windows Güvenlik Duvarı Engeli**  
   Windows sanal adaptörü (Wintun) varsayılan olarak "Public" yapar. Windows Güvenlik Duvarı Kerberos, LDAP, SMB ve RPC trafiğini anında keser; etki alanı bağlantısı kopar.
3. **AD DNS & NetBIOS / Suffix Kaybı**  
   Split-tunneling yapılmadığında veya DNS suffix'leri iletilmediğinde `\\dosyasunucusu\paylasim` ve intranet kaynakları çözülemez.
4. **Farklı VPN Protokolleri (WireGuard & OpenVPN İhtiyacı)**  
   Kurumsal altyapılarda hem `.conf` (WireGuard) hem de `.ovpn` (OpenVPN auth-user-pass) sunucuları bulunur; kullanıcıların birden çok farklı istemci kurması gerekir.

### 💡 PoseaVPN Nasıl Çözer?
- **Çift Protokol Desteği (WireGuard & OpenVPN):** Hem modern WireGuard `.conf` hem de OpenVPN `.ovpn` profillerini aynı kitaplıkta yönetebilir ve aynı anda çalıştırabilirsiniz.
- **Dahili Bağımsız OpenVPN 2.6 Motoru:** Sisteme harici hiçbir program kurmaya gerek kalmadan, projeye gömülü (bundled) portable OpenVPN Community binary'si ve Wintun sürücüsü kullanılır.
- **Kimlik Bilgisi (auth-user-pass) Yönetimi:** OpenVPN kullanıcı adı ve şifreleri tek seferde kaydedilebilir; sonrasında tek tıkla bağlantı kurulur.
- **Arka Plan Servis Mimarisi (LocalSystem):** `poseavpn-service.exe` Windows Servisi olarak `SYSTEM` yetkisinde çalışır.
- **Domain Users İzinli Named Pipe (`\\.\pipe\poseavpn`):** Özel DACL (`D:(A;;GRGW;;;AU)`) sayesinde yerel admin olmayan standart domain kullanıcıları UAC şifresi girmeden tüneli açıp kapatabilir.
- **Otomatik Özel Ağ (Private Network):** Tünel açıldığında adaptör kategorisini otomatik olarak `Private` yapar; AD trafiğinin engellenmesini önler.
- **Windows NRPT Split-DNS Entegrasyonu:** Şirket içi etki alanı suffix'lerini (`*.sirket.local`) doğrudan Domain Controller DNS'ine yönlendirir.
- **1:1 IP NAT & Adres Eşleme (Conflicting Subnet Mapping):** Çakışan alt ağlara sahip birden fazla VPN tüneli aynı anda açıldığında, paket seviyesinde şeffaf adres eşleme (`198.18.0.0/15`) yaparak her iki ağa da erişim sağlar.

---

## 📂 Dizin Yapısı

```
poseavpn/
├── bin/
│   ├── openvpn/                  # Bağımsız OpenVPN Community motoru ve DLL'leri
│   │   ├── openvpn.exe
│   │   ├── libcrypto-3-x64.dll
│   │   ├── libssl-3-x64.dll
│   │   └── wintun.dll
│   ├── poseavpn-tray.exe         # Sistem tepsisi uygulaması (otomatik başlar)
│   ├── poseavpn-single.exe       # Tekli arayüz (tek yapılandırma)
│   ├── poseavpn-multi.exe        # Çoklu arayüz (çoklu profil / eşzamanlı tüneller)
│   ├── poseavpn.exe              # Kullanıcı CLI istemcisi (Admin yetkisi istemez)
│   ├── poseavpn-service.exe      # Windows Arka Plan Servisi (LocalSystem)
│   └── wintun.dll               # Resmi imzalı Wintun 64-bit sürücüsü
├── cmd/
│   ├── poseavpn-tray/            # Sistem tepsisi kaynak kodları
│   ├── poseavpn-single/          # Tekli arayüz (Wails)
│   ├── poseavpn-multi/           # Çoklu arayüz (Wails)
│   ├── poseavpn/                 # İstemci CLI kaynak kodları
│   └── poseavpn-service/         # Servis kaynak kodları
├── ui/common/                   # İki arayüzün ortak CSS / JS dosyaları
├── pkg/
│   ├── guiapp/                  # Arayüzlerin ortak arka ucu (WG + OVPN desteği)
│   ├── config/                  # Standart WireGuard .conf ayrıştırıcı
│   ├── ovpn/                    # OpenVPN .ovpn ayrıştırıcı
│   ├── dns/                     # Windows NRPT & Split-DNS yöneticisi
│   ├── ipc/                     # AD uyumlu Named Pipe protokolü & DACL
│   ├── netprofile/              # Ağ profili (Private) ayarlayıcısı
│   ├── transport/               # İletim katmanı (UDP & WebSocket Fallback)
│   └── tunnel/                  # Wintun + WireGuard-Go + OpenVPN Runner
├── build.ps1                    # PowerShell derleme betiği
├── install_service.bat          # Servis kurulum betiği (Admin gerektirir)
└── uninstall_service.bat        # Servis kaldırma betiği
```

---

## 🚀 Kurulum ve Kullanım

### 1. Servisi Kurma (Yalnızca bir kez - Yönetici veya GPO ile)
Yönetici olarak PowerShell veya CMD açın:
```cmd
install_service.bat
```
*(Veya doğrudan: `bin\poseavpn-service.exe --install`)*

Servis kurulduktan sonra arka planda otomatik başlar ve her Windows açılışında hazır bekler.

---

### 2. İstemciyi Çalıştırma (Admin Şifresi Gerekmez!)

#### A) Masaüstü uygulamaları (Önerilen):
Saatin yanındaki **sistem tepsisi** simgesi (`poseavpn-tray.exe`) ilk çalıştırmada Windows başlangıcına kendini ekler; sonraki oturumlarda kendiliğinden açılır. Arayüzlerden biri açıldığında tepsi çalışmıyorsa otomatik başlatılır.
- **Sol tık:** arayüzü açar. **Sağ tık menüsü:** her profil için tek tıkla bağlan/kes, "Çoklu arayüzü aç", "Tekli arayüzü aç", "Tümünü kes", "Windows ile başlat".
- **Tekli (`poseavpn-single.exe`):** tek `.conf` seç, büyük düğmeyle bağlan. Son seçilen dosya hatırlanır.
- **Çoklu (`poseavpn-multi.exe`):** profil kitaplığı, içe aktarma/yapıştırma ve aynı anda birden fazla tünel.
- Servis kurulu değilse arayüzde ve tepsi menüsünde "Servisi kur ve başlat" seçeneği çıkar (tek seferlik yönetici onayı).

#### B) Komut Satırı (CLI) ile:
Kullanıcı masaüstünden kendi standart etki alanı hesabıyla (Domain User) CMD veya PowerShell açar:

```cmd
# Tüneli başlatma
bin\poseavpn.exe connect C:\yol\ayar.conf

# Durumu kontrol etme
bin\poseavpn.exe status

# Tüneli kapatma (ad verilmezse hepsi)
bin\poseavpn.exe disconnect [ad]
```

---

### 3. UDP Engelli Ağlar İçin WebSocket Fallback Ağ Geçidi (Opsiyonel)
Kurumsal ağınız saf UDP paketlerini engelliyorsa, WireGuard sunucunuzun yanında WebSocket Fallback ağ geçidini başlatabilirsiniz:

```cmd
# Sunucu tarafında (WebSocket 8443 -> WireGuard UDP 51820)
poseavpn.exe fallback-server --listen :8443 --target 127.0.0.1:51820
```

İstemci `ayar.conf` dosyasına şu satırları eklemeniz yeterlidir:
```ini
[Interface]
Transport = wss
FallbackURL = wss://vpn.sirket.com:8443/wg
```
İstemci WireGuard trafiğini standart HTTPS (Port 443/8443) WebSocket paketleri halinde kurumsal vekil sunucuların (Proxy) içinden sorunsuz geçirir.


---

### 4. Aynı Adresleri Kullanan Ağlara Aynı Anda Bağlanma (Adres Eşleme)

İki farklı siteye aynı anda bağlanırsanız ve ikisinin de iç ağı aynı aralığı kullanıyorsa (örn. ikisinde de `192.168.0.0/22`), Windows bir hedef adresi tek bir yere gönderebilir. Servis bunu otomatik çözer; **önce bağlanan tünel** gerçek adresleri alır, **sonra bağlanan tünel** çakışan ağlar için sanal bir aralık kazanır (`198.18.0.0/15` havuzundan, aynı büyüklükte):

```
Site-A (önce)  : 192.168.0.0/22  -> gerçek adreslerle erişilir      (192.168.1.1)
Site-B (sonra) : 192.168.0.0/22  -> 198.18.0.0/22 sanal aralığı     (198.18.1.1)
```

- Servis, Wintun ile WireGuard arasındaki IPv4 paketlerinde adresi çevirir ve IP/TCP/UDP/ICMP sağlama toplamlarını düzeltir.
- Çakışma yoksa hiçbir şey değişmez. Bağlantı sırası tersine çevrilirse sanal aralığı diğer tünel alır.
- Arayüzde (çoklu) ve `poseavpn status` çıktısında eşleme görünür.
- **DNS vekili:** Çakışan veya eşlenen bir ağdaki DNS sunucusu, tüneli kendi arayüzüne zorlayan küçük bir yerel vekil (`127.77.x.1`) üzerinden kullanılır. Eşlenen ağdaki isimlerin cevapları sanal adrese çevrilir; yani isimle de erişebilirsiniz.
- Tünelin kendi VPN alt ağı çakışıyorsa (iki sunucuda da `10.0.0.0/24`) adres `/32` olarak eklenir.

**Sınırlar:** Yalnızca IPv4 desteklenir. Paket içinde IP adresi taşıyan protokoller (ör. bazı VoIP/FTP etkin kipleri) eşlenen ağlarda sorun çıkarabilir. `0.0.0.0/0` ve `/16`'dan geniş aralıklar eşlenmez. Aynı `Address` değerini kullanan iki yapılandırma yine birlikte açılmaz.

---

### 5. Active Directory (GPO) ve MSI ile Dağıtım

Kurumsal ortamlarda veya Active Directory etki alanlarında istemci makinelerine tek tıkla veya uzaktan sessiz dağıtım için MSI paketi hazırlanmıştır:

```cmd
# MSI Kurulumu (Sessiz / GPO Dağıtımı):
msiexec /i PoseaVPN-Setup-x64.msi /qn /norestart

# MSI Kaldırma:
msiexec /x PoseaVPN-Setup-x64.msi /qn /norestart
```

* **Yönetici Ayrıcalıkları:** Kurulum makine genelinde (`perMachine`) `C:\Program Files\PoseaVPN\` altına yapılır ve `PoseaVPN` Windows servisini otomatik kurup başlatır.
* **Son Kullanıcı Hakları:** Son kullanıcılar local admin olmadan `PoseaVPN` arayüzünü Başlat menüsünden açıp tünelleri yönetebilir.
* **Paket Oluşturma:** `installer\build_msi.ps1` scripti çalıştırılarak güncel ikililer tek bir `.msi` dosyasına paketlenebilir.

