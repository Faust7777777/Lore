# Review Handoff: SDK E2E Go Toolchain Fallback

## Scope

This A-line verification fix makes the SDK E2E test work in environments without the repo-local `.tools/go` toolchain.

Changed files:

- `sdk/go/lore/e2e_test.go`

No SDK public API, MCP behavior, runtime behavior, TUI behavior, or operator-agent behavior changed.

## Why

`TestSDKEndToEndWithLoreMCP` is only run on explicit E2E paths, including `verify.ps1 -E2E` and manual release-gate workflow runs with `e2e=true`. GitHub Actions uses `actions/setup-go`, which provides `go` on PATH but does not create `.tools/go/bin/go.exe`. The test previously hard-coded `.tools/go/bin/go(.exe)`, so manual E2E workflow could fail before exercising SDK/MCP behavior.

## Fix

The test now resolves the Go toolchain as:

1. repo-local `.tools/go/bin/go(.exe)` if present;
2. `exec.LookPath(go(.exe))` fallback.

This matches `verify.ps1`, `release-gate.ps1`, and `smoke-mcp-stdio.ps1` behavior.

## Boundaries

- E2E remains opt-in behind `LORE_SDK_E2E=1`.
- Default SDK tests remain model-free and do not run this path.
- The built binary path and workdir still use `t.TempDir()`.

## Review Focus

- Confirm the fallback does not change SDK behavior under test.
- Confirm failure message is actionable when neither local nor PATH Go exists.
- Confirm this supports GitHub Actions manual `e2e=true` runs.

## Verification

```powershell
Push-Location .\sdk\go\lore
$env:LORE_SDK_E2E='1'
go test ./... -run TestSDKEndToEndWithLoreMCP -count=1 -v
Remove-Item Env:LORE_SDK_E2E
Pop-Location
```
