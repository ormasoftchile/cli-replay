#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Azure App Service TSG — demonstrates value chaining across cli-replay steps.

.DESCRIPTION
  Each command's JSON output is parsed and its values feed into the next
  command's arguments, exactly like a real Troubleshooting Guide would.

  The scenario YAML uses {{ .any }} and {{ .regex }} so it accepts
  whatever names the script discovers at runtime.

.EXAMPLE
  cli-replay run examples/azure-tsg.yaml | Invoke-Expression
  .\examples\run-azure-tsg.ps1
  cli-replay verify
  cli-replay clean
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Write-Host "`n=== Azure App Service TSG ===" -ForegroundColor Cyan

# ── Step 1: Which subscription are we in? ─────────────────────────────
Write-Host "`n► Step 1: Checking active subscription..." -ForegroundColor Yellow
$account = az account show | ConvertFrom-Json

Write-Host "  Subscription : $($account.name)"
Write-Host "  ID           : $($account.id)"
Write-Host "  Tenant       : $($account.tenantId)"

# ── Step 2: List webapps — discover names + resource group ────────────
$rg = "contoso-prod-rg"                          # typically from a TSG constant or earlier lookup
Write-Host "`n► Step 2: Listing webapps in resource group '$rg'..." -ForegroundColor Yellow
$apps = az webapp list --resource-group $rg | ConvertFrom-Json

Write-Host "  Found $($apps.Count) app(s):"
foreach ($a in $apps) {
    $marker = if ($a.state -eq 'Stopped') { ' ⚠' } else { '' }
    Write-Host "    - $($a.name)  [$($a.state)]$marker"
}

# Pick the first unhealthy app — this is the value that flows forward
$sick = $apps | Where-Object { $_.state -ne 'Running' } | Select-Object -First 1
if (-not $sick) {
    Write-Host "`n  All apps are healthy — nothing to do." -ForegroundColor Green
    return
}

$appName = $sick.name
$appRg   = $sick.resourceGroup
Write-Host "`n  Investigating: $appName (rg=$appRg)" -ForegroundColor Magenta

# ── Step 3: Get details — args built from step 2 output ───────────────
Write-Host "`n► Step 3: Inspecting '$appName'..." -ForegroundColor Yellow
$detail = az webapp show --name $appName --resource-group $appRg | ConvertFrom-Json

Write-Host "  State         : $($detail.state)"
Write-Host "  Availability  : $($detail.availabilityState)"
Write-Host "  Runtime       : $($detail.siteConfig.linuxFxVersion)"
Write-Host "  Start command : $($detail.siteConfig.appCommandLine)"
Write-Host "  Last modified : $($detail.lastModifiedTimeUtc)"

# ── Step 4: Tail logs — same $appName / $appRg ────────────────────────
Write-Host "`n► Step 4: Tailing logs for '$appName'..." -ForegroundColor Yellow
$logs = az webapp log tail --name $appName --resource-group $appRg

Write-Host $logs

# ── Step 5: Restart — same $appName / $appRg ──────────────────────────
Write-Host "`n► Step 5: Restarting '$appName'..." -ForegroundColor Yellow
az webapp restart --name $appName --resource-group $appRg | Out-Null

Write-Host "  Restart issued."

# ── Step 6: Confirm recovery — same $appName / $appRg ─────────────────
Write-Host "`n► Step 6: Verifying '$appName' recovered..." -ForegroundColor Yellow
$after = az webapp show --name $appName --resource-group $appRg | ConvertFrom-Json

if ($after.state -eq 'Running' -and $after.availabilityState -eq 'Normal') {
    Write-Host "  ✅ $appName is Running / Normal" -ForegroundColor Green
} else {
    Write-Host "  ❌ $appName is $($after.state) / $($after.availabilityState)" -ForegroundColor Red
}

Write-Host "`n=== TSG Complete ===" -ForegroundColor Cyan
