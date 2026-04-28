# Compact Handoff: Lore Reviewer Role

## Purpose

This document is for the next compacted reviewer session.

The reviewer should use this as the active project memory before reviewing future changes in `C:\Users\15892\Desktop\obsidian-harness`.

The user expects this agent to act as a reviewer first. Do not implement unless the user explicitly asks for implementation.

## Role

Primary role:

- Code and architecture reviewer for Lore / `obsidian-harness`.
- Focus on business correctness, governance boundaries, code reasonableness, and test adequacy.
- Review not only whether tests pass, but whether implementation matches the intended product workflow.

Default behavior:

- Read code and docs carefully before responding.
- Surface blockers clearly.
- Separate blocker, non-blocker, and optional suggestions.
- Do not silently accept a change just because tests pass.
- Do not modify code when the user asks for review.
- If asked to write a plan or handoff document, docs-only changes are acceptable.

Communication style:

- Direct, pragmatic, factual.
- No cheerleading.
- Explain why a boundary matters.
- Keep final answers concise unless the user asks for detail.

## User Preferences And Constraints

Important user instructions:

- The user has repeatedly stated: “你是负责 review 的”.
- Review must include code structure and product semantics, not only tests.
- The user wants business workflow to drive architecture.
- The user is actively coordinating with GPT-5.5 as main developer and Opus as TUI developer.
- Do not disturb Opus-owned TUI files unless explicitly asked.

Repository rules:

- Root repo: `C:\Users\15892\Desktop\obsidian-harness`.
- Root instructions: `AGENTS.md`.
- Current sandbox allows file access, but use restraint.
- Do not commit unless explicitly requested.
- Use `.\scripts\verify.ps1` for cross-module verification when needed.
- For review-only tasks, targeted reads/tests are OK; do not change files unless asked.

Current untracked file created by this reviewer line:

- `docs/plans/2026-04-28-external-agent-governed-intake.md`

This file is a planning document, not code. It has been revised several times after GPT review.

## Project Summary

Lore is a governed knowledge-ops harness for an Obsidian vault.

Core product goal:

- External agents can produce content and interact with the user.
- Lore ingests process traces and provides context.
- Lore governs what enters the long-term knowledge base.
- Managed/core documents and durable knowledge changes should go through proposal/review/apply, not silent mutation.

Main surfaces:

- Local Lore console/TUI.
- Runtime/orchestrator.
- Vault read and write governance.
- Draft/review/apply state machine.
- Process-sink ingestion and rollup.
- MCP stdio server for external agents.
- Go SDK for MCP read-only typed methods.

Important directories:

- `internal/orchestrator`: runtime governance, read APIs, proposals, apply logic.
- `internal/mcp`: MCP stdio server and tool contracts.
- `internal/console`: local Lore agent tool runtime.
- `internal/operatoragent`: model loop, prompt/history/tool call handling.
- `internal/app`: runtime wiring, daemon, process-sink imports, smoke.
- `internal/domain/docclass`: document classifier.
- `internal/domain/processsink`: checkpoint/daily report materialization.
- `internal/sessionlog`: session transcript/resume.
- `internal/tui`: Opus-owned TUI presentation/shell areas.
- `docs`: ADRs, integration docs, review handoffs, plans.
- `sdk/go/lore`: Go SDK submodule.

## Current Architecture Decisions

### Agent Docs Semantics

Frozen intended semantics:

- `agent.md` = external-first shared operating manual.
  - External agents should read it first through `system_doc_get("agent")`.
  - Local Lore may also read it as shared workspace rules.
  - It must not contain Lore self identity.
- `identity.md` = local Lore self identity.
  - External agents should not default-read it.
  - External agents must not adopt Lore identity.
- `人物画像.md` / persona doc = user long-term profile.
- `系统说明.md` = workspace governance rules.
- `文档进度总表.md` = managed document status/progress.

### External MCP Boundary

Latest accepted business boundary:

> External MCP is an intake surface, not a write surface.

Meaning:

- External MCP v1 should expose read tools and proposal-intake tools.
- External MCP v1 should not expose direct vault writes.
- `vault_write_low` must not be part of external MCP v1.
- `vault_write_low` may remain local/internal runtime capability.
- All external-agent durable markdown writes that go through Lore should be proposals first.
- Local Lore/user then reviews, may supersede with a revised draft, approves, and applies.

Forbidden in external MCP:

- `vault_write_low`
- shell execution
- generic workspace write/edit
- draft approve
- draft apply
- draft supersede/refine
- direct persona write
- direct managed core write
- direct progress/plan/process-sink write
- generic unconstrained proposal API

Allowed in external MCP:

- L0 read tools.
- Narrow L1 proposal tools:
  - existing `persona_update_propose`
  - planned `markdown_note_propose`

### Business Workflow

The user’s intended workflow has three lanes:

1. External chat/process trace:
   - User chats with external agent.
   - Every ~30 minutes transcript is imported into Lore.
   - Lore produces process-sink checkpoints and daily reports.
   - This is process memory, not formal knowledge.

2. Context supply:
   - External agent connects to Lore MCP.
   - It reads `agent.md`, system/persona/progress/context packs, relevant vault docs.
   - Lore provides context; external agent does work.

3. Governed knowledge intake:
   - External agent produces classroom notes, meeting notes, development summaries, etc.
   - External agent submits proposal to Lore.
   - Lore reviews with persona/weakness/system/progress context.
   - Lore may create a superseding revised draft.
   - Local/user approves.
   - Lore applies to vault.

External agent direct shell/file writes:

- Cannot be prevented by Lore.
- Are out-of-band.
- Lore should later post-scan, detect, audit, and create findings/conflicts/drafts where needed.
- Post-scan is compensation/governance discovery, not a security boundary.

## Major Implemented Work Before This Handoff

### Sessionlog / Resume

Areas reviewed:

- `internal/sessionlog/*`
- `internal/console/session.go`
- `internal/cli/cli.go`

Important decisions:

- Default does not load memory/session history unless explicitly resumed.
- `resume` must not replay tool calls.
- JSONL corrupted lines and large lines need tolerance.
- Workspace-local session storage.
- Non-TTY `--resume` should not block; scripts should use `--resume-id`.

Relevant commits mentioned by user:

- `c018aed fix: make resume noninteractive safe`
- `59381ff feat: show session transcript summaries`
- `775f21c fix: allow large sessionlog jsonl lines`
- `f941e7f test: cover session resume index refresh`
- `b9011ed fix: rebuild missing session index`

### Vault Resolve

Areas reviewed:

- `internal/orchestrator/readapi.go`
- `internal/mcp/server.go`
- `internal/operatoragent/tool_schema.go`
- `internal/operatoragent/model.go`

Important decisions:

- MCP/native schema must match.
- `vault_resolve` uses `dir`, with `path` deprecated alias.
- Unique match threshold should be conservative.
- `selected_path` should only be trusted when status is `unique`.
- Working set should remember `selected_path` for unique vault_resolve only.
- Ambiguous/not_found must not pollute working set.

Relevant commits mentioned:

- `dbc4301 fix: align vault resolve tool schema`
- `1237ea8 fix: remember vault resolve selected path`
- `d357c0a fix: gate vault resolve working set on unique`

### Shell Confirm / Tool Boundary

Important decisions:

- Non-shell tools should not use keyword gating.
- `shell_exec` requires confirmation.
- Confirm state must not be replayed through transcript/resume.
- Local console/TUI tool surface differs from MCP.
- External MCP must not expose shell.

Relevant commits:

- `03c61d2 test: freeze shell confirmation boundary`

### SDK / MCP Contract

Go SDK v0 was added and hardened.

Important decisions:

- SDK lives in same repo under `sdk/go/lore` with separate `go.mod`.
- SDK typed methods are read-only.
- Raw `CallTool` can call live MCP tools and may see proposal-intake tools.
- SDK must not import `internal/*`.
- MCP v0 read-only contract artifact remains:
  - `docs/contracts/mcp-sdk-tools-v0.json`
- Do not mutate SDK v0 artifact to include proposal tools.
- If a v1 external proposal contract artifact is needed, create a separate artifact.

Relevant files:

- `sdk/go/lore/README.md`
- `sdk/go/lore/boundary_test.go`
- `docs/contracts/mcp-sdk-tools-v0.json`
- `internal/mcp/tool_contract.go`
- `internal/mcp/server_test.go`

Relevant commits mentioned:

- `d734dfd feat: add Go SDK stdio transport`
- `d688f31 fix: harden Go SDK stdio cancellation`
- `1e37453 fix: avoid SDK post-success cancel race`
- `0026b23 test: add Go SDK e2e smoke`
- `657b92d docs: publish mcp sdk contract artifact`
- `f30c113 test: enforce go sdk internal boundary`
- `3790a38 test: scan sdk boundary recursively`
- `4ea48bb test: close go sdk v0 readiness gaps`

### External Agent Docs And Persona Proposal

Implemented direction:

- External MCP setup docs exist.
- External agent onboarding says read `system_doc_get("agent")`.
- `persona_update_propose` exists.
- It creates pending `persona_update` draft only.
- MCP does not expose approve/apply.
- Local reviewed persona apply exists and appends structured record to persona doc.

Important reviewed risk:

- Persona apply must validate target path/class at apply time, not trust draft store.
- Confidence enum must be revalidated at apply time.

The current code appears to include:

- `validateDraftTarget` in `internal/orchestrator/harness.go`.
- Confidence enum validation in `appendPersonaUpdateRecord`.

Relevant commits mentioned:

- `3b79cfa docs: define external agent proposal intake`
- `145b12e feat: add persona update proposal intake`
- `69de43c test: lock mcp proposal intake tool surface`
- `da14b57 feat: apply reviewed persona update drafts`

### Chat History / Context Continuity

Issue reviewed:

- Lore failed to continue from prior assistant response in short user follow-up like “给”.

Fix direction:

- Recent history should be sent as role-aware messages.
- Older summary should be untrusted user context, not system/developer.
- Do not hard-code “only one system message”; instead enforce trust boundary:
  - runtime/developer rules can be privileged
  - user/assistant/history/summary content cannot be privileged

Relevant files:

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`

## Current Planning Document

New plan created in this review session:

- `docs/plans/2026-04-28-external-agent-governed-intake.md`

Purpose:

- Detailed executable plan for external-agent governed markdown intake.
- It merges reviewer and developer thinking.
- It should guide future implementation but is docs-only.

Current status:

- It has been revised after multiple GPT reviews.
- `git diff --check` passed after each revision.
- It remains untracked under `docs/plans/`.

Key content in the plan:

- External MCP v1 = read + proposal intake.
- `vault_write_low` stays local/internal, not external MCP.
- Add `markdown_note_propose`.
- Apply markdown note drafts locally only after approval.
- Use supersede/revised draft, not in-place refine.
- Add CoreContext MVP.
- Add post-scan later for out-of-band writes.

Important final corrections already made:

- CoreContext trust boundary:
  - usage rules may be system/developer-level
  - persona/weakness/progress/pending draft text is vault/user-authored context, not privileged instruction
- Size limits:
  - `content` max 256 KiB
  - `evidence`, `reason`, `task_context` max 16 KiB
  - short fields max 512 bytes
  - MCP `Content-Length` above 1 MiB rejected at frame/read layer before body read
- Supersede:
  - MVP uses supersede only
  - no in-place `RefineDraft`
  - store boundary must be atomic or rollback-safe
- `Draft.Target.Class = note`:
  - semantic governed target class
  - classifier may return `unknown` for ordinary notes
  - validation allows `unknown|note` after rejecting governed/unsafe paths
- `target_path` is required in MVP.
- new file base version uses sentinel `DraftBaseVersionNewFile = "new"`, not empty string.
- no frontmatter in MVP.
- TUI out of scope for first implementation.
- MCP `markdown_note_propose` omits `tags` and `related_paths` in v1; arrays can be added later with helpers.
- CoreContext MVP decision:
  - orchestrator builds
  - console/operatoragent injects

## Planned Implementation Sequence

If user asks to implement, follow the plan document rather than improvising.

Recommended task order:

1. Update ADR and external docs to remove external MCP `vault_write_low` roadmap.
2. Update managed `agent.md` template to proposal-only write boundary.
3. Strengthen MCP forbidden direct-write tests.
4. Add markdown note proposal model.
5. Add orchestrator `ProposeMarkdownNote`.
6. Expose `markdown_note_propose` through MCP.
7. Apply reviewed markdown note drafts locally.
8. Add local draft supersede.
9. Build CoreContext MVP.
10. Add business-level smoke.
11. Add post-scan for out-of-band writes.

Do not reorder CoreContext too late if business review requires persona/weakness-aware approval. At minimum, define review context contract alongside note proposal/apply work.

## Current Highest-Priority Review Focus

Future review should focus on these risks:

### External MCP Boundary Drift

Block if MCP exposes:

- `vault_write_low`
- shell
- workspace write/edit
- draft approve/apply
- draft supersede/refine
- generic proposal submit

### Proposal Side Effects

Block if `markdown_note_propose`:

- writes markdown file during proposal creation
- creates directories in vault unnecessarily
- approves/applies draft
- mutates persona/system/progress/agent/identity
- accepts unsafe target path
- accepts governed target path
- has no size limits

### Apply Trust And Validation

Block if markdown note apply:

- trusts proposal-time validation only
- writes outside vault root
- overwrites unexpected existing file
- allows managed core/plan/process-sink target
- treats empty base version as new-file signal
- lacks audit record
- transitions state incorrectly on invalid payload

### Supersede Atomicity

Block if supersede:

- updates old draft to superseded before ensuring new draft is saved
- lacks transaction/rollback strategy
- mutates original proposed content
- is exposed through MCP

### CoreContext Trust Boundary

Block if:

- persona/weakness/progress vault text is injected as system/developer instruction
- older conversation summary becomes privileged
- CoreContext is only documented but not actually available to review/supersede prompt

### TUI Scope

Block if early backend proposal/apply implementation casually edits TUI presentation files.

TUI is Opus-owned. First implementation should prove flow through:

- orchestrator
- app
- console or CLI
- tests

## Useful Commands

Status:

```powershell
git status --short
```

Search:

```powershell
rg -n "vault_write_low|markdown_note_propose|persona_update_propose|DraftKind|ApplyDraft|toolContracts" internal docs -S
```

Targeted MCP tests:

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp -count=1
```

Targeted orchestrator tests:

```powershell
.\.tools\go\bin\go.exe test ./internal/orchestrator -count=1
```

Cross-module verification:

```powershell
.\scripts\verify.ps1
```

MCP E2E if protocol behavior changes:

```powershell
.\scripts\verify.ps1 -E2E
```

Docs whitespace:

```powershell
git diff --check
```

## Important Files To Read First After Compact

Start here:

1. `AGENTS.md`
2. `docs/compact-handoff-reviewer.md`
3. `docs/plans/2026-04-28-external-agent-governed-intake.md`
4. `docs/adr-lore-v1-architecture.md`
5. `docs/integrations/mcp-client-setup.md`
6. `internal/mcp/tool_contract.go`
7. `internal/mcp/server.go`
8. `internal/mcp/server_test.go`
9. `internal/orchestrator/harness.go`
10. `internal/orchestrator/readapi.go`
11. `internal/console/tool_runtime.go`
12. `internal/operatoragent/model.go`

If reviewing process-sink:

- `internal/app/import_codex.go`
- `internal/app/daemon.go`
- `internal/domain/processsink/service.go`

If reviewing session/resume:

- `internal/sessionlog/*`
- `internal/cli/cli.go`
- `internal/console/session.go`

If reviewing SDK:

- `sdk/go/lore/*`
- `docs/contracts/mcp-sdk-tools-v0.json`

## Known Documentation Drift To Watch

The current ADR may still mention external MCP later exposing `vault_write_low`.

That is now considered wrong by latest business decision.

If asked to implement Task 1, update:

- `docs/adr-lore-v1-architecture.md`
- `docs/integrations/mcp-client-setup.md`
- `docs/compact-handoff-lore-v1.md`
- `internal/bootstrap/templates.go`
- `internal/bootstrap/templates_test.go`

## Reviewer Response Template

When reviewing a change, use this structure:

```text
Review conclusion:
- Pass / blocked / pass with non-blocking notes.

Blockers:
- ...

Non-blocking:
- ...

Verified:
- commands run
- important files checked

Business alignment:
- whether the change preserves Lore as governance layer
```

For quick answers, shorter is fine.

## Final Mental Model

Lore is not a generic agent platform and not an external file-write proxy.

Lore is a governed intake, review, and apply layer for a long-term Obsidian knowledge base.

External agents can:

- provide process traces
- request context
- submit proposals

Lore decides:

- what should become durable knowledge
- how it should be structured
- where it should live
- whether it conflicts with persona, weaknesses, system rules, progress, or existing notes

Local Lore/runtime applies:

- after review
- after approval
- with audit
- with conflict checks

This is the product line to defend in future reviews.
