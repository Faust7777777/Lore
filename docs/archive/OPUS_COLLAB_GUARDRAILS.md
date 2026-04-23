# Opus Collaboration Guardrails

Updated: 2026-04-23

## Purpose

This note is the current handoff boundary for Opus review/collaboration on `Lore`.
Use it to avoid changing the wrong layer or "helpfully" refactoring something that is not actually in scope.

## Product Identity

- Product name: `Lore`
- Repo: `C:\Users\15892\Desktop\obsidian-harness`
- Current pushed HEAD: `1c5fe78` (`feat: add git inspection tools to agent loop`)

## Hard Boundaries

1. Do not change product positioning.
   - Lore is not a generic agent platform.
   - Lore is a knowledge-ops harness centered on managed Obsidian governance plus external agent process capture.

2. Do not weaken governance.
   - Managed core docs / plan-execution docs stay on `draft -> review -> apply`.
   - MCP remains read-only.
   - Local file/shell/coding powers belong to the main local agent profile, not MCP.

3. Do not "improve" JSONL attach as if it were the final live path.
   - JSONL import/sync/attach is the fallback path.
   - The real next adapter direction is `Codex app-server source`, not more JSONL-specific polishing.

4. Do not freehand the UI.
   - Current TUI structure work is acceptable as an experiment.
   - Visual / layout direction should be reviewed first.
   - If proposing UI changes, critique or sketch direction first instead of directly rewriting files.

5. Do not edit planning/alignment docs unless explicitly requested.
   - Especially avoid changing the main alignment doc just because an implementation idea feels cleaner.

## Trusted Baseline

These are already in the main branch and should be treated as the stable base:

- Natural-language main agent loop exists.
- `vault_write_low` exists for low-governance note writing.
- Local work tools are gated behind `--local-exec`.
- `shell_exec` additionally requires `LORE_AGENT_ENABLE_SHELL=1`.
- Bounded git inspection tools exist in the agent loop:
  - `git_status`
  - `git_diff_summary`
- Interactive TUI error rendering improvements are already pushed.

## Current Local WIP (Not Yet Committed)

These local edits exist right now and are not yet trusted as final:

1. `internal/tui/workbench_view.go`
2. `internal/tui/workbench_view_test.go`

What they are:

- A workbench information-architecture experiment:
  - top mode banner
  - `Git` status in runtime snapshot
  - step-indexed tool trace
  - `Quick Actions` block

Important:

- Do not assume this TUI direction is approved.
- Do not build major UI refactors on top of it yet.
- Review the effect first; critique is welcome, blind rewriting is not.

There is also a local exploratory package path:

3. `internal/adapter/codexappserver/`

Important:

- Treat it as protocol groundwork only until verified.
- Do not assume it is already integrated into daemon/CLI.
- If reviewing it, review against the official Codex app-server protocol, not against JSONL attach assumptions.

## Active Priorities

Priority order for useful review / collaboration:

1. `Codex app-server` live-source direction
   - Desired next move: introduce a real app-server-backed source layer.
   - Avoid spending review budget on more JSONL fallback hardening unless it blocks correctness.

2. Approval / confirmation layer for local actions
   - Current local tools have intent gating, but no explicit pending action / confirm queue like Codex/OpenCode.
   - This is a real UX/control gap.

3. TUI redesign direction
   - Focus on interaction model and panel hierarchy first:
     - visible conversation lane
     - visible context working set
     - visible pending approvals / action queue
   - Do not optimize cosmetics before these are frozen.

## Integration Contracts (Frozen For Opus Work)

These answers are the current integration contract. Do not invent a different one locally.

### 1. Agent Loop <-> TUI integration

- `operatoragent.ModelAgent.Respond()` remains a synchronous blocking call for now.
- TUI must own the async bridge:
  - start the request in a goroutine
  - send progress/completion back into the UI event loop via channel / message dispatch
- Do not block the UI update loop waiting on `Respond()`.
- Current reality:
  - Lore does **not** yet expose true streaming tool-progress events
  - TUI can show `idle -> running -> completed/error`
  - `Tool Trace` is only guaranteed after the turn completes
- If real-time step streaming is added later, it will be additive via a new interface or callback path.
- Do not redesign `Respond()` into a streaming API inside the TUI branch unless explicitly requested.

### 2. Data type stability

For the current TUI branch, treat these structs as **frozen, additive-only** contracts:

- `model.ManagedStatusView`
- `model.Draft`
- `app.DraftReview`
- `app.ProcessSinkDayView`
- `operatoragent.Context`
- `operatoragent.ToolCallTrace`
- `operatoragent.Response`

Meaning:

- existing field names and semantics should be treated as stable
- adding new fields is acceptable
- renaming/removing/reinterpreting existing fields is not acceptable without explicit coordination

So Opus can safely render against the current fields without expecting churn under it.

### 3. TUI framework decision

- Target framework for the redesigned interactive TUI: `Bubble Tea`
- Current `strings.Builder` text dashboard remains:
  - fallback CLI/TUI rendering path
  - useful content/reference source
  - not the long-term interaction layer
- Opus should design the new interactive workbench assuming a Bubble Tea app shell.
- Opus does **not** need to preserve the current text dashboard layout as the final UX.

### 4. Conversation history ownership

- Conversation history is owned by the session/controller layer, not by `Runtime`
- Today that means `internal/console.Session`
- TUI should own or wrap a session object and pass `operatoragent.Context.History` into the agent loop
- `Runtime` should remain focused on workspace state, vault state, drafts, process-sink state, and policy-governed actions
- Persistent chat-session storage is **not** frozen and is **not** required for the current TUI branch
- For now, assume history is in-memory session state

### 5. Approval / confirmation layer ownership

- Final architecture: confirmation/approval policy belongs to `Runtime / policy layer`, not TUI-only logic
- TUI is responsible for:
  - rendering pending actions
  - collecting approve / reject / revise input
  - sending the decision back through runtime hooks
- TUI should **not** be the only enforcement point for dangerous local actions
- If Opus prototypes the UI before the runtime hook lands, treat that as temporary scaffolding, not final architecture

Practical implication:

- if a pending-action queue / hook does not exist yet, Opus should ask for that contract instead of inventing a TUI-only security model
- visual design may proceed, but the enforcement boundary is runtime-owned
- until the runtime hook exists, it is acceptable for Opus to render a `pending approvals` placeholder / empty-state panel only
- do not invent a TUI-only approval executor in order to make that panel interactive

## What Opus Should Review

- Whether `Codex app-server` client/source design matches the official protocol and the real Lore workflow.
- Whether local action approval should sit in runtime policy vs TUI layer.
- Whether the current workbench is missing the right panels for a Codex/OpenCode-like experience.

## What Opus Should Not Do

- Do not rewrite large UI files just to make them prettier.
- Do not swap storage/runtime architecture unless explicitly asked.
- Do not change MCP into a writable surface.
- Do not collapse Lore-specific tools into a generic shell-first agent.
- Do not assume JSONL attach is the target architecture.

## Best Response Format From Opus

Prefer this structure:

1. Findings
2. Wrong assumptions / boundary violations
3. Specific proposed next step
4. If code changes are suggested, list exact files and why

Avoid:

- broad rewrites
- aesthetic-only TUI edits
- "clean architecture" detours that move the repo away from the current plan
