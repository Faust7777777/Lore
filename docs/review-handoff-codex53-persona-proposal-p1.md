# Review Handoff: Persona Update Proposal Intake P1

## Scope

This change implements the first MCP L1 proposal-intake tool:

- `persona_update_propose`

Changed:

- `internal/model/draft.go`
- `internal/orchestrator/harness.go`
- `internal/orchestrator/harness_test.go`
- `internal/mcp/tool_contract.go`
- `internal/mcp/server.go`
- `internal/mcp/server_test.go`
- `docs/integrations/mcp-client-setup.md`
- `docs/adr-lore-v1-architecture.md`

Not changed:

- No MCP shell.
- No generic proposal API.
- No generic workspace write/edit.
- No MCP `vault_write_low`.
- No MCP draft approve/apply.
- No persona apply implementation.
- No SDK typed method for proposal intake.

## Contract

`persona_update_propose` input:

- `field` required
- `proposed_value` required
- `evidence` required
- `confidence` required; enum: `low`, `medium`, `high`
- `reason` required
- `source` required
- `observed_at` required; RFC3339 or `YYYY-MM-DD`
- `current_value` optional

Output:

- `status: "draft_created"`
- `draft_id`
- `target`
- `review_required: true`

## Boundary

This is L1 proposal intake only.

It creates a pending `DraftKindPersonaUpdate` draft and stores the proposal payload as JSON in `Draft.ProposedContent`.

It does not write or apply the persona document. The persona document remains byte-for-byte unchanged after proposal creation.

`vault_write_low` remains L2 direct write and is still not exposed through MCP.

The SDK v0 contract artifact `docs/contracts/mcp-sdk-tools-v0.json` remains the read-only SDK-facing contract. Raw MCP `tools/list` now includes proposal intake in addition to the SDK v0 read-only subset.

## Review Focus

- Confirm `persona_update_propose` cannot write the persona document.
- Confirm invalid proposals do not create drafts.
- Confirm missing or invalid `observed_at` returns a tool error.
- Confirm `confidence` is restricted to `low`, `medium`, or `high`.
- Confirm `tools/list` exposes no shell, no generic write, no draft apply/approve, and no `vault_write_low`.
- Confirm the v0 SDK artifact remains read-only and SDK tests still pass.
- Confirm the external setup doc no longer says proposal intake is unavailable.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/orchestrator ./internal/mcp -count=1
pushd sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; popd
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```
