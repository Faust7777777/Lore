# Review Handoff: Go SDK Read-Only Typed Methods

## Scope

This change implements P2 from `docs/adr-sdk-go-v0.md`: public DTOs and typed convenience methods for the MCP read-only surface.

Changed files:

- `sdk/go/lore/models.go`
- `sdk/go/lore/tools_readonly.go`
- `sdk/go/lore/tools_readonly_test.go`

## SDK Methods Added

- `ManagedStatus(ctx)`
- `SystemDocGet(ctx, SystemDocGetRequest)`
- `VaultRead(ctx, VaultReadRequest)`
- `VaultList(ctx, VaultListRequest)`
- `VaultSearchText(ctx, VaultSearchTextRequest)`
- `VaultResolve(ctx, VaultResolveRequest)`
- `VaultBacklinks(ctx, VaultBacklinksRequest)`
- `ContextPack(ctx, ContextPackRequest)`
- `DocClassify(ctx, DocClassifyRequest)`

The SDK uses only standard argument names:

- `vault_list` sends `dir`, never deprecated `path`.
- `context_pack` sends `target_path`, never deprecated `path`.
- `vault_search_text` and `vault_resolve` send `dir`, never deprecated `path`.

## DTO Policy

The SDK duplicates public JSON shapes instead of importing `internal/model`.

This preserves the boundary from the ADR: SDK is a client of the MCP protocol, not a runtime/orchestrator library.

## Tests

`TestReadOnlyMethodsUseStandardToolArguments` verifies each typed method:

- calls the expected MCP tool name;
- sends expected standard arguments;
- does not send deprecated `path` aliases for tools that have moved to `dir` or `target_path`;
- decodes representative `structuredContent` into SDK DTOs.

Verification commands:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
Push-Location .\sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; Pop-Location
```

## Review Focus

Please check:

- DTO JSON tags match `internal/model/readapi.go` without importing it.
- Typed methods use only SDK-standard args.
- `DocClassify` is exposed as a first-class v0 method as agreed after ADR review.
- No TUI, governance, or MCP server behavior changed in this P2 step.
