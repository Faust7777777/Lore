# Compact Handoff: Lore External Agent Governance

## Current Product Direction

The main product decision has changed:

Lore should not be an external agent file-write proxy. Lore should be the governance and review layer for knowledge-base writes.

External agents can still write files directly with their own shell/file tools. Lore cannot prevent that. Those writes are out-of-band and can only be discovered later through daemon/post-scan governance.

When an external agent writes through Lore, the intended path is:

```text
external agent output
-> Lore proposal intake
-> Lore review/refine with core context
-> local/user approve
-> Lore apply
-> vault
```

This means external MCP v1 should be `read + proposal intake`, not `read + low-risk direct write`.

## Important Boundary Decision

Treat this as the newest alignment:

- External MCP should expose L0 read tools and L1 proposal-intake tools.
- External MCP should not expose `vault_write_low` as a primary or planned direct-write path.
- `vault_write_low` may remain a local Lore/runtime/internal capability.
- External MCP must not expose shell, generic workspace write/edit, draft approve/apply, or governed direct writes.
- Any `.md` write through Lore should be proposed first and applied by Lore only after review.

The older ADR text still mentions future MCP L2 `vault_write_low`. That is now stale and should be corrected before implementing note writing.

## Read First

Priority documents:

- `docs/adr-lore-v1-architecture.md`
  Current architecture ADR, but it is partially stale. It still mentions future MCP `vault_write_low`; update this to external proposal-only before continuing.
- `docs/integrations/mcp-client-setup.md`
  External MCP client setup. This is mostly aligned because it says there are no direct vault-write MCP tools.
- `docs/review-handoff-codex53-persona-proposal-p1.md`
  Handoff for `persona_update_propose`.
- `docs/review-handoff-codex53-persona-apply-p2.md`
  Handoff for local reviewed persona update apply.
- `docs/contracts/mcp-sdk-tools-v0.json`
  SDK-facing read-only contract artifact. It intentionally remains read-only.
- `sdk/go/lore/README.md`
  Go SDK v0 docs. Typed methods are read-only; raw `CallTool` can call live MCP tools, including proposal intake.

Optional background:

- `docs/archive/AGENT_INTERACTION_MODEL_ALIGNMENT.md`
  Historical local-agent write model. Useful context, but do not treat its `vault_write_low` direction as the current external MCP plan.

## Code To Inspect

MCP surface:

- `internal/mcp/tool_contract.go`
  Canonical live MCP tool contracts. Current live tools are read tools plus `persona_update_propose`.
- `internal/mcp/server.go`
  MCP server. `tools/call` routes `persona_update_propose` to the orchestrator.
- `internal/mcp/server_test.go`
  Important test: `TestMCPV1ExposesOnlyReadAndPersonaProposalTools` locks MCP to read tools + `persona_update_propose`. This prevents accidental shell/generic write/apply exposure.

Persona proposal and apply:

- `internal/model/draft.go`
  Defines `PersonaUpdateProposal` and result DTOs.
- `internal/orchestrator/harness.go`
  `ProposePersonaUpdate`, `ApplyDraft`, `validateDraftTarget`, and `appendPersonaUpdateRecord` are the key functions.
- `internal/orchestrator/harness_test.go`
  Covers proposal creation, local apply, conflict detection, invalid payloads, forged target rejection, wrong class rejection, and invalid confidence rejection.

Managed docs and prompts:

- `internal/bootstrap/templates.go`
  `agent.md` template is external-first shared operating manual. It still says low-risk writing may be added later; that should be revised to proposal-only for external agents.
- `internal/bootstrap/templates_test.go`
  Locks important `agent.md` / `identity.md` wording.
- `internal/operatoragent/model.go`
  Local Lore prompt construction. It preloads `agent.md` and `identity.md`; full CoreContext is not implemented yet.

Process sink and external session ingestion:

- `internal/config/config.go`
  Process-sink defaults include 30-minute windowing.
- `internal/app/import_codex.go`
  Imports external/codex-style logs into process-sink flow.
- `internal/domain/processsink/service.go`
  Writes checkpoints and reports.

Local write/runtime:

- `internal/console/tool_runtime.go`
  Local console tool runtime still has `vault_write_low`.
- `internal/orchestrator/harness.go`
  `WriteLowRiskNote` exists and should be considered local/internal for now, not external MCP.

Daemon/post-scan:

- `internal/app/daemon.go`
  Vault watcher/post-scan path. This is the place for out-of-band external file writes to become governance findings later.

## Current Completed Work

Recent relevant commits:

- `145b12e feat: add persona update proposal intake`
  Added MCP `persona_update_propose`.
- `69de43c test: lock mcp proposal intake tool surface`
  Added exact MCP allowlist: read tools + `persona_update_propose`.
- `da14b57 feat: apply reviewed persona update drafts`
  Added local reviewed apply for persona update drafts.
- `9d20045 fix: constrain persona update draft targets`
  Hardened persona apply so forged approved drafts cannot write arbitrary targets and confidence is revalidated.

Current implemented behavior:

- External MCP can read context and submit `persona_update_propose`.
- `persona_update_propose` creates a pending `DraftKindPersonaUpdate`; it does not write `人物画像.md`.
- Local/runtime apply can apply an approved persona update by appending a structured record under `## Applied Persona Updates`.
- Persona apply requires target path to equal configured persona doc and target class to be `persona`.
- MCP does not expose approve/apply, shell, generic write/edit, or `vault_write_low`.

## User's Intended Business Workflow

The workflow to preserve:

1. User chats with an external agent.
   Every 30 minutes, the external-agent conversation can be ingested into Lore and summarized as process-sink checkpoint/report material.
2. External agent connects to Lore.
   Lore provides task context through MCP read tools: managed docs, vault search/read/resolve, context packs.
3. External agent produces class notes, meeting notes, development summaries, or knowledge-base content.
   If it wants the content written through Lore, it submits a proposal. Lore checks persona, weak points, system rules, current progress, and relevant notes. Lore then refines/reviews the proposal and applies it locally only after approval.
4. External agent may bypass Lore and write files directly with its own tools.
   Lore cannot stop this. It should be handled later by post-scan detection, audit, conflict marking, or draft creation.

## Current Gaps

The biggest missing feature is ordinary markdown note proposal intake.

Missing:

- `note_write_propose` or `markdown_note_propose`.
- Draft kind/model for proposed markdown note writes.
- Local reviewed apply for note-write drafts.
- Draft refine/edit path so Lore can modify external-agent proposed content before approval.
- Strong CoreContext for review/refine, especially persona, weak points, system rules, progress, and pending review state.
- Post-scan governance findings for out-of-band external shell/file writes.

Potentially stale:

- `docs/adr-lore-v1-architecture.md` still says future MCP L2 `vault_write_low`.
- `internal/bootstrap/templates.go` still says low-governance note writing may be allowed later.
- Older review handoff docs may mention external MCP `vault_write_low`; treat them as historical.

## Recommended Next Steps

1. Update architecture docs and templates.
   Remove external MCP `vault_write_low` from v1 direction. State that external agents submit write proposals and Lore applies after review.
2. Design `note_write_propose`.
   Keep it narrow. Do not add generic `proposal_submit`.
3. Implement note proposal creation.
   Input should include target path or title, markdown content, evidence/source, reason, and observed/task context.
4. Implement note draft apply.
   Approved note drafts can write ordinary markdown notes only. Reject managed core docs, persona, progress, agent, identity, plans/execution docs, process-sink outputs, hidden paths, non-markdown files, and path traversal.
5. Implement draft refine/edit.
   Lore needs a way to modify proposed content after reading core context and before approval.
6. Implement CoreContext for review/refine.
   Lore should not rely on the model to remember to read persona/system/progress/weakness. Review flows need these summaries injected or explicitly loaded.
7. Implement post-scan governance for out-of-band writes.
   Treat external direct writes as findings, not as approved Lore writes.

## Roles

- User: product and boundary decision owner.
- Codex: main implementation and architecture owner.
- Codex 5.3: review owner. Focus: governance boundary, tests, contract drift, forged state/draft cases, MCP exposure.
- Opus: TUI owner. Do not mix TUI changes into MCP/CoreContext/proposal commits.
- DeepSeek: supplemental reviewer from earlier backend/stability passes.

## Verification

Preferred verification:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```

Targeted checks for this line:

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp -run TestMCPV1ExposesOnlyReadAndPersonaProposalTools -count=1 -v
.\.tools\go\bin\go.exe test ./internal/orchestrator -run TestApplyPersonaUpdateDraft -count=1 -v
```

## Worktree State At This Handoff

Before editing, run:

```powershell
git status --short
```

At the time this handoff was written, the worktree was clean after `9d20045`.
