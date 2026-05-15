# Review Handoff: Unified Verification Script

## Scope

This change adds one repo-level verification entrypoint that includes the Go SDK submodule.

Changed files:

- `scripts/verify.ps1`
- `README.md`
- `AGENTS.md`
- `docs/review-handoff-codex53-verify-script.md`

## Why

The root command `go test ./...` does not enter the independent `sdk/go/lore` module. After adding the SDK, cross-cutting verification needs to run both modules.

## Behavior

`scripts/verify.ps1`:

1. Locates the repo-managed Go binary at `.tools/go/bin/go.exe`.
2. Runs root module tests with `go test ./... -count=1`.
3. If `sdk/go/lore/go.mod` exists, enters that module and runs `go test ./... -count=1`.

README Quick Validation now points to `.\scripts\verify.ps1`.

AGENTS.md now tells agents to use `.\scripts\verify.ps1` for cross-cutting completion claims.

## Verification

```powershell
.\scripts\verify.ps1
```

Passed; output included both root packages and `obsidian-harness/sdk/go/lore`.
