$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$Go = Join-Path $RepoRoot ".tools\go\bin\go.exe"

if (-not (Test-Path $Go)) {
    throw "Go toolchain not found at $Go"
}

Push-Location $RepoRoot
try {
    & $Go test ./... -count=1
    if (Test-Path (Join-Path $RepoRoot "sdk\go\lore\go.mod")) {
        Push-Location (Join-Path $RepoRoot "sdk\go\lore")
        try {
            & $Go test ./... -count=1
        } finally {
            Pop-Location
        }
    }
} finally {
    Pop-Location
}
