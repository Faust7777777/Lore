# Next Developer Handoff: Lore v1 External Agent Governance

## Current Position

You are taking over a large uncommitted feature line in `C:\Users\15892\Desktop\obsidian-harness`.

This line is not a small patch. It implements the Lore v1 external-agent governance direction:

```text
external agent output
-> Lore proposal intake
-> local Lore review/supersede with CoreContext
-> local/user approve
-> Lore apply
-> vault
```

The core product boundary is:

- External MCP is read + proposal intake.
- External MCP is not a write surface.
- `vault_write_low` is local/internal only.
- MCP must not expose shell, generic workspace write/edit, draft approve/apply/supersede, or direct vault writes.
- External shell/file writes are out-of-band; Lore can only detect, audit, and govern them after the fact.

## Read First

Read these documents in order:

1. `docs/compact-handoff-lore-v1.md`
   Current compact handoff and architecture summary.
2. `docs/adr-lore-v1-architecture.md`
   Main ADR. Treat this as the source of product boundary truth.
3. `docs/plans/2026-04-28-external-agent-governed-intake.md`
   Original implementation plan. Some early wording is historical, but the task sequence explains why the code is shaped this way.
4. `docs/integrations/mcp-client-setup.md`
   External MCP onboarding and client setup.
5. Review handoffs:
   - `docs/review-handoff-codex53-external-mcp-proposal-only.md`
   - `docs/review-handoff-codex53-markdown-note-proposal.md`
   - `docs/review-handoff-codex53-mcp-markdown-note-propose.md`
   - `docs/review-handoff-codex53-markdown-note-apply.md`
   - `docs/review-handoff-codex53-draft-supersede.md`
   - `docs/review-handoff-codex53-core-context.md`
   - `docs/review-handoff-codex53-governed-note-smoke.md`
   - `docs/review-handoff-codex53-out-of-band-post-scan.md`
   - `docs/review-handoff-codex53-findings-cli.md`
   - `docs/review-handoff-codex53-external-transcript-import.md`

## Code To Inspect

MCP and external boundary:

- `internal/mcp/tool_contract.go`
- `internal/mcp/server.go`
- `internal/mcp/server_test.go`

Draft/proposal/apply:

- `internal/model/draft.go`
- `internal/orchestrator/harness.go`
- `internal/orchestrator/harness_test.go`
- `internal/app/drafts.go`
- `internal/domain/drafts/draft.go`

Local review tools:

- `internal/console/tool_runtime.go`
- `internal/console/tool_runtime_test.go`
- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`

CoreContext:

- `internal/model/core_context.go`
- `internal/orchestrator/core_context.go`
- `internal/orchestrator/core_context_test.go`
- `internal/app/core_context.go`
- `internal/console/session.go`

Post-scan and findings:

- `internal/app/daemon.go`
- `internal/app/daemon_test.go`
- `internal/model/finding.go`
- `internal/app/findings.go`
- `internal/store/store.go`
- `internal/store/memory/store.go`
- `internal/store/jsonstore/store.go`
- `internal/store/sqlitestore/store.go`

External transcript import:

- `internal/adapter/externaljsonl/parser.go`
- `internal/adapter/externaljsonl/parser_test.go`
- `internal/app/import_external.go`
- `internal/app/import_external_test.go`
- `internal/cli/cli.go`
- `cmd/obsidian-harness/main_test.go`

Templates/docs:

- `internal/bootstrap/templates.go`
- `internal/bootstrap/templates_test.go`
- `docs/integrations/mcp-client-setup.md`

## Implemented Capabilities

### External MCP Proposal Intake

- `persona_update_propose`
  Creates pending persona update draft only.
  Does not write persona doc.
  Local apply appends structured records under `## Applied Persona Updates`.

- `markdown_note_propose`
  Creates pending markdown note write draft only.
  Does not write the target note.
  Rejects governed/core/process-sink/plan/hidden/path-escape/non-md targets.

MCP allowlist is locked in `internal/mcp/server_test.go`.

### Local Markdown Note Apply

Approved `DraftKindMarkdownNoteWrite` drafts can be applied locally.

Apply revalidates:

- draft kind,
- target path,
- target class,
- base version,
- source kind,
- content payload,
- path governance.

New file drafts use `DraftBaseVersionNewFile = "new"`.

### Local Draft Supersede

Local Lore runtime can supersede markdown note drafts:

- original external proposal remains immutable evidence,
- original draft becomes `superseded`,
- revised draft is new and pending review,
- MCP does not expose supersede/refine.

### CoreContext MVP

Local Lore builds and injects CoreContext into operator-agent prompts as user-role vault context.

It includes persona/system/progress/pending draft context. `identity.md` remains outside CoreContext.

Important trust boundary:

- runtime instructions remain system/developer level,
- vault content from CoreContext is contextual data, not privileged instructions,
- chat history summaries must not be promoted to system/developer messages.

### Business Smoke

App smoke now covers:

- proposal created,
- target note not written during proposal,
- review/approve/apply,
- note written after local apply,
- audit chain,
- MCP no-direct-write boundary.

### Out-of-Band Post-Scan

Daemon post-scan now scans all markdown docs.

Behavior:

- initial full scan primes cursors,
- after baseline, new files are not silently primed,
- plan docs keep progress-sync draft behavior,
- ordinary notes produce `AuditOutOfBandVaultWrite` and open `FindingOutOfBandVaultWrite`,
- managed core/process-sink produce `AuditGovernanceFinding` and open `FindingGovernanceReviewNeeded`,
- cursor advances only after audit/finding succeeds.

### Local Findings CLI

Local CLI commands:

```powershell
lore findings list [--workdir <dir>] [--limit N]
lore findings resolve [--workdir <dir>] <id>
lore findings ignore [--workdir <dir>] <id>
```

Notes:

- `findings list` outputs full IDs, not shortened IDs.
- Long daemon-style IDs are tested and resolvable.
- `resolve` and `ignore` append `AuditFindingStateChange`.
- This is local CLI only, not MCP.

Known non-blocking issue:

- `ResolveFinding` / `IgnoreFinding` update finding state before appending audit. If audit fails, state may change without audit. If this becomes a hard governance requirement, implement store-level atomic state-change + audit. Do not patch casually without designing cross-store transaction semantics.

### External Transcript Import

Command:

```powershell
lore import-external-jsonl --workdir <workdir> --input <path> [--agent <id>] [--session <id>] [--window 30m] [--skip-rollup]
```

This accepts Lore external transcript JSONL schema, not arbitrary third-party JSONL.

It is one-shot only:

- writes process-sink checkpoints/reports,
- does not use MCP,
- does not create note proposals,
- does not mutate persona/system/progress.

## Current Worktree

At handoff time the worktree is intentionally dirty with the full v1 feature line.

Run:

```powershell
git status --short
```

Expect many modified files plus new review handoffs and new packages/files. Do not assume unrelated changes are safe to revert.

Notable warning from shell:

- Some PowerShell invocations print an execution-policy warning for the user profile.
- Prefer the project verification script form below, or use `powershell.exe -NoProfile` when needed.

## Verification Already Passing

Recent full verification passed:

```powershell
.\scripts\verify.ps1
```

Also passed recently:

```powershell
.\.tools\go\bin\go.exe test ./cmd/obsidian-harness -run "TestRunFindings" -count=1 -v
.\.tools\go\bin\go.exe test ./internal/app ./internal/store/... ./internal/model -count=1
.\.tools\go\bin\go.exe test ./internal/mcp -run "TestMCPV1ExposesOnlyReadAndProposalTools|TestExternalMCPDoesNotExposeDirectWrites" -count=1 -v
git diff --check
```

`git diff --check` only reports LF/CRLF warnings.

## Review Status

Recent Codex 5.3-style review outcomes:

- Post-scan audit + persisted finding: pass.
- Finding ordering and recreated-core finding assertion: addressed.
- Findings CLI:
  - full ID output regression added,
  - long ID resolve regression added,
  - ignore path audit assertion added.

Potential next review focus:

- Verify no MCP contract drift.
- Verify findings CLI remains local-only.
- Verify `findings list` output does not imply resolve/ignore rewrites vault content.
- Verify external transcript import remains process-sink only.
- Verify old handoff docs that mention `vault_write_low` are treated as historical.

## Do Not Change Without Product Decision

- Do not expose `vault_write_low` through MCP.
- Do not expose draft approve/apply/supersede through MCP.
- Do not add generic `proposal_submit`.
- Do not add shell/workspace write to external MCP.
- Do not make external transcript import auto-create persona/note/progress proposals.
- Do not silently apply out-of-band governed changes.
- Do not mix unrelated TUI redesign work into this feature line.

## Recommended Next Steps

1. Re-run `.\scripts\verify.ps1`.
2. Ask reviewer to review the full dirty feature line by these slices:
   - MCP proposal-only boundary,
   - markdown note proposal/apply/supersede,
   - CoreContext trust boundary,
   - post-scan + findings,
   - external transcript import.
3. If review passes, split commits in logical order matching `docs/plans/2026-04-28-external-agent-governed-intake.md`.
4. Decide whether v1 needs more than open findings:
   - conflict markers,
   - draft creation for governed out-of-band changes,
   - or keep current findings-only MVP.
5. Decide whether to add incremental attach/sync for Lore external transcript JSONL. Current support is import-only.

## Suggested Commit Slices

Do not commit all files as one blob unless the user explicitly asks.

Reasonable commit slices:

1. Docs/templates proposal-only boundary.
2. MCP proposal-only boundary tests.
3. Markdown note proposal model/orchestrator.
4. MCP `markdown_note_propose`.
5. Local markdown note apply.
6. Local draft supersede.
7. CoreContext MVP.
8. Governed note smoke.
9. Out-of-band post-scan detection + findings store.
10. Local findings CLI.
11. Lore external transcript JSONL import.

## Role Context

- User: product/boundary owner.
- Current outgoing developer: implemented and stabilized this v1 governance feature line.
- Next developer: should review, verify, and either finish commit slicing or continue with findings/conflict-marker work.
- Codex 5.3: expected reviewer for boundary, test rigor, and contract drift.
- Opus: TUI owner. Do not mix TUI work unless explicitly assigned.
