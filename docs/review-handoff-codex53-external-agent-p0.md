# Review Handoff: External Agent Proposal Intake P0

## Scope

This is the P0 semantic/documentation pass for external agent proposal intake.

Changed:

- `internal/bootstrap/templates.go`
- `internal/bootstrap/templates_test.go`
- `internal/orchestrator/readapi_test.go`
- `docs/integrations/mcp-client-setup.md`
- `docs/adr-lore-v1-architecture.md`

Not changed:

- MCP server behavior.
- MCP tool contract artifact.
- SDK behavior.
- `persona_update_propose` implementation.
- `vault_write_low` MCP exposure.
- TUI behavior.

## Decisions Captured

- `agent.md` is now the external-first shared agent operating manual.
- `identity.md` remains Lore self identity and is not default onboarding material for external agents.
- External agents should read `system_doc_get("agent")` first after connecting.
- MCP v0 remains read-only.
- MCP v1 direction is staged: read + proposal intake first, low-risk write later.
- The first L1 proposal tool should be `persona_update_propose`, not generic `proposal_submit`.
- `persona_update_propose` may create a pending `persona_update` draft/review item before full persona apply semantics are implemented.
- Proposal creation is not apply and does not update the persona document.
- If proposal tooling is unavailable, external agents should output a structured `Persona Update Candidate` block.

## Review Focus

- Confirm the new `agent.md` template is safe for external agents and does not define Lore self identity.
- Confirm `identity.md` explicitly says it is for local Lore self identity and not default external onboarding.
- Confirm the external MCP doc still states current v0 is read-only and does not imply `persona_update_propose` exists yet.
- Confirm the ADR orders `persona_update_propose` before MCP `vault_write_low`.
- Confirm `vault_write_low` remains L2 direct write and is not described as returning `draft_created`.
- Confirm no MCP shell, generic workspace write, draft approve/apply, or governed direct write is introduced.

## Suggested Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/bootstrap -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```
