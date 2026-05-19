# Review Handoff: Preferred Lore CLI Release Gate

## Scope

This A-line change adds a minimal release-gate check for the preferred `cmd/lore` entrypoint.

Changed files:

- `scripts/release-gate.ps1`
- `README.md`

No CLI implementation, runtime behavior, TUI behavior, or MCP surface changed.

## Why

The README names `cmd/lore` as the preferred CLI entrypoint, while `cmd/obsidian-harness` remains a compatibility entrypoint. The targeted release gate already covered the compatibility command's smoke and daemon tests, but not the preferred wrapper at all.

## Release Gate Addition

`release-gate.ps1` now runs:

```powershell
go test ./cmd/lore -run TestRunVersion$ -count=1 -v
```

This is intentionally minimal: it verifies the preferred wrapper is buildable and can dispatch through `cli.Run` without pulling the heavier compatibility CLI smoke into a duplicate path.

## Boundaries

- Does not duplicate all `cmd/obsidian-harness` command smoke tests.
- Does not require real model configuration.
- Does not touch `cmd/lore` implementation.

## Review Focus

- Confirm the wrapper test is enough for targeted gate coverage today.
- Confirm PR gate remains deterministic and fast.
- Confirm README release-gate wording matches the script.

## Verification

```powershell
go test ./cmd/lore -run TestRunVersion$ -count=1 -v
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
