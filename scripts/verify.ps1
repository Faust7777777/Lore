param(
    [switch]$E2E
)

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$Go = Join-Path $RepoRoot ".tools\go\bin\go.exe"

if (-not (Test-Path $Go)) {
    throw "Go toolchain not found at $Go"
}

function Invoke-GoTest {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    & $Go @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "go $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

function Invoke-PowerShellScript {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    & $Path
    if ($LASTEXITCODE -ne 0) {
        throw "$Path failed with exit code $LASTEXITCODE"
    }
}

Push-Location $RepoRoot
try {
    Invoke-GoTest -Arguments @("test", "./...", "-count=1")

    # --- Smoke gate (business-level, post unit tests) ---
    Write-Host "`n--- Smoke P0 --full ---" -ForegroundColor Cyan
    $SmokeBin = Join-Path $RepoRoot ".build\lore-smoke.exe"
    $SmokeBinDir = Split-Path -Parent $SmokeBin
    if (-not (Test-Path $SmokeBinDir)) {
        New-Item -ItemType Directory -Path $SmokeBinDir -Force | Out-Null
    }
    & $Go build -o $SmokeBin (Join-Path $RepoRoot "cmd\obsidian-harness\main.go")
    if ($LASTEXITCODE -ne 0) {
        throw "smoke binary build failed"
    }
    $SmokeWorkDir = Join-Path $RepoRoot ".smoke-workdir"
    if (Test-Path $SmokeWorkDir) {
        Remove-Item -LiteralPath $SmokeWorkDir -Recurse -Force
    }
    & $SmokeBin smoke p0 --workdir $SmokeWorkDir --full
    if ($LASTEXITCODE -ne 0) {
        throw "smoke p0 --full failed with exit code $LASTEXITCODE"
    }
    Write-Host "--- Smoke P0 passed ---`n" -ForegroundColor Green
    # --- End smoke gate ---

    if (Test-Path (Join-Path $RepoRoot "sdk\go\lore\go.mod")) {
        Push-Location (Join-Path $RepoRoot "sdk\go\lore")
        try {
            Invoke-GoTest -Arguments @("test", "./...", "-count=1")
            if ($E2E) {
                $previousE2E = $env:LORE_SDK_E2E
                try {
                    $env:LORE_SDK_E2E = "1"
                    Invoke-GoTest -Arguments @("test", "./...", "-run", "TestSDKEndToEndWithLoreMCP", "-count=1", "-v")
                } finally {
                    if ($null -eq $previousE2E) {
                        Remove-Item Env:LORE_SDK_E2E -ErrorAction SilentlyContinue
                    } else {
                        $env:LORE_SDK_E2E = $previousE2E
                    }
                }
            }
        } finally {
            Pop-Location
        }
    }
    if ($E2E) {
        Invoke-PowerShellScript -Path (Join-Path $RepoRoot "scripts\smoke-mcp-stdio.ps1")
    }
} finally {
    Pop-Location
}
