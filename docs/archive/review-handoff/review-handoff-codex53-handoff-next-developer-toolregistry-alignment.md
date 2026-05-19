# Review Handoff: Handoff-Next-Developer ToolRegistry Alignment

## Scope

This docs-only slice updates the active `docs/handoff-next-developer-2026-04-29.md`
handoff so it points at the live ToolRegistry source instead of the deleted
`internal/mcp/tool_contract.go` file.

Changed files:

- `docs/handoff-next-developer-2026-04-29.md`
- `docs/archive/review-handoff/review-handoff-codex53-handoff-next-developer-toolregistry-alignment.md`

## Why

The active handoff is intended for the next developer. It must not send them to
a file that no longer exists.

The live MCP contract source of truth is now `internal/tools/`, with
`internal/mcp/server.go` deriving `tools/list` and `tools/call` from the
registry.

## Change

- Replaced the stale `internal/mcp/tool_contract.go` reference with
  `internal/tools/`.

## Boundaries

- No code changed.
- No archived history changed.
- This is a guidance-only correction for the active handoff.

## Verification

```powershell
git diff --check -- docs/handoff-next-developer-2026-04-29.md docs/archive/review-handoff/review-handoff-codex53-handoff-next-developer-toolregistry-alignment.md
```

Result: passed.

## Reviewer Focus

- Confirm the next-developer handoff now points to the real source of truth.
- Confirm this does not alter any archived historical docs.
