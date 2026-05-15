# Review Handoff: MCP No Direct Write Boundary

## Scope

This change strengthens tests around the external MCP tool surface.

Changed:

- `internal/mcp/server_test.go`

Not changed:

- MCP server behavior.
- MCP tool contracts.
- SDK artifacts.
- Local console/runtime tool surface.

## Decision Captured

External MCP may expose read tools and narrow proposal intake, but it must not expose direct write, shell, generic workspace edit, draft approve/apply, or generic proposal tools.

## Review Focus

- Confirm the test blocks `vault_write_low`.
- Confirm the test blocks shell and generic workspace write/edit tool names.
- Confirm the test blocks draft approve/apply/supersede/refine tool names.
- Confirm the test does not block existing read tools or proposal-intake tools such as `persona_update_propose` and `markdown_note_propose`.

## Suggested Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp -run "TestMCPV1ExposesOnlyReadAndProposalTools|TestExternalMCPDoesNotExposeDirectWrites" -count=1 -v
```
