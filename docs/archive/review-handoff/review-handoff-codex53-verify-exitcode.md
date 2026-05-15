# Review Handoff: verify.ps1 Native Exit Codes

## Change

`scripts/verify.ps1` now runs Go tests through `Invoke-GoTest`, which checks `$LASTEXITCODE` after each native `go.exe` invocation and throws on non-zero exit.

## Reason

Windows PowerShell does not convert native command non-zero exit codes into terminating errors when only `$ErrorActionPreference = "Stop"` is set. A failing `go test` could previously allow the script to continue and eventually report success.

## Files

- `scripts/verify.ps1`

## Verification

- `./scripts/verify.ps1` passes.
- A temporary failure probe using `go test ./does-not-exist -count=1` returns exit code 1 and emits the `Invoke-GoTest` error.

## Review Focus

- Confirm every native `go test` in `verify.ps1` goes through `Invoke-GoTest`.
- Confirm the script remains compatible with Windows PowerShell 5 and PowerShell 7.
