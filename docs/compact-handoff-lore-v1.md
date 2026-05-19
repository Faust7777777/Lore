# Compact Handoff: Lore External Agent Governance

## Current Product Direction

The main product decision has changed:

Lore should not be an external agent file-write proxy. Lore should be the governance and review layer for knowledge-base writes.

External agents can still write files directly with their own shell/file tools. Lore cannot prevent that. Those writes are out-of-band and can only be discovered later through daemon/post-scan governance.

When an external agent writes through Lore, the intended path is:

```text
external agent output
-> Lore proposal intake
-> Lore review/supersede with core context
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

The ADR and default `agent.md` template should stay aligned with this proposal-only external MCP direction.

## Read First

Priority documents:

- `docs/adr-lore-v1-architecture.md`
  Current architecture ADR. It treats external MCP as read + proposal intake and keeps `vault_write_low` local/internal if retained.
- `docs/integrations/mcp-client-setup.md`
  External MCP client setup. This is mostly aligned because it says there are no direct vault-write MCP tools.
- `docs/review-handoff-codex53-persona-proposal-p1.md`
  Handoff for `persona_update_propose`.
- `docs/review-handoff-codex53-persona-apply-p2.md`
  Handoff for local reviewed persona update apply.
- `docs/review-handoff-codex53-out-of-band-post-scan.md`
  Handoff for daemon post-scan handling of out-of-band vault writes.
- `docs/contracts/mcp-sdk-tools-v0.json`
  SDK-facing read-only contract artifact. It intentionally remains read-only.
- `sdk/go/lore/README.md`
  Go SDK v0 docs. Typed methods are read-only; raw `CallTool` can call live MCP tools, including proposal intake.

Optional background:

- `docs/archive/AGENT_INTERACTION_MODEL_ALIGNMENT.md`
  Historical local-agent write model. Useful context, but do not treat its `vault_write_low` direction as the current external MCP plan.

## Code To Inspect

MCP surface:

- `internal/tools/`
  Canonical ToolRegistry source for live MCP schema and dispatch. Current live MCP tools are read tools plus proposal-intake tools such as `persona_update_propose` and `markdown_note_propose`.
- `internal/mcp/server.go`
  MCP server. `tools/list` and `tools/call` are derived from the registry; there is no separate `internal/mcp/tool_contract.go` fallback.
- `internal/mcp/server_test.go`
  Important test: `TestMCPV1ExposesOnlyReadAndProposalTools` locks MCP to read tools + narrow proposal intake. This prevents accidental shell/generic write/apply exposure.

Persona proposal and apply:

- `internal/model/draft.go`
  Defines `PersonaUpdateProposal` and result DTOs.
- `internal/orchestrator/harness.go`
  `ProposePersonaUpdate`, `ApplyDraft`, `validateDraftTarget`, and `appendPersonaUpdateRecord` are the key functions.
- `internal/orchestrator/harness_test.go`
  Covers proposal creation, local apply, conflict detection, invalid payloads, forged target rejection, wrong class rejection, and invalid confidence rejection.

Managed docs and prompts:

- `internal/bootstrap/templates.go`
  `agent.md` template is external-first shared operating manual. It should state external MCP has read tools plus narrow proposal intake and no direct vault markdown write.
- `internal/bootstrap/templates_test.go`
  Locks important `agent.md` / `identity.md` wording.
- `internal/operatoragent/model.go`
  Local Lore prompt construction. It preloads `agent.md` and `identity.md`; CoreContext MVP is injected as user-role vault context, `identity.md` remains outside CoreContext, and the existing working set remains a separate prompt section.

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
  Vault watcher/post-scan path. It now scans all markdown docs: plan changes still create progress drafts, ordinary note changes create out-of-band audit records and open findings, and managed core/process-sink changes create governance audit records and open review-needed findings.
- `internal/model/finding.go`
  Persisted governance finding model for post-scan review visibility.
- `internal/app/findings.go`
  Local runtime finding list/resolve/ignore operations. State changes write audit records.
- `internal/cli/cli.go`
  Local `lore findings list|resolve|ignore` commands. These are not MCP tools.
- `internal/store/store.go`
  Store interface now includes `Findings()`; memory/json/sqlite stores persist findings.

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

- External MCP can submit `markdown_note_propose` for ordinary markdown note candidates.
- `markdown_note_propose` creates a pending `DraftKindMarkdownNoteWrite`; it does not write the target note.
- Local/runtime apply can apply an approved markdown note draft to ordinary markdown notes only, with target/path/base-version validation.
- Local/runtime supersede can create a revised markdown-note draft and mark the original proposal superseded.
- CoreContext is now built for local Lore and injected into the operator agent as vault context, not as system instructions.
- External MCP can read context and submit `persona_update_propose`.
- `persona_update_propose` creates a pending `DraftKindPersonaUpdate`; it does not write `人物画像.md`.
- Local/runtime apply can apply an approved persona update by appending a structured record under `## Applied Persona Updates`.
- Persona apply requires target path to equal configured persona doc and target class to be `persona`.
- MCP does not expose approve/apply, shell, generic write/edit, or `vault_write_low`.
- Business smoke covers proposal -> no target write -> review/approve/apply -> note written -> MCP no-direct-write boundary.
- Daemon post-scan now detects out-of-band ordinary note changes and governed/process-sink changes through audit records plus persisted open findings.
- Local CLI can inspect and close findings with `lore findings list`, `lore findings resolve <id>`, and `lore findings ignore <id>`.
- Lore external transcript JSONL import is available through `import-external-jsonl`; it writes process-sink checkpoints/reports through the same 30-minute window pipeline as Codex import.

## User's Intended Business Workflow

The workflow to preserve:

1. User chats with an external agent.
   Every 30 minutes, the external-agent conversation can be ingested into Lore and summarized as process-sink checkpoint/report material.
2. External agent connects to Lore.
   Lore provides task context through MCP read tools: managed docs, vault search/read/resolve, context packs.
3. External agent produces class notes, meeting notes, development summaries, or knowledge-base content.
   If it wants the content written through Lore, it submits a proposal. Lore checks persona, weak points, system rules, current progress, and relevant notes. Lore then creates a revised/superseding draft when needed and applies it locally only after approval.
4. External agent may bypass Lore and write files directly with its own tools.
   Lore cannot stop this. Current post-scan detects these changes, audits them, and persists open findings for later review. Future work can add richer conflict markers or automatic draft creation.

## Current Gaps

The biggest remaining feature is richer post-scan reconciliation beyond locally visible findings.

Still missing:

- Richer post-scan reconciliation beyond open findings, such as conflict markers or draft creation for governed changes.
- Incremental attach/sync mode for Lore external transcript JSONL. Current external transcript import is one-shot only.

Potentially stale:

- Older review handoff docs may mention external MCP `vault_write_low`; treat them as historical unless they were updated in the proposal-only pass.

## Recommended Next Steps

1. Run full verification and hand the feature line to Codex 5.3 for boundary review.
2. Decide whether locally visible open findings are sufficient for v1, or whether conflict markers/draft creation are needed before release.
3. If needed, add external transcript attach/sync after the one-shot Lore JSONL schema is reviewed.

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
.\.tools\go\bin\go.exe test ./internal/mcp -run TestMCPV1ExposesOnlyReadAndProposalTools -count=1 -v
.\.tools\go\bin\go.exe test ./internal/orchestrator -run TestApplyPersonaUpdateDraft -count=1 -v
```

## Worktree State At This Handoff

Before editing, run:

```powershell
git status --short
```

At the time this handoff was updated, the worktree contains the v1 external-agent governance feature line. Do not treat it as a single small diff.
