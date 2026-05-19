# Review Handoff: Backend Task-Turn Release Gate

## Scope

This A-line change wires already-landed deterministic backend task/turn guardrails into `scripts/release-gate.ps1`.

Changed files:

- `scripts/release-gate.ps1`

No TUI implementation, operator-agent behavior, MCP contract, or product UI files are changed.

## Why

The visible task/turn TUI acceptance test is still intentionally skipped until TUI rendering is stable. However, two backend guarantees are already implemented and deterministic:

1. A single console turn can complete `vault_resolve -> vault_read -> final` with a fake model and real runtime/vault stack.
2. Console/sessionlog persist task-turn end metadata for success and failure paths.

These are prerequisites for visible task progress. They should be protected by the PR gate now, without pretending the TUI acceptance scaffold is active.

## Release Gate Additions

`release-gate.ps1` now runs:

```powershell
go test ./internal/console -run "Test(EndToEndResolveReadFinalFileInspectionTurn|SessionHandleEmitsTaskTurnEndOnSuccess|SessionHandleEmitsTaskTurnEndOnFailure|SessionHandleSkipsTaskTurnEndForLegacyDecidePath)$" -count=1 -v

go test ./internal/sessionlog -run "TestRecordTaskTurnEnd(WritesEvent|EmptyReasonIsNoOp)$" -count=1 -v
```

These tests are deterministic and do not require real model secrets.

## Boundaries

- `TestRunTUIOnceShowsResolveReadFinalTaskVisibility` remains skipped and is still not wired into the gate.
- This change does not touch `internal/tui/*` implementation files.
- This change does not touch `internal/operatoragent/*` implementation or test files.
- This change does not alter MCP tool exposure.

## Review Focus

- Confirm PR gate remains model-free.
- Confirm this only gates backend prerequisites, not skipped TUI acceptance.
- Confirm the regexes are exact enough that unrelated console/sessionlog tests are not pulled in accidentally.
- Confirm no non-A dirty files are staged with this slice.

## Verification

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
