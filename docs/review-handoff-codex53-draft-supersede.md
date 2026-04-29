# Review Handoff: Local Draft Supersede

## Scope

This change implements local-only draft supersede for markdown note proposals.

It does not expose supersede through external MCP.

## Product Boundary

- External MCP remains read + proposal intake.
- External MCP still must not expose draft approve/apply/supersede/refine, shell, workspace write/edit, or `vault_write_low`.
- Supersede is a local Lore review capability: Lore can create a revised pending draft while preserving the original external proposal as immutable evidence.

## Implementation Notes

- Added `model.DraftSupersedeUpdate`.
- Added `DraftStore.SupersedeDraft(oldID, newDraft, updatedAt)` for atomic old-draft superseded + new-draft creation.
- Implemented the store method for memory, JSON, and SQLite stores.
- Added `Harness.SupersedeDraft`.
- Supersede currently supports only `DraftKindMarkdownNoteWrite`.
- The revised draft recomputes the target base version, including `DraftBaseVersionNewFile` for new paths.
- Unsafe targets are revalidated with the same low-governance markdown target guard used by proposal/apply.
- Added local console tool `draft_supersede`; it is explicitly local-only and not MCP.

## Review Focus

- Confirm the original draft transitions to `superseded` only together with successful creation of the revised draft.
- Confirm failed supersede attempts do not mutate the original draft.
- Confirm original `ProposedContent` remains unchanged.
- Confirm revised drafts have `Supersedes = oldDraft.ID` and start in `pending_review`.
- Confirm MCP tool allowlist still excludes `draft_supersede`, `draft_refine`, `draft_apply`, and direct writes.
- Confirm supersede does not support persona/progress drafts in this MVP.

## Verification

Passed:

```powershell
.\.tools\go\bin\go.exe test ./internal/orchestrator -run "TestSupersedeMarkdownNoteDraft|TestSupersedeDraftRejects|TestApplyMarkdownNoteDraft" -count=1 -v
.\.tools\go\bin\go.exe test ./internal/console ./internal/operatoragent ./internal/mcp -count=1
.\.tools\go\bin\go.exe test ./internal/model ./internal/domain/drafts ./internal/store/... ./internal/orchestrator ./internal/app ./internal/console ./internal/operatoragent ./internal/mcp -count=1
```
