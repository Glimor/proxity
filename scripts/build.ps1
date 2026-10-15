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
    packaging\windows\installer\project.nsi).

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts/build.ps1 -Version 0.1.0
#>
[CmdletBinding()]
param(
    # Semver stamped into both binaries via -ldflags -X (no "v" prefix).
    [string]$Version = "0.1.0",
    # Skip the NSIS installer step (when makensis is not installed).
    [switch]$NoInstaller
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath (Split-Path -Parent $PSScriptRoot)

if (-not $NoInstaller) {
    $nsisCommand = Get-Command makensis -ErrorAction SilentlyContinue
    if (-not $nsisCommand) {
        foreach ($programRoot in @(${env:ProgramFiles(x86)}, $env:ProgramFiles)) {
            if (-not $programRoot) { continue }
            $nsisDir = Join-Path $programRoot 'NSIS'
            if (Test-Path -LiteralPath (Join-Path $nsisDir 'makensis.exe')) {
                $env:PATH = "$nsisDir;$env:PATH"
                break
            }
        }
    }
    if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) {
        throw 'NSIS (makensis.exe) is required for the installer. Install with: winget install NSIS.NSIS. Use -NoInstaller only to build binaries.'
    }
}

$ldflags = "-X github.com/Glimor/proxity/internal/version.Version=$Version"

& (Join-Path $PSScriptRoot 'prepare-build.ps1')

Write-Host "==> Building proxity-service.exe ($Version)" -ForegroundColor Cyan
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags $ldflags -o build\bin\proxity-service.exe .\cmd\proxity-service
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Write-Host "==> Staging WinDivert driver files into build\bin" -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path build\bin | Out-Null
foreach ($driverName in @('WinDivert.dll', 'WinDivert64.sys')) {
    $driverSource = Join-Path 'third_party\windivert' $driverName
    $driverTarget = Join-Path 'build\bin' $driverName
    # An active service may lock the staged DLL. Identical files need no copy.
    if ((Test-Path -LiteralPath $driverTarget) -and
        ((Get-FileHash -LiteralPath $driverSource).Hash -eq
         (Get-FileHash -LiteralPath $driverTarget).Hash)) { continue }
    Copy-Item -LiteralPath $driverSource -Destination $driverTarget -Force
}

Write-Host "==> Installing locked frontend dependencies and collecting notices" -ForegroundColor Cyan
npm ci --prefix frontend
if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
go mod download
if ($LASTEXITCODE -ne 0) { throw 'go mod download failed' }
& (Join-Path $PSScriptRoot 'collect-licenses.ps1')

# NOTE: no -clean - it would wipe build\bin (and the service + driver
# files staged above) before makensis packs them into the installer.
$nsis = if ($NoInstaller) { "" } else { "-nsis" }
Write-Host "==> wails build $nsis" -ForegroundColor Cyan
wails build -trimpath -ldflags $ldflags @($nsis | Where-Object { $_ })
if ($LASTEXITCODE -ne 0) { throw "wails build failed" }
if (-not $NoInstaller -and -not (Test-Path -LiteralPath 'build/bin/Proxity-amd64-installer.exe')) {
    throw 'The installer was not produced. Check the NSIS output above.'
}

Write-Host ""
Write-Host "==> Artifacts in build\bin:" -ForegroundColor Green
Get-ChildItem build\bin | ForEach-Object { Write-Host ("    " + $_.Name) }
Write-Host ""
Write-Host "NOTE: binaries are unsigned - SmartScreen will warn on first launch." -ForegroundColor Yellow
