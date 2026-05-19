# A-Line Review Handoff: Visible Task-Turn Documentation Alignment

## Scope

This docs-only slice updates active documentation after the visible task-turn
acceptance test was activated and wired into the release gate.

Changed files:

- `README.md`
- `docs/plans/2026-05-19-task-turn-visible-progress.md`
- `docs/archive/review-handoff/review-handoff-codex53-visible-task-turn-doc-alignment.md`

## Why

The plan and README still described the visible task-turn command-level test as
a future/skipped scaffold. That is no longer true after
`TestRunTUIOnceShowsResolveReadFinalTaskVisibility` became active and was added
to `scripts/release-gate.ps1`.

## Change

- README now says the release gate covers TUI approval/task-step render tests
  and the deterministic visible task-turn acceptance scenario.
- The task-turn plan now marks A2/A3 as implemented for the deterministic
  fake-model scenario.
- The completed checklist records that the test uses `Task Steps`, asserts an
  observation excerpt, avoids a real model dependency, and is wired into PR
  gate.

## Boundaries

- No code changed.
- No release script changed.
- No historical archived handoff was rewritten.

## Verification

```powershell
git diff --check -- README.md docs/plans/2026-05-19-task-turn-visible-progress.md docs/archive/review-handoff/review-handoff-codex53-visible-task-turn-doc-alignment.md
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```

Result: passed.

## Reviewer Focus

- Confirm docs no longer imply the visible task-turn scaffold is skipped.
- Confirm wording stays scoped to deterministic fake-model PR gate, not live
  model E2E.
- Confirm optional future live streaming remains explicitly later work.
