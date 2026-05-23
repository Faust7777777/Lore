# Review Handoff: A-Line SDK Max Frame Round 2

Date: 2026-05-23

## Scope

- `sdk/go/lore/transport_stdio.go`
- `sdk/go/lore/transport_stdio_test.go`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-sdk-max-frame-2026-05-23.md`

## Summary

Fixes the round-2 blocker in the SDK oversized-frame guard.

The first max-frame slice rejected an oversized `Content-Length` before allocation, but it left the transport usable. That is unsafe: the stdio reader is no longer synchronized with frame boundaries, so a later call could read bytes or a normal frame that followed the rejected header.

New behavior:

- `FrameSizeError` marks the transport closed.
- The transport closes stdin/stdout and kills/waits the child process when present.
- Later SDK calls return `ErrClosed`.
- Existing context-cancellation behavior remains unchanged.

## Regression Test

`TestCallClosesTransportAfterOverLimitFrameWithResidualStream` builds a stream with:

1. an oversized frame header;
2. a valid normal frame after it.

The first call must return `FrameSizeError`; the second call must return `ErrClosed`, proving the SDK does not continue reading the residual stream.

## Boundary

- No MCP live surface change.
- No SDK typed method surface change.
- No default SDK E2E behavior change.
- No runtime/operator/TUI behavior change.

## Validation

```powershell
Push-Location .\sdk\go\lore
go test ./... -count=1 -v
Pop-Location
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
