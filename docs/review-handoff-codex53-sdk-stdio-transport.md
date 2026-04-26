# Review Handoff: Go SDK v0 Stdio Transport

## Scope

This change starts P1 from `docs/adr-sdk-go-v0.md`: scaffold the Go SDK module and implement the base stdio JSON-RPC transport.

Changed areas:

- `sdk/go/lore/go.mod`
- `sdk/go/lore/client.go`
- `sdk/go/lore/transport_stdio.go`
- `sdk/go/lore/types.go`
- `sdk/go/lore/errors.go`
- `sdk/go/lore/transport_stdio_test.go`
- `internal/mcp/server_test.go` adds explicit alias-only compatibility coverage from the prior review note.

## SDK Boundary

The SDK module does not import `internal/*`. It talks to Lore by starting `lore mcp <workdir>` and using MCP-style stdio frames.

Implemented:

- `Start(ctx, Options)`
- `Close()`
- `Initialize(ctx)`
- `Ping(ctx)` with JSON-RPC `method not found` fallback to initialize for older servers
- `Tools(ctx)`
- generic `CallTool(ctx, name, args, out)`
- Content-Length frame read/write
- JSON-RPC error handling
- tool `isError=true` handling
- structuredContent decode handling

## Error Layers Covered

- `TransportError`
- `JSONRPCError`
- `ToolError`
- `DecodeError`

Context cancellation currently returns the context error directly from the call path. This matches the ADR error layer but does not yet wrap it in a dedicated timeout/cancel type.

## Tests

SDK tests cover:

- frame parsing
- JSON-RPC error decoding
- tool business error mapping
- structuredContent decode success
- structuredContent decode failure

MCP compatibility test added:

- `TestMCPDeprecatedAliasesRemainSupported` verifies `vault_list(path=...)` and `context_pack(path=...)` still work.

Verification commands:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
Push-Location .\sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; Pop-Location
```

## Review Focus

Please check:

- SDK module has no dependency on root `internal/*` packages.
- Stdio framing is compatible with the MCP server implementation.
- `Close` process cleanup is acceptable for Windows P1; E2E lifecycle hardening remains P3.
- Error layer names are acceptable before typed tool DTOs are added in P2.
