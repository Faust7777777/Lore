# Review Handoff: A-Line Release Gate Hardening Refresh

Date: 2026-05-23

## Scope

- `scripts/release-gate.ps1`
- `README.md`

## Summary

Extends the deterministic A-line release gate to cover the new hardening tests added in the latest slices.

New targeted coverage:

- Operatoragent: bounded tool-result reinjection and turn-level cancellation before model call, during model call, and during tool call.
- Console: `HandleContext` pre-cancel path and context-aware tool runtime pre-dispatch cancellation.
- TUI: `View()` purity plus approval/findings/process-sink panel viewport height ownership tests.

The SDK oversized-frame blocker fix is already covered by the SDK module gate, which runs `go test ./...` inside `sdk/go/lore`.

## Boundary

- No product behavior change.
- No full/real-model gate dependency added to PR path.
- No MCP/persona/governance surface change.
- README release-gate wording now matches the expanded targeted set.

## Validation

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
