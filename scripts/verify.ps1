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

Push-Location $RepoRoot
try {
    Invoke-GoTest -Arguments @("test", "./...", "-count=1")
    if (Test-Path (Join-Path $RepoRoot "sdk\go\lore\go.mod")) {
        Push-Location (Join-Path $RepoRoot "sdk\go\lore")
        try {
            Invoke-GoTest -Arguments @("test", "./...", "-count=1")
        } finally {
            Pop-Location
        }
    }
} finally {
    Pop-Location
}
