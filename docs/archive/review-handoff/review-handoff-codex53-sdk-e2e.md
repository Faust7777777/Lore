# Review Handoff: Go SDK E2E Smoke

## Scope

This change implements P3 from `docs/adr-sdk-go-v0.md`: an opt-in end-to-end SDK test against a real `lore mcp` subprocess.

Changed files:

- `sdk/go/lore/e2e_test.go`

## Behavior

`TestSDKEndToEndWithLoreMCP` is gated by `LORE_SDK_E2E=1` so normal SDK unit tests remain fast and do not build binaries.

When enabled, the test:

1. Builds `./cmd/lore` into a temporary binary.
2. Bootstraps a temporary workdir using `lore bootstrap <workdir>`.
3. Starts the SDK with `Start(ctx, Options{Command: binPath, WorkDir: workDir})`.
4. Calls `Ping`.
5. Calls `ManagedStatus` and expects ready managed mode.
6. Calls `VaultResolve`.
7. Calls `ContextPack` and expects a progress doc.

## Commands

Default SDK test:

```powershell
Push-Location .\sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; Pop-Location
```

E2E SDK smoke:

```powershell
Push-Location .\sdk\go\lore; $env:LORE_SDK_E2E='1'; ..\..\..\.tools\go\bin\go.exe test ./... -run TestSDKEndToEndWithLoreMCP -count=1 -v; Pop-Location
```

Root regression:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
```

## Review Focus

Please check:

- E2E remains opt-in and does not slow normal `go test`.
- The test exercises the actual `lore mcp <workdir>` subprocess path, not internal packages.
- The SDK still does not import `internal/*`.
