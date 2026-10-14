@echo off
set OUT=C:\Users\kaank\OneDrive\Belgeler\Development\Proxity\fix-result.txt
echo == fix-service start == > "%OUT%"
sc.exe stop ProxityEngine >> "%OUT%" 2>&1
timeout /t 3 /nobreak >nul
taskkill /f /im proxity-service.exe >> "%OUT%" 2>&1
timeout /t 3 /nobreak >nul
copy /y "C:\Users\kaank\OneDrive\Belgeler\Development\Proxity\build\bin\proxity-service.exe" "C:\Program Files\Proxity\Proxity\proxity-service.exe" >> "%OUT%" 2>&1
echo --- netsh firewall probe (10s timeout) --- >> "%OUT%"
powershell -NoProfile -Command "$j = Start-Job { netsh advfirewall show allprofiles state }; if (Wait-Job $j -Timeout 10) { Receive-Job $j } else { Stop-Job $j; 'NETSH_HUNG_BFE_WEDGED' }" >> "%OUT%" 2>&1
echo --- service start --- >> "%OUT%"
sc.exe start ProxityEngine >> "%OUT%" 2>&1
timeout /t 5 /nobreak >nul
sc.exe query ProxityEngine >> "%OUT%" 2>&1
echo == done == >> "%OUT%"
