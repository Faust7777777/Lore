# Review Handoff: A-Line TUI Panel Viewport Heights

Date: 2026-05-23

## Scope

- `internal/tui/interactive_workbench.go`
- `internal/tui/interactive_render.go`
- `internal/tui/panel_state_test.go`

## Summary

Makes right-bottom panel height ownership explicit for the TUI workbench.

Before this slice, findings and process-sink list offset clamping read `approvalViewport.Height`. That coupled three separate panel state machines to the approval viewport implementation detail. A future layout change could make findings/process-sink scrolling clamp against the wrong height.

New behavior:

- `approvalHeight`, `findingsHeight`, and `sinkHeight` are separate fields on `interactiveWorkbenchModel`.
- `resize()` assigns each panel its own visible content height.
- Approval/findings/sink offset clamps read their own panel height.
- Render dispatch passes each panel its own configured visible height, with a pure fallback for older test construction.
- Sink clamp now reserves the same two rows as `renderSinkTimelineList` (`report`/position rows), keeping clamp math aligned with rendering.

## Boundary

- No runtime/operatoragent/console behavior change.
- No persona/MCP/governance change.
- No visual copy change.
- This stays inside TUI shell/state and render plumbing; it does not add a new product view.

## Regression Tests

- `TestFindingsOffsetUsesFindingsPanelHeight`
- `TestSinkOffsetUsesSinkPanelHeight`
- `TestApprovalOffsetUsesApprovalPanelHeight`

These tests intentionally set `approvalViewport.Height` to a conflicting value and assert each panel still clamps against its own height.

## Validation

```powershell
go test ./internal/tui -run "PanelHeight|OffsetUses|ApprovalFlow|Findings|Sink|RenderInteractive" -count=1 -v
go test ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
