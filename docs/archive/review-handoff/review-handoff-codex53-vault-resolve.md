# 5.3 Codex Review Handoff: vault_resolve Tool Surface Consistency

## Scope

This change tightens the `vault_resolve` tool surface across MCP and native OpenAI tool schema.

Changed files:
- `internal/mcp/server.go`
- `internal/mcp/server_test.go`
- `internal/operatoragent/model_test.go`

## Intent

`vault_resolve` is the deterministic first-turn resolver for arbitrary user-defined vault notes. It complements:
- fixed managed core aliases via `system_doc_get`
- cross-turn short references via `WorkingSet`

This change does not add keyword gating. The model still chooses tools naturally, while runtime boundaries enforce safety.

## Contract To Review

- `vault_resolve` uses `query`, `dir`, and `limit` as the primary argument names.
- MCP accepts `path` as a deprecated alias for `dir` for compatibility, but the advertised schema now includes `dir`.
- MCP `tools/list` and `tools/call` remain consistent: a listed `vault_resolve` call is executable.
- Native tool schema preserves the guidance: read `selected_path` only when `status == "unique"`.
- The schema test verifies `dir` is exposed to the model for native tool calling.

## Review Focus

- Confirm no MCP caller loses compatibility because `getDirArg` falls back from `dir` to `path`.
- Confirm `vault_search_text` and `vault_resolve` both use the same directory argument convention.
- Confirm no prompt-level keyword rule was added.
- Confirm no TUI presentation files were touched.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp ./internal/operatoragent ./internal/console -count=1
```