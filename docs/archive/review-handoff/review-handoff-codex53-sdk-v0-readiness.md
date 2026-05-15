# Review Handoff: Go SDK v0 Readiness Batch

## Scope

This batch closes SDK v0 engineering-readiness gaps without changing SDK runtime behavior.

Changed files:

- `sdk/go/lore/errors_test.go`
- `sdk/go/lore/contract_docs_test.go`
- `docs/adr-sdk-go-v0.md`

## What Changed

Added error model coverage:

- `TransportError` unwraps and matches its cause with `errors.Is`.
- `DecodeError` unwraps and matches its cause with `errors.Is`.
- `ToolError`, `JSONRPCError`, and the SDK `As` helper are covered with `errors.As` semantics.

Added README/contract consistency coverage:

- SDK README argument table is checked against `docs/contracts/mcp-sdk-tools-v0.json`.
- Standard SDK args and deprecated MCP aliases must stay in sync with the published contract artifact.

Updated ADR open questions:

- The only remaining SDK v0 open question is final public module path and semantic version tag strategy.
- HTTP/WebSocket transport is deferred beyond v0 instead of being treated as a v0 open question.

## What Did Not Change

- SDK public API is unchanged.
- MCP contract is unchanged.
- No transport behavior changed.
- No runtime, TUI, or governance behavior changed.

## Verification

```powershell
Push-Location .\sdk\go\lore
..\..\..\.tools\go\bin\go.exe test ./... -count=1
Pop-Location
.\scripts\verify.ps1
```

## Review Focus

- Confirm README argument parsing is intentionally limited to the `Argument Contract` table shape.
- Confirm contract docs test treats `none` and empty properties as equivalent.
- Confirm ADR wording does not imply HTTP/WebSocket are planned for v0.
- Confirm no SDK implementation files changed.
