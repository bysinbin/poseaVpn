#!/usr/bin/env bash
set -e

if [ "$EUID" -ne 0 ]; then
  echo "[-] Hata: Servis kaldırma için yönetici (root/sudo) yetkisi gereklidir."
  echo "    Kullanım: sudo ./uninstall_service_mac.sh"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="${ROOT_DIR}/bin/poseavpn-service"

if [ -f "${BIN_PATH}" ]; then
  "${BIN_PATH}" --uninstall
else
  launchctl unload -w /Library/LaunchDaemons/com.poseavpn.service.plist 2>/dev/null || true
  rm -f /Library/LaunchDaemons/com.poseavpn.service.plist
  echo "[✓] Servis kaldırıldı."
fi
