# Lore

Vertical Obsidian knowledge-operations harness for managed vault workflows.

Current implementation target follows the aligned P0 plan:

- P0-A: managed document change -> classify -> draft -> review -> controlled apply
- P0-B: Codex reads managed vault context via MCP -> transcript import/sync/attach -> checkpoint -> process-sink write -> daily report

Key product constraints already baked into the scaffold:

- Managed mode requires five core documents: `00-系统/系统说明.md`, `0-排期/00-系统/文档进度总表.md`, `03-画像/人物画像.md`, `agent.md`, and `identity.md`
- External agents do not write vault content directly
- P0 exposes a read + proposal-intake MCP surface (context reads and proposal tools; no direct write/apply/shell)
- P0 includes manual `import-codex-jsonl` and `import-codex-appserver` paths for Codex sessions
- P0 includes `sync-codex-jsonl` and `attach-codex-jsonl` for local JSONL-based incremental attach mode with watcher-first wakeups
- `process-sink` writes are internal Harness writes with audit
- Runtime state is persisted in SQLite at `state/store.db`, with legacy `state/store.json` migration
- Model availability is a hard dependency for the main chain
- Cost control is observe-first, not a hard gate in P0/P1
- Vault daemon uses recursive fsnotify watching with debounce and polling fallback
- When `daemon run --codex-jsonl ...` is enabled, Codex transcript sync also uses watcher-first wakeups with polling fallback

## Layout

- `cmd/lore`: preferred Lore CLI entrypoint
- `cmd/obsidian-harness`: legacy-compatible CLI entrypoint retained during rename
- `internal/config`: runtime and vault configuration with a layered loader (defaults + user-global + workspace overrides)
- `internal/model`: shared domain/runtime types
- `internal/runtime`: event bus, health, audit services
- `internal/store`: storage interfaces plus in-memory, JSON legacy, and SQLite stores
- `internal/adapter/codexjsonl`: Codex session JSONL parsing and windowing
- `internal/adapter/codexappserver`: Codex app-server thread import support
- `internal/vault`: atomic vault I/O, version hashing, and markdown attachment reference parsing
- `internal/bootstrap`: default managed-mode scaffold templates
- `internal/tui`: text fallback and Bubble Tea interactive workbench

## Configuration Layers

Lore loads configuration from these layers in increasing precedence:

1. Built-in defaults from `config.Default(workDir)`.
2. User-global overrides at `~/.lore/config.json` (skipped silently if absent).
3. Workspace overrides at `<workDir>/.lore/config.json` (skipped silently if absent).

Each layer's JSON file may set any subset of fields. Fields it does not mention keep the lower layer's value.

Example user-global override:

```json
{
  "runtime": {
    "proactive_mode": "quiet"
  }
}
```

Example workspace override:

```json
{
  "vault": {
    "managed_core": {
      "agent_doc": "runtime/agent.md"
    }
  }
}
```

Limitations:

- Duration fields (e.g. `vault.debounce_window`, `process_sink.checkpoint_every`) currently must be expressed as nanoseconds in JSON (for example `500000000` for 500ms). String forms such as `"500ms"` are not yet supported.
- `paths.work_dir` is derived from the runtime invocation; setting it from a layer file is allowed but unusual.

Run `lore status [workdir]` to see which layer files were loaded, missing, or errored.

## Current Commands

The repo exposes both `./cmd/lore` and `./cmd/obsidian-harness`. `lore` is the preferred binary surface; `obsidian-harness` remains as a compatibility entrypoint.

- `status [workdir]`
- `bootstrap [workdir]`
- `demo-p0a [workdir]`
- `demo-p0b [workdir]`
- `smoke p0 --workdir <dir> [--full]`
- `console --workdir <dir> [--once "<request>"]`
- `tui --workdir <dir> [--agent <id>] [--day YYYY-MM-DD] [--once "<request>"]`
- `daemon run --workdir <dir> [--once] [--poll 2s] [--debounce 500ms] [--codex-jsonl <session.jsonl>]`
- `mcp [workdir]`
- `import-codex-jsonl --workdir <dir> --input <session.jsonl> [--agent <id>] [--session <id>] [--window 30m] [--skip-rollup]`
- `import-codex-appserver --workdir <dir> --thread <id> -- <codex-app-server-command>`
- `sync-codex-jsonl --workdir <dir> --input <session.jsonl> [--agent <id>] [--session <id>] [--window 30m] [--skip-rollup]`
- `attach-codex-jsonl --workdir <dir> --input <session.jsonl> [--agent <id>] [--session <id>] [--window 30m] [--poll 5s] [--once] [--skip-rollup]`

## Quick Validation

Use the repo-managed Go toolchain on Windows PowerShell:

```powershell
.\scripts\release-gate.ps1
.\scripts\verify.ps1
.\.tools\go\bin\go.exe run ./cmd/lore smoke p0 --workdir .\tmp\p0-smoke
.\.tools\go\bin\go.exe run ./cmd/lore tui --workdir .\tmp\p0-smoke --once "show current status"
```

`release-gate.ps1` is the targeted governance gate for release candidates. It locks ToolRegistry schema/dispatch tests, MCP boundary/contract tests, daemon watcher and post-scan guardrails, governed note smoke, CLI smoke, the preferred `lore` CLI wrapper, Go SDK v0 contract/transport tests, backend resolve-read-final task-turn coverage, sessionlog task-turn persistence, and TUI approval state tests. Use `.\scripts\release-gate.ps1 -Full` to append the full `verify.ps1` suite.

Run the opt-in SDK end-to-end smoke with:

```powershell
.\scripts\release-gate.ps1 -E2E
```

In GitHub Actions, pull requests run the deterministic release gate, pushes to
`main` run the full gate, and manual `workflow_dispatch` runs can enable the
`e2e` input to append the opt-in MCP/SDK E2E checks.

`smoke p0` verifies the current P0 chain: managed core bootstrap, managed document draft/apply, checkpoint materialization, daily report write, and audit records. Add `--full` to also run governed markdown note intake.
It requires a configured model provider via `LORE_LLM_BASE_URL`, `LORE_LLM_API_KEY`, and `LORE_LLM_MODEL` because checkpoint and daily report summarization are model-backed.

## Toolchain

This repo expects Go. A local portable toolchain is intended to live under `/.tools/go`.
