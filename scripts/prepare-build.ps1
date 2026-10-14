# Materialize disposable Wails resources from versioned source files.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
& (Join-Path $PSScriptRoot 'update-branding.ps1')

$windowsDir = Join-Path $repoRoot 'build/windows'
$installerDir = Join-Path $windowsDir 'installer'
New-Item -ItemType Directory -Force -Path $installerDir | Out-Null
Copy-Item -LiteralPath (Join-Path $repoRoot 'assets/branding/logo.png') -Destination (Join-Path $repoRoot 'build/appicon.png') -Force
Copy-Item -LiteralPath (Join-Path $repoRoot 'assets/branding/icon.ico') -Destination (Join-Path $windowsDir 'icon.ico') -Force
foreach ($templateName in @('info.json', 'wails.exe.manifest')) {
    Copy-Item -LiteralPath (Join-Path $repoRoot "packaging/windows/$templateName") -Destination (Join-Path $windowsDir $templateName) -Force
}
Copy-Item -LiteralPath (Join-Path $repoRoot 'packaging/windows/installer/project.nsi') -Destination (Join-Path $installerDir 'project.nsi') -Force
