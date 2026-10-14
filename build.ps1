<#
.SYNOPSIS
    Proxity release build: engine service + Wails GUI + NSIS installer.

.DESCRIPTION
    One command produces every release artifact in build\bin:

        proxity.exe                  Wails GUI (version-stamped)
        proxity-service.exe          engine service (version-stamped)
        WinDivert.dll/WinDivert64.sys driver files staged for the installer
        Proxity-amd64-installer.exe  NSIS installer (requires makensis)

    Requires: Go >= the go.mod version, Node/npm, the Wails CLI
    (go install github.com/wailsapp/wails/v2/cmd/wails@latest), and NSIS
    (https://nsis.sourceforge.io - `winget install NSIS.NSIS`).

    Binaries are unsigned; Windows SmartScreen will warn on first run.
    Sign proxity.exe, proxity-service.exe and the installer with a real
    certificate when one becomes available (see !finalize hooks in
    build\windows\installer\project.nsi).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File build.ps1 -Version 0.1.0
#>
[CmdletBinding()]
param(
    # Semver stamped into both binaries via -ldflags -X (no "v" prefix).
    [string]$Version = "0.1.0",
    # Skip the NSIS installer step (when makensis is not installed).
    [switch]$NoInstaller
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$ldflags = "-X github.com/Glimor/proxity/internal/version.Version=$Version"

Write-Host "==> Building proxity-service.exe ($Version)" -ForegroundColor Cyan
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags $ldflags -o build\bin\proxity-service.exe .\cmd\proxity-service
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Write-Host "==> Staging WinDivert driver files into build\bin" -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path build\bin | Out-Null
Copy-Item third_party\windivert\WinDivert.dll   build\bin\ -Force
Copy-Item third_party\windivert\WinDivert64.sys build\bin\ -Force

# NOTE: no -clean - it would wipe build\bin (and the service + driver
# files staged above) before makensis packs them into the installer.
$nsis = if ($NoInstaller) { "" } else { "-nsis" }
Write-Host "==> wails build $nsis" -ForegroundColor Cyan
wails build -trimpath -ldflags $ldflags @($nsis | Where-Object { $_ })
if ($LASTEXITCODE -ne 0) { throw "wails build failed" }

Write-Host ""
Write-Host "==> Artifacts in build\bin:" -ForegroundColor Green
Get-ChildItem build\bin | ForEach-Object { Write-Host ("    " + $_.Name) }
Write-Host ""
Write-Host "NOTE: binaries are unsigned - SmartScreen will warn on first launch." -ForegroundColor Yellow
