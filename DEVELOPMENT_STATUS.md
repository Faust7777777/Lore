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
- Domain services:
  - document classification
  - draft lifecycle
  - process-sink checkpoint and daily rollup
- Orchestrator:
  - bootstrap managed vault
  - managed doc change -> draft -> approve -> apply
  - session window ingest -> checkpoint -> daily report
- CLI:
  - `status [workdir]`
  - `bootstrap [workdir]`
  - `demo-p0a [workdir]`
  - `demo-p0b [workdir]`

## Verified

Commands verified locally with the repo-managed Go toolchain:

```powershell
.tools\go\bin\go.exe test ./...
.tools\go\bin\go.exe run ./cmd/obsidian-harness bootstrap .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness demo-p0a .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness demo-p0b .\tmp\demo
.tools\go\bin\go.exe run ./cmd/obsidian-harness status .\tmp\demo
```

## Current Gaps

- No real daemon loop yet
- No file watcher yet
- No provider/model adapter yet
- No external Codex transcript adapter yet
- No real TUI framework yet; current output is text-mode status and demo commands
- No SQLite state store yet; JSON store is the minimal persisted recovery layer
- Draft apply is append-based, not diff/patch-based
- Process-sink currently uses direct content input; summarizer/model integration not wired

## Suggested Next Steps

1. Add a real daemon service and watcher loop around `orchestrator.Harness`.
2. Add adapter interfaces for external agent transcript ingestion.
3. Replace append-only draft apply with structured patch/diff application.
4. Move from text status view to a real interactive TUI.
5. Add SQLite-backed state/audit store once the shape stabilizes.

## Git Checkpoints

- `286f178` `feat: scaffold p0 harness core`
- `83249fb` `feat: add persistent json state store`
- `0b84665` `feat: wire cli demos to persistent runtime`

