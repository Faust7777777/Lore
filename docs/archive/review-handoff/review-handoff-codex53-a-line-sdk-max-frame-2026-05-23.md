# Review Handoff: A-Line SDK Max Frame Guard

Date: 2026-05-23

## Scope

- `sdk/go/lore/client.go`
- `sdk/go/lore/errors.go`
- `sdk/go/lore/errors_test.go`
- `sdk/go/lore/transport_stdio.go`
- `sdk/go/lore/transport_stdio_test.go`
- `sdk/go/lore/README.md`
- `sdk/go/lore/CHANGELOG.md`

## Reading Gate

Read before implementation:

- `docs/handoff-full-project-review-2026-05-23.md`
- `docs/review-handoff-index.md`
- `docs/archive/review-handoff/review-handoff-codex53-sdk-release-gate.md`
- `docs/archive/review-handoff/review-handoff-codex53-verify-e2e-switch.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-sdk-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-infra-handover.md`

## Summary

Adds an SDK-side stdio response frame size cap.

Before this change, `readFrame` trusted `Content-Length` and allocated exactly that many bytes. A bad or corrupt MCP server stream could advertise a very large response and trigger excessive allocation before the SDK could fail safely.

New behavior:

- `DefaultMaxFrameBytes = 10 MiB`
- `Options.MaxFrameBytes` can override the cap per client
- `readFrameWithLimit` rejects oversized `Content-Length` before payload allocation
- `FrameSizeError` reports `ContentLength` and `MaxBytes`
- `TransportError` wraps `FrameSizeError` through normal error chaining

## Boundary

- No SDK E2E default behavior change.
- No MCP live tool surface change.
- No typed SDK read-only surface expansion.
- No runtime/operator/TUI behavior change.

## Review Focus

- Confirm zero/negative `Options.MaxFrameBytes` preserves the default cap.
- Confirm oversized frames fail before reading/allocating payload bytes.
- Confirm callers can match `FrameSizeError` through `errors.As`.
- Confirm README/CHANGELOG describe the new option and error type.

## Validation

```powershell
Push-Location .\sdk\go\lore
go test ./... -count=1
Pop-Location
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
