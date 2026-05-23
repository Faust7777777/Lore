# Lore Go SDK v0

Experimental Go SDK for Lore MCP typed read-only methods, with a low-level raw MCP escape hatch.

The SDK talks to Lore by starting `lore mcp <workdir>` over stdio JSON-RPC. It does not import Lore `internal/*` packages and does not bypass runtime governance.

## Status

- Version: v0, in-repository module. See `CHANGELOG.md` for the current unreleased preview notes.
- Transport: stdio only.
- Surface: typed methods cover read-only MCP tools only.
- `CallTool` exposes the live raw MCP tool surface and may include proposal-intake tools depending on the Lore server version.
- Direct vault-write typed methods, shell execution, TUI behavior, draft approve/apply, and runtime policy changes are not part of this SDK.

## Install

The final public module path is not frozen yet. For local development inside this repository:

```powershell
Push-Location sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -count=1
Pop-Location
```

## Quickstart

The target workdir must already be bootstrapped, for example with `lore bootstrap ./tmp/chat-demo`.

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

`Options.MaxFrameBytes` optionally overrides the maximum stdio response frame accepted from the child process. The default is `lore.DefaultMaxFrameBytes` (10 MiB). Oversized `Content-Length` values fail before payload allocation.

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

A lower-level `CallTool(ctx, name, arguments, out)` is available for raw MCP calls. Unlike the typed methods, it is not restricted to the SDK v0 read-only contract; inspect the live server `tools/list` before calling server-version-specific proposal-intake tools.

## Argument Contract

Typed methods only send standard argument names. The lower-level `CallTool` accepts caller-provided arguments and is responsible for compatibility.

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
- `FrameSizeError`: stdio frame `Content-Length` exceeds the configured maximum; it is wrapped by `TransportError`.
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

The SDK includes an opt-in end-to-end test that builds the real `lore` CLI and starts `lore mcp <workdir>`. From the repository root:

```powershell
.\scripts\verify.ps1 -E2E
```

Default `go test ./...` and `.\scripts\verify.ps1` skip this E2E test.

## Compatibility

- SDK v0 targets the MCP contract documented in `docs/adr-sdk-go-v0.md`.
- The SDK-facing MCP tool contract artifact is `docs/contracts/mcp-sdk-tools-v0.json`.
- The MCP server keeps deprecated `path` aliases for compatibility, but SDK methods do not send them.
- HTTP and WebSocket transports are intentionally out of scope for v0.
