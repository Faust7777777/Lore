# B-Line Plan: Codex-style Task/Turn Backend Semantics

Created: 2026-05-19
Owner: B (business capability / data pipeline)
Scope: operatoragent, console, sessionlog, app, store, cli (non-TUI surface only)
Out of scope: TUI rendering, TUI state machine, MCP contract, CI workflows, smoke gates

## Goal

After the cost/usage work line closed, B refocuses on a single direction:
make "one user task = multiple bounded tool turns" first-class in the
operator agent backend, so external review can audit multi-step
behavior and so future TUI work has stable progress signals to render.

Usage work is in maintenance only -- B0 bugfix below, no new features.

## Slice Sequence

### B0 -- Usage local-day bucket fix (CLOSED)

Status: shipped in `7e0dd65 fix(model,store): bucket usage records by
local calendar day`.

Scope:

- `internal/model/runtime.go` -- new `NormalizeUsageDay` helper.
- `internal/store/{memory,jsonstore,sqlitestore}/store.go` -- usage
  Append/Summarize both go through local-day bucket; sqlite gains
  `usageDayString` separate from process-sink's `dayString`.
- `internal/store/sqlitestore/store.go` -- migration helper
  `appendUsageTx` also routed through `usageDayString`.
- Regression tests across model unit, app integration, sqlite backend,
  sqlite legacy JSON migration.
- End-to-end coverage: `internal/console/session_usage_e2e_test.go`
  (real session + real runtime + sqlite store) and
  `cmd/obsidian-harness/main_test.go::TestRunUsageCLIReadsFailedTurnUsageEndToEnd`
  (real failure turn -> real CLI).

No further usage-line slices unless a concrete bug surfaces. Explicitly
out: cost estimation, `--json`, provider/model breakdown, TUI usage
pane.

### B1 -- Loop capability audit (READ-ONLY, produces a one-pager)

Goal: before touching the loop, document what already works so we do
not rebuild existing capability.

Read-only scope:

- `internal/operatoragent`
- `internal/console`
- `internal/tools`
- `internal/cli/tui_workbench.go` (only to map driver expectations,
  not to modify)

Audit questions:

1. Does the current loop already support multiple tool calls within
   one user turn?
2. After each tool call, is the result fed back to the model?
3. What are today's stop conditions: final / max steps / repeated
   tool / parse failure / something else?
4. Is the existing `ToolCallTrace` expressive enough for a multi-step
   process, or does it miss step ordering / outcomes?
5. When "look at file X" fails today, where is the break: at
   `vault_resolve`, at `vault_read`, in the prompt, or invisible in
   trace?
6. Are `vault_resolve` and `vault_read` already wired such that a
   resolve -> read -> final sequence is mechanically possible?

Deliverable: a short companion doc
`docs/plans/2026-05-19-b1-loop-audit.md` covering:

- Current capability
- Current gaps
- Smallest-change plan to close gaps
- Expected files touched
- Test plan outline

### B2 -- Surface `StopReason` and `StepCount` on Response

Goal: make turn-shape observable without rewriting the loop. Smallest
useful step toward Task/Turn semantics.

Additive contract change on `operatoragent.Response` (frozen contract:
must update consuming layers + tests in the same commit per
AGENTS.md):

```go
type TurnStopReason string

const (
    TurnStopFinal      TurnStopReason = "final"
    TurnStopMaxSteps   TurnStopReason = "max_steps"
    TurnStopToolError  TurnStopReason = "tool_error"
    TurnStopModelError TurnStopReason = "model_error"
)

type Response struct {
    // existing fields preserved verbatim
    Final     string
    Decision  *Decision
    Trace     []ToolCallTrace
    Usage     []ModelCallUsage
    // additive
    StopReason TurnStopReason
    StepCount  int
}
```

Mapping for the existing return paths:

- final envelope -> `StopReason = "final"`, `StepCount = step+1`.
- legacy decision -> `StopReason = "final"` (treats decision as
  terminal).
- shell-confirm short circuit -> `StopReason = "final"`.
- max loop exceeded -> `StopReason = "max_steps"`.
- model parse / loop validation failure that goes through
  `failWithUsage` -> `StopReason = "model_error"`.
- ChatCompletion call itself errored (pre-usage) -> `StopReason =
  "model_error"`.

`tool_error` is reserved for B5 (bounded recovery work) and not
emitted by B2; defining the constant now keeps the enum stable.

Tests in `internal/operatoragent/model_test.go`:

- final-only -> stop=final, steps=1.
- tool_call then final -> stop=final, steps=2.
- max steps exceeded -> stop=max_steps.
- parse failure -> stop=model_error, usage still carried.

Consuming layers (must compile cleanly with the new fields):
`internal/console/session.go`, `internal/cli/tui_workbench.go`.
Neither needs to consume the new fields yet; just compile.

Commit: `feat(operatoragent): expose turn stop reason and step count`.

### B3 -- File workflow prompt / tool contract reinforcement

Goal: make the model reliably go resolve -> read -> final when the
user asks to inspect a vault file by name.

Scope:

- `internal/operatoragent/model.go`
- `internal/operatoragent/tool_schema.go`
- tests

Required instructions block (added to system / native tool guidance):

```
When the user asks to inspect, read, summarize, or explain a vault
file by name:
1. call vault_resolve first unless an exact path is already known;
2. if status is unique, call vault_read with selected_path;
3. answer from the file content;
4. if multiple matches, ask the user to choose instead of reading
   arbitrarily.
```

Constraints:

- MCP contract unchanged.
- No new write tools.
- No TUI changes.

Tests:

- Native tool schema for `vault_resolve` documents how
  `selected_path` should feed the next call.
- Fake model script (`resolve -> read -> final`) runs end-to-end.
- Multiple-matches case: model asks user to choose; does not
  auto-pick the first match.

Commit: `feat(operatoragent): guide file inspection through resolve-read-final workflow`.

### B4 -- Read-only file inspection e2e

Goal: prove "one natural-language utterance, multi-step completion"
on a real `*app.Runtime` with a real `console.Session`.

Scope:

- `internal/console`
- `cmd/obsidian-harness/main_test.go` or a new e2e test file
- Fake LLM client if needed

Fixture:

```
workdir/vault/03-notes/target.md
# Target
This file explains project onboarding.
```

Trace expected:

1. `vault_resolve {"query":"target"}`
2. `vault_read {"path":"03-notes/target.md"}`
3. final answering from the content

Assertions:

- User input occurs exactly once.
- Trace contains at least 2 tool steps.
- Final references content from the file.
- Session WorkingSet records the resolved path.
- No prompt for a second user input.

Commit: `test(console): cover resolve-read-final file inspection turn`.

### B5 -- Bounded recovery after read-only tool failure

Goal: a failed tool result in step N must let the model try one
correction in step N+1 within the same turn, with full trace
visibility.

Scope:

- `internal/operatoragent` (loop body and / or trace shaping)
- `internal/console/session_test.go` (if signal must surface)
- new tests

Scenarios:

- `vault_read` returns not-found -> tool result fed back -> model
  switches to `vault_resolve` or corrects path -> final.
- Same failing tool repeated three times in a row -> existing repeat
  protection still fires; `StopReason = "tool_error"`.
- Hitting max steps mid-recovery -> trace + `StopReason = "max_steps"`
  preserved.

Constraints:

- Do not relax safety boundaries.
- Do not swallow tool errors -- every failing step must remain in the
  trace.

Commit: `feat(operatoragent): allow bounded recovery after read-only tool failures`.

### B6 -- sessionlog records Task/Turn

Goal: future TUI / A line can consume per-turn shape without
re-deriving it. B only writes; no UI.

Scope:

- `internal/sessionlog`
- `internal/console/session.go`

Approach:

- Prefer to extend existing tool-trace event with `stop_reason` and
  `step_count` fields rather than a fresh schema.
- Old JSONL must still load; reader must ignore unknown fields.
- No change to `lore sessions show` behavior.

Tests:

- transcript shows step count for a sample turn.
- old JSONL without these fields still loads.
- reader skips unknown fields cleanly.

Commit: `feat(sessionlog): record task turn stop reason and step count`.

### B7 -- Developer trace command (OPTIONAL)

Goal: a small CLI helper for reviewing what happened on a stored turn.

Shape:

```
lore sessions show <id> --trace
```

Renders:

```
Turn 3
Stop: final
Steps: 3
1. vault_resolve ok query=target selected=03-notes/target.md
2. vault_read    ok path=03-notes/target.md bytes=128
3. final
```

Constraints: review tool only, not a user feature. Skip unless B has
spare bandwidth after B6.

## First Round Dispatch

B0 + B1 + B2.

- B0 already shipped (`7e0dd65`).
- B1 next: produce `docs/plans/2026-05-19-b1-loop-audit.md`.
- B2 next: implement `StopReason` / `StepCount` per the contract above.

After B1 lands, the user decides which of B3-B6 follow and whether B5
(recovery) is needed or if existing repeat-protection plus the audit
findings already cover the gap.

## Hard rules

- B never edits TUI files. If a feature needs TUI render changes, B
  produces a precise diff description and hands off.
- B never widens the MCP surface. New behavior stays in the local
  operator agent path.
- Frozen contracts (`operatoragent.Response`, `model.ManagedStatusView`,
  `model.Draft`, `app.DraftReview`, `app.ProcessSinkDayView`,
  `operatoragent.Context`, `operatoragent.ToolCallTrace`): additive
  changes only, with same-commit consumer compile fixes plus tests.
- Every slice ships independently and `go test ./... -count=1` green.
