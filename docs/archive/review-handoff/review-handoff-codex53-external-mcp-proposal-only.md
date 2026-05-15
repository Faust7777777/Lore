# Review Handoff: External MCP Proposal-Only Boundary

## Scope

This change freezes the external MCP boundary as proposal-only for write-adjacent flows.

Changed:

- `docs/adr-lore-v1-architecture.md`
- `docs/integrations/mcp-client-setup.md`
- `docs/compact-handoff-lore-v1.md`

Not changed:

- MCP server behavior.
- MCP tool contracts.
- SDK behavior.
- Local `vault_write_low` runtime behavior.
- TUI behavior.

## Decisions Captured

- External MCP is an intake surface, not a write surface.
- External MCP v1 is read tools plus narrow proposal intake.
- `vault_write_low` is not on the external MCP roadmap.
- `vault_write_low` may remain local/internal if retained.
- Ordinary markdown writes through Lore should use proposal intake, then local Lore review/supersede/approve/apply.
- Out-of-band shell/file writes remain a daemon/post-scan governance problem, not a prevention boundary.

## Review Focus

- Confirm the ADR no longer says MCP v1 may expose `vault_write_low`.
- Confirm the integration doc does not imply external MCP can write vault markdown directly.
- Confirm `markdown_note_propose` is described as planned proposal intake, not already implemented.
- Confirm no MCP shell, generic workspace write, draft approve/apply, or governed direct write is introduced.
- Confirm supersede/revised-draft language is used instead of in-place refine.

## Suggested Verification

```powershell
git diff --check
```

Optional regression:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```
