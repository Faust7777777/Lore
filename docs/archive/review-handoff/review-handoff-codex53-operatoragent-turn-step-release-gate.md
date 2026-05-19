# A-Line Review Handoff: Operator-Agent Turn-Step Release Gate

## Scope

This A-line slice adds the newly landed operator-agent turn-step / observation excerpt contract to the deterministic release gate.

Changed files:

- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-operatoragent-turn-step-release-gate.md`

No production runtime, TUI, MCP, SDK, or operator-agent implementation files were changed in this slice.

## Why

The B-line backend now exposes structured `operatoragent.Response.Steps` with:

- ordered tool step indexes;
- tool name and arguments;
- status and error capture;
- bounded `ObservationExcerpt`;
- binary-content redaction;
- usage-error carry-through;
- deep-copy protection for step argument maps.

A-line should not implement the product UI, but it should lock this backend contract into the release gate so future changes cannot silently remove the data TUI needs for visible task-turn rendering.

## Gate Change

`scripts/release-gate.ps1` now includes:

```powershell
Invoke-GoGate `
    -Label "operator-agent turn-step observation guardrails" `
    -Package "./internal/operatoragent" `
    -Run "Test(ModelAgentRespondAppendsTurnStepPerToolCall|ModelAgentRespondTurnStepTruncatesLongObservation|ModelAgentRespondTurnStepRedactsBinaryObservation|ModelAgentRespondTurnStepCapturesToolError|ModelAgentRespondTurnStepsCarriedThroughUsageError|CloneTurnStepsDeepCopiesArguments)$"
```

## Boundaries

- `TestRunTUIOnceShowsResolveReadFinalTaskVisibility` remains skipped.
- The skipped TUI acceptance scaffold is still not wired into PR gate.
- This slice does not change `internal/tui/*`.
- This slice does not change `internal/operatoragent/*`; it only gates existing tests.
- This slice does not require a real model and is PR-safe.

## Verification

Targeted operator-agent gate:

```powershell
go test ./internal/operatoragent -run "Test(ModelAgentRespondAppendsTurnStepPerToolCall|ModelAgentRespondTurnStepTruncatesLongObservation|ModelAgentRespondTurnStepRedactsBinaryObservation|ModelAgentRespondTurnStepCapturesToolError|ModelAgentRespondTurnStepsCarriedThroughUsageError|CloneTurnStepsDeepCopiesArguments)$" -count=1 -v
```

Result: passed.

Release gate quick path:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```

Result: passed.

`-SkipDiffCheck` was used because the collaborative main worktree currently contains unrelated non-A dirty files.

## Reviewer Focus

- Confirm the new gate only locks backend step/excerpt data and does not assert TUI product rendering.
- Confirm the added tests remain deterministic and model-free.
- Confirm the skipped visible task-turn TUI acceptance remains skipped until B/TUI explicitly finishes the render path.
