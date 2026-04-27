# Compact Handoff: Lore v1 Architecture Closure

## Read First

Priority documents:

- `docs/adr-lore-v1-architecture.md`
  Current v1 architecture ADR and the main frozen decision record. Latest relevant commit: `9e101d3 docs: add lore v1 architecture adr`.
- `docs/review-handoff-codex53-v1-architecture-adr.md`
  Review handoff for Codex 5.3. It states that the current change is architecture documentation only, not an MCP write-tool implementation.
- `docs/integrations/mcp-client-setup.md`
  External agent MCP setup guide for Claude Desktop, Claude Code, OpenCode, and Gemini CLI.
- `docs/adr-sdk-go-v0.md`
  Go SDK v0 and the MCP read-only baseline. Treat it as the previous phase, not the final v1 boundary.
- `docs/contracts/mcp-sdk-tools-v0.json`
  Current public MCP/SDK contract artifact. It is still the read-only v0 tool contract.

Optional background:

- `docs/archive/AGENT_INTERACTION_MODEL_ALIGNMENT.md`
  Earlier alignment notes for local agent behavior, `vault_write_low`, shell boundaries, and why MCP must not expose shell.

## Code To Inspect

Core MCP surface:

- `internal/mcp/server.go`
  MCP stdio server. It currently handles `initialize`, `ping`, `tools/list`, and `tools/call`.
- `internal/mcp/tool_contract.go`
  Canonical MCP tool contract source. It currently does not include `vault_write_low`.
- `internal/console/tool_runtime.go`
  Local Lore agent tool runtime. `vault_write_low` already exists here, but only for the local console/TUI agent path.

Runtime and governance:

- `internal/orchestrator/harness.go`
  Contains `WriteLowRiskNote()`, `ObserveDocumentChange()`, and `applyDraftPatch()`.
  Important current fact: `applyDraftPatch()` supports `progress_sync`; it does not yet apply `persona_update`.
- `internal/app/readapi_runtime.go`
  Runtime wiring for read APIs and governance-facing operations.
- `internal/app/daemon.go`
  Vault watcher and post-scan related logic. It already has daemon scanning paths that can feed governed runtime handling.

Managed docs and prompts:

- `internal/bootstrap/templates.go`
  Default templates for `agent.md` and `identity.md`. Next semantic cleanup should happen here.
- `internal/bootstrap/templates_test.go`
  Template tests that need updates when `agent.md` becomes the external operating manual.
- `internal/operatoragent/model.go`
  Local Lore prompt construction. It currently preloads `agent.md` and `identity.md`; CoreContext for persona/system/progress is not implemented yet.

## Current Work

The current mainline is Lore v1 architecture closure, not scattered feature development.

Already completed:

- External MCP client setup documentation.
- MCP example configuration tests.
- Raw stdio MCP smoke test.
- v1 architecture ADR draft.
- Codex 5.3 review handoff for the v1 architecture ADR.

Current architecture position:

- `identity.md` is the self identity for the local Lore agent.
- `agent.md` is the external agent operating manual.
- MCP v1 moves from read-only to graded capability:
  - L0 read.
  - L1 proposal intake.
  - L2 low-risk direct write.
  - L3 governed apply.
- MCP v1 may later expose `vault_write_low`, but must not expose shell, generic workspace file write/edit, or governed apply.
- Managed core documents, persona, plans, and execution documents still require `draft -> review -> apply`.
- If an external agent bypasses Lore through shell/file writes, Lore handles that through daemon/post-scan discovery, classification, audit, conflict, or draft paths.

Important correction:

- `vault_write_low` is L2 direct low-risk write. Its output should be shaped like `status=written`, `path`, `doc_class`, and `base_version`.
- `persona_update_propose` is L1 proposal intake. Its output should be shaped like `status=draft_created`, `draft_id`, `target`, and `review_required`.
- Do not mix these two result contracts.

## Recommended Next Steps

1. Tighten the v1 ADR or meeting notes if any text still mixes `vault_write_low` and `persona_update_propose` result shapes.
2. Update the managed `agent.md` template so it becomes the external agent operating manual.
3. Keep `identity.md` as Lore self identity and avoid making external agents default-read it.
4. Update the external MCP docs so external agents call `system_doc_get("agent")` first.
5. Implement MCP `vault_write_low` by reusing `WriteLowRiskNote()` only after contract and rejection tests are ready.
6. Implement local CoreContext: inject short persona/system/progress summaries into the local Lore agent prompt.
7. Implement `persona_update_propose` as the first narrow L1 proposal tool, then add persona draft/apply support.
8. Expand daemon/post-scan governance findings for out-of-band shell/file writes.

## Roles

- User: final product decision owner. Decides semantics and boundaries.
- Codex: main implementation and architecture owner. Turns decisions into ADRs, code, tests, and handoff documents.
- Codex 5.3: current review owner. Focus areas are boundaries, missing tests, contract drift, and whether implementation matches the ADR.
- Opus: TUI owner. TUI changes are separate and should not be mixed into MCP/CoreContext/ADR commits.
- DeepSeek: previous backend and stable-layer reviewer. Useful as a supplemental reviewer, but not the current primary review owner.

## Worktree Warning

At the time of this handoff, there are unrelated uncommitted TUI changes:

- `internal/tui/interactive_render.go`
- `internal/tui/interactive_render_test.go`

Do not touch, stage, or commit these files as part of MCP/CoreContext/v1 architecture work unless the user explicitly asks.

Preferred verification command:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```

Optional E2E verification:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1 -E2E"
```
