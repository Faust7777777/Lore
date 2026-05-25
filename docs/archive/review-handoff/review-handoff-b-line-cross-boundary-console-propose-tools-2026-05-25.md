# Handoff: B-line cross-boundary — console propose tools — 2026-05-25

Audience: A-line operatoragent reviewer + next B-line contributor.

Trigger: live operator chat session showed `lore` in-app TUI agent
unable to act on "走审核流程啊" / "那你提交draft啊" — it had `draft_list`,
`draft_review`, `draft_approve`, but no draft-creation tool. Operator
verified the gap is real: console agent has `vault_write_low` but cannot
create review drafts for managed-core docs (03-画像/人物画像.md) or
low-governance notes via the review flow.

Cross-boundary protocol followed (per memory rule):
1. Authorization received from operator: "你能修就给它修上"
2. Minimal-surface change: 6 files touched, all in B-line scope
   (`internal/console/`, `internal/app/readapi_runtime.go`,
   `internal/cli/tui_workbench_test.go` test stub only)
3. This handoff written
4. Commit message tagged `cross-boundary (B-line)`

## TL;DR

| What | Detail |
|---|---|
| Commit | `9eefb6f` |
| Touched | 6 files / 226+/13- |
| Tests | console + cli + app + tools + mcp + orchestrator all green |
| Surface change | Console agent `DescribeTools` now lists 20 tools (was 18); two added are `persona_update_propose` and `markdown_note_propose` |

## Root cause

`internal/console/tool_runtime.go::DescribeTools` ships a hardcoded list
of 18 tool definitions. `internal/tools/builtin_proposal.go:20` declares
the proposal tools with `SurfaceMCP|SurfaceConsole|SurfaceProposal` —
i.e. they were always *meant* to appear on the console surface. The MCP
server reaches them via `Registry.ListBySurface(SurfaceMCP)` at
`internal/mcp/server.go:174`. The console agent never queried the
registry, so the SurfaceConsole flag had no effect on the console path.

Net effect: an end user chatting with the in-app `lore` agent could ask
"propose a draft for 人物画像.md" and the agent had no callable tool to
do it. The operator transcript showed the agent guessing through
`draft_list` / `draft_review` / `draft_approve` and eventually
explaining it "had no draft-creation tool" — accurate observation,
broken contract.

## What changed

1. **`internal/console/session.go`** — added one method to the
   `Runtime` interface:
   ```go
   ProposalTools() []tools.Tool
   ```
   Narrow seam; intentionally exposes the registered Tool objects rather
   than re-implementing field parsing in console.

2. **`internal/app/readapi_runtime.go`** — implemented `ProposalTools()`
   on `*app.Runtime`:
   ```go
   func (r *Runtime) ProposalTools() []tools.Tool {
       registry := tools.NewRegistry()
       if err := tools.RegisterProposal(registry, r.Harness); err != nil {
           panic(err)
       }
       return registry.ListBySurface(tools.SurfaceConsole)
   }
   ```
   Same `tools.RegisterProposal` MCP uses. Constructed per call (cheap:
   2 registrations); no shared mutable state, no init-order coupling.

3. **`internal/console/tool_runtime.go`**:
   - `DescribeTools` appends entries for every Tool in
     `r.runtime.ProposalTools()`, with a JSON-skeleton hint synthesized
     from the Tool's argument list (`renderProposalToolArgsHint`).
   - `CallTool` default branch first scans `ProposalTools()` for a name
     match before falling through to `unknown tool`. Match dispatches
     via `tool.Call(args)` — same code path MCP uses, so field
     validation, schema enforcement, and harness call all happen in
     `internal/tools`.

4. **`internal/console/session_test.go`** — fakeRuntime gains
   `ProposalTools()` that builds a registry against a small
   `fakeProposalHarness` exposing the two `Propose*` hooks plus the
   read-only methods `tools.Harness` requires (all return zero values).

5. **`internal/console/tool_runtime_test.go`** — new
   `TestToolRuntimeProposalToolsExposeAndDispatch` asserts:
   - `DescribeTools` contains both tool names
   - `markdown_note_propose` dispatch reaches the harness with
     `target_path` and `source_kind` preserved
   - `persona_update_propose` dispatch reaches the harness with `field`
     and `confidence` preserved

6. **`internal/cli/tui_workbench_test.go`** — workbench stub gains the
   one-line `ProposalTools()` returning nil (workbench test doesn't
   exercise tools).

## A-line review surface

Two things A-line owners should sanity-check (these are why this is
flagged cross-boundary, not a pure B-line edit):

**1. Operator agent prompt assumptions.** The console agent's prompt
template (in `internal/operatoragent/`) describes the available tools to
the LLM. With `DescribeTools` now returning 20 tools instead of 18, the
prompt the LLM sees on every turn now mentions `persona_update_propose`
and `markdown_note_propose`. If the prompt currently says something like
"you have 18 tools" or enumerates a fixed set, that text needs an
update. I did not edit the prompt — that is operatoragent's surface.

Quick check: grep `internal/operatoragent/` for any tool-count literal,
hardcoded tool name lists, or "available tools" enumeration.

**2. Audit / governance link.** The proposal tools call into
`Harness.ProposeMarkdownNote` and `Harness.ProposePersonaUpdate`, which
already produce drafts in `pending_review` and write to the audit log.
Same call path MCP uses, so the audit fields should be identical. Worth
spot-checking on first live exercise: drive the console agent through a
proposal turn and confirm the resulting draft has the same
`source`/`source_kind`/`evidence` audit columns as a draft created via
MCP.

## Boundary statement

Did NOT touch:
- A-line vault symlink S-1.
- A-line runtimeDocs S-2 isolation.
- A-line sessionlog S-3 lock-order documentation.
- TUI Q-8 context-background audit.
- A-line `internal/operatoragent/` (prompt is its surface — flagged
  above for review, not edited).
- SDK / MCP server (`internal/mcp/server.go` unchanged).

## Why three propose tools, not unified

For future readers wondering why we don't just collapse
`vault_write_low` / `markdown_note_propose` / `persona_update_propose`
into one tool:

| Tool | Trust | Path | Schema |
|---|---|---|---|
| `vault_write_low` | direct write | low-governance only (validateLowGovernanceMarkdownTarget rejects all ManagedCore) | path / content / overwrite |
| `markdown_note_propose` | review-gated | low-governance notes via draft → approve | target_path / source_kind / content / evidence / reason / source / observed_at |
| `persona_update_propose` | review-gated | managed core docs (画像 etc.) | field / proposed_value / confidence / evidence / reason / source / observed_at |

Three governance quadrants, three schemas. Unifying would require
either dropping the schema asymmetry (worse than current) or adding a
discriminator field (same as current, just spelled `kind` instead of
tool name) — no actual surface reduction, harder to validate, and the
LLM's prompt would still need to know which kind to pick. Current shape
is the cheaper of the two.

## Push readiness

Working tree still carries A-line edits (S-1/S-2/S-3 plus README/
release-gate/operatoragent touches and four codex53 a-line handoff
docs) — those are NOT part of this commit. Pathspec was used so the
slice is reviewable in isolation.

## Reading order for a new contributor

1. This document.
2. The bug demo: operator's lore-chat transcript (in conversation log,
   not committed) showing the agent unable to create a draft.
3. `internal/tools/builtin_proposal.go:20` — the surface declaration
   that was being ignored on the console side.
4. `internal/console/tool_runtime.go` — DescribeTools + CallTool
   changes.
5. `internal/console/tool_runtime_test.go` — the new test, which is
   also the live spec.

## Follow-ups (not in scope here)

- A-line: confirm operatoragent prompt enumeration matches the new
  tool set.
- A-line: live exercise audit-field parity vs MCP-created drafts.
- B-line: when the next FINAL slice touches console, consider whether
  any of the remaining hardcoded tool definitions in `DescribeTools`
  could similarly be sourced from the registry.
