# 5.3 Codex Review Handoff: P0 Smoke Coverage Expansion

## Scope

This change expands the existing offline `smoke p0` check set.

Changed files:
- `internal/app/smoke.go`
- `cmd/obsidian-harness/main_test.go`

## New Checks

- `vault_resolve_unique`: verifies the smoke-created demo week note can be resolved through `Harness.VaultResolve` as a unique match.
- `sqlite_state_present`: verifies the default runtime created the SQLite state file at `<workdir>/state/store.db`.

## Contract

- This remains an offline deterministic smoke; it does not call the model provider.
- Existing P0 smoke coverage remains: managed core, draft/apply, process-sink checkpoint/report, audit chain.
- The vault resolve check avoids hard-coding Chinese path literals in source; it validates stable structural suffixes instead.
- Session transcript smoke stays in CLI/session tests and `docs/session-resume-smoke.md`; it is not mixed into app-level smoke.

## Review Focus

- Confirm `SmokeP0` remains deterministic and side-effect scoped to the supplied workdir.
- Confirm `sqlite_state_present` correctly tracks the current default runtime storage decision.
- Confirm `vault_resolve_unique` verifies resolver behavior without introducing brittle locale/encoding assertions.
- Confirm no TUI files are part of this change.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/app ./internal/cli ./cmd/obsidian-harness -count=1
```