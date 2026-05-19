# Review Handoff: Governed Intake Plan ToolRegistry Alignment

## Scope

This docs-only slice updates the active governed-intake plan so it no longer
points at the deleted `internal/mcp/tool_contract.go` file.

Changed files:

- `docs/plans/2026-04-28-external-agent-governed-intake.md`
- `docs/archive/review-handoff/review-handoff-codex53-governed-intake-plan-toolregistry-alignment.md`

## Why

The live MCP schema/dispatch source of truth is now the shared ToolRegistry in
`internal/tools/`, and `internal/mcp/server.go` derives `tools/list` from that
registry.

The active governed-intake plan still referred to the old file-based contract
implementation. That mismatch would mislead the next developer into editing a
nonexistent source file.

## Change

- Replaced `internal/mcp/tool_contract.go` with `internal/tools/` in Task 6.
- Clarified that `internal/mcp/server.go` derives `tools/list` from the
  registry.
- Updated the example `git add` command to the new source-of-truth path.

## Boundaries

- No code changed.
- No MCP contract behavior changed.
- No archived history was rewritten.

## Verification

```powershell
git diff --check -- docs/plans/2026-04-28-external-agent-governed-intake.md docs/archive/review-handoff/review-handoff-codex53-governed-intake-plan-toolregistry-alignment.md
```

Result: passed.

## Reviewer Focus

- Confirm the active plan now points at the real ToolRegistry ownership.
- Confirm `docs/integrations/mcp-client-setup.md` already reflects the live
  proposal-intake tools and does not need a second change.
- Confirm the plan no longer directs work toward a deleted file.
