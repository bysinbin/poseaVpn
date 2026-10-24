#!/usr/bin/env bash
set -e

if [ "$EUID" -ne 0 ]; then
  echo "[-] Hata: Servis kurulumu için yönetici (root/sudo) yetkisi gereklidir."
  echo "    Kullanım: sudo ./install_service_mac.sh"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="${ROOT_DIR}/bin/poseavpn-service"

if [ ! -f "${BIN_PATH}" ]; then
  echo "[*] poseavpn-service bulunamadı, derleniyor..."
  "${ROOT_DIR}/build_mac.sh"
fi

echo "[+] PoseaVPN LaunchDaemon servisi kuruluyor..."
"${BIN_PATH}" --install
echo "[✓] Kurulum tamamlandı. Arka plan servisi aktif."
