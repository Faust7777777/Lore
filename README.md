# Lore

Vertical Obsidian knowledge-operations harness for managed vault workflows.

Current implementation target follows the aligned P0 plan:

- P0-A: managed document change -> classify -> draft -> review -> controlled apply
- P0-B: Codex reads managed vault context via MCP -> transcript import/sync/attach -> checkpoint -> process-sink write -> daily report

Key product constraints already baked into the scaffold:

- Managed mode requires three core documents: `系统说明`, `进度总表`, `人物画像`
- External agents do not write vault content directly
- P0 exposes a read-only MCP subset for context access
- P0 now includes a manual `import-codex-jsonl` path for Codex session files
- P0 now includes `sync-codex-jsonl` and `attach-codex-jsonl` for local JSONL-based incremental attach mode with watcher-first wakeups
- `process-sink` writes are internal Harness writes with audit
- Model availability is a hard dependency for the main chain
- Cost control is observe-first, not a hard gate in P0/P1
- Vault daemon now uses recursive fsnotify watching with debounce and polling fallback
- When `daemon run --codex-jsonl ...` is enabled, Codex transcript sync also uses watcher-first wakeups with polling fallback

## Layout

- `cmd/obsidian-harness`: current CLI source entrypoint for the Lore binary surface
- `internal/config`: runtime and vault configuration
- `internal/model`: shared domain/runtime types
- `internal/runtime`: event bus, health, audit services
- `internal/store`: storage interfaces and in-memory reference store
- `internal/adapter/codexjsonl`: Codex session JSONL parsing and windowing
- `internal/vault`: atomic vault I/O, version hashing, and markdown attachment reference parsing
- `internal/bootstrap`: default managed-mode scaffold templates

## Current Commands

The source entrypoint still lives under `./cmd/obsidian-harness`, but the user-facing CLI surface is branded as `lore`.

- `status [workdir]`
- `bootstrap [workdir]`
- `demo-p0a [workdir]`
- `demo-p0b [workdir]`
- `console --workdir <dir> [--once "<request>"]`
- `tui --workdir <dir> [--agent <id>] [--day YYYY-MM-DD] [--once "<request>"]`
- `daemon run --workdir <dir> [--once] [--poll 2s] [--debounce 500ms] [--codex-jsonl <session.jsonl>]`
- `mcp [workdir]`
- `import-codex-jsonl --workdir <dir> --input <session.jsonl> [--agent <id>] [--session <id>] [--window 30m] [--skip-rollup]`
- `sync-codex-jsonl --workdir <dir> --input <session.jsonl> [--agent <id>] [--session <id>] [--window 30m] [--skip-rollup]`
- `attach-codex-jsonl --workdir <dir> --input <session.jsonl> [--agent <id>] [--session <id>] [--window 30m] [--poll 5s] [--once] [--skip-rollup]`

## Toolchain

This repo expects Go. A local portable toolchain is intended to live under `/.tools/go`.
