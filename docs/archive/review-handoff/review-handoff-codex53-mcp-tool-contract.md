# Review Handoff: MCP Tool Contract Single Source

## Scope

This change implements the P1.5 SDK contract source-of-truth cleanup.

Changed files:

- `internal/mcp/tool_contract.go`
- `internal/mcp/server.go`
- `internal/mcp/server_test.go`

## What Changed

Added canonical MCP read-only tool contracts in `internal/mcp/tool_contract.go`.

The contract captures:

- tool name;
- description;
- arguments;
- required fields;
- enum values;
- deprecated alias target.

`tools/list` now generates MCP schemas from these contracts instead of a hand-written map in `server.go`.

## What Did Not Change

- MCP exposed tool names remain the same.
- `dir` / `target_path` standard args remain the SDK-facing names.
- Deprecated `path` aliases remain advertised and accepted where intended.
- Alias behavior tests remain separate and still exercise runtime behavior.
- SDK still does not import `internal/*`.
- No TUI or runtime governance changes.

## Tests

`TestToolDefinitionsExposeSDKContract` now derives expected tools/properties/required/alias descriptions from `toolContracts()`, so `server.go` and `server_test.go` no longer maintain separate hand-written tool parameter lists.

Behavior tests remain:

- `TestMCPDeprecatedAliasesRemainSupported`
- `TestMCPStandardArgsOverrideDeprecatedAliases`

Verification commands:

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp ./internal/console ./internal/operatoragent -count=1
.\.tools\go\bin\go.exe test ./... -count=1
Push-Location .\sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; Pop-Location
```

## Review Focus

Please check:

- `tools/list` output shape is unchanged.
- Contract generation preserves `required` arrays and deprecated alias descriptions.
- Behavior tests still cover alias-only and standard-over-alias execution paths.
