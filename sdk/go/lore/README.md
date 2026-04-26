# Lore Go SDK v0

Experimental Go SDK for the Lore MCP read-only surface.

The SDK talks to Lore by starting `lore mcp <workdir>` over stdio JSON-RPC. It does not import Lore `internal/*` packages and does not bypass runtime governance.

## Status

- Version: v0, in-repository module.
- Transport: stdio only.
- Surface: read-only MCP tools only.
- Writable tools, shell execution, TUI behavior, and runtime policy are not part of this SDK.

## Install

The final public module path is not frozen yet. For local development inside this repository:

```powershell
Push-Location sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -count=1
Pop-Location
```

## Quickstart

```go
ctx := context.Background()
client, err := lore.Start(ctx, lore.Options{
    Command: "lore",
    WorkDir: "./tmp/chat-demo",
    ClientKey: os.Getenv("LORE_CLIENT_KEY"),
})
if err != nil {
    return err
}
defer client.Close()

if err := client.Ping(ctx); err != nil {
    return err
}

resolved, err := client.VaultResolve(ctx, lore.VaultResolveRequest{
    Query: "persona",
    Limit: 5,
})
if err != nil {
    return err
}
fmt.Println(resolved.Status)
```

`ClientKey` is sent to the child process as `LORE_CLIENT_KEY`. It is distinct from the server-side `LORE_MCP_API_KEY` expected by `lore mcp` when process auth is enabled.

## Typed Methods

- `ManagedStatus(ctx)`
- `SystemDocGet(ctx, SystemDocGetRequest)`
- `VaultRead(ctx, VaultReadRequest)`
- `VaultList(ctx, VaultListRequest)`
- `VaultSearchText(ctx, VaultSearchTextRequest)`
- `VaultResolve(ctx, VaultResolveRequest)`
- `VaultBacklinks(ctx, VaultBacklinksRequest)`
- `ContextPack(ctx, ContextPackRequest)`
- `DocClassify(ctx, DocClassifyRequest)`

A lower-level `CallTool(ctx, name, arguments, out)` is available for read-only forward-compatible use.

## Argument Contract

The SDK only sends standard argument names.

| Tool | SDK args | Deprecated MCP aliases still accepted by server |
| --- | --- | --- |
| `managed_status` | none | none |
| `system_doc_get` | `name` | none |
| `vault_read` | `path` | none |
| `vault_list` | `dir` | `path` alias for `dir` |
| `vault_search_text` | `query`, `dir`, `limit` | `path` alias for `dir` |
| `vault_resolve` | `query`, `dir`, `limit` | `path` alias for `dir` |
| `vault_backlinks` | `path`, `limit` | none |
| `context_pack` | `target_path`, `task`, `limit` | `path` alias for `target_path` |
| `doc_classify` | `path` | none |

## Errors

The SDK exposes separate error types:

- `TransportError`: process, pipe, or frame transport failures.
- `JSONRPCError`: JSON-RPC response contains an `error` object.
- `ToolError`: MCP `tools/call` returns `isError=true`.
- `DecodeError`: response JSON cannot be decoded into the requested DTO.

Context cancellation/deadline errors are returned as context errors. If a call is canceled while reading stdout, the client closes the transport instead of risking stream corruption; create a new client for later calls.

Example:

```go
var toolErr *lore.ToolError
if errors.As(err, &toolErr) {
    log.Printf("tool %s failed: %s", toolErr.Tool, toolErr.Message)
}
```

## E2E Smoke

The SDK includes an opt-in end-to-end test that builds the real `lore` CLI and starts `lore mcp <workdir>`:

```powershell
Push-Location sdk\go\lore
$env:LORE_SDK_E2E='1'
..\..\..\.tools\go\bin\go.exe test ./... -run TestSDKEndToEndWithLoreMCP -count=1 -v
Pop-Location
```

Default `go test ./...` skips this E2E test.

## Compatibility

- SDK v0 targets the MCP contract documented in `docs/adr-sdk-go-v0.md`.
- The MCP server keeps deprecated `path` aliases for compatibility, but SDK methods do not send them.
- HTTP and WebSocket transports are intentionally out of scope for v0.
