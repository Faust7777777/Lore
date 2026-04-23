# Lore Repository Instructions

This file is for agents working on the `obsidian-harness` repository itself.

It defines repository collaboration rules, code ownership boundaries, and
verification expectations.

It is not the runtime identity of the Lore agent shown to end users.

## Priority

Repository work should follow this order of precedence:

1. hard runtime and policy constraints implemented in code
2. this `AGENTS.md`
3. task-specific instructions

Markdown instructions never override governance, approval, or policy logic
implemented by runtime code.

## Scope

Lore is a governed knowledge-ops harness.

Primary surfaces today:

- local runtime and daemon
- CLI and TUI shells
- read-only MCP surface
- managed vault governance
- process-sink ingest and rollup
- external agent context and session ingest

Do not steer the project toward a generic agent platform unless explicitly
directed.

## Architecture Rules

- Keep governance, approval, and writeback enforcement in runtime code.
- Keep MCP read-only.
- Keep process-sink writes internal to the harness.
- Managed docs and plan/execution docs stay on `draft -> review -> apply`.
- Low-governance direct writes must still go through runtime checks.
- Reuse existing ingest and import paths when extending sources. Do not build a
  parallel pipeline if a current one can be adapted.

## Language Rules

- Default implementation language is Go.
- Do not add another language just to mimic other harnesses.
- Add TypeScript only when required by web, editor, or plugin surfaces.
- Add Python only when required by model or data workflows that do not fit well
  in Go.

## Collaboration Boundaries

Treat the repository as split into three collaboration layers:

- runtime, policy, and adapter
- TUI shell and state
- TUI presentation

### Runtime, policy, and adapter layer

Typical locations:

- `internal/app/*`
- `internal/adapter/*`
- `internal/orchestrator/*`
- `internal/operatoragent/*`
- `internal/console/*`
- `internal/mcp/*`

Rules:

- presentation work should not edit this layer without explicit alignment
- keep stable data contracts intact
- avoid runtime behavior changes from UI-focused tasks

### TUI shell and state layer

Typical locations:

- `internal/tui/interactive_workbench.go`
- `internal/cli/tui_workbench.go`
- related workbench state and view-model plumbing

Rules:

- this layer owns event loop, focus model, async bridge, and shell behavior
- it must not absorb adapter, policy, or ingest logic
- it should consume stable view models instead of reaching into lower layers

### TUI presentation layer

Typical locations:

- `internal/tui/interactive_render.go`
- `internal/tui/interactive_theme.go`

Rules:

- keep visual and copy changes here whenever possible
- do not change runtime semantics from here
- when parallel work is happening, structure-layer changes must not casually
  rewrite these files

## Frozen Contracts

The following types are treated as cross-layer contracts:

- `model.ManagedStatusView`
- `model.Draft`
- `app.DraftReview`
- `app.ProcessSinkDayView`
- `operatoragent.Context`
- `operatoragent.ToolCallTrace`
- `operatoragent.Response`

If one must change:

1. state why
2. identify consuming layers
3. update tests in the same change

## TUI Rules

- TUI is a client shell, not the system core.
- Keep the agent loop synchronous; the TUI owns the async bridge.
- Do not invent UI-only approval semantics.
- Approval and confirmation must be runtime-backed.
- Prefer view-model driven rendering over implicit string-built state.

## Runtime Identity Docs

Do not confuse repository instructions with runtime agent docs.

Repository-level rules live here.

Runtime agent behavior and identity belong in managed workspace docs such as:

- `agent.md`
- `identity.md`

Those files are workspace content, not repository governance files.

## Git Hygiene

- Never revert unrelated user or collaborator changes.
- Read the worktree before staging broad edits.
- Keep commits scoped to one change chain.
- Do not mix backend/runtime work with presentation-only work unless the feature
  truly requires both.
- Use non-interactive git commands.

## Verification

- Run targeted package tests while iterating.
- Run `go test ./... -count=1` before claiming a cross-cutting change is done,
  when feasible.
- If a change touches prompt loading, governance, bootstrap, ingest, runtime
  contracts, or TUI shell behavior, full-suite verification is expected.

## Real Pitfalls Already Seen

These are not hypothetical:

- rebuilding the whole Bubble Tea shell in parallel with structure work caused
  collision and wasted work
- presentation changes can drift into state-machine ownership if boundaries are
  vague
- adapter identity fields can leak provider semantics into process-sink primary
  keys if not reviewed carefully
- silent data dropping in ingest paths is unacceptable for audit-oriented flows

When in doubt, tighten scope and align first.
