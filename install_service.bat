@echo off
:: PoseaVPN Windows Servis Kurulum Betigi
:: Bu betik Yonetici (Administrator) olarak veya GPO Startup Script ile calistirilmalidir.
cd /d "%~dp0"

echo [1/3] Eski servisler kontrol ediliyor...
sc.exe stop PoseaWireGuard >nul 2>&1
sc.exe delete PoseaWireGuard >nul 2>&1

echo [2/3] PoseaVPN Windows Servisi kuruluyor...
bin\poseavpn-service.exe --install

echo [3/3] Kurulum tamamlandi. Standart Domain Users artik 'bin\poseavpn-multi.exe' veya 'bin\poseavpn.exe' ile baglanabilir.
pause
