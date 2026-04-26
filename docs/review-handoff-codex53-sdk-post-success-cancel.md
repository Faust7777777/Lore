# Review Handoff: SDK Post-Success Cancel Race

## Scope

This change tightens the previous stdio cancellation hardening after review found a remaining race.

Changed files:

- `sdk/go/lore/transport_stdio.go`
- `sdk/go/lore/transport_stdio_test.go`

## Issue

The previous implementation used a watcher goroutine:

```go
select {
case <-ctx.Done():
    close transport
case <-done:
}
```

There was a small race where the call could successfully read a response, then caller cleanup could cancel the context before the watcher observed `done`, causing the watcher to close an otherwise healthy transport.

## Fix

`readResponse` now races the read result and context cancellation from the main call path:

- A single goroutine performs the blocking stdout read.
- The caller selects between `read result` and `ctx.Done`.
- If the read result wins, no cancellation watcher remains that can later close the transport.
- If cancellation wins, the transport is closed to unblock the reader and the call returns the context error.

## Test

Added `TestCancelAfterSuccessfulCallDoesNotCloseTransport`:

- First call succeeds with a cancellable context.
- The context is canceled after success.
- A second call must still succeed, proving post-success cancellation does not close the transport.

Verification commands:

```powershell
Push-Location .\sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; Pop-Location
.\.tools\go\bin\go.exe test ./... -count=1
```
