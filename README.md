# Lore

Vertical Obsidian knowledge-operations harness for managed vault workflows.

Current implementation target follows the aligned P0 plan:

- P0-A: managed document change -> classify -> draft -> review -> controlled apply
- P0-B: Codex reads managed vault context via MCP -> transcript import/sync/attach -> checkpoint -> process-sink write -> daily report

Key product constraints already baked into the scaffold:

- Managed mode requires five core documents: `00-系统/系统说明.md`, `0-排期/00-系统/文档进度总表.md`, `03-画像/人物画像.md`, `agent.md`, and `identity.md`
- External agents do not write vault content directly
- P0 exposes a read-only MCP subset for context access
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
- `internal/config`: runtime and vault configuration
- `internal/model`: shared domain/runtime types
- `internal/runtime`: event bus, health, audit services
- `internal/store`: storage interfaces plus in-memory, JSON legacy, and SQLite stores
- `internal/adapter/codexjsonl`: Codex session JSONL parsing and windowing
- `internal/adapter/codexappserver`: Codex app-server thread import support
- `internal/vault`: atomic vault I/O, version hashing, and markdown attachment reference parsing
- `internal/bootstrap`: default managed-mode scaffold templates
- `internal/tui`: text fallback and Bubble Tea interactive workbench

## Current Commands

The repo exposes both `./cmd/lore` and `./cmd/obsidian-harness`. `lore` is the preferred binary surface; `obsidian-harness` remains as a compatibility entrypoint.

- `status [workdir]`
- `bootstrap [workdir]`
- `demo-p0a [workdir]`
- `demo-p0b [workdir]`
- `smoke p0 --workdir <dir>`
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
.\scripts\verify.ps1
.\.tools\go\bin\go.exe run ./cmd/lore smoke p0 --workdir .\tmp\p0-smoke
.\.tools\go\bin\go.exe run ./cmd/lore tui --workdir .\tmp\p0-smoke --once "show current status"
```

`smoke p0` verifies the current P0 chain: managed core bootstrap, managed document draft/apply, checkpoint materialization, daily report write, and audit records.
It requires a configured model provider via `LORE_LLM_BASE_URL`, `LORE_LLM_API_KEY`, and `LORE_LLM_MODEL` because checkpoint and daily report summarization are model-backed.

## Toolchain

This repo expects Go. A local portable toolchain is intended to live under `/.tools/go`.
