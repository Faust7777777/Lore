# Review Handoff: MCP SDK Contract Golden Snapshot

## Change

The SDK-facing MCP contract snapshot moved from an inline Go literal to a JSON golden file:

- `internal/mcp/testdata/sdk_tool_contract_snapshot.json`
- `internal/mcp/server_test.go`
- `docs/adr-sdk-go-v0.md`

## Reason

The inline snapshot already protected the external contract, but it was not reusable by future non-Go SDK work. A JSON golden file makes the SDK-facing contract visible as data while keeping the SDK boundary intact.

## What Did Not Change

- MCP tool names did not change.
- `tools/list` generation still comes from `internal/mcp/tool_contract.go`.
- Deprecated aliases and standard argument precedence did not change.
- SDK still does not import `internal/*`.
- No TUI or runtime governance behavior changed.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp -run TestSDKFacingToolContractSnapshot -count=1 -v
.\scripts\verify.ps1
```

## Review Focus

- Confirm the JSON golden exactly preserves the previous inline snapshot.
- Confirm `TestSDKFacingToolContractSnapshot` still checks exact tool count, exact properties, exact required fields, deprecated aliases, and forbidden properties.
- Confirm the ADR no longer lists P1.5 contract centralization as an unresolved question.
