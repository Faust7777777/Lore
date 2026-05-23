# Review Handoff: A-Line TUI View Purity

Date: 2026-05-23

## Scope

- `internal/tui/interactive_workbench.go`
- `internal/tui/interactive_workbench_purity_test.go`

## Reading Gate

Read before implementation:

- `AGENTS.md`
- `docs/handoff-full-project-review-2026-05-23.md`
- `docs/archive/OPUS_COLLAB_GUARDRAILS.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\tui-module-handoff.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-console-llm-handover.md`

## Summary

Removes model-derived content refresh from Bubble Tea `View()`.

Before this change, `View()` called both `resize()` and `refreshContent(false)`. That meant rendering rebuilt viewport content from the latest view model, which violates the intended Bubble Tea separation where `Update()` mutates model state and `View()` renders the already-prepared state.

New behavior:

- `newInteractiveWorkbenchModel` performs initial `resize()` before the first `refreshContent(true)`.
- `WindowSizeMsg` and existing update/action paths remain responsible for refreshes.
- `View()` only calls `renderInteractiveWorkbenchLayout(m)`.
- Regression test proves `View()` does not pick up model-derived content changes unless an update/init refresh path ran.

## Boundary

- TUI shell/render lifecycle only.
- No runtime/governance change.
- No MCP surface change.
- No operator-agent behavior change.
- No persona state-machine change.

## Review Focus

- Confirm initial layout is still prepared before first render.
- Confirm `View()` no longer calls mutating helpers.
- Confirm repeated or direct `View()` calls cannot refresh stale model-derived content by themselves.

## Validation

```powershell
go test ./internal/tui -run "TestInteractiveWorkbenchViewDoesNotRefreshContent|TestRenderInteractiveConversationShowsTaskSteps|TestApprovalFlow" -count=1 -v
go test ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
