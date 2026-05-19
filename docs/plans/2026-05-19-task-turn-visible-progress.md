# Plan: Task/Turn Visible Progress

Created: 2026-05-19
Scope owners: A line, TUI line
Backend dependency: B-line Task/Turn metadata and tool observation excerpts

## Goal

Make multi-step user tasks visible without exposing hidden chain-of-thought.

For a request such as "open the persona/background file and write an essay",
Lore should not render only the final essay. The UI and tests should make the
auditable task shape visible:

1. user request;
2. resolve/search step;
3. observation showing the selected file or candidates;
4. read step;
5. observation showing a bounded file excerpt or summary;
6. final answer.

This is visible progress and tool observation, not raw reasoning trace.

## Non-Goals

- Do not expose hidden chain-of-thought.
- Do not restore the old generic `Latest Output` block.
- Do not show full large files by default; long observations must be truncated.
- Do not require live streaming in the first slice.
- Do not change the external MCP live contract in this track.

## A1 Acceptance Scenario

Deterministic user-visible scenario:

```text
User: 打开人物背景，基于它写一篇散文。
Expected visible output:
1. User request
2. Step 1: vault_resolve 人物背景
3. Observation: selected 03-画像/人物背景.md
4. Step 2: vault_read 03-画像/人物背景.md
5. Observation: bounded excerpt from the file
6. Step 3: final
7. 最终散文
```

Acceptance rules:

- Do not require hidden chain-of-thought.
- Do not require full file rendering; long content must be truncated.
- Do not require a real model in PR gate.
- Do require a visible `vault_resolve` step.
- Do require a visible `vault_read` step.
- Do require a visible observation excerpt or summary.
- Do require the final answer to remain visible.

## Backend Dependency

TUI cannot reliably render file content observations from the existing
`ToolCallTrace` alone because it currently carries tool name, arguments,
status, and error, but not a safe result excerpt.

B line should provide a structured post-turn representation before TUI takes
the full rendering dependency. A minimal shape is:

```go
type TurnStep struct {
    Index              int
    Tool               string
    Arguments          map[string]any
    Status             string
    ObservationExcerpt string
    Error              string
}
```

The exact type name can change, but the contract must support:

- ordered tool steps;
- ok/error/pending status;
- bounded observation excerpt;
- error text;
- enough arguments to show paths, queries, and selected files.

## A-Line Schedule

A owns acceptance coverage and release-gate wiring only. A must not implement
TUI rendering or operator-agent loop behavior.

### A1: Acceptance Scenario Definition

Document the deterministic scenario:

```text
User: 打开人物背景，基于它写一篇散文。

Expected visible output:
1. Step: vault_resolve / search for 人物背景
2. Observation: selected 人物背景.md or candidate paths
3. Step: vault_read 人物背景.md
4. Observation: bounded excerpt from 人物背景.md
5. Final: essay based on the file content
```

Acceptance rules:

- The user sends one request only.
- The output must include visible resolve/read steps.
- The output must include either a file excerpt or an observation summary.
- The output must include the final generated essay.
- It is a failure if only the final essay is rendered.
- The test must not assert hidden chain-of-thought.

### A2: Deterministic E2E

Status: implemented and wired into the release gate.

- `cmd/obsidian-harness/main_test.go`
- `TestRunTUIOnceShowsResolveReadFinalTaskVisibility`
- The test is active. It uses the fake model provider and does not require a
  real model in PR gate.

The fake-model e2e drives a fixed sequence:

```text
vault_resolve -> vault_read -> final
```

Assertions:

- one user turn completes the whole task;
- visible output includes `vault_resolve`;
- visible output includes `vault_read`;
- visible output includes a bounded observation from the target file;
- visible output includes the final answer;
- non-error `lastOutput` is still not rendered as the old `Latest Output`.

Placement:

- command-level test under `cmd/obsidian-harness`;
- no real model dependency in PR gate.

Completed activation checklist:

- removed the `t.Skip(...)` from
  `TestRunTUIOnceShowsResolveReadFinalTaskVisibility`;
- updated expected labels to `Task Steps` instead of the
  legacy `Tool Trace` label;
- asserted that output includes a bounded observation excerpt, not just tool
  names and arguments;
- ran the test without a real model provider;
- wired the test into `scripts/release-gate.ps1` PR gate.

### A3: Release-Gate Integration

Status: implemented for the deterministic fake-model scenario.

- PR gate runs the deterministic fake-model scenario.
- Full gate may run a real-model smoke only on `push main` or
  `workflow_dispatch`, using existing model secrets.
- Failure messages should identify whether resolve, read, observation render,
  or final render is missing.

Allowed files:

- `scripts/release-gate.ps1`
- `cmd/obsidian-harness/*_test.go`
- `internal/cli/*_test.go`
- `docs/plans/*`

Forbidden in A line:

- `internal/tui/*` implementation files;
- `internal/operatoragent/*` behavior changes;
- MCP contract files.

## TUI-Line Schedule

TUI owns presentation. TUI should consume backend turn steps and observations;
it should not change agent loop semantics.

### T1: Post-Turn Task Timeline

Render a task timeline after a turn completes. First version is post-turn only,
not live streaming.

Example:

```text
Task Steps
  1. vault_resolve  ok
     query=人物背景
     selected=03-画像/人物画像.md

  2. vault_read     ok
     path=03-画像/人物画像.md
     observation: 用户背景包括...

Final
  散文正文...
```

Rules:

- Render in the conversation lane or a dedicated task timeline area.
- Keep the existing focus model stable.
- Do not reintroduce a generic `Latest Output` section.
- Keep error-only `lastOutput` rendering behavior.

### T2: Observation Excerpts

Render bounded observations for read/search/resolve steps.

Rules:

- Large observations are truncated with an explicit marker.
- Binary/media content is not displayed.
- Tool errors are visible and styled as errors.
- Arguments should be compact: query, path, selected path, candidate count.
- The final answer remains visually separate from tool observations.

### T3: Empty, Error, and Ambiguous States

Cover non-happy paths:

- `vault_resolve` returns multiple candidates: show candidates and final
  disambiguation request.
- `vault_resolve` returns not found: show not-found observation.
- `vault_read` returns error: show failed step and error text.
- Max-step/model-error stop reason should be visible when backend exposes it.

### T4: Optional Interaction

Only after static rendering lands:

- `Enter` may expand/collapse a selected step.
- `d` may show full step detail.
- `j/k` scrolling should keep working in conversation focus.

Do not make this a blocker for the first visible-progress slice.

### T5: TUI Tests

Add render/state tests:

- multi-step turn renders ordered steps;
- `vault_read` observation excerpt is displayed and truncated;
- tool error step is visible;
- final answer still renders;
- old `Latest Output` block does not return;
- `Error:` lastOutput still renders as compact status.

Allowed files:

- `internal/tui/workbench_model.go`
- `internal/tui/workbench_view.go`
- `internal/tui/interactive_render.go`
- `internal/cli/tui_workbench.go`
- corresponding tests

Forbidden in TUI line:

- operator-agent loop changes;
- store/runtime accounting changes;
- CI/release-gate wiring.

## Recommended Order

1. B line lands turn-step / observation excerpt backend data. Done.
2. TUI line renders post-turn task timeline from that data. Done.
3. A line activates the existing deterministic fake-model e2e scaffold and
   wires it into the PR release gate. Done.
4. Optional later work: live streaming progress events.

Do not start with live streaming. Post-turn visibility solves the immediate
problem that only the final output is visible.
