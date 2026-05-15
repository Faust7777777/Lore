# Review Handoff: Exact Required Snapshot Check

## Scope

This change tightens the MCP SDK-facing contract snapshot test.

Changed files:

- `internal/mcp/server_test.go`
- `docs/review-handoff-codex53-mcp-required-snapshot.md`

## Change

`assertRequired` now checks exact set equality:

- required count must match expected count;
- every expected required field must be present.

Previously it only checked that expected fields were present, so an accidental extra `required` field could pass the snapshot test.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp -run TestSDKFacingToolContractSnapshot -count=1 -v
.\.tools\go\bin\go.exe test ./internal/mcp ./internal/console ./internal/operatoragent -count=1
.\scripts\verify.ps1
```

All passed.
