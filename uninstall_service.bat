@echo off
:: PoseaVPN Windows Servis Kaldirma Betigi
cd /d "%~dp0"

echo PoseaVPN servisi durduruluyor ve kaldiriliyor...
bin\poseavpn-service.exe --uninstall
sc.exe stop PoseaWireGuard >nul 2>&1
sc.exe delete PoseaWireGuard >nul 2>&1

pause
