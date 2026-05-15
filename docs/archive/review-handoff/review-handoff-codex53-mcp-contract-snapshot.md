# Review Handoff: MCP SDK Contract Snapshot

## Scope

This change adds an external-contract snapshot test after the MCP tool contract was centralized.

Changed files:

- `internal/mcp/server_test.go`
- `docs/review-handoff-codex53-mcp-contract-snapshot.md`

## Why

`TestToolDefinitionsExposeSDKContract` now intentionally uses `toolContracts()` as its source. That verifies contract-to-schema generation, but it cannot catch accidental edits to the canonical contract itself.

This change adds `TestSDKFacingToolContractSnapshot`, a hand-written allowlist of the SDK-facing MCP contract.

## What It Locks

The snapshot test locks:

- tool count and tool names;
- exact exposed property names;
- required fields;
- deprecated alias mapping.

It specifically protects the SDK-facing standard args from regressing:

- `vault_list.dir`
- `vault_search_text.dir`
- `vault_resolve.dir`
- `context_pack.target_path`

## Test Split

- `TestToolDefinitionsExposeSDKContract`: verifies canonical contract generation into `tools/list` schema.
- `TestSDKFacingToolContractSnapshot`: freezes the external SDK-facing contract.
- `TestMCPDeprecatedAliasesRemainSupported`: verifies alias-only runtime behavior.
- `TestMCPStandardArgsOverrideDeprecatedAliases`: verifies standard-over-alias runtime behavior.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp ./internal/console ./internal/operatoragent -count=1
.\.tools\go\bin\go.exe test ./... -count=1
```

Both passed.
