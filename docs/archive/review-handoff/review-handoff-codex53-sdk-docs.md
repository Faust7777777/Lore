# Review Handoff: Go SDK Docs and Examples

## Scope

This change implements P4 from `docs/adr-sdk-go-v0.md`: minimal SDK documentation and compile-checked examples.

Changed files:

- `sdk/go/lore/README.md`
- `sdk/go/lore/examples_test.go`
- `docs/review-handoff-codex53-sdk-docs.md`

## README Coverage

The SDK README documents:

- v0 status and stdio-only boundary;
- no `internal/*` import / no governance bypass rule;
- local test command;
- quickstart using `Start`, `Ping`, and `VaultResolve`;
- `ClientKey` vs `LORE_CLIENT_KEY` / `LORE_MCP_API_KEY` distinction;
- typed read-only methods;
- standard argument contract and deprecated MCP aliases;
- error types;
- opt-in E2E smoke command;
- compatibility notes.

## Examples

`examples_test.go` contains compile-checked examples for:

- `Start` + `VaultResolve` usage;
- `ToolError` handling with `errors.As`.

The examples intentionally have no `Output:` block, so they compile as documentation without adding behavioral assertions to normal SDK tests.

## Verification

```powershell
Push-Location .\sdk\go\lore; ..\..\..\.tools\go\bin\go.exe test ./... -count=1; Pop-Location
.\.tools\go\bin\go.exe test ./... -count=1
```

## Review Focus

Please check:

- README does not imply writable SDK behavior.
- Argument table matches MCP contract and typed methods.
- Error handling docs match current exported error types.
- Examples remain safe for normal `go test`.
