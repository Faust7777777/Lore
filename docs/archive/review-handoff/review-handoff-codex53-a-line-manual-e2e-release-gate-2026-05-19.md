# A-Line Review Handoff: Manual E2E Release Gate Verification 2026-05-19

## Scope

This handoff records the A-line verification pass for the opt-in manual E2E release gate.

A-line ownership for this check:

- release-gate E2E wiring;
- SDK/MCP E2E opt-in behavior;
- clean-worktree verification;
- gate documentation for reviewers.

Explicitly out of scope:

- TUI product implementation;
- operator-agent behavior changes;
- activation of the skipped visible task-turn acceptance scaffold;
- MCP surface expansion;
- approval pane behavior changes.

## Current HEAD Under Test

```text
cc17fea feat(operatoragent): expose turn steps with observation excerpts
```

The gate was run against a detached clean worktree created from the current
`HEAD`, so the local collaborative dirty files in the main worktree did not
affect the result.

## Verification Performed

Environment prerequisites were present before running the manual E2E gate:

```text
LORE_LLM_BASE_URL = set
LORE_LLM_API_KEY  = set
LORE_LLM_MODEL    = set
LORE_SDK_E2E      = unset
```

Command shape:

```powershell
$Repo = "C:\Users\15892\Desktop\obsidian-harness"
$TempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("lore-release-e2e-" + [System.Guid]::NewGuid().ToString("N"))
git worktree add --detach $TempRoot HEAD
Push-Location $TempRoot
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -E2E -AssertClean
Pop-Location
git worktree remove --force $TempRoot
```

Result:

```text
[gate] release gate passed
```

## What Passed

The manual E2E gate exercised:

- ToolRegistry schema and dispatch source-of-truth tests;
- MCP live v1 contract snapshot and proposal-only external boundary tests;
- governed smoke, daemon watcher, and post-scan guardrail tests;
- CLI smoke and daemon command guardrail tests;
- preferred `cmd/lore` wrapper smoke;
- Go SDK v0 contract and transport guardrails with default E2E skipped;
- console resolve-read-final and task-turn guardrails;
- sessionlog task-turn persistence guardrails;
- TUI approval state guardrails;
- full `verify.ps1`;
- `smoke p0 --full` using a temp workdir;
- explicit SDK E2E via `LORE_SDK_E2E=1`;
- `scripts/smoke-mcp-stdio.ps1`;
- `-AssertClean` worktree cleanliness assertion.

## Important Boundary Confirmations

- PR/default release gate remains deterministic and does not require real model secrets.
- Manual `-E2E` implies `-Full` and runs SDK/MCP E2E intentionally.
- Default SDK gate still clears inherited `LORE_SDK_E2E`; explicit E2E turns it back on only for the E2E test.
- Smoke workdirs are created under the system temp directory and cleaned up.
- `-AssertClean` passed in a detached clean worktree.
- `TestRunTUIOnceShowsResolveReadFinalTaskVisibility` remains a skipped scaffold and is not wired into the gate.

## Reviewer Focus

- Confirm this proves the GitHub `workflow_dispatch` path can run with `e2e=true` when repository secrets/vars are configured.
- Confirm E2E remains opt-in and does not leak into PR gates.
- Confirm no product behavior is changed by this documentation-only handoff.
- Confirm visible task-turn acceptance remains unactivated until the owning B/TUI line declares the UI/data path stable.

## Current Main Worktree Note

At the time of this handoff, the main worktree still contains unrelated non-A
dirty files that were intentionally left untouched:

- `DEVELOPMENT_STATUS.md`
- `--workdir/`
- `scripts/start-tui.ps1`
