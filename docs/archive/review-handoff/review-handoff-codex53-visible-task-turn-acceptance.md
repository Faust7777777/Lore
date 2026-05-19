# 5.3 Codex Review Handoff: Visible Task/Turn Acceptance Scaffold

## Scope

This A-line change records the acceptance target for visible multi-step task
turns. It does not implement UI rendering or operator-agent behavior.

Changed files:
- `cmd/obsidian-harness/main_test.go`
- `docs/plans/2026-05-19-task-turn-visible-progress.md`

## Business Scenario

User request:

```text
打开人物背景，基于它写一篇散文。
```

Expected visible shape:

1. user request
2. `vault_resolve` / search step for `人物背景`
3. observation with selected `人物背景.md` or candidates
4. `vault_read` step for the selected path
5. bounded observation excerpt or summary from the file
6. final answer
7. generated essay

## Current State

- `TestRunTUIOnceShowsResolveReadFinalTaskVisibility` exists as an acceptance
  scaffold.
- The test is intentionally skipped with `t.Skip(...)`.
- The test is not wired into `scripts/release-gate.ps1`.
- This avoids pretending B/TUI has already implemented stable task-step
  rendering and observation excerpts.

## Activation Conditions

Before enabling the test or adding it to PR gate:

- B-line must expose stable task/turn step data with bounded observation
  excerpts.
- TUI line must render ordered steps and observation excerpts without exposing
  hidden chain-of-thought.
- The final UI labels must be confirmed, for example `Task Steps` versus the
  previous `Tool Trace` label.
- The test must run with a fake model only; PR gate must not require a real
  model provider.
- Failure messages should distinguish missing resolve, read, observation, and
  final output.

## Boundaries

- A line owns acceptance docs, test scaffold, and release-gate wiring.
- A line must not edit `internal/tui/*` implementation for this feature.
- A line must not edit `internal/operatoragent/*` behavior for this feature.
- Do not wire this scaffold into PR gate until the skip is removed and the test
  passes on a clean worktree.

## Review Focus

- Confirm the skipped scaffold does not create false CI confidence.
- Confirm the plan requires visible tool steps and observations, not hidden
  chain-of-thought.
- Confirm PR gate remains deterministic and model-free for this scenario.
- Confirm no TUI/operator-agent implementation files are part of the A-line
  acceptance scaffold.

## Verification

```powershell
go test ./cmd/obsidian-harness -run TestRunTUIOnceShowsResolveReadFinalTaskVisibility -count=1 -v
```

Expected result today: `SKIP`.
