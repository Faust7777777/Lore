# Review Handoff: Lore v1 Architecture ADR

## Scope

Added `docs/adr-lore-v1-architecture.md`.

This is architecture documentation only. It does not implement MCP write tools, persona proposal tools, CoreContext, daemon changes, or TUI changes.

## Decisions Captured

- `agent.md` is frozen as the external agent operating manual.
- `identity.md` is frozen as Lore self identity.
- MCP v1 moves from read-only toward graded write, but still forbids shell and governed direct writes.
- Write levels are L0 read, L1 proposal intake, L2 low-risk direct write, L3 governed apply.
- MCP v1 may expose L2 `vault_write_low` later, but must continue blocking governed paths through runtime validation.
- `persona_update_propose` is the first planned L1 proposal tool; generic `proposal_submit` is explicitly deferred.
- Shell/file writes made outside Lore are handled by daemon/post-scan as reconciliation, not as implicit approval.

## Review Focus

- Check that the ADR does not imply MCP shell, MCP governed apply, or generic workspace write.
- Check that L2 low-risk write is narrow enough to reuse current `WriteLowRiskNote` governance.
- Check that `agent.md` and `identity.md` audience definitions are internally consistent.
- Check that persona update remains draft/review/apply and is not silently persisted.
- Check that TUI is not in scope.

## Current Implementation Facts To Compare

- MCP contract is currently still read-only in `internal/mcp/tool_contract.go`.
- Local console tool runtime already has `vault_write_low`.
- Runtime already rejects governed paths in `WriteLowRiskNote`.
- Daemon already scans vault changes and can call `ObserveDocumentChange`.
- `DraftKindPersonaUpdate` exists, but `applyDraftPatch` currently only supports `progress_sync`.

## Suggested Verification

Docs-only targeted checks:

```powershell
git diff --check
```

Optional regression:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```
