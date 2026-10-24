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

echo "[4/5] Building poseavpn-single desktop app..."
go build -ldflags "-s -w" -o "${BIN_DIR}/poseavpn-single" ./cmd/poseavpn-single

echo "[5/5] Building poseavpn-multi desktop app..."
go build -ldflags "-s -w" -o "${BIN_DIR}/poseavpn-multi" ./cmd/poseavpn-multi

chmod +x "${BIN_DIR}/poseavpn" "${BIN_DIR}/poseavpn-service" "${BIN_DIR}/poseavpn-tray" "${BIN_DIR}/poseavpn-single" "${BIN_DIR}/poseavpn-multi"

echo "=== Derleme başarıyla tamamlandı! ==="
echo "Çıktılar ${BIN_DIR} dizininde:"
ls -lh "${BIN_DIR}" | grep poseavpn
