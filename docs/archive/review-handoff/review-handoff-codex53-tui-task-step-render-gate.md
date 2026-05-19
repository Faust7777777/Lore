# A-Line Review Handoff: TUI Task-Step Render Gate

## Scope

This A-line slice extends the deterministic release gate to include existing TUI task-step render unit tests.

Changed files:

- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-tui-task-step-render-gate.md`

No TUI implementation files were changed in this slice.

## Why

The repository already has deterministic `internal/tui` render tests for the visible task-step section:

- task-step section appears when steps exist;
- tool name and status are rendered;
- `vault_resolve` / `vault_read` argument summaries are rendered;
- observation excerpts render with truncation;
- error steps render the error detail;
- normal non-error `lastOutput` is not rendered as a legacy output block.

A-line should gate these stable render-level behaviors without implementing or changing the TUI product flow.

## Gate Change

`scripts/release-gate.ps1` changed the TUI gate label and regex from approval-only:

```powershell
TestApprovalFlow_
```

to approval plus task-step render guardrails:

```powershell
Test(ApprovalFlow_|RenderInteractiveConversationShowsTaskSteps|RenderTaskStepsArgSummary|RenderTaskStepsTruncatesObservation|RenderTaskStepsErrorStep|RenderTaskStepsNonErrorLastOutputNotShown)
```

## Boundaries

- This does not activate `TestRunTUIOnceShowsResolveReadFinalTaskVisibility`.
- This does not assert the full resolve -> read -> final TUI end-to-end scenario.
- This does not modify `internal/tui/*`.
- This remains model-free and PR-safe.

## Verification

Clean detached worktree targeted test:

```powershell
go test ./internal/tui -run "Test(RenderInteractiveConversationShowsTaskSteps|RenderTaskStepsArgSummary|RenderTaskStepsTruncatesObservation|RenderTaskStepsErrorStep|RenderTaskStepsNonErrorLastOutputNotShown)$" -count=1 -v
```

Result: passed.

Clean detached worktree release gate with this script diff applied:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```

Result: passed.

The clean worktree run was necessary because the collaborative main worktree currently contains unrelated B/TUI/operatoragent dirty files that are outside this A-line slice.

## Reviewer Focus

- Confirm this only gates pre-existing TUI render tests.
- Confirm this is not claiming the full visible task-turn user scenario is implemented.
- Confirm the release-gate regex includes approval state tests and task-step render tests, but does not include the skipped scaffold.
