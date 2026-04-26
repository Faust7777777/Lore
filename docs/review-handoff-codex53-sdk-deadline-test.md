# Review Handoff: SDK Deadline Cancellation Test

## Change

Added `TestCallDeadlineWhileReadingClosesTransport` in `sdk/go/lore/transport_stdio_test.go`.

The test covers a call whose context deadline expires while the SDK is blocked reading stdout from the MCP subprocess.

## Reason

SDK docs state that context cancellation and deadline errors are returned as context errors, and that the transport closes after an interrupted read to avoid stdio stream corruption. Existing tests covered manual cancellation but not `context.DeadlineExceeded`.

## Expected Behavior

- The blocked call returns an error matching `context.DeadlineExceeded`.
- The transport is closed after the timed-out read.
- A later call on the same client fails with `ErrClosed`.

## What Did Not Change

- SDK implementation is unchanged; this is regression coverage only.
- MCP contract is unchanged.
- No runtime, TUI, or governance behavior changed.

## Verification

```powershell
Push-Location .\sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -count=3
Pop-Location
.\scripts\verify.ps1
```

## Review Focus

- Confirm the test locks `context.DeadlineExceeded`, not a generic transport error.
- Confirm the transport-closed assertion matches the existing cancellation safety policy.
- Confirm the 10ms timeout is not flaky on Windows in local runs.
