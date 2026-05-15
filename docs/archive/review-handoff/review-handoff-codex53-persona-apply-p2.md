# Review Handoff: Persona Update Apply P2

## Scope

This change implements local reviewed apply for `DraftKindPersonaUpdate`.

Changed:

- `internal/orchestrator/harness.go`
- `internal/orchestrator/harness_test.go`
- `internal/bootstrap/templates.go`
- `internal/bootstrap/templates_test.go`
- `docs/adr-lore-v1-architecture.md`

Not changed:

- No MCP draft approve/apply.
- No MCP direct persona write.
- No MCP `vault_write_low`.
- No generic proposal API.
- No field-level persona document rewrite.
- No SDK typed method for proposal or apply.

## Apply Semantics

`persona_update_propose` still only creates a pending draft.

Local runtime apply now supports approved `persona_update` drafts:

- `ApplyDraft` still requires `DraftApproved`.
- Base-version conflict detection remains unchanged.
- The persona proposal JSON is decoded from `Draft.ProposedContent`.
- Apply appends a structured record under `## Applied Persona Updates`.
- Existing persona content is preserved and not rewritten.
- Malformed persona proposal payloads return `ErrInvalidDraftPatch`.

The appended record includes:

- `field`
- optional `current_value`
- `proposed_value`
- `evidence`
- `reason`
- `confidence`
- `source`
- `observed_at`
- `draft_id`

## Boundary

This is local Lore/runtime governed apply only.

MCP still exposes only read tools plus `persona_update_propose`. It does not expose draft approve/apply or direct persona writes.

The append-only format is intentionally conservative. It closes the external-agent handoff loop without trying to perform brittle freeform edits to `人物画像.md`.

## Review Focus

- Confirm pending persona drafts cannot apply before approval.
- Confirm approved persona drafts append a structured record and preserve existing persona content.
- Confirm external persona edits after proposal creation trigger conflict instead of overwriting.
- Confirm MCP tool allowlist remains read tools + `persona_update_propose` only.
- Confirm `agent.md` template no longer claims MCP is purely read-only.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/orchestrator ./internal/bootstrap ./internal/mcp -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```
