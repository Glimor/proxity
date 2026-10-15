[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $repoRoot
$licenseRoot = Join-Path $repoRoot 'build/bin/licenses'
New-Item -ItemType Directory -Force -Path $licenseRoot | Out-Null
Copy-Item -LiteralPath LICENSE -Destination $licenseRoot -Force
Copy-Item -LiteralPath THIRD_PARTY_NOTICES.md -Destination $licenseRoot -Force
foreach ($component in @('windivert', 'wireguard')) {
    $target = Join-Path $licenseRoot $component
    New-Item -ItemType Directory -Force -Path $target | Out-Null
    Copy-Item -LiteralPath "third_party/$component/LICENSE" -Destination $target -Force
}
$inventory = [System.Collections.Generic.List[object]]::new()
function Copy-LicenseFiles([string]$Directory, [string]$Target) {
    $licenseFiles = @(Get-ChildItem -LiteralPath $Directory -File | Where-Object Name -Match '^(LICENSE|LICENCE|COPYING|COPYRIGHT|NOTICE|PATENTS)(\.|$|-)')
    if ($licenseFiles.Count -gt 0) {
        New-Item -ItemType Directory -Force -Path $Target | Out-Null
        foreach ($item in $licenseFiles) { Copy-Item -LiteralPath $item.FullName -Destination $Target -Force }
    }
    return @($licenseFiles.Name)
}
$modules = @(go list -m -f '{{.Path}}|{{.Version}}|{{.Dir}}' all)
if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate Go dependencies' }
foreach ($module in $modules) {
    $fields = $module.Split('|')
    if (-not $fields[1]) { continue }
    if (-not $fields[2]) { throw "Download the dependency first: $($fields[0])" }
    $target = Join-Path $licenseRoot ('go/' + ($fields[0] -replace '[^A-Za-z0-9._-]', '_'))
    $files = @(Copy-LicenseFiles $fields[2] $target)
    $inventory.Add(@{ ecosystem = 'go'; name = $fields[0]; version = $fields[1]; licenseFiles = $files })
}
$nodeModules = Join-Path $repoRoot 'frontend/node_modules'
if (-not (Test-Path -LiteralPath $nodeModules)) { throw 'Run npm ci in frontend first' }
$packageFiles = @(Get-ChildItem -LiteralPath $nodeModules -Filter package.json -Recurse -File | Where-Object FullName -Match '[\\/]node_modules[\\/](?:@[^\\/]+[\\/])?[^\\/]+[\\/]package\.json$')
foreach ($packageFile in $packageFiles) {
    $package = Get-Content -LiteralPath $packageFile.FullName -Raw | ConvertFrom-Json
    if (-not $package.name -or -not $package.version) { continue }
    $target = Join-Path $licenseRoot ('npm/' + (($package.name + '@' + $package.version) -replace '[^A-Za-z0-9._-]', '_'))
    $files = @(Copy-LicenseFiles $packageFile.DirectoryName $target)
    $inventory.Add(@{ ecosystem = 'npm'; name = $package.name; version = $package.version; license = $package.license; licenseFiles = $files })
}
$inventory | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $licenseRoot 'dependencies.json') -Encoding utf8
Write-Host "Staged license notices and inventory for $($inventory.Count) dependencies."
