# ADR: Go SDK v0 for Lore MCP Read-Only Surface

## Status

Proposed for three-party review.

Review owners:

- Product / final decision: user
- Backend / runtime / SDK implementation: Codex
- TUI impact review: Opus
- Code review: Codex 5.3

## Context

Lore already exposes a read-only MCP surface through `lore mcp`. External callers can use the MCP protocol directly, but doing so requires each integration to duplicate process startup, JSON-RPC framing, tool argument naming, typed decoding, and error handling.

The SDK should make this read-only surface reusable without weakening existing governance boundaries.

Current baseline:

- MCP server exists under `internal/mcp`.
- CLI can start the MCP server through `lore mcp <workdir>`.
- MCP protocol version and process-level auth already exist.
- Read-only tools are exposed through `tools/list` and `tools/call`.

Known contract drift to close before SDK work:

- `context_pack` currently uses `path` in MCP but `target_path` in the local/native tool surface.
- `vault_list` currently uses `path` in MCP but `dir` in the local/native tool surface.
- `vault_search_text` and `vault_resolve` already use `dir` with `path` as a deprecated alias.

## Decision

Build a Go SDK v0 in the same repository, under `sdk/go/lore`, with its own `go.mod`.

The SDK is a stdio JSON-RPC client for the existing MCP server. It must not import `internal/*`, must not bypass the MCP server, and must not change runtime governance behavior.

## Goals

- Provide a reusable Go client for Lore MCP read-only tools.
- Freeze SDK-facing argument names before exposing them.
- Keep MCP read-only and preserve all existing policy/runtime boundaries.
- Provide strong request/response types for common tool calls.
- Provide predictable error categories for callers.
- Add contract tests so `tools/list` drift is caught early.

## Non-Goals

- No writable MCP tools.
- No direct import of runtime, orchestrator, model, TUI, or store internals from the SDK.
- No HTTP or WebSocket transport in v0.
- No cross-session memory behavior changes.
- No TUI behavior changes.
- No schema migration.
- No TS SDK in v0; TS can reuse the same contract later.

## Repository Shape

Initial location:

```text
sdk/go/lore/
  go.mod
  client.go
  transport_stdio.go
  tools_readonly.go
  types.go
  errors.go
  contract_test.go
  examples/
    readme_example_test.go
```

The SDK gets a separate module so external users do not inherit the root CLI/runtime dependency graph.

Module path remains a release decision. For local v0 development, use a repository-relative module path that can later become:

```text
github.com/<org>/obsidian-harness/sdk/go/lore
```

## SDK Boundary

The SDK communicates only by spawning or attaching to `lore mcp` over stdio JSON-RPC.

Allowed:

- Start `lore mcp <workdir>` as a child process.
- Send MCP `initialize`, `tools/list`, and `tools/call` messages; SDK `Ping` should call MCP `ping` by default and fall back to `initialize` only for legacy servers that do not implement `ping`.
- Decode returned `structuredContent` into SDK DTOs.
- Surface MCP `isError=true` as tool business errors.

Forbidden:

- Import `internal/orchestrator`, `internal/model`, `internal/mcp`, `internal/app`, or TUI packages.
- Call runtime or store APIs directly.
- Add writable MCP tools as part of SDK work.
- Add keyword gating or agent-intent heuristics.

## MCP Contract Freeze

Standard SDK-facing names:

| Tool | Standard args | Compatibility aliases |
| --- | --- | --- |
| `managed_status` | none | none |
| `system_doc_get` | `name` | none |
| `vault_read` | `path` | none |
| `vault_list` | `dir` | `path` deprecated alias |
| `vault_search_text` | `query`, `dir`, `limit` | `path` deprecated alias for `dir` |
| `vault_resolve` | `query`, `dir`, `limit` | `path` deprecated alias for `dir` |
| `vault_backlinks` | `path`, `limit` | none |
| `doc_classify` | `path` | none |
| `context_pack` | `target_path`, `task`, `limit` | `path` deprecated alias for `target_path` |

`tools/list` should advertise the standard args and include alias descriptions where compatibility aliases remain accepted.

`tools/call` should prefer standard args over aliases when both are present.

## Go SDK v0 API

Sketch:

```go
client, err := lore.Start(ctx, lore.Options{
    Command: "lore",
    WorkDir: "./tmp/chat-demo",
    ClientKey: os.Getenv("LORE_CLIENT_KEY"), // client credential sent to the child as LORE_CLIENT_KEY; distinct from server-side LORE_MCP_API_KEY
})
if err != nil {
    return err
}
defer client.Close()

if err := client.Ping(ctx); err != nil {
    return err
}

resolved, err := client.VaultResolve(ctx, lore.VaultResolveRequest{
    Query: "persona",
    Dir: "",
    Limit: 5,
})
if err != nil {
    return err
}
```

Planned methods:

- `Start(ctx context.Context, opts Options) (*Client, error)`
- `Close() error`
- `Ping(ctx context.Context) error`
- `Tools(ctx context.Context) ([]ToolDefinition, error)`
- `ManagedStatus(ctx context.Context) (*ManagedStatus, error)`
- `SystemDocGet(ctx context.Context, req SystemDocGetRequest) (*VaultDocument, error)`
- `VaultRead(ctx context.Context, req VaultReadRequest) (*VaultDocument, error)`
- `VaultList(ctx context.Context, req VaultListRequest) ([]VaultEntry, error)`
- `VaultSearchText(ctx context.Context, req VaultSearchTextRequest) ([]SearchHit, error)`
- `VaultResolve(ctx context.Context, req VaultResolveRequest) (*ResolveResult, error)`
- `VaultBacklinks(ctx context.Context, req VaultBacklinksRequest) ([]SearchHit, error)`
- `ContextPack(ctx context.Context, req ContextPackRequest) (*ContextPack, error)`
- `DocClassify(ctx context.Context, req DocClassifyRequest) (*DocClassification, error)`

## Error Model

SDK errors are grouped into five layers:

1. Transport errors: process start failure, broken pipe, malformed MCP frame, child process exit.
2. Timeout/cancel errors: context deadline exceeded or canceled while waiting for a response.
3. JSON-RPC errors: non-nil JSON-RPC `error` object.
4. Tool business errors: `tools/call` returns `isError=true`.
5. Decode errors: response shape is valid MCP but cannot decode into the expected SDK DTO.

Each error should be testable with `errors.As` or sentinel classification helpers.

## Implementation Plan

### P0: MCP Contract Alignment

- Change `context_pack` advertised schema from `path` to `target_path`.
- Keep `path` as a deprecated alias for `context_pack` calls.
- Change `vault_list` advertised schema from `path` to `dir`.
- Keep `path` as a deprecated alias for `vault_list` calls.
- Ensure standard args win over aliases if both are provided.
- Update MCP tests for backward compatibility and new advertised names.

### P0.5: Contract Tests

- Add tests that lock `tools/list` names, required fields, and input properties.
- Verify the SDK-facing read-only tool set stays stable.
- Verify deprecated aliases are advertised only where intentionally supported.

### P1.5: MCP Contract Source of Truth

- Centralize MCP read-only tool contracts in `internal/mcp/tool_contract.go`.
- Generate `tools/list` schemas from that contract instead of maintaining a separate hand-written map.
- Publish the SDK-facing v0 contract as `docs/contracts/mcp-sdk-tools-v0.json` so external SDK consumers have a stable contract sample without importing `internal/*`.
- Keep alias behavior tests separate from schema-generation tests.

### P1: Stdio Transport

- Implement process startup.
- Implement MCP frame read/write.
- Implement request ID correlation.
- Implement `initialize`, SDK `Ping`, `tools/list`, and generic `CallTool`; `Ping` should use MCP `ping` first and fall back to `initialize` only for compatibility with legacy servers.
- Honor context cancellation and deadline.
- Ensure `Close` terminates the child process cleanly.

### P2: Typed Read-Only Methods

- Add request/response DTOs.
- Add wrappers for the read-only tool set.
- Decode `structuredContent` into typed values.
- Keep generic `CallTool` available for forward-compatible read-only experiments if desired.

### P3: End-to-End Tests

- Build or locate the current `lore` binary.
- Start `lore mcp <tmp-workdir>`.
- Bootstrap a temporary workdir/vault as needed.
- Run `vault_resolve` and `context_pack` through the SDK.
- Assert typed outputs.

### P4: Docs and Examples

- Minimal quickstart.
- Error handling example.
- Version compatibility statement.
- Tool argument compatibility table.

## Acceptance Criteria

- Existing MCP callers using `path` for `vault_list` and `context_pack` still work.
- New `tools/list` advertises `dir` for `vault_list` and `target_path` for `context_pack`.
- SDK exposes only standard argument names.
- Tests assert standard args override deprecated aliases when both are provided.
- Contract tests fail if tool names, required fields, or input properties drift unexpectedly.
- `go test ./... -count=1` passes for the root module.
- `go test ./... -count=1` passes inside `sdk/go/lore` after SDK work starts.
- SDK changes do not touch TUI semantics.
- SDK changes do not change runtime governance or writable capability boundaries.

## Rollback Plan

If SDK implementation causes instability:

- Revert `sdk/go/lore` independently; MCP contract alignment can remain if tests pass.
- If contract alignment breaks an external MCP caller, keep standard names in `tools/list` but preserve `path` alias in `tools/call`; rollback should not remove alias compatibility unless explicitly approved.
- If process management is flaky on Windows, keep SDK package experimental and block release until lifecycle tests pass reliably.

## Open Questions

- Final public module path and semantic version tag strategy.
- Whether HTTP or WebSocket transport is needed after stdio v0 is validated.
