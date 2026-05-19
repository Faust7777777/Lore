# Review Handoff: SDK v0 Release Gate

## Scope

This A-line change wires the Go SDK v0 module into `scripts/release-gate.ps1`.

Changed files:

- `scripts/release-gate.ps1`
- `README.md`

No SDK implementation, runtime behavior, MCP behavior, TUI behavior, or operator-agent behavior changed.

## Why

The SDK is a separately rooted Go module under `sdk/go/lore`, so root-module `go test ./...` does not cover it. The full `verify.ps1` covers SDK tests, but the targeted PR release gate did not. That left SDK boundary/contract/transport regressions outside the fast release-candidate gate.

## Release Gate Addition

`release-gate.ps1` now includes `Invoke-GoModuleGate`, which temporarily enters the module directory and runs:

```powershell
go test ./... -count=1 -v
```

for `sdk\go\lore`.

The default SDK test set is model-free. `TestSDKEndToEndWithLoreMCP` remains skipped unless `LORE_SDK_E2E=1`; E2E stays behind `release-gate.ps1 -E2E` / manual workflow opt-in.

## Boundaries

- Does not add SDK E2E to PR gate.
- Does not change SDK public API or typed surface.
- Does not change MCP live tool exposure.
- Does not duplicate `verify.ps1 -E2E` behavior.

## Review Focus

- Confirm `Invoke-GoModuleGate` restores the previous working directory in `finally`.
- Confirm `-Repeat` applies consistently to SDK module tests.
- Confirm PR gate remains deterministic and does not require `LORE_SDK_E2E` or model secrets.
- Confirm README release-gate wording matches the script.

## Verification

```powershell
Push-Location .\sdk\go\lore; go test ./... -count=1 -v; Pop-Location
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
