#!/usr/bin/env bash
set -e

echo "=== PoseaVPN macOS Build Script (v1.1.0) ==="

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${ROOT_DIR}/bin"
mkdir -p "${BIN_DIR}"

echo "[1/5] Building poseavpn CLI..."
go build -ldflags "-s -w" -o "${BIN_DIR}/poseavpn" ./cmd/poseavpn

echo "[2/5] Building poseavpn-service daemon..."
go build -ldflags "-s -w" -o "${BIN_DIR}/poseavpn-service" ./cmd/poseavpn-service

echo "[3/5] Building poseavpn-tray menu bar app..."
go build -ldflags "-s -w" -o "${BIN_DIR}/poseavpn-tray" ./cmd/poseavpn-tray

export CGO_LDFLAGS="-framework UniformTypeIdentifiers"

echo "[4/5] Building poseavpn-single desktop app..."
go build -tags production -ldflags "-s -w" -o "${BIN_DIR}/poseavpn-single" ./cmd/poseavpn-single

echo "[5/5] Building poseavpn-multi desktop app..."
go build -tags production -ldflags "-s -w" -o "${BIN_DIR}/poseavpn-multi" ./cmd/poseavpn-multi

# OpenVPN bundling (standalone binary)
if [ ! -f "${BIN_DIR}/openvpn/openvpn" ] && [ ! -f "${BIN_DIR}/openvpn" ]; then
    for candidate in \
        "/Applications/Tunnelblick.app/Contents/Resources/openvpn/openvpn-2.5.4-openssl-1.1.1l/openvpn" \
        "/Applications/Tunnelblick.app/Contents/Resources/openvpn/openvpn-2.4.11-openssl-1.1.1l/openvpn" \
        "/opt/homebrew/sbin/openvpn" \
        "/opt/homebrew/bin/openvpn" \
        "${HOME}/homebrew/sbin/openvpn" \
        "${HOME}/homebrew/bin/openvpn" \
        "/usr/local/sbin/openvpn" \
        "/usr/local/bin/openvpn"
    do
        if [ -f "$candidate" ]; then
            echo "[*] Bundling OpenVPN binary from $candidate..."
            mkdir -p "${BIN_DIR}/openvpn"
            cp "$candidate" "${BIN_DIR}/openvpn/openvpn"
            chmod +x "${BIN_DIR}/openvpn/openvpn"
            break
        fi
    done
fi

chmod +x "${BIN_DIR}/poseavpn" "${BIN_DIR}/poseavpn-service" "${BIN_DIR}/poseavpn-tray" "${BIN_DIR}/poseavpn-single" "${BIN_DIR}/poseavpn-multi"
[ -f "${BIN_DIR}/openvpn/openvpn" ] && chmod +x "${BIN_DIR}/openvpn/openvpn"

echo "=== Derleme başarıyla tamamlandı! ==="
echo "Çıktılar ${BIN_DIR} dizininde:"
ls -lh "${BIN_DIR}" | grep poseavpn
