# A-Line Session Handoff: Release Gate Hardening 2026-05-19

## Scope

This handoff summarizes the A-line governance/CI gate work completed after the visible task-turn scaffold review.

A-line ownership for this batch:

- release gate and verification scripts;
- CI workflow wiring;
- deterministic backend/contract/smoke gate coverage;
- test-only fixes required for release-gate health;
- reviewer handoff docs.

Explicitly out of scope:

- TUI product implementation;
- operator-agent behavior changes;
- Cost/Usage product display;
- MCP surface expansion;
- approval pane behavior changes.

## Commits in this batch

```text
6d179f1 test: expand release gate governance guardrails
558eb2e ci: add manual e2e release gate input
02b25d2 test: gate preferred lore cli wrapper
58539c7 test: include go sdk in release gate
8d080d1 test: keep sdk release gate model-free
0c93d99 test: isolate sdk e2e in verify script
3c9185d ci: assert release gate worktree cleanliness
39430dd test: fix tui approval driver stub
```

## What changed

### 1. Release gate coverage expanded

`scripts/release-gate.ps1` now gates:

- `internal/tools`: ToolRegistry schema and dispatch source-of-truth;
- `internal/mcp`: live MCP v1 snapshot, SDK-facing snapshot, proposal-only allowlist, no direct write/apply/shell;
- `internal/app`: governed smoke, daemon watcher, post-scan guardrails;
- `cmd/obsidian-harness`: CLI smoke and daemon command guardrails;
- `cmd/lore`: preferred CLI wrapper build/dispatch smoke;
- `sdk/go/lore`: SDK v0 boundary, README contract, read-only methods, transport/error tests;
- `internal/console`: deterministic resolve-read-final and task-turn end emission;
- `internal/sessionlog`: task-turn persistence;
- `internal/tui`: approval state guardrails.

### 2. CI full/E2E split made explicit

`.github/workflows/release-gate.yml` now supports manual `workflow_dispatch` input:

- PR: deterministic release gate only;
- push to `main`: full gate, no E2E by default;
- manual workflow with `e2e=true`: full gate plus opt-in MCP/SDK E2E.

### 3. SDK E2E isolation fixed

Both scripts now avoid accidental SDK E2E execution from inherited shell env:

- `release-gate.ps1` clears/restores `LORE_SDK_E2E` around SDK unit gate;
- `verify.ps1` clears/restores `LORE_SDK_E2E` around default SDK tests;
- explicit `-E2E` still runs SDK E2E intentionally.

### 4. Cleanliness assertion added

`release-gate.ps1` has opt-in `-AssertClean`:

- rejects repo-local `.smoke-workdir`;
- rejects any `git status --short` output;
- used by CI PR and full gates;
- not used by default local dirty-worktree runs.

### 5. Clean worktree blocker fixed

A clean worktree run exposed a compile failure in `internal/tui/approval_state_test.go`: the test stub missed `ExecuteFindingAction` after the driver interface grew. Fixed by adding a minimal test-only stub method. No production TUI behavior changed.

## Verification performed

### Dirty collaborative worktree

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck -Repeat 2
```

Both passed.

### SDK E2E env isolation

```powershell
$env:LORE_SDK_E2E='1'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
$env:LORE_SDK_E2E
```

Passed. SDK E2E stayed skipped in the targeted gate and env restored to `1`.

```powershell
$env:LORE_SDK_E2E='1'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
$env:LORE_SDK_E2E
```

Passed. Default `verify.ps1` skipped SDK E2E and restored env to `1`.

### Clean temporary worktree

```powershell
git worktree add --detach <temp> HEAD
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -AssertClean
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -Full -AssertClean
git worktree remove --force <temp>
```

Both clean-worktree gates passed.

## Current working tree note

At the end of this A-line batch, A-line files are clean. Existing non-A dirty files remain intentionally untouched:

- `DEVELOPMENT_STATUS.md`
- `internal/operatoragent/agent.go`
- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `--workdir/`
- `scripts/start-tui.ps1`

## Reviewer focus

- Confirm release-gate additions are still deterministic and PR-safe.
- Confirm SDK E2E remains opt-in only.
- Confirm `-AssertClean` is opt-in locally but mandatory in CI.
- Confirm the TUI approval stub change is test-only and does not alter behavior.
- Confirm skipped visible task-turn TUI scaffold remains skipped and is not wired into PR gate.
