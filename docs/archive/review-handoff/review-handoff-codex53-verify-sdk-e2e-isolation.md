# Review Handoff: verify.ps1 SDK E2E Isolation

## Scope

This A-line verification fix makes default `scripts/verify.ps1` deterministic by clearing `LORE_SDK_E2E` around the SDK module's default test run.

Changed files:

- `scripts/verify.ps1`

No runtime, SDK implementation, MCP behavior, TUI behavior, or operator-agent behavior changed.

## Why

The SDK module contains `TestSDKEndToEndWithLoreMCP`, which intentionally runs only when `LORE_SDK_E2E=1`. Default `verify.ps1` is supposed to skip that E2E test unless the user passes `-E2E`. If a shell already has `LORE_SDK_E2E=1`, default `verify.ps1` could accidentally run SDK E2E before reaching the explicit `if ($E2E)` block.

## Fix

Before the default SDK `go test ./... -count=1`, `verify.ps1` now:

1. saves `LORE_SDK_E2E`;
2. removes it for the default SDK test run;
3. restores it in `finally`.

The explicit `verify.ps1 -E2E` path is unchanged: it still sets `LORE_SDK_E2E=1` only for `TestSDKEndToEndWithLoreMCP`.

## Review Focus

- Confirm default `verify.ps1` cannot accidentally run SDK E2E due to inherited environment.
- Confirm `verify.ps1 -E2E` still runs the SDK E2E test explicitly.
- Confirm environment restoration is correct for both previously unset and previously set values.

## Verification

```powershell
$env:LORE_SDK_E2E='1'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
$env:LORE_SDK_E2E
```

Expected: default SDK package output shows `TestSDKEndToEndWithLoreMCP` skipped, and the environment variable remains `1` after the script returns.
