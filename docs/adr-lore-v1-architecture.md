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
- External agents can connect through MCP. The first proposal-intake path is `persona_update_propose`; low-risk write is still deferred.
- `agent.md` and `identity.md` both exist as managed core docs, but their intended audiences need to be frozen.
- `DraftKindPersonaUpdate` now supports proposal intake and local reviewed apply; MCP still cannot approve or apply drafts.
- The daemon already scans vault changes and can trigger governed drafts for plan/progress flows.

This ADR freezes the v1 architecture direction before adding new tools.

## Decision

Lore v1 uses a graded write model.

MCP v1 evolves from read-only to read + graded write, but it must not expose shell execution and must not let external agents write governed documents directly.

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
- use low-risk writing only for low-governance markdown notes when explicitly allowed by a later MCP phase.

`identity.md` should not contain instructions for external agents.

## Write Levels

Lore v1 uses four write levels:

| Level | Name | Meaning | MCP v1 |
| --- | --- | --- | --- |
| L0 | Read | Read status, core docs, vault docs, search, resolve, backlinks, context packs | Allowed |
| L1 | Proposal intake | Submit a proposal that creates a pending draft or review item; no vault write | Allowed for `persona_update_propose` |
| L2 | Low-risk direct write | Write low-governance markdown notes through runtime validation and audit | Planned for MCP v1 |
| L3 | Governed apply | Apply approved drafts to managed core, plans, execution docs, and other governed targets | Not exposed through MCP |

L2 is intentionally narrow. It reuses runtime governance and must reject:

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
- Later L2 `vault_write_low` for low-governance markdown notes, if the runtime accepts the path and document class.

Forbidden in v1:

- MCP shell execution.
- Generic workspace file write/edit tools.
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
- Treat low-risk note changes as auditable low-governance changes.
- Treat governed document changes as governance findings that require conflict handling, draft creation, or manual review.
- Never silently bless direct external edits to governed documents.

Post-scan is not a replacement for MCP governance. It is the fallback that catches out-of-band writes.

## Core Context Direction

Local Lore currently preloads `agent.md` and `identity.md`, but it does not yet consistently inject persona, system, and progress context.

v1 direction:

- Local Lore should have stable access to a short CoreContext covering persona, system rules, progress status, working set, and pending review state.
- CoreContext should not become implicit cross-session transcript memory.
- CoreContext should be short and structured; full docs remain available through tools.
- Persona conflicts must not be resolved silently. Lore should ask whether a new statement is a profile update, this-turn-only context, or an example.

CoreContext is not implemented by this ADR; it is a follow-up architecture item.

## Implementation Order

1. Freeze this ADR and review it with Codex 5.3.
2. Update managed templates so `agent.md` becomes an external-first operating manual and `identity.md` remains Lore self identity.
3. Update external MCP docs to tell external agents to read `system_doc_get("agent")` first and not default to `identity`.
4. Add MCP L1 `persona_update_propose` after its minimal draft payload is frozen. It may create a pending draft before full persona apply semantics are implemented, but must not claim the persona document has changed.
5. Support local reviewed persona update apply as append-only structured records.
6. Add MCP L2 `vault_write_low` only after contract tests and runtime rejection/audit tests are ready.
7. Add daemon/post-scan tests for low-risk changes and governed-document findings.
8. Implement CoreContext for local Lore after the document semantics and write levels are stable.

## Acceptance Criteria

- v1 docs distinguish `agent.md`, `identity.md`, user persona, system rules, and progress status.
- MCP contract tests fail if shell, generic workspace write, or governed apply tools are exposed.
- L2 low-risk MCP writes pass only through runtime validation and audit.
- Governed paths remain blocked from MCP direct write.
- Daemon/post-scan can detect out-of-band governed document changes and does not silently approve them.
- Persona update proposal creates a draft/review item only; applying it remains a user-reviewed local Lore/runtime action and appends a structured persona update record.

## Non-Goals

- No MCP shell.
- No generic MCP file write/edit.
- No direct MCP write to managed core docs.
- No MCP draft approval/apply.
- No generic `proposal_submit` in the first proposal phase.
- No TUI behavior change in this ADR.

## Rollback

If L2 MCP write proves too broad, v1 can keep L0 read tools and postpone MCP writes while preserving daemon/post-scan for out-of-band changes.

If persona proposal semantics prove unstable, keep `persona_update_propose` out of MCP and require external agents to emit structured Persona Update Candidate text for local Lore review.
