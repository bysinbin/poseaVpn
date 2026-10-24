$ErrorActionPreference = 'Stop'
Write-Host "[*] PoseaVPN MSI Kurulum Paketi olusturuluyor..."

$root = Split-Path -Parent $PSScriptRoot
Set-Location $PSScriptRoot

# Check WiX
if (-not (Get-Command "wix" -ErrorAction SilentlyContinue)) {
    Write-Host "[!] WiX bulunamadi. Yukleniyor..."
    dotnet tool install -g wix --version 5.0.2
}

# WiX Build
wix build Package.wxs -arch x64 -o PoseaVPN-Setup-x64.msi

if ($LASTEXITCODE -eq 0) {
    Write-Host "[OK] MSI basariyla olusturuldu: $PSScriptRoot\PoseaVPN-Setup-x64.msi"
    Copy-Item "PoseaVPN-Setup-x64.msi" "$root\bin\PoseaVPN-Setup-x64.msi" -Force -ErrorAction SilentlyContinue
    Copy-Item "PoseaVPN-Setup-x64.msi" "c:\Users\Ulmo\OneDrive\Masaüstü\PoseaVPN-Setup-x64.msi" -Force -ErrorAction SilentlyContinue
} else {
    Write-Error "MSI olusturulamadi!"
}
