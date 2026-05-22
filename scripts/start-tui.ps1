param(
    [string]$WorkDir = "",
    [switch]$Reset,
    [switch]$SeedFindings,
    [switch]$SeedProcessSink,
    [switch]$LocalExec,
    [switch]$NoLaunch
)

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$Go = Join-Path $RepoRoot ".tools\go\bin\go.exe"
$LoreExe = Join-Path $RepoRoot "bin\lore-tui.exe"

if (-not (Test-Path -LiteralPath $Go)) {
    $GoCommand = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($null -eq $GoCommand) {
        throw "Go toolchain not found at $Go and go.exe is not on PATH"
    }
    $Go = $GoCommand.Source
}

function Resolve-WorkDirPath {
    param(
        [string]$Path
    )

    if ([string]::IsNullOrWhiteSpace($Path)) {
        $Path = ".tmp\tui-demo"
    }
    if ([System.IO.Path]::IsPathRooted($Path)) {
        return [System.IO.Path]::GetFullPath($Path)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $RepoRoot $Path))
}

function Assert-ResetTargetUnderRepo {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $repo = [System.IO.Path]::GetFullPath($RepoRoot).TrimEnd("\", "/")
    $target = [System.IO.Path]::GetFullPath($Path).TrimEnd("\", "/")
    if ($target.Equals($repo, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "-Reset refuses to remove the repository root"
    }

    $repoPrefix = $repo + [System.IO.Path]::DirectorySeparatorChar
    if (-not $target.StartsWith($repoPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "-Reset only removes workdirs under the repo root. Refusing to remove: $target"
    }
}

function Invoke-Lore {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    Write-Host "[tui] lore $($Arguments -join ' ')"
    & $LoreExe @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "lore $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

function Assert-LLMEnvForProcessSink {
    $required = @("LORE_LLM_BASE_URL", "LORE_LLM_API_KEY", "LORE_LLM_MODEL")
    $missing = @()
    foreach ($name in $required) {
        if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))) {
            $missing += $name
        }
    }
    if ($missing.Count -gt 0) {
        throw "-SeedProcessSink requires model config: $($missing -join ', ')"
    }
}

$ResolvedWorkDir = Resolve-WorkDirPath $WorkDir

Push-Location $RepoRoot
try {
    $binDir = Split-Path -Parent $LoreExe
    if (-not (Test-Path -LiteralPath $binDir)) {
        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    }

    Write-Host "[tui] building $LoreExe"
    & $Go build -o $LoreExe ".\cmd\obsidian-harness"
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }

    if ($Reset) {
        Assert-ResetTargetUnderRepo $ResolvedWorkDir
        if (Test-Path -LiteralPath $ResolvedWorkDir) {
            Write-Host "[tui] reset workdir $ResolvedWorkDir"
            Remove-Item -LiteralPath $ResolvedWorkDir -Recurse -Force
        }
    }

    $workDirParent = Split-Path -Parent $ResolvedWorkDir
    if (-not (Test-Path -LiteralPath $workDirParent)) {
        New-Item -ItemType Directory -Path $workDirParent -Force | Out-Null
    }

    Invoke-Lore -Arguments @("bootstrap", $ResolvedWorkDir)

    if ($SeedProcessSink) {
        Assert-LLMEnvForProcessSink
        Invoke-Lore -Arguments @("demo-p0b", $ResolvedWorkDir)
    }

    if ($SeedFindings) {
        # First scan establishes the baseline; the second scan detects
        # the intentionally out-of-band note edit as an open finding.
        Invoke-Lore -Arguments @("daemon", "run", "--workdir", $ResolvedWorkDir, "--once", "--debounce", "0ms")

        $notePath = Join-Path $ResolvedWorkDir "vault\03-notes\class\tui-demo-finding.md"
        $noteDir = Split-Path -Parent $notePath
        if (-not (Test-Path -LiteralPath $noteDir)) {
            New-Item -ItemType Directory -Path $noteDir -Force | Out-Null
        }
        $noteContent = @"
# TUI Demo Finding

This note was intentionally changed outside Lore so the daemon creates a finding.

Run ID: $([System.Guid]::NewGuid().ToString("N"))
"@
        Set-Content -LiteralPath $notePath -Value $noteContent -Encoding UTF8

        Invoke-Lore -Arguments @("daemon", "run", "--workdir", $ResolvedWorkDir, "--once", "--debounce", "0ms")
    }

    if ($NoLaunch) {
        Write-Host "[tui] prepared workdir: $ResolvedWorkDir"
        Write-Host "[tui] skipped launch because -NoLaunch was set"
        exit 0
    }

    $tuiArgs = @("tui", "--workdir", $ResolvedWorkDir)
    if ($LocalExec) {
        $tuiArgs += "--local-exec"
    }

    Write-Host "[tui] launching interactive TUI"
    & $LoreExe @tuiArgs
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
