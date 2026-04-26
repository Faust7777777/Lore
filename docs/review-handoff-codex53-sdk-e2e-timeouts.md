# Review Handoff: SDK E2E Timeout Hardening

## Scope

This change addresses review feedback on `TestSDKEndToEndWithLoreMCP` timeout behavior.

Changed files:

- `sdk/go/lore/e2e_test.go`
- `docs/review-handoff-codex53-sdk-e2e-timeouts.md`

## Changes

- `go build` now uses `exec.CommandContext` with a 30s timeout.
- `lore bootstrap <workdir>` now uses `exec.CommandContext` with a 10s timeout.
- SDK `Start` gets its own 5s context.
- SDK read calls share a separate 10s context after startup.

This avoids one short context covering build, bootstrap, startup, and all SDK calls. It also prevents build/bootstrap hangs from relying only on the outer `go test` timeout.

## Verification

```powershell
Push-Location .\sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -count=1
$env:LORE_SDK_E2E='1'; ..\..\..\.tools\go\bin\go.exe test ./... -run TestSDKEndToEndWithLoreMCP -count=1 -v
Pop-Location
.\.tools\go\bin\go.exe test ./... -count=1
```

All passed.
