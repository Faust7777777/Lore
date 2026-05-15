# Review Handoff: MCP SDK Contract Alignment

## Scope

This change implements P0 from `docs/adr-sdk-go-v0.md`: align the MCP read-only tool contract before the Go SDK is built.

Changed files:

- `internal/mcp/server.go`
- `internal/mcp/server_test.go`

## Contract Changes

### `vault_list`

- Advertised standard arg is now `dir`.
- `path` remains accepted as a deprecated alias for compatibility.
- If both are present, `dir` wins.

### `context_pack`

- Advertised standard arg is now `target_path`.
- `path` remains accepted as a deprecated alias for compatibility.
- If both are present, `target_path` wins.

### Existing behavior preserved

- `vault_search_text` and `vault_resolve` continue using `dir` with `path` as a deprecated alias.
- The MCP surface remains read-only.
- No TUI, runtime governance, or SDK code was added in this change.

## Tests

Added/updated tests in `internal/mcp/server_test.go`:

- `TestToolDefinitionsExposeSDKContract` locks the SDK-facing MCP tool set and key input properties.
- It checks deprecated alias descriptions for `vault_list`, `vault_search_text`, `vault_resolve`, and `context_pack`.
- `TestMCPStandardArgsOverrideDeprecatedAliases` verifies standard args override aliases for `vault_list` and `context_pack`.

Verification run:

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp ./internal/console ./internal/operatoragent -count=1
.\.tools\go\bin\go.exe test ./... -count=1
```

## Review Focus

Please check:

- `tools/list` advertises standard names intended for SDK v0.
- Existing callers using `path` still work for `vault_list` and `context_pack`.
- Standard-over-alias precedence is correct and test-covered.
- No writable tools were added to MCP.
