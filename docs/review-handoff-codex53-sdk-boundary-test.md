# Review Handoff: SDK Internal Import Boundary Test

## Change

Added `TestSDKDoesNotImportLoreInternalPackages` in `sdk/go/lore/boundary_test.go`.

The test parses Go imports in the SDK module and fails if any SDK file imports:

- `obsidian-harness/internal`
- `obsidian-harness/internal/...`

## Reason

The SDK boundary says the SDK must talk to Lore over stdio MCP and must not import Lore `internal/*` packages. This was previously enforced by review and docs only. The new test makes the boundary executable.

## What Did Not Change

- SDK public API is unchanged.
- MCP contract is unchanged.
- No runtime, TUI, or governance behavior changed.
- Third-party packages with their own `/internal/` path are not blocked; the test only blocks this repository's `obsidian-harness/internal` imports.

## Verification

```powershell
Push-Location .\sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -count=1
Pop-Location
.\scripts\verify.ps1
```

## Review Focus

- Confirm the test checks Go imports, not README/CHANGELOG text.
- Confirm the blocked import pattern is scoped to Lore internal packages only.
- Confirm this does not affect SDK consumers or MCP behavior.
