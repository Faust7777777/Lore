# Review Handoff: SDK Stdio Transport Hardening

## Scope

This change addresses two review findings in the Go SDK stdio transport.

Changed files:

- `sdk/go/lore/transport_stdio.go`
- `sdk/go/lore/transport_stdio_test.go`

## Fixes

### 1. No abandoned stdout reader after context cancellation

Previous behavior:

- `readResponse(ctx)` spawned a goroutine to read from stdout.
- If `ctx` was canceled first, `Call` returned while the read goroutine could continue reading the shared `bufio.Reader`.
- A later call could then race with the abandoned reader and corrupt request/response ordering.

New behavior:

- The active `Call` remains the only goroutine that reads stdout.
- A cancellation watcher only closes the underlying stdout pipe / process to unblock the active read.
- If a call is canceled, the transport is marked closed before returning, so the client cannot reuse a potentially partial stream.

### 2. Start context no longer owns child process lifetime

Previous behavior:

- `startStdioTransport` used `exec.CommandContext(ctx, ...)`.
- A caller using a short startup timeout could accidentally bind that context to the MCP child process lifetime.

New behavior:

- `startStdioTransport` checks `ctx.Err()` before start.
- The child is launched with `exec.Command(...)`.
- The client owns the process lifetime through `Close()`.

## Tests

Added coverage:

- `TestStartStdioTransportHonorsPreCanceledContext`
- `TestCallCanceledWhileReadingClosesTransport`

Verification commands:

```powershell
Push-Location .\sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; Pop-Location
.\.tools\go\bin\go.exe test ./... -count=1
```

## Review Focus

Please check:

- Cancellation no longer leaves a live goroutine reading stdout after `Call` returns.
- Closing stdout/process on cancellation is an acceptable P1 tradeoff: canceled calls make the client unusable rather than risking stream corruption.
- `Start` context is only used to gate startup, not child process lifetime.
