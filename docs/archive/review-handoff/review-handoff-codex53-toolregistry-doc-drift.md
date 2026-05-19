# A-Line Review Handoff: ToolRegistry Documentation Drift Cleanup

## Scope

This docs-only slice removes stale active handoff references to the deleted
`internal/mcp/tool_contract.go` source-of-truth file.

Changed files:

- `docs/compact-handoff-lore-v1.md`
- `docs/compact-handoff-reviewer.md`
- `docs/archive/review-handoff/review-handoff-codex53-toolregistry-doc-drift.md`

## Why

ToolRegistry migration moved live MCP schema and dispatch ownership into
`internal/tools/` with `internal/mcp/server.go` deriving `tools/list` and
`tools/call` from that registry. The old `internal/mcp/tool_contract.go` file
no longer exists.

Two active compact handoff documents still pointed future reviewers at
`internal/mcp/tool_contract.go`, which would cause wasted review time or
incorrect implementation direction after compact.

## Change

- Replaced `internal/mcp/tool_contract.go` references with `internal/tools/`.
- Clarified that `internal/mcp/server.go` derives both `tools/list` and
  `tools/call` from the registry.
- Clarified there is no separate MCP contract fallback file.

## Boundaries

- No code changed.
- No MCP contract artifacts changed.
- Archived historical handoffs and research notes were intentionally left
  unchanged; they describe the state at the time they were written.

## Verification

```powershell
git diff --check -- docs/compact-handoff-lore-v1.md docs/compact-handoff-reviewer.md docs/archive/review-handoff/review-handoff-codex53-toolregistry-doc-drift.md
```

Result: passed.

## Reviewer Focus

- Confirm active handoff docs now point to the real ToolRegistry source.
- Confirm SDK v0 artifact remains separate and read-only.
- Confirm this does not rewrite archived history.
