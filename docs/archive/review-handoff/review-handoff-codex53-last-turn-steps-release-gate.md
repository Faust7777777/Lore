# Review Handoff: LastTurnSteps Compile Closure and Release Gate

## Scope

This slice closes a clean-HEAD compile blocker introduced by the TUI/session
task-step wiring and extends the release gate to cover the supporting contract.

Changed files:

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `internal/console/session.go`
- `internal/console/session_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-last-turn-steps-release-gate.md`

## Why

Clean detached `HEAD` failed in `scripts/release-gate.ps1` during the CLI smoke
package build:

```text
internal\tui\workbench_model.go:97:32: undefined: operatoragent.CloneTurnSteps
```

The already-committed TUI work switched `WorkbenchViewModel.TurnSteps` to
`[]operatoragent.TurnStep` and called `operatoragent.CloneTurnSteps`, but the
operator-agent helper was still private in the dirty tree.

## Fix

- Exported `operatoragent.CloneTurnSteps`.
- Updated internal callers/tests from `cloneTurnSteps` to `CloneTurnSteps`.
- Added `console.Session.LastTurnSteps` as an isolated snapshot of the most
  recent loop-agent `Response.Steps`.
- Cleared `LastTurnSteps` at the start of every `Session.Handle` so stale
  steps cannot leak across turns.
- Preserved steps on the `UsageError` path so failed turns can still show
  executed tool steps.
- Kept legacy `Decide` turns with empty `LastTurnSteps`.

## Release Gate Change

`scripts/release-gate.ps1` now covers:

- operator-agent step isolation and rune-boundary observation excerpt tests;
- console `LastTurnSteps` population, clearing, error-path preservation, legacy
  empty behavior, and snapshot isolation.

## Boundaries

- No MCP surface change.
- No external write/apply/shell expansion.
- No new product UI behavior is introduced here; the TUI code consuming
  `LastTurnSteps` already landed before this slice.
- The skipped command-level visible task-turn acceptance scaffold remains
  skipped.

## Verification

Targeted packages:

```powershell
go test ./internal/operatoragent ./internal/console ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1
```

Result: passed.

New gate regexes:

```powershell
go test ./internal/operatoragent -run "Test(ModelAgentRespondAppendsTurnStepPerToolCall|ModelAgentRespondTurnStepTruncatesLongObservation|ModelAgentRespondTurnStepRedactsBinaryObservation|ModelAgentRespondTurnStepCapturesToolError|ModelAgentRespondTurnStepsCarriedThroughUsageError|CloneTurnStepsDeepCopiesArguments|ModelAgentRespondStepsAreIsolatedFromTraceAndOtherSnapshots|BuildObservationExcerptRuneBoundaryTruncation)$" -count=1 -v

go test ./internal/console -run "Test(EndToEndResolveReadFinalFileInspectionTurn|SessionHandleEmitsTaskTurnEndOnSuccess|SessionHandleEmitsTaskTurnEndOnFailure|SessionHandleSkipsTaskTurnEndForLegacyDecidePath|SessionHandlePopulatesLastTurnStepsFromLoopAgentResponse|SessionHandleLastTurnStepsEmptyForFinalOnlyTurn|SessionHandleLastTurnStepsPreservedOnUsageErrorPath|SessionHandleClearsLastTurnStepsBetweenTurns|SessionHandleLastTurnStepsEmptyForLegacyDecidePath|SessionHandleLastTurnStepsIsolatedFromAgentResponse)$" -count=1 -v
```

Result: passed.

## Reviewer Focus

- Confirm the exported clone helper is the minimum stable API needed by TUI and
  console consumers.
- Confirm `Session.LastTurnSteps` is cleared before all early-return paths.
- Confirm `UsageError` keeps executed steps visible.
- Confirm the release gate now catches the exact clean-HEAD compile/contract
  gap that was observed.
