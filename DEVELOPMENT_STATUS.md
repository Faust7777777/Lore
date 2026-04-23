# Development Status

Updated: 2026-04-23

## Implemented

- Go project scaffold with local portable toolchain support under `/.tools`
- Shared model layer for:
  - tool/runtime status
  - managed document classes
  - drafts
  - process-sink checkpoints and daily reports
  - audit and usage records
- Runtime foundations:
  - in-process event broker
  - dependency health service
  - audit service
  - vault daemon with one-shot mode, recursive fsnotify watching, debounce, and polling fallback
  - daemon-side Codex JSONL watching with poll fallback
- Storage:
  - in-memory store for tests
  - persistent JSON state store for restart recovery
- Vault I/O:
  - atomic write helper
  - content hashing helper
  - markdown attachment reference extraction
- Domain services:
  - document classification
  - draft lifecycle
  - process-sink checkpoint and daily rollup
- Orchestrator:
  - bootstrap managed vault
  - managed doc change -> draft -> approve -> apply
  - session window ingest -> checkpoint -> daily report
  - read-only vault/context API for managed status, system docs, search, backlinks, and context pack
- Codex import adapter:
  - session JSONL parsing from local Codex transcript files
  - 30 minute windowing with empty-slot placeholder generation
  - manual import path from CLI into checkpoint and daily report writes
  - cursor-backed tail sync with replay-offset resume and boundary-anchor validation
  - local attach mode with file-watch wakeups and polling fallback around incremental sync
- LLM/operator path:
  - OpenAI-compatible provider client
  - model discovery and operator-model selection
  - GPT-5 `/responses` transport for compatible providers
  - retry hardening for `429` / `5xx` / transport EOF
- CLI:
  - `status [workdir]`
  - `bootstrap [workdir]`
  - `demo-p0a [workdir]`
  - `demo-p0b [workdir]`
  - `tui --workdir <dir> [--once "<request>"]`
  - `console --workdir <dir> [--once "<request>"]`
  - `daemon run --workdir <dir> [--once] [--codex-jsonl <session.jsonl>]`
  - `mcp [workdir]`
  - `import-codex-jsonl --workdir <dir> --input <session.jsonl>`
  - `sync-codex-jsonl --workdir <dir> --input <session.jsonl>`
  - `attach-codex-jsonl --workdir <dir> --input <session.jsonl> --once`

## Verified

Commands verified locally with the repo-managed Go toolchain:

```powershell
.tools\go\bin\go.exe test ./...
.tools\go\bin\go.exe run ./cmd/obsidian-harness bootstrap .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness demo-p0a .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness demo-p0b .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness status .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness mcp .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness import-codex-jsonl --workdir .\tmp\import-demo --input <codex-session.jsonl>
.tools\go\bin\go.exe run ./cmd/obsidian-harness sync-codex-jsonl --workdir .\tmp\attach-demo --input <codex-session.jsonl>
.tools\go\bin\go.exe run ./cmd/obsidian-harness attach-codex-jsonl --workdir .\tmp\attach-demo --input <codex-session.jsonl> --once
```

## Current Gaps

 - No live app-server Codex adapter yet; current P0-B supports manual import plus cursor-backed JSONL tail sync/attach over local files
- No full-screen TUI framework yet; `tui` is a text dashboard, not a Bubble Tea-style interface
- No SQLite state store yet; JSON store is the minimal persisted recovery layer
- Attachment refs are surfaced in read APIs, but binary/media extraction is not implemented yet

## Suggested Next Steps

1. Upgrade JSONL attach from file-fingerprint sync to a stronger live adapter with source-specific streaming or app-server integration.
2. Move from the text dashboard to a fuller interactive TUI once panel structure stabilizes.
3. Add SQLite-backed state/audit store once the shape stabilizes.
4. Extend attachment/media extraction beyond markdown ref surfacing.

## Git Checkpoints

- `286f178` `feat: scaffold p0 harness core`
- `83249fb` `feat: add persistent json state store`
- `0b84665` `feat: wire cli demos to persistent runtime`
- `e24ae21` `feat: add read-only mcp context tools`
- `225e7ab` `fix: track vault query helpers and root vault ignore`
- `dadcc98` `fix: track vault atomic helpers`
- `0a2e02b` `feat: surface markdown attachment refs in read api`
- `6817ce5` `feat: import codex transcript jsonl`
