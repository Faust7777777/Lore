# Review Handoff: SDK Gate E2E Env Isolation

## Scope

This A-line fix makes the targeted release-gate SDK step explicitly model-free by clearing `LORE_SDK_E2E` around the default SDK test run.

Changed files:

- `scripts/release-gate.ps1`

No SDK implementation, runtime behavior, MCP behavior, TUI behavior, or CI workflow behavior changed.

## Why

`sdk/go/lore` has an opt-in E2E test guarded by `LORE_SDK_E2E=1`. The targeted PR release gate should run SDK contract/boundary/transport unit tests only. If a developer or CI environment accidentally has `LORE_SDK_E2E=1`, the SDK gate could unexpectedly run the E2E path.

## Fix

Added `Invoke-SDKGate`, which:

1. saves the existing `LORE_SDK_E2E` value;
2. removes `LORE_SDK_E2E` while running `go test ./...` in `sdk/go/lore`;
3. restores the previous environment value in `finally`.

This mirrors the intended separation:

- default `release-gate.ps1`: deterministic, model-free SDK unit/contract tests;
- `release-gate.ps1 -E2E`: explicit opt-in E2E via `verify.ps1 -E2E`.

## Review Focus

- Confirm `LORE_SDK_E2E` is restored after the SDK gate.
- Confirm PR gate cannot accidentally run SDK E2E due to inherited environment.
- Confirm explicit `-E2E` behavior remains owned by `verify.ps1` and unchanged.

## Verification

```powershell
$env:LORE_SDK_E2E='1'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
$env:LORE_SDK_E2E
```

Expected: release gate passes, SDK E2E is skipped during the targeted SDK gate, and the environment variable remains `1` after the script returns.
