<#
.SYNOPSIS
    Build script for cli-replay on Windows.

.DESCRIPTION
    Provides build, test, lint, and clean targets for cli-replay.

.PARAMETER Test
    Run the test suite with race detection and coverage.

.PARAMETER Lint
    Run golangci-lint (must be installed separately).

.PARAMETER Clean
    Remove build artifacts.

.PARAMETER All
    Build, test, and lint.

.EXAMPLE
    .\build.ps1          # Build only
    .\build.ps1 -Test    # Run tests
    .\build.ps1 -Lint    # Run linter
    .\build.ps1 -Clean   # Remove artifacts
    .\build.ps1 -All     # Build + test + lint
#>

[CmdletBinding()]
param(
    [switch]$Test,
    [switch]$Lint,
    [switch]$Clean,
    [switch]$All
)

$ErrorActionPreference = 'Stop'

$BinaryName = 'cli-replay.exe'
$Module = '.'

function Invoke-Build {
    Write-Host '==> Building cli-replay...' -ForegroundColor Cyan
    go build -o $BinaryName $Module
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    Write-Host "    Built $BinaryName" -ForegroundColor Green
}

function Invoke-Test {
    Write-Host '==> Running tests...' -ForegroundColor Cyan
    go test -race -cover ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
    Write-Host '    All tests passed' -ForegroundColor Green
}

function Invoke-Lint {
    Write-Host '==> Running linter...' -ForegroundColor Cyan
    $linter = Get-Command golangci-lint -ErrorAction SilentlyContinue
    if (-not $linter) {
        Write-Warning 'golangci-lint not found. Install from https://golangci-lint.run/usage/install/'
        return
    }
    golangci-lint run ./...
    if ($LASTEXITCODE -ne 0) { throw 'Lint failed' }
    Write-Host '    Lint passed' -ForegroundColor Green
}

function Invoke-Clean {
    Write-Host '==> Cleaning...' -ForegroundColor Cyan
    if (Test-Path $BinaryName) {
        Remove-Item $BinaryName -Force
        Write-Host "    Removed $BinaryName" -ForegroundColor Green
    } else {
        Write-Host '    Nothing to clean' -ForegroundColor Yellow
    }
}

# Default: build only (when no switches provided)
$noSwitch = -not ($Test -or $Lint -or $Clean -or $All)

if ($Clean) { Invoke-Clean }
if ($noSwitch -or $All) { Invoke-Build }
if ($Test -or $All) { Invoke-Test }
if ($Lint -or $All) { Invoke-Lint }

Write-Host '==> Done.' -ForegroundColor Cyan
