# Review Handoff: Release Gate README Alignment

## Scope

This A-line docs-only change updates the README release gate description to match the current gate surface.

Changed files:

- `README.md`

## Why

`release-gate.ps1` now covers more than the original MCP/daemon/CLI/TUI set. It also gates:

- ToolRegistry schema and dispatch source-of-truth tests;
- backend resolve-read-final task-turn coverage;
- sessionlog task-turn persistence.

The README should describe the real release-candidate gate so developers know what failures mean.

## Boundaries

- No script behavior changed in this slice.
- No CI workflow changed.
- No product/runtime/TUI/operator behavior changed.

## Review Focus

- Confirm the README wording matches `scripts/release-gate.ps1`.
- Confirm it does not imply the skipped TUI visible task-turn scaffold is active.
- Confirm it still states `-Full` appends `verify.ps1`.

## Verification

```powershell
git diff --check -- README.md docs/archive/review-handoff/review-handoff-codex53-release-gate-readme-alignment.md
```
