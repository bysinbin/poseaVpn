$ErrorActionPreference = 'Stop'
Write-Host "[*] PoseaVPN Setup.exe (Inno Setup) olusturuluyor..."

$root = Split-Path -Parent $PSScriptRoot
Set-Location $PSScriptRoot

$iscc = "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
if (-not (Test-Path $iscc)) {
    $iscc = "C:\Program Files (x86)\Inno Setup 6\ISCC.exe"
}
if (-not (Test-Path $iscc)) {
    Write-Error "Inno Setup (ISCC.exe) bulunamadi!"
    exit 1
}

& $iscc setup.iss

if ($LASTEXITCODE -eq 0) {
    Write-Host "[OK] Setup.exe basariyla olusturuldu: $PSScriptRoot\PoseaVPN-Setup-x64.exe"
    Copy-Item "PoseaVPN-Setup-x64.exe" "$root\bin\PoseaVPN-Setup-x64.exe" -Force -ErrorAction SilentlyContinue
    
    $desktop = [Environment]::GetFolderPath('Desktop')
    if (Test-Path $desktop) {
        Copy-Item "PoseaVPN-Setup-x64.exe" (Join-Path $desktop "PoseaVPN-Setup-x64.exe") -Force -ErrorAction SilentlyContinue
        Write-Host "[OK] Masaustune kopyalandi: $desktop\PoseaVPN-Setup-x64.exe"
    }
} else {
    Write-Error "Setup.exe olusturulamadi!"
}
