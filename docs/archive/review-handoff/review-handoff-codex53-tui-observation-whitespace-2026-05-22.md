# Review Handoff: TUI Observation Whitespace

Date: 2026-05-22

Owner line: TUI presentation

## Scope

Small presentation-only fix for task-step observation excerpts.

Changed files:

- `internal/tui/interactive_render.go`
- `internal/tui/interactive_render_test.go`
- `docs/archive/review-handoff/review-handoff-codex53-tui-observation-whitespace-2026-05-22.md`

## Problem

`renderObservationExcerpt` only replaced `\n` with spaces. Windows CRLF,
tabs, and repeated spaces could leak into the one-line task-step observation
display.

## Fix

Use `strings.Fields` + `strings.Join` to collapse all whitespace runs into a
single space before rune-safe truncation.

## Validation

```powershell
go test ./internal/tui -run "TestRenderObservationExcerpt" -count=1 -v
git diff --check -- internal/tui/interactive_render.go internal/tui/interactive_render_test.go docs/archive/review-handoff/review-handoff-codex53-tui-observation-whitespace-2026-05-22.md
```

## Review Focus

- Confirm this stays presentation-only.
- Confirm marker preservation and rune-safe truncation tests still pass.
