# A-Line Review Handoff: Visible Task-Turn Gate Activated

## Scope

This A-line slice activates the deterministic command-level visible task-turn
acceptance test and wires it into the release gate.

Changed files:

- `cmd/obsidian-harness/main_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-visible-task-turn-gate-activated.md`

## Why

The original scaffold `TestRunTUIOnceShowsResolveReadFinalTaskVisibility` was
intentionally skipped until backend `Response.Steps`, console `LastTurnSteps`,
and TUI task-step rendering existed.

Those dependencies are now present:

- operator-agent emits bounded `Response.Steps`;
- console stores `Session.LastTurnSteps`;
- TUI view model receives `LastTurnSteps`;
- TUI render path displays `Task Steps`, arguments, and observation excerpts.

So A-line can now turn the scaffold into a deterministic PR-safe acceptance
test.

## Test Behavior

The test uses the existing fake model server and a temp workdir:

1. writes `vault/03-鐢诲儚/浜虹墿鑳屾櫙.md`;
2. runs one TUI `--once` request: `鎵撳紑浜虹墿鑳屾櫙锛屽熀浜庡畠鍐欎竴绡囨暎鏂囥€俙;
3. fake model drives `vault_resolve -> vault_read -> final`;
4. output must include:
   - `Task Steps`;
   - `vault_resolve`;
   - `vault_read`;
   - `query=浜虹墿鑳屾櫙`;
   - `path=03-鐢诲儚/浜虹墿鑳屾櫙.md`;
   - `obs:`;
   - file content excerpt `娴疯竟鑷範`;
   - final essay marker `鏈€缁堟暎鏂嘸;
5. output must not reintroduce the old `Latest Output` block.

This verifies visible progress and observation excerpts, not hidden
chain-of-thought.

## Release Gate Change

`scripts/release-gate.ps1` now includes:

```powershell
TestRunTUIOnceShowsResolveReadFinalTaskVisibility
```

in the `cmd/obsidian-harness` CLI smoke and daemon guardrail group.

## Verification

Targeted test:

```powershell
go test ./cmd/obsidian-harness -run "TestRun(TUIOnceShowsResolveReadFinalTaskVisibility|SmokeP0|SmokeP0FullIncludesGovernedNoteIntake)$" -count=1 -v
```

Result: passed.

Release gate quick path:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```

Result: passed.

`-SkipDiffCheck` was used because the local collaborative worktree still has
unrelated non-A files: `DEVELOPMENT_STATUS.md`, `--workdir/`, and
`scripts/start-tui.ps1`.

## Reviewer Focus

- Confirm this test is deterministic and does not use a real model provider.
- Confirm the assertions cover resolve, read, observation excerpt, final output,
  and absence of legacy `Latest Output`.
- Confirm the test does not require or expose hidden chain-of-thought.
- Confirm the release gate now fails if the visible task-turn scenario regresses.
