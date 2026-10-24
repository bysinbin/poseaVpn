#!/usr/bin/env bash
set -e

echo "=================================================="
echo " 🍏 PoseaVPN macOS App & DMG Builder (v1.1.0)"
echo "=================================================="

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${ROOT_DIR}/bin"
BUILD_DIR="${ROOT_DIR}/build"
DIST_DIR="${ROOT_DIR}/dist"
APP_NAME="PoseaVPN.app"
APP_DIR="${DIST_DIR}/${APP_NAME}"
DMG_STAGING="${DIST_DIR}/dmg_staging"
DMG_OUTPUT="${DIST_DIR}/PoseaVPN-1.1.0.dmg"

# 1. Compile all macOS binaries
echo "[1/4] Derleme yapılıyor (Go binaries)..."
"${ROOT_DIR}/build_mac.sh"

# 2. Generate Apple Retina AppIcon.icns
echo "[2/4] Retina simge dosyası üretiliyor (AppIcon.icns)..."
mkdir -p "${BUILD_DIR}"
go run "${ROOT_DIR}/installer/build_icon.go" "${BUILD_DIR}/AppIcon.icns"

# 3. Create PoseaVPN.app bundle
echo "[3/4] macOS uygulama paketi (${APP_NAME}) oluşturuluyor..."
rm -rf "${APP_DIR}"
mkdir -p "${APP_DIR}/Contents/MacOS"
mkdir -p "${APP_DIR}/Contents/Helpers"
mkdir -p "${APP_DIR}/Contents/Resources"

# Main GUI application (PoseaVPN)
cp "${BIN_DIR}/poseavpn-multi" "${APP_DIR}/Contents/MacOS/PoseaVPN"

# Helper daemons and companion tools
cp "${BIN_DIR}/poseavpn-service" "${APP_DIR}/Contents/Helpers/"
cp "${BIN_DIR}/poseavpn-tray" "${APP_DIR}/Contents/Helpers/"
cp "${BIN_DIR}/poseavpn-single" "${APP_DIR}/Contents/Helpers/"
cp "${BIN_DIR}/poseavpn" "${APP_DIR}/Contents/Helpers/"
if [ -f "${BIN_DIR}/openvpn/openvpn" ]; then
    cp "${BIN_DIR}/openvpn/openvpn" "${APP_DIR}/Contents/Helpers/openvpn"
    echo "[*] Included OpenVPN binary in app bundle."
elif [ -f "${BIN_DIR}/openvpn" ] && [ ! -d "${BIN_DIR}/openvpn" ]; then
    cp "${BIN_DIR}/openvpn" "${APP_DIR}/Contents/Helpers/openvpn"
    echo "[*] Included OpenVPN binary in app bundle."
fi

# Icons
cp "${BUILD_DIR}/AppIcon.icns" "${APP_DIR}/Contents/Resources/AppIcon.icns"

# Info.plist
cat <<'EOF' > "${APP_DIR}/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleName</key>
    <string>PoseaVPN</string>
    <key>CFBundleDisplayName</key>
    <string>PoseaVPN</string>
    <key>CFBundleIdentifier</key>
    <string>com.posea.vpn</string>
    <key>CFBundleVersion</key>
    <string>1.1.0</string>
    <key>CFBundleShortVersionString</key>
    <string>1.1.0</string>
    <key>CFBundleExecutable</key>
    <string>PoseaVPN</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key>
    <true/>
</dict>
</plist>
EOF

# PkgInfo
echo -n "APPL????" > "${APP_DIR}/Contents/PkgInfo"

chmod +x "${APP_DIR}/Contents/MacOS/PoseaVPN"
chmod +x "${APP_DIR}/Contents/Helpers/"*

# Remove quarantine flags and sign ad-hoc for Gatekeeper
xattr -cr "${APP_DIR}" 2>/dev/null || true
codesign --force --deep --sign - "${APP_DIR}" 2>/dev/null || true

# 4. Create DMG (Disk Image with Drag-to-Applications)
echo "[4/4] DMG disk görüntüsü oluşturuluyor (${DMG_OUTPUT})..."
rm -rf "${DMG_STAGING}" "${DMG_OUTPUT}"
mkdir -p "${DMG_STAGING}"

cp -R "${APP_DIR}" "${DMG_STAGING}/"
ln -s /Applications "${DMG_STAGING}/Applications"

hdiutil create -volname "PoseaVPN" \
               -srcfolder "${DMG_STAGING}" \
               -ov \
               -format UDZO \
               "${DMG_OUTPUT}"

rm -rf "${DMG_STAGING}"

echo ""
echo "=================================================="
echo " 🎉 PoseaVPN macOS Paketi Hazır!"
echo " Uygulama: ${APP_DIR}"
echo " DMG İmajı: ${DMG_OUTPUT}"
echo "=================================================="
ls -lh "${DMG_OUTPUT}"
