# ADR: Lore v1 Agent Architecture and Write Boundaries

## Status

Proposed for three-party review.

Review owners:

- Product / final decision: user
- Architecture / implementation planning: Codex
- Code review: Codex 5.3
- TUI impact review: Opus

## Context

Lore currently has several working pieces, but their product boundaries have started to overlap:

- Local Lore chat can use read tools, governed draft actions, `vault_write_low`, workspace tools, git tools, and optional shell confirmation.
- MCP started as read-only and exposes `initialize`, `ping`, `tools/list`, and `tools/call` over stdio.
- External agents can connect through MCP. The current write-adjacent path is proposal intake, starting with `persona_update_propose`.
- `agent.md` and `identity.md` both exist as managed core docs, but their intended audiences need to be frozen.
- `DraftKindPersonaUpdate` now supports proposal intake and local reviewed apply; MCP still cannot approve or apply drafts.
- The daemon already scans vault changes and can trigger governed drafts for plan/progress flows.

This ADR freezes the v1 architecture direction before adding new tools.

## Decision

Lore v1 uses a proposal-first write governance model.

MCP v1 evolves from read-only to read + proposal intake. External MCP is an intake surface, not a write surface: it must not expose shell execution and must not let external agents write vault markdown directly. Lore is the review and governance layer, not an external file-write proxy.

`agent.md` is the external-first shared agent operating manual. External agents should read it first after connecting to Lore. Local Lore may also read it as shared workspace rules. It must be safe for non-Lore agents to read and must not make them adopt Lore's self identity.

`identity.md` is Lore self identity. It describes the local Lore agent's name, role, temperament, and collaboration defaults. External agents should not be required to read it by default.

## Core Document Semantics

| Document | v1 role | Default reader |
| --- | --- | --- |
| `agent.md` | External-first agent operating manual and shared workspace rules | External agents and local Lore |
| `identity.md` | Lore self identity and collaboration persona | Local Lore |
| persona doc (`system_doc_get("persona")`) | User long-term profile | Local Lore; external agents only when task-relevant |
| system doc (`system_doc_get("system")`) | Workspace governance rules | Local Lore; external agents when policy-relevant |
| progress index (`system_doc_get("progress")`) | Managed document status index | Local Lore; external agents when planning/status-relevant |

`agent.md` should describe how an external agent works through Lore:

- read `agent.md` first;
- prefer Lore MCP tools over generic file guessing;
- use `system_doc_get`, `context_pack`, and vault read/search/resolve tools before acting;
- never directly edit governed documents;
- submit profile or governance changes through Lore proposal/draft paths;
- emit a structured `Persona Update Candidate` when proposal tooling is unavailable;
- submit markdown changes through Lore proposal paths so local Lore can review, create a revised draft when needed, approve, and apply them after checking core context.

`identity.md` should not contain instructions for external agents.

## Write Levels

Lore v1 separates external MCP capabilities from local Lore/runtime capabilities:

| Level | Name | Meaning | MCP v1 |
| --- | --- | --- | --- |
| L0 | Read | Read status, core docs, vault docs, search, resolve, backlinks, context packs | Allowed |
| L1 | Proposal intake | Submit a proposal that creates a pending draft or review item; no vault write | Allowed for `persona_update_propose` |
| L2 | Internal/runtime write | Lore runtime writes low-governance markdown after local governance judgment | Not exposed through external MCP v1 |
| L3 | Governed apply | Apply approved drafts to managed core, plans, execution docs, and other governed targets | Not exposed through MCP |

`vault_write_low` may remain a local/internal runtime capability, but it is not an external MCP v1 capability. External agents that want Lore to write markdown notes must submit a narrow note proposal first. Local Lore can then review, create a superseding draft if needed, approve, and apply it.

External proposal/apply flows must reject:

- managed core docs, including persona, system, progress, agent, and identity docs;
- plan and execution docs;
- process-sink outputs;
- hidden directories and ignored paths;
- non-markdown files;
- paths outside the vault.

L3 remains local Lore/runtime controlled. MCP must not expose approve/apply for governed drafts in v1.

## MCP v1 Boundary

Allowed in v1:

- Existing L0 read tools.
- L1 `persona_update_propose`.
- L1 `markdown_note_propose` for ordinary markdown note candidates. It creates pending drafts only; local Lore handles supersede, approve, and apply.

Forbidden in v1:

- MCP shell execution.
- Generic workspace file write/edit tools.
- Direct markdown vault-write tools, including `vault_write_low`.
- Direct writes to managed core docs.
- Direct writes to plan/execution docs.
- Direct writes to process-sink outputs.
- Applying approved drafts through MCP.
- A generic `proposal_submit` tool before narrower proposal tools prove the model.

The first proposal tool is `persona_update_propose`, not a generic proposal API. It creates a `DraftKindPersonaUpdate` pending review and returns `draft_created`, `draft_id`, `target`, and `review_required: true`. It must not change the persona document by itself. Proposal creation is not persona apply.

Persona apply is local Lore/runtime controlled. After a user-reviewed draft is approved, local apply appends a structured record under `## Applied Persona Updates` in the persona document. It does not attempt freeform field rewriting in v1. MCP must not expose the approve/apply steps.

## Shell Post-Scan Model

External agents may still have their own shell or file editing capabilities outside Lore. Lore cannot assume every vault change came through MCP.

Therefore v1 keeps daemon/post-scan as a safety and reconciliation layer:

- Scan vault changes after external shell/file activity.
- Classify changed markdown paths.
- Treat ordinary note changes as out-of-band changes that require audit and reconciliation.
- Treat governed document changes as governance findings that require conflict handling, draft creation, or manual review.
- Never silently bless direct external edits to governed documents.

Post-scan is not a replacement for MCP governance. It is the fallback that catches out-of-band writes. The MVP is implemented as persisted finding-backed detection: ordinary note changes produce out-of-band write audit records and open findings; managed core and process-sink changes produce governance finding audit records and open review-needed findings. Findings are visible and closable through local Lore CLI commands, not MCP. Richer conflict markers or draft creation can be added later without changing the external MCP boundary.

## Core Context Direction

Local Lore now preloads `agent.md` and `identity.md`, and the CoreContext MVP injects persona, weakness, system, progress, and pending-draft context into the local operator agent.

v1 direction:

- Local Lore should have stable access to a short CoreContext covering persona, system rules, progress status, and pending review state.
- The existing session working set remains a separate prompt section; it is not required to live inside the CoreContext struct.
- CoreContext should not become implicit cross-session transcript memory.
- CoreContext should be short and structured; full docs remain available through tools.
- Persona conflicts must not be resolved silently. Lore should ask whether a new statement is a profile update, this-turn-only context, or an example.

CoreContext MVP is implemented for local Lore review/supersede prompts. Future work may improve summaries, routing, conflict detection, and weakness extraction, but the local review/supersede prompt no longer depends on the model remembering to load core docs.

## Implementation Order

1. Freeze this ADR and review it with Codex 5.3.
2. Update managed templates so `agent.md` becomes an external-first operating manual and `identity.md` remains Lore self identity.
3. Update external MCP docs to tell external agents to read `system_doc_get("agent")` first and not default to `identity`.
4. MCP L1 `persona_update_propose` is implemented. It creates a pending draft and must not claim the persona document has changed.
5. Local reviewed persona update apply is implemented as append-only structured records.
6. MCP L1 `markdown_note_propose` is implemented for ordinary markdown note candidates; local markdown note apply and supersede are local Lore/runtime capabilities.
7. CoreContext is implemented for local Lore review/supersede prompts with vault content injected as contextual data, not system instructions.
8. Business-level smoke coverage is implemented for governed markdown note intake.
9. Daemon/post-scan detection is implemented for ordinary note changes and governed/process-sink document findings, with audit records plus persisted open findings and local `findings` CLI review commands.
10. One-shot Lore external transcript JSONL import is implemented for process-sink checkpoint/report ingestion; incremental attach/sync remains future work.

## Acceptance Criteria

- v1 docs distinguish `agent.md`, `identity.md`, user persona, system rules, and progress status.
- MCP contract tests fail if shell, generic workspace write, direct vault-write, or governed apply tools are exposed.
- External MCP exposes proposal intake only for write-adjacent flows; Lore applies accepted changes locally after review.
- Governed paths remain blocked from MCP direct write.
- Daemon/post-scan can detect out-of-band governed document changes and does not silently approve them.
- Persona update proposal creates a draft/review item only; applying it remains a user-reviewed local Lore/runtime action and appends a structured persona update record.

## Non-Goals

- No MCP shell.
- No generic MCP file write/edit.
- No MCP direct markdown vault write, including `vault_write_low`.
- No direct MCP write to managed core docs.
- No MCP draft approval/apply.
- No generic `proposal_submit` in the first proposal phase.
- No TUI behavior change in this ADR.

## Rollback

If markdown note proposal semantics prove too broad, v1 can keep L0 read tools plus persona proposal intake while preserving daemon/post-scan for out-of-band changes.

If persona proposal semantics prove unstable, keep `persona_update_propose` out of MCP and require external agents to emit structured Persona Update Candidate text for local Lore review.
