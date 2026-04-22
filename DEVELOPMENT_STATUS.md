# Development Status

Updated: 2026-04-22

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
  - cursor-backed file fingerprint sync to skip unchanged transcripts
  - local attach mode via polling wrapper around sync
- CLI:
  - `status [workdir]`
  - `bootstrap [workdir]`
  - `demo-p0a [workdir]`
  - `demo-p0b [workdir]`
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

- No real daemon loop yet
- No file watcher yet
- No provider/model adapter yet
- No live app-server / offset-tail Codex adapter yet; current P0-B supports manual import plus file-fingerprint sync/attach over local JSONL files
- No real TUI framework yet; current output is text-mode status and demo commands
- No SQLite state store yet; JSON store is the minimal persisted recovery layer
- Draft apply is append-based, not diff/patch-based
- Process-sink import currently uses deterministic transcript rendering; model summarizer integration is not wired
- Attachment refs are surfaced in read APIs, but binary/media extraction is not implemented yet

## Suggested Next Steps

1. Add a real daemon service and watcher loop around `orchestrator.Harness`.
2. Upgrade JSONL attach from whole-file resync to a true offset/tail adapter with partial-line handling and source identity checks.
3. Replace append-only draft apply with structured patch/diff application.
4. Connect transcript import to a real summarizer/model pipeline instead of deterministic window rendering.
5. Move from text status view to a real interactive TUI.
6. Add SQLite-backed state/audit store once the shape stabilizes.

## Git Checkpoints

- `286f178` `feat: scaffold p0 harness core`
- `83249fb` `feat: add persistent json state store`
- `0b84665` `feat: wire cli demos to persistent runtime`
- `e24ae21` `feat: add read-only mcp context tools`
- `225e7ab` `fix: track vault query helpers and root vault ignore`
- `dadcc98` `fix: track vault atomic helpers`
- `0a2e02b` `feat: surface markdown attachment refs in read api`
- `6817ce5` `feat: import codex transcript jsonl`
