# A-Line Handoff: Dirty-Tree TUI Task-Step Blocker

## Scope

This note records a blocker found while validating the current collaborative dirty tree after the A-line release-gate updates.

No implementation files are changed by this handoff.

## Current Dirty-Tree Failure

Command:

```powershell
go test ./internal/operatoragent ./internal/console ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1
```

Result:

```text
ok   obsidian-harness/internal/operatoragent
ok   obsidian-harness/internal/console
FAIL obsidian-harness/internal/tui
ok   obsidian-harness/internal/cli
ok   obsidian-harness/cmd/obsidian-harness
```

Failing test:

```text
TestRenderTaskStepsTruncatedMarker
```

Failure shape:

```text
truncated marker should be preserved
```

The rendered observation line shows a generic ellipsis before the original
`[truncated ... bytes]` marker, so the marker is no longer visible.

## Likely Cause

The dirty tree changes `internal/tui/interactive_render.go` to re-truncate
`ObservationExcerpt` during rendering:

```text
excerpt := step.ObservationExcerpt
if len(excerpt) > 200 {
    excerpt = excerpt[:200] + "..."
}
```

This conflicts with the B-line semantics documented in
`docs/plans/2026-05-19-b-line-task-turn-semantics.md`: `ObservationExcerpt` is
already bounded at the backend boundary and UI should not re-truncate in a way
that drops backend markers such as `[truncated ... bytes]`.

## Boundary

This is not fixed in the A-line slice because it requires TUI implementation
behavior. A-line owns gates and handoff, not product rendering.

Recommended B/TUI fix direction:

- render `ObservationExcerpt` without dropping backend truncation markers;
- if UI needs an additional one-line cap, preserve an existing trailing
  `[truncated ... bytes]` marker;
- keep `TestRenderTaskStepsTruncatedMarker` in the TUI test suite.

## A-Line Status

The current release gate at `HEAD` was validated in a clean detached worktree
before this dirty-tree issue was observed. The dirty-tree failure belongs to
uncommitted B/TUI/operatoragent changes and must be resolved before those
changes can be considered merge-ready.
