param(
    [switch]$Full,
    [switch]$E2E,
    [int]$Repeat = 1,
    [switch]$SkipDiffCheck
)

$ErrorActionPreference = "Stop"

if ($Repeat -lt 1) {
    throw "-Repeat must be >= 1"
}
if ($E2E) {
    $Full = $true
}

$RepoRoot = Split-Path -Parent $PSScriptRoot
$Go = Join-Path $RepoRoot ".tools\go\bin\go.exe"

if (-not (Test-Path $Go)) {
    $GoCommand = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($null -eq $GoCommand) {
        throw "Go toolchain not found at $Go and go.exe is not on PATH"
    }
    $Go = $GoCommand.Source
}

function Invoke-NativeChecked {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Label,
        [Parameter(Mandatory = $true)]
        [string]$FilePath,
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    Write-Host "[gate] $Label"
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

function Invoke-GoGate {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Label,
        [Parameter(Mandatory = $true)]
        [string]$Package,
        [Parameter(Mandatory = $true)]
        [string]$Run
    )

    for ($i = 1; $i -le $Repeat; $i++) {
        $suffix = ""
        if ($Repeat -gt 1) {
            $suffix = " ($i/$Repeat)"
        }
        Invoke-NativeChecked -Label "$Label$suffix" -FilePath $Go -Arguments @(
            "test",
            $Package,
            "-run",
            $Run,
            "-count=1",
            "-v"
        )
    }
}

Push-Location $RepoRoot
try {
    if (-not $SkipDiffCheck) {
        Invoke-NativeChecked -Label "git diff --check" -FilePath "git" -Arguments @("diff", "--check")
    }

    Invoke-GoGate `
        -Label "ToolRegistry schema and dispatch source-of-truth" `
        -Package "./internal/tools" `
        -Run "Test"

    Invoke-GoGate `
        -Label "MCP registry contract and external boundary" `
        -Package "./internal/mcp" `
        -Run "Test(LiveMCPToolContractV1Snapshot|SDKFacingToolContractSnapshot|MCPV1ExposesOnlyReadAndProposalTools|ExternalMCPDoesNotExposeDirectWrites)$"

    Invoke-GoGate `
        -Label "governed smoke, daemon watcher, and post-scan guardrails" `
        -Package "./internal/app" `
        -Run "Test(RuntimeSmokeP0|RuntimeSmokeGovernedMarkdownNoteIntake|RuntimeSmokeExternalMCPGovernedMarkdownNoteIntake|RunVaultDaemonWatcherCreatesDraftAfterFileChange|RunVaultDaemonWatcherIgnoresObsidianDirectory|RunVaultDaemonWatcherSyncsCodexJSONLBeforePoll|RuntimeScanVaultChangesPrimesThenCreatesDraft|RuntimeScanVaultChangesWaitsForDebounceBeforeCreatingDraft|RuntimeScanVaultChangesAuditsOutOfBandOrdinaryNote|RuntimeScanVaultChangesAuditsGovernedCoreOutOfBandChange|RuntimeScanVaultChangesAuditsProcessSinkOutOfBandChange|RuntimeScanVaultChangesAuditsNewOutOfBandOrdinaryNoteAfterBaseline|RuntimeScanVaultChangesAuditsRecreatedGovernedCoreAfterBaseline|RuntimeScanVaultChangesAuditsNewProcessSinkAfterBaseline)$"

    Invoke-GoGate `
        -Label "CLI smoke and daemon command guardrails" `
        -Package "./cmd/obsidian-harness" `
        -Run "TestRun(SmokeP0|SmokeP0FullIncludesGovernedNoteIntake|DaemonOnceTriggersDraftAfterStablePlanChange|DaemonOnceSyncsCodexJSONLWhenConfigured|DaemonOnceMissingCodexJSONLRemainsNonFatal)$"

    Invoke-GoGate `
        -Label "console resolve-read-final and task-turn guardrails" `
        -Package "./internal/console" `
        -Run "Test(EndToEndResolveReadFinalFileInspectionTurn|SessionHandleEmitsTaskTurnEndOnSuccess|SessionHandleEmitsTaskTurnEndOnFailure|SessionHandleSkipsTaskTurnEndForLegacyDecidePath)$"

    Invoke-GoGate `
        -Label "sessionlog task-turn persistence guardrails" `
        -Package "./internal/sessionlog" `
        -Run "TestRecordTaskTurnEnd(WritesEvent|EmptyReasonIsNoOp)$"

    Invoke-GoGate `
        -Label "TUI approval state guardrails" `
        -Package "./internal/tui" `
        -Run "TestApprovalFlow_"

    if ($Full) {
        $verifyArgs = @(
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-File",
            (Join-Path $RepoRoot "scripts\verify.ps1")
        )
        if ($E2E) {
            $verifyArgs += "-E2E"
        }
        Invoke-NativeChecked -Label "full verify.ps1" -FilePath "powershell.exe" -Arguments $verifyArgs
    }

    Write-Host "[gate] release gate passed"
} finally {
    Pop-Location
}
