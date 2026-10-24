$ErrorActionPreference = 'Stop'
Write-Host "[*] PoseaVPN derleniyor..."

New-Item -ItemType Directory -Force -Path bin | Out-Null

# Copies an exe into place even when the old one is running: Windows refuses to
# overwrite a running exe but allows renaming it, so the old copy is moved aside.
function Copy-Replace([string]$from, [string]$to) {
    try {
        Copy-Item $from $to -Force -ErrorAction Stop
    } catch {
        $old = "$to.old"
        Remove-Item $old -Force -ErrorAction SilentlyContinue
        Move-Item $to $old -Force -ErrorAction Stop
        Copy-Item $from $to -Force -ErrorAction Stop
        Write-Host "     ($to was running; the old copy was kept as $old, restart the app to use the new one)"
    }
}

# 1) Servis ve CLI
Get-Process -Name poseavpn-multi, poseavpn-single, poseavpn-tray -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
$serviceRunning = $false
$svc = Get-Service -Name PoseaVPN -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq 'Running') {
    $serviceRunning = $true
    Write-Host "[*] PoseaVPN servisi durduruluyor..."
    try {
        $p = New-Object System.IO.Pipes.NamedPipeClientStream(".", "poseavpn", [System.IO.Pipes.PipeDirection]::InOut)
        $p.Connect(1000)
        $w = New-Object System.IO.StreamWriter($p)
        $w.WriteLine('{"action":"stop_service"}')
        $w.Flush()
        $p.Close()
        Start-Sleep -Milliseconds 600
    } catch {}
    Stop-Service -Name PoseaVPN -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 500
}

go build -o bin/poseavpn-service.exe ./cmd/poseavpn-service
if ($LASTEXITCODE -ne 0) { Write-Error "poseavpn-service derleme hatasi"; exit 1 }

go build -o bin/poseavpn.exe ./cmd/poseavpn
if ($LASTEXITCODE -ne 0) { Write-Error "poseavpn derleme hatasi"; exit 1 }

# Sistem tepsisi uygulamasi (konsol penceresi olmadan)
Get-Process -Name poseavpn-tray -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
go build -ldflags="-H windowsgui" -o bin/poseavpn-tray.exe ./cmd/poseavpn-tray
if ($LASTEXITCODE -ne 0) { Write-Error "poseavpn-tray derleme hatasi"; exit 1 }

# 2) Masaustu uygulamalari (Wails): tekli ve coklu
foreach ($app in 'single', 'multi') {
    $dir = "cmd/poseavpn-$app"
    Push-Location $dir
    wails build -s -platform windows/amd64 -o "poseavpn-$app.exe" | Out-Null
    $code = $LASTEXITCODE
    Pop-Location
    if ($code -ne 0) { Write-Error "poseavpn-$app (wails) derleme hatasi"; exit 1 }
    Copy-Replace "$dir/build/bin/poseavpn-$app.exe" "bin/poseavpn-$app.exe"
}

if (-not (Test-Path "bin/wintun.dll")) {
    Copy-Item "wintun.dll" "bin/wintun.dll" -Force
}

if ($serviceRunning) {
    Write-Host "[*] PoseaVPN servisi yeniden baslatiliyor..."
    Start-Service -Name PoseaVPN -ErrorAction SilentlyContinue
}

Write-Host "[OK] Derleme tamam:"
Get-ChildItem bin -Filter *.exe | ForEach-Object { "     {0}  ({1:N1} MB)" -f $_.Name, ($_.Length / 1MB) }
