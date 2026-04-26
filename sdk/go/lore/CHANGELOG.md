# Changelog

All notable changes to the Lore Go SDK are tracked here.

The SDK is currently experimental v0. The public module path and semantic version tag strategy are not frozen yet.

## v0.1.0-unreleased

Initial in-repository SDK preview.

### Added

- Stdio JSON-RPC transport that starts `lore mcp <workdir>` as a child process.
- `Start`, `Close`, `Initialize`, `Ping`, `Tools`, and generic `CallTool`.
- Typed read-only methods:
  - `ManagedStatus`
  - `SystemDocGet`
  - `VaultRead`
  - `VaultList`
  - `VaultSearchText`
  - `VaultResolve`
  - `VaultBacklinks`
  - `ContextPack`
  - `DocClassify`
- Public DTOs matching the MCP `structuredContent` JSON shapes.
- Error types:
  - `TransportError`
  - `JSONRPCError`
  - `ToolError`
  - `DecodeError`
- Opt-in E2E smoke test gated by `LORE_SDK_E2E=1`.
- README quickstart, argument contract, error handling notes, and E2E command.
- SDK-facing MCP tool contract artifact at `docs/contracts/mcp-sdk-tools-v0.json`.

### Contract

- SDK typed methods send standard argument names only.
- `vault_list` uses `dir`.
- `context_pack` uses `target_path`.
- `vault_search_text` and `vault_resolve` use `dir`.
- The MCP server still accepts deprecated `path` aliases for compatibility where documented.

### Boundaries

- SDK imports no Lore `internal/*` packages.
- SDK is read-only and does not expose writable MCP tools.
- SDK does not alter runtime governance, shell execution, TUI behavior, or cross-session memory behavior.
- Transport is stdio-only; HTTP/WebSocket are out of scope for v0.

### Known Limitations

- Public module path is not frozen.
- Context cancellation during a call closes the transport to avoid stdio stream corruption; create a new client for later calls.
- E2E test is opt-in and builds the local `cmd/lore` binary.
