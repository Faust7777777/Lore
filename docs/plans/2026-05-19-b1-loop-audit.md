# B1: Operator Agent Loop Audit (read-only)

Created: 2026-05-19
Companion to: docs/plans/2026-05-19-b-line-task-turn-semantics.md

## Current capability

### Multi-step within a single user turn

Yes. `ModelAgent.Respond` runs a `for step := 0; step < maxLoopSteps;
step++` loop with `maxLoopSteps = 8`. Each iteration issues one
`ChatCompletion` call and either returns terminal or continues to the
next step.
(internal/operatoragent/model.go:266)

### Tool result fed back

Yes. Both the native `resp.ToolCalls` path and the JSON-envelope
`{"type":"tool_call"}` path append two messages to `messages` before
the next iteration:

1. an assistant message restating the tool call (synthetic JSON or
   raw `resp.Content`), and
2. a user message `"Tool result for <tool>:\n<content>"` via
   `buildToolResultPrompt`.
   (internal/operatoragent/model.go:320 and :383)

Errors are surfaced into the same user message with the err
serialized inline (model.go:317-323), so the model sees both the
success content and any failure on the next turn.

### Stop conditions in effect today

| Trigger | Where | Outcome |
|---|---|---|
| `{"type":"final"}` envelope | model.go:341-350 | success, Response.Final |
| Legacy `{"action": ...}` decision | model.go:332-338 | success, Response.Decision |
| Shell-confirm pending result | model.go:310-316 / :373-379 | success, Response.Final (one-step short circuit) |
| Multiple native ToolCalls in one response | model.go:286 | `*UsageError("model returned N tool calls")` |
| Empty tool name (native) | model.go:291 | `*UsageError("tool_call.name is required")` |
| Empty tool name (envelope) | model.go:354 | `*UsageError("tool_call.tool is required")` |
| Same call repeated 3x in a row | model.go:295 / :358 via `hasRepeatedToolLoop` | `*UsageError("repeated tool loop detected for X")` |
| 6-step AB-AB-AB ping-pong | model.go:298 / :361 via `hasPingPongToolLoop` | `*UsageError("alternating tool loop detected")` |
| Parse failure | model.go:330 | `*UsageError("invalid loop response: ...")` |
| Empty final message | model.go:344 | `*UsageError("final response is empty")` |
| Unsupported envelope type | model.go:389 | `*UsageError("unsupported response type X")` |
| 8 steps exceeded | model.go:393 | `*UsageError("exceeded max loop steps (8)")` |
| ChatCompletion call itself errored | model.go:273 | bare error, no usage billed |

### Trace expressiveness

`ToolCallTrace { Name, Arguments, Status, Error }`:

- `Status` is one of `"ok"`, `"error"`, `"pending"` (pending only for
  shell confirmation).
- Order is preserved -- trace is appended step-by-step.
- Error string is captured.

This is enough to reconstruct what tools were called and in what
order, including failures. It does NOT carry:

- whole-turn shape (final reached vs aborted, and how);
- step count summary;
- correlation between trace step and the model response index;
- the model envelope that produced the call (envelope vs native).

For B2 the missing piece is whole-turn shape -- adding `StopReason`
and `StepCount` to `Response` lets a single read tell us whether the
turn finished, errored, or hit the step ceiling, without scanning the
trace.

### "Look at file X" failure modes today

Assume the model is asked "what does target.md say".

- **Path-guessing risk**: nothing in the loop or prompt forces a
  `vault_resolve` first. The model can call `vault_read {path: "X"}`
  directly. If the path is wrong, `vault_read` returns an error, the
  next iteration sees `Tool result for vault_read: ... error ...`,
  and the model is free to retry. So mechanically retry works, but
  the dance is not deterministic.

- **Where the break sits**: prompt. `loopSystemPrompt` instructs the
  model on JSON envelope shape, governance, and tool preference, but
  not on a resolve-read-final discipline
  (internal/operatoragent/model.go:596-678). Tool schemas describe
  individual tools but never link `vault_resolve` to `vault_read`.

- **Trace visibility**: failed reads do appear in trace with
  Status=error, so post-hoc the break is auditable. But the user
  observing the conversation only sees the final answer; they have no
  current way to see "the agent tried 3 paths" without inspecting
  transcript.

### vault_resolve and vault_read availability

Both already exist as read-only tools on `SurfaceMCP | SurfaceConsole`:

- `vault_resolve` (internal/tools/builtin_readonly.go:114) -- args
  `query / dir / limit`, description "Resolve a natural-language note
  reference to vault markdown paths. Returns unique, ambiguous, or
  not_found."
- `vault_read` (internal/tools/builtin_readonly.go:67) -- args `path`,
  description "Read a markdown document from the vault."

A resolve -> read -> final sequence is mechanically supported today.
The gap is purely prompt guidance plus, ideally, schema-level
description on how `vault_resolve`'s `selected_path` output feeds
`vault_read`.

## Current gaps

1. **No turn-shape signal in Response.** Callers (console, sessionlog,
   any future TUI progress UI) cannot tell why a turn ended without
   string-matching `response.Final` or scanning `trace`.

2. **No prompt discipline for the inspect-file path.** The model is
   free to guess paths. When it guesses wrong it eventually recovers
   via the error-feedback channel, but the dance is non-deterministic
   and burns extra ChatCompletion calls.

3. **No schema-level link between resolve and read.** `vault_resolve`
   description does not point the model at `vault_read` as the next
   call; `vault_read` description does not mention `selected_path`
   from `vault_resolve` as the expected input.

4. **No structured recovery semantics.** Today recovery is implicit
   through the same loop -- a failed tool result is just the next
   user message. There is no explicit "tool_error" stop reason or
   recovery budget separate from `maxLoopSteps`. If the model gets
   stuck retrying with similar arguments, `hasRepeatedToolLoop`
   eventually fires (threshold 3, byte-identical signature), but
   "trying three different wrong paths" still chews through the
   8-step budget silently.

5. **Trace lacks step-count summary.** Counting steps for
   user-visible "3 steps to answer" requires `len(trace)`, which is
   accurate today but couples consumers to the trace internal shape.

## Smallest-change plan to close the gaps

### Address gap 1 (turn shape) -- B2

Add `StopReason TurnStopReason` and `StepCount int` to
`operatoragent.Response`. Populate at every existing return point.
`tool_error` constant defined now but emitted only when B5 lands.
Tests and consumer compile fixes in the same commit per AGENTS.md.

### Address gaps 2 and 3 (file workflow) -- B3

In `loopSystemPrompt` add a short paragraph describing the
resolve-read-final discipline. Optionally also extend
`vault_resolve` / `vault_read` Descriptions to reference each other
without changing the live schema shape (the MCP `tools/list` output
stays the same fields, just clearer description text). MCP boundary
unchanged.

### Address gap 4 (structured recovery) -- B5 only if needed

Defer until B4 (e2e) demonstrates whether B3 alone is enough. If the
e2e shows the model still wastes steps on bad guesses, introduce
`StopReason = "tool_error"` and an explicit recovery budget separate
from `maxLoopSteps`.

### Address gap 5 (step count) -- subsumed by B2

`StepCount` lands as part of B2.

## Expected files touched (per slice)

**B2** (turn shape):

- `internal/operatoragent/agent.go` -- add `TurnStopReason` consts,
  add fields on `Response`.
- `internal/operatoragent/model.go` -- populate at every return point
  in `Respond`.
- `internal/operatoragent/model_test.go` -- assertions on `StopReason`
  and `StepCount`.
- Consumer compile path verification (no behavior change needed):
  `internal/console/session.go`, `internal/cli/tui_workbench.go`.

**B3** (file workflow guidance):

- `internal/operatoragent/model.go` -- extend `loopSystemPrompt` text.
- `internal/operatoragent/tool_schema.go` -- if any guidance is
  schema-level, edit there; otherwise no change.
- `internal/tools/builtin_readonly.go` -- if Description tweaks for
  `vault_resolve` / `vault_read` help the model, edit there. MCP
  schema shape unchanged.
- `internal/operatoragent/model_test.go` -- assertions on prompt
  content and on a fake-LLM-driven resolve-read-final transcript.

**B4** (e2e):

- `internal/console/session_*_test.go` (new file or extend existing
  e2e) -- real Session + real Runtime + fake completion client. Same
  pattern as the existing usage e2e
  `internal/console/session_usage_e2e_test.go`.

**B5 (deferred)**:

- `internal/operatoragent/model.go` -- emit `tool_error` stop reason
  if added.
- Tests adjusted.

**B6** (sessionlog):

- `internal/sessionlog/{model.go,writer.go}` -- additive fields on the
  tool-trace event (`stop_reason`, `step_count`); reader still
  tolerates missing fields.
- `internal/console/session.go` -- call site updated.
- Tests for backward compatibility with old JSONL.

**B7 (optional)**:

- `internal/cli/sessions_command.go` or `internal/cli/cli.go` -- add
  `--trace` subcommand to `lore sessions show`.

## Test plan outline

**B2**:

- final-only fake completion -> `StopReason="final"`, `StepCount=1`.
- tool_call then final fake completion -> `StopReason="final"`,
  `StepCount=2`.
- max steps fake (8 tool_calls in a row, never final) ->
  `StopReason="max_steps"`, `StepCount=maxLoopSteps`.
- malformed JSON -> `StopReason="model_error"`, `StepCount=1`,
  Usage still carried via `*UsageError`.
- legacy decision -> `StopReason="final"`, `StepCount=1` (decision is
  terminal).
- shell-confirm short-circuit -> `StopReason="final"`, `StepCount=1`.

**B3**:

- `loopSystemPrompt` output contains the resolve-read-final block.
- Fake LLM that returns `{type: tool_call, tool: vault_resolve, ...}`
  -> `{type: tool_call, tool: vault_read, path: selected}` ->
  `{type: final, ...}` produces the expected trace and final.
- Fake LLM that on `vault_resolve` returns `status=ambiguous` (via
  mocked `runtime.VaultResolve`) -> model returns a `final` asking
  user to disambiguate, does not call `vault_read`.

**B4**:

- Real Runtime with a sample workdir containing
  `vault/03-notes/target.md`.
- Session + fake completion client driving resolve -> read -> final.
- Assert: one user input, trace length >= 2, final references file
  content, WorkingSet records the resolved path.

**B5** (only if needed):

- Failing read with wrong path -> next step resolves -> reads ->
  final. Trace preserves the failed step.
- Same wrong path retried three times -> repeat protection fires;
  `StopReason="tool_error"`.

**B6**:

- Recorded transcript event carries `stop_reason` and `step_count`.
- Older transcripts without these fields still load.

## Recommendation

Proceed with **B2 immediately** -- it is purely additive on a frozen
contract, mechanical, and gives every downstream consumer (sessionlog,
CLI, eventual TUI) the signal they need.

Hold **B3** until B2 is in HEAD so the test assertions for B3 can
naturally also check that `StopReason="final"` and `StepCount` is
sane on the resolve-read-final happy path.

Hold **B5** until **B4** has run and shown whether B3 alone closes
the inspect-file flake. If B4 shows the model wastes steps despite
the prompt, B5 becomes worthwhile; otherwise it is YAGNI.
