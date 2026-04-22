# Obsidian Harness

Vertical Obsidian knowledge-operations harness for managed vault workflows.

Current implementation target follows the aligned P0 plan:

- P0-A: managed document change -> classify -> draft -> review -> controlled apply
- P0-B: Codex reads managed vault context via MCP -> session ingest -> checkpoint -> process-sink write -> daily report

Key product constraints already baked into the scaffold:

- Managed mode requires three core documents: `系统说明`, `进度总表`, `人物画像`
- External agents do not write vault content directly
- P0 exposes a read-only MCP subset for context access
- `process-sink` writes are internal Harness writes with audit
- Model availability is a hard dependency for the main chain
- Cost control is observe-first, not a hard gate in P0/P1

## Layout

- `cmd/obsidian-harness`: CLI entrypoint
- `internal/config`: runtime and vault configuration
- `internal/model`: shared domain/runtime types
- `internal/runtime`: event bus, health, audit services
- `internal/store`: storage interfaces and in-memory reference store
- `internal/vault`: atomic vault I/O, version hashing, and markdown attachment reference parsing
- `internal/bootstrap`: default managed-mode scaffold templates

## Toolchain

This repo expects Go. A local portable toolchain is intended to live under `/.tools/go`.
