# Findings & Decisions

Archived for V1 RC review on 2026-06-14.

Source: root-level `findings.md` scratch notes from the V1 RC stabilization
session. Treat this as analysis evidence and background context. The current
release-candidate status is `docs/v1-rc-status-2026-06-14.md`.

## Requirements
- Read all relevant Lore integration/onboarding/handoff/architecture documentation.
- Build a detailed product understanding in Chinese.
- Be careful and comprehensive.

## Research Findings
- Repository instruction (`AGENTS.md`) defines Lore as a governed knowledge-ops harness, not a generic agent platform.
- Primary current surfaces from `AGENTS.md`: local runtime/daemon, CLI/TUI shells, MCP intake surface, managed vault governance, process-sink ingest/rollup, external agent context/session ingest.
- Repository architecture rules: MCP remains read + proposal-only; no direct write/apply/shell. Process-sink writes stay internal. Managed docs and plan/execution docs follow `draft -> review -> apply`.
- Collaboration layers: runtime/policy/adapter, TUI shell/state, and TUI presentation. This matters because product responsibilities are intentionally separated.
- Documentation inventory found 183 files under `docs/`.
- Formal/current-looking doc clusters:
  - `README.md`, `DEVELOPMENT_STATUS.md`, `AGENTS.md`
  - `docs/adr-lore-v1-architecture.md`, `docs/adr-sdk-go-v0.md`
  - `docs/integrations/*`, `docs/contracts/*`
  - `docs/persona-memory-*`, `docs/review-*`, `docs/handoff-*`, `docs/plans/*`
  - `docs/archive/review-handoff/*` as historical implementation and review context.
- `README.md` current product framing: Lore is a local-first, governed Obsidian knowledge-operations agent. The primary product surface is the TUI; CLI is automation/diagnostics/model-callable surface; MCP is external-agent intake, not a write proxy.
- Governance invariant: important vault changes are proposed first, reviewed locally, and only then applied. External agents can submit proposals but only local Lore/runtime can approve/apply.
- Managed workspace has five protected core documents: system doc, progress index, persona doc, `agent.md`, and `identity.md`.
- `agent.md` is external-first shared operating manual; `identity.md` is Lore's local self identity and should not be imposed on external agents.
- Persona pipeline: user chat -> async extraction -> evidence validation -> candidate store/dedup -> operator review -> persona_update draft -> local approve/apply. No automatic persona mutation.
- Process sink imports Codex/external transcripts into checkpoints and daily reports, but does not automatically create durable notes or persona updates.
- Usage/cost attribution is by day, purpose, and model; purposes include `chat`, `persona_extract`, and `process_sink`.
- `DEVELOPMENT_STATUS.md` says implemented foundations include runtime broker/health/audit, vault daemon, SQLite store with JSON migration, Codex adapters, local operator loop, TUI, CLI, MCP, persona candidates, usage summaries, and audit-followup hardening.
- Current gaps in status docs: no long-running app-server attach loop; TUI approval queue placeholder; binary/media extraction not implemented; richer TUI usage panels not implemented; external transcript import not connected to persona extraction.
- `docs/adr-lore-v1-architecture.md` freezes the v1 direction: MCP L0 read + L1 proposal intake only. L2 internal writes and L3 governed apply are not MCP-exposed.
- MCP forbidden surface includes shell, generic file write/edit, direct markdown vault write, managed core write, process-sink write, runtime policy mutation, and draft approve/apply.
- ADR explicitly keeps daemon/post-scan as fallback reconciliation for out-of-band writes done by tools outside Lore.
- `docs/compact-handoff-lore-v1.md` is the clearest current alignment memo: external MCP must be read + proposal-only; `vault_write_low` may exist locally but is not external MCP plan.
- Review handoff index lists 116 archived handoffs. It should be used as implementation history, not as a stronger source than README/ADR/current compact handoff.
- `docs/integrations/mcp-client-setup.md` is the main external integration guide. It supports stdio only: `lore mcp [workdir]`.
- MCP protocol scope: `initialize`, `ping`, `tools/list`, `tools/call`. No MCP resources or prompts yet.
- MCP auth: optional process-level auth via server-side `LORE_MCP_API_KEY`/`OBSIDIAN_HARNESS_MCP_API_KEY` and client-side `LORE_CLIENT_KEY`/`OBSIDIAN_HARNESS_CLIENT_KEY`.
- External client configs exist for generic MCP, Claude Desktop, Claude Code project `.mcp.json`, OpenCode, and Gemini CLI. All launch `lore.exe mcp <workdir>` with optional `LORE_CLIENT_KEY`.
- External onboarding sequence: connect, call `system_doc_get` with `name=agent`, then use persona/system/progress/context_pack/read/search/resolve as task-relevant. Do not default-read `identity`.
- External persona update handoff uses `persona_update_propose` or a text fallback `Persona Update Candidate`; both create/describe pending review only and do not write `人物画像.md`.
- External markdown handoff uses `markdown_note_propose` or a text fallback `Markdown Note Candidate`; it creates a pending markdown-note draft only and cannot apply/write.
- External transcript import is CLI/process-sink, not MCP. Schema is NDJSON with optional `session_meta` and message rows. It creates checkpoints/daily reports, not formal notes/persona updates.
- Live MCP v1 contract (`docs/contracts/mcp-tools-v1.json`) exposes 11 tools: 9 read tools plus `persona_update_propose` and `markdown_note_propose`.
- SDK v0 contract (`docs/contracts/mcp-sdk-tools-v0.json`) exposes only the 9 read tools: `managed_status`, `system_doc_get`, `vault_read`, `vault_list`, `vault_search_text`, `vault_resolve`, `vault_backlinks`, `doc_classify`, `context_pack`.
- Tool argument standardization: `vault_list`, `vault_search_text`, `vault_resolve` prefer `dir`; `context_pack` prefers `target_path`; legacy `path` aliases remain accepted for compatibility.
- `docs/adr-sdk-go-v0.md` and `sdk/go/lore/README.md`: Go SDK is stdio JSON-RPC client for `lore mcp`; it must not import `internal/*`; typed API remains read-only. Raw `CallTool` is the forward-compatible escape hatch and may call proposal tools.
- SDK error taxonomy: transport/frame, context cancel/deadline, JSON-RPC, tool business error, decode error.
- `docs/plans/2026-04-28-external-agent-governed-intake.md` is the most detailed end-to-end integration plan. It defines the target external-agent flow as: external agent reads context -> submits narrow proposal -> Lore creates pending draft -> local Lore reviews/supersedes with CoreContext -> local/user approves -> Lore applies to vault.
- `markdown_note_propose` MVP decisions: `target_path` required; `source_kind` enum is `class|meeting|development|conversation|research|other`; MCP contract omits `tags` and `related_paths`; proposal creation must not write files; apply writes full reviewed markdown content with trailing newline and no frontmatter.
- Markdown note draft safety: proposals reject managed core, persona, progress, agent, identity, plans, process-sink, hidden paths, path escapes, drive paths, non-md files, and oversized fields. New-file drafts use `DraftBaseVersionNewFile = "new"`.
- Supersede is local-only. It creates a revised pending draft, marks the original `superseded`, preserves the original external proposal as immutable evidence, and must not be exposed through MCP.
- CoreContext is implemented for local review/supersede prompts. It includes deterministic excerpts from persona, persona `## Weaknesses`, system, progress, and pending/approved drafts. `identity.md` is excluded. Vault content in CoreContext is injected as contextual user-role evidence, not privileged instructions.
- `docs/handoff-next-developer-2026-04-29.md` says implemented v1 capabilities now include `markdown_note_propose`, local markdown-note apply, local draft supersede, CoreContext MVP, governed-note smoke, post-scan findings, local findings CLI, and one-shot external transcript import.
- Daemon post-scan is compensation, not prevention. After baseline, ordinary-note out-of-band changes create out-of-band audit + open findings; managed core and process-sink changes create governance review-needed findings; plan docs keep progress-sync draft behavior.
- Local findings CLI exists: `lore findings list|resolve|ignore`. Findings are local runtime governance state; not MCP tools. Resolve/ignore append `AuditFindingStateChange`.
- External transcript import accepts Lore's own NDJSON schema via `lore import-external-jsonl`; it normalizes `agent`/`model` roles to assistant, rejects unsupported roles, preserves first-bound session identity, writes process-sink checkpoints/reports, and does not create notes/persona/proposals.
- Client config examples are all stdio/local process configs for `lore.exe mcp <workdir>`: Generic MCP, Claude Desktop, Claude Code project `.mcp.json`, OpenCode, and Gemini CLI. They optionally pass `LORE_CLIENT_KEY`.
- Corrected MCP contract inventory: live `docs/contracts/mcp-tools-v1.json` is a root array with 11 names; SDK `docs/contracts/mcp-sdk-tools-v0.json` is a root array with 9 read-only names. The initial `{tools:[...]}` jq shape assumption was wrong.
- Local console tool surface is intentionally broader than external MCP. It can include `vault_write_low`, `draft_approve`, `draft_apply`, `draft_supersede`, shell/workspace/git tools depending on local runtime/profile, while MCP remains read + proposal intake.
- Session/resume semantics: transcripts live under `<workdir>/state/sessions/`, new sessions do not load history by default, `--resume`/`--resume-id` opt into history, old tool calls are not replayed, and transcripts are not cross-session memory.
- Task/turn visible-progress line: operatoragent now treats one user task as multiple bounded tool steps, exposes stop reason/step count/step observations, and TUI renders post-turn progress without exposing hidden chain-of-thought.
- Persona memory backend is complete enough for CLI/operator use: async extraction, candidate lifecycle, draft promotion, recovery/retry, errors reader, summary dashboard, usage purpose bucket. TUI persona panel has backend DTO/action helpers but rendering is a separate TUI-owned line.
- Usage/cost backend now breaks usage down by purpose and provider/model, with JSON carrying uncapped `purpose_breakdown.<purpose>.by_model` and human output adding top-5 model details, token share, hidden-tail token sum, and cross-purpose model rollup.
- Config enforcement review fixed `usage.track_usage`, `usage.soft_warning_tokens`, and `process_sink.write_empty_slots`; `process_sink.retention_days`, `daily_rollup_at`, and most `runtime.*` config knobs remain dead/deferred or owner-boundary items.
- Failure semantics invariant: read/status/aggregation paths are best-effort; mutating paths fail hard and leave a visible governance record when external side effects already happened; audit failures degrade health rather than silently disappearing.
- Product comparison research frames Lore's differentiation from Codex/OpenCode/Gemini: those are coding harnesses centered on sessions/files/git; Lore is a governed vault harness centered on durable knowledge, drafts, audit, base_version, and post-scan findings.
- High-ROI future engineering directions from research: unified ToolRegistry, config layering, PendingActionQueue, provider abstraction, turn/session split, and finer event stream. Red lines: no auto-memory rewrite, no generic agent marketplace/platform drift, no external MCP shell/direct writes.

## Technical Decisions
| Decision | Rationale |
|----------|-----------|
| Use current repo docs plus archived handoffs | Product state is split between stable docs and handoff history. |

## Issues Encountered
| Issue | Resolution |
|-------|------------|

## Resources
- Repository: `/mnt/c/Users/15892/Desktop/obsidian-harness`
- Repository instructions: `AGENTS.md`
- Docs inventory root: `docs/`
- Product overview: `README.md`
- Status: `DEVELOPMENT_STATUS.md`
- Architecture ADR: `docs/adr-lore-v1-architecture.md`
- Current governance handoff: `docs/compact-handoff-lore-v1.md`
- Handoff index: `docs/review-handoff-index.md`
- External MCP setup: `docs/integrations/mcp-client-setup.md`
- MCP live contract: `docs/contracts/mcp-tools-v1.json`
- SDK read-only contract: `docs/contracts/mcp-sdk-tools-v0.json`
- SDK ADR: `docs/adr-sdk-go-v0.md`
- SDK README: `sdk/go/lore/README.md`
- SDK changelog: `sdk/go/lore/CHANGELOG.md`
- External governed intake plan: `docs/plans/2026-04-28-external-agent-governed-intake.md`
- Next developer handoff: `docs/handoff-next-developer-2026-04-29.md`
- Reviewer compact handoff: `docs/compact-handoff-reviewer.md`
- External transcript import handoff: `docs/archive/review-handoff/review-handoff-codex53-external-transcript-import.md`
- Markdown note proposal/apply/supersede handoffs under `docs/archive/review-handoff/`
- CoreContext/post-scan/findings handoffs under `docs/archive/review-handoff/`
- Persona and TUI manual docs: `docs/persona-memory-pipeline.md`, `docs/persona-memory-manual-test.md`, `docs/tui-manual-test.md`
- Usage/TUI backend docs: `docs/handoff-b-line-usage-by-model-2026-05-28.md`, `docs/handoff-b-line-tui-backend-2026-05-28.md`
- Review/audit docs: `docs/review-v1-triage-2026-06-04.md`, `docs/review-b-line-config-enforcement-2026-06-04.md`, `docs/review-b-line-failure-semantics-2026-06-04.md`, `docs/handoff-cross-boundary-review-v1-2026-06-05.md`

## Visual/Browser Findings
- Not applicable; no visual/browser artifacts used yet.
