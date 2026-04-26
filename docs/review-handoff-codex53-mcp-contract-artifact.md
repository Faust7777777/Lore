# Review Handoff: Published MCP SDK Contract Artifact

## Change

The SDK-facing MCP v0 tool contract is now a documented artifact:

- `docs/contracts/mcp-sdk-tools-v0.json`

`TestSDKFacingToolContractSnapshot` reads this artifact directly, so the published contract and the regression test cannot drift.

## Reason

The previous golden file lived under `internal/mcp/testdata`, which was useful for tests but not a natural reference point for future non-Go SDKs. Moving it under `docs/contracts` resolves the ADR open question about publishing a contract artifact without letting SDK code import `internal/*`.

## What Did Not Change

- MCP `tools/list` output is still generated from `internal/mcp/tool_contract.go`.
- Tool names, required fields, properties, and deprecated aliases are unchanged.
- SDK API and transport behavior are unchanged.
- No TUI or runtime governance behavior changed.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp -run TestSDKFacingToolContractSnapshot -count=1 -v
.\scripts\verify.ps1
```

## Review Focus

- Confirm `docs/contracts/mcp-sdk-tools-v0.json` matches the previous snapshot exactly.
- Confirm `server_test.go` reads the published artifact rather than an internal-only copy.
- Confirm ADR and SDK docs no longer say the contract artifact is missing or undecided.
