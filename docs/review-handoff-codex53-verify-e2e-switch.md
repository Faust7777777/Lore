# Review Handoff: verify.ps1 E2E Switch

## Change

`scripts/verify.ps1` now accepts an optional `-E2E` switch.

- Default `./scripts/verify.ps1` behavior is unchanged: root tests plus SDK module tests.
- `./scripts/verify.ps1 -E2E` additionally runs the opt-in SDK end-to-end smoke by temporarily setting `LORE_SDK_E2E=1` inside `sdk/go/lore`.

Docs updated:

- `README.md`
- `sdk/go/lore/README.md`

## Reason

The SDK E2E smoke was previously documented as a manual multi-command sequence. This change keeps default verification fast while providing a single standard entrypoint for the real MCP subprocess path.

## Safety

- The switch is opt-in.
- The previous `LORE_SDK_E2E` environment value is restored after the E2E run.
- Native `go test` failures still go through `Invoke-GoTest` and fail the script.

## Verification

```powershell
.\scripts\verify.ps1
.\scripts\verify.ps1 -E2E
```

Both passed locally.

## Review Focus

- Confirm `param([switch]$E2E)` remains at the top of the PowerShell script.
- Confirm default verification does not run the SDK E2E test.
- Confirm `-E2E` runs `TestSDKEndToEndWithLoreMCP` and restores `LORE_SDK_E2E` afterward.
