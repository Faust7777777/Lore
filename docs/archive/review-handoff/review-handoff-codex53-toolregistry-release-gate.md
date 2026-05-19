# Review Handoff: ToolRegistry Release Gate

## Scope

This A-line change wires the `internal/tools` package into `scripts/release-gate.ps1`.

Changed files:

- `scripts/release-gate.ps1`

This is gate-only. It does not change registry implementation, MCP behavior, tool schemas, or operator/TUI code.

## Why

`internal/tools` is now the source of truth for:

- registry ordering and surface filtering;
- MCP schema generation from tool metadata;
- read-only tool dispatch and argument alias handling;
- proposal-intake tool dispatch and argument translation.

The release gate already checks MCP's externally visible contract, but that catches the server surface after registration. Running the registry package directly gives an earlier failure when source-of-truth behavior drifts.

## Release Gate Addition

`release-gate.ps1` now runs:

```powershell
go test ./internal/tools -run Test -count=1 -v
```

The package is deterministic, fast, and model-free.

## Boundaries

- No MCP contract artifacts changed.
- No external MCP tool exposure changed.
- No TUI or operator-agent files changed.
- This does not replace existing MCP boundary tests; it runs before them.

## Review Focus

- Confirm adding the full `internal/tools` package is acceptable for PR gate time.
- Confirm this does not create a model/network dependency.
- Confirm the gate still separately checks MCP live contract snapshots and forbidden tool exposure.
- Confirm no unrelated dirty files are included in this slice.

## Verification

```powershell
go test ./internal/tools -count=1 -v
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
