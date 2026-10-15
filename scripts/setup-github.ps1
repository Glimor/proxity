# Apply the versioned policy to a public repository using GitHub CLI authentication.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string]$Repository,
    [switch]$Apply
)
$ErrorActionPreference = 'Stop'
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw 'Install GitHub CLI and run gh auth login first.' }
$policyDir = Join-Path (Split-Path -Parent $PSScriptRoot) '.github/repository'
function Invoke-GitHubJson([string]$Method, [string]$Endpoint, $Body) {
    $argsList = @('api', '--method', $Method, $Endpoint, '-H', 'Accept: application/vnd.github+json')
    $inputFile = $null
    try {
        if ($null -ne $Body) {
            $inputFile = [System.IO.Path]::GetTempFileName()
            [System.IO.File]::WriteAllText($inputFile, ($Body | ConvertTo-Json -Depth 30), [System.Text.UTF8Encoding]::new($false))
            $argsList += @('--input', $inputFile)
        }
        $result = & gh @argsList
        if ($LASTEXITCODE -ne 0) { throw "GitHub request failed: $Method $Endpoint" }
        if ($result) { return (($result -join "`n") | ConvertFrom-Json) }
    } finally {
        if ($inputFile) { Remove-Item -LiteralPath $inputFile -Force }
    }
}
$repo = Invoke-GitHubJson GET "repos/$Repository" $null
if ($repo.private) { throw 'This policy requires a public repository. Visibility is never changed by this script.' }
if (-not $repo.permissions.admin) { throw 'Repository administration permission is required.' }
if (-not $Apply) {
    Write-Host "Plan for ${Repository}: repository settings, labels, Actions restrictions, security reporting/scanning, and branch/tag rules."
    Write-Host 'Review .github/repository/*.json and docs/template-guide.md; then rerun with -Apply.'
    return
}
$settings = Get-Content (Join-Path $policyDir 'settings.json') -Raw | ConvertFrom-Json
$null = Invoke-GitHubJson PATCH "repos/$Repository" $settings
$null = Invoke-GitHubJson PUT "repos/$Repository/actions/permissions" @{ enabled = $true; allowed_actions = 'selected'; sha_pinning_required = $true }
$null = Invoke-GitHubJson PUT "repos/$Repository/actions/permissions/selected-actions" @{ github_owned_allowed = $true; verified_allowed = $false; patterns_allowed = @('softprops/action-gh-release@*') }
$null = Invoke-GitHubJson PUT "repos/$Repository/actions/permissions/workflow" @{ default_workflow_permissions = 'read'; can_approve_pull_request_reviews = $false }
$null = Invoke-GitHubJson PUT "repos/$Repository/actions/permissions/fork-pr-contributor-approval" @{ approval_policy = 'all_external_contributors' }
$null = Invoke-GitHubJson PUT "repos/$Repository/private-vulnerability-reporting" $null
$null = Invoke-GitHubJson PUT "repos/$Repository/vulnerability-alerts" $null
$null = Invoke-GitHubJson PUT "repos/$Repository/automated-security-fixes" $null
$null = Invoke-GitHubJson PATCH "repos/$Repository" @{ security_and_analysis = @{ secret_scanning = @{ status = 'enabled' }; secret_scanning_push_protection = @{ status = 'enabled' } } }
$existing = @(Invoke-GitHubJson GET "repos/$Repository/labels?per_page=100" $null)
foreach ($label in (Get-Content (Join-Path $policyDir 'labels.json') -Raw | ConvertFrom-Json)) {
    if ($existing.name -contains $label.name) {
        $null = Invoke-GitHubJson PATCH "repos/$Repository/labels/$([Uri]::EscapeDataString($label.name))" $label
    } else { $null = Invoke-GitHubJson POST "repos/$Repository/labels" $label }
}
$rulesets = @(Invoke-GitHubJson GET "repos/$Repository/rulesets" $null)
foreach ($file in @('branch-ruleset.json', 'tag-ruleset.json')) {
    $rule = Get-Content (Join-Path $policyDir $file) -Raw | ConvertFrom-Json
    $existingRule = @($rulesets | Where-Object name -eq $rule.name)
    if ($existingRule.Count -gt 0) {
        $null = Invoke-GitHubJson PUT "repos/$Repository/rulesets/$($existingRule[0].id)" $rule
    } else { $null = Invoke-GitHubJson POST "repos/$Repository/rulesets" $rule }
}
Write-Host "Policy applied to $Repository. Run CI and CodeQL, verify Security settings, and review the template guide's remaining project-specific steps."
