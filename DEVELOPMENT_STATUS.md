# Development Status

Updated: 2026-04-23

## Implemented

- Go project scaffold with local portable toolchain support under `/.tools`.
- Shared model layer for runtime status, document classes, drafts, process-sink checkpoints/daily reports, audit records, usage records, and managed core status.
- Runtime foundations:
  - In-process event broker.
  - Dependency health service.
  - Audit service.
  - Vault daemon with one-shot mode, recursive fsnotify watching, debounce, and polling fallback.
  - Daemon-side Codex JSONL watching with poll fallback.
- Storage:
  - In-memory store for tests.
  - JSON legacy store retained for compatibility tests and migration.
  - SQLite runtime store at `state/store.db`, with automatic legacy `state/store.json` migration.
- Vault I/O:
  - Atomic write helper.
  - Content hashing helper.
  - Markdown attachment reference extraction.
- Domain services:
  - Document classification.
  - Draft lifecycle.
  - Process-sink checkpoint and daily rollup.
- Orchestrator:
  - Bootstrap managed vault with five core docs.
  - Managed doc change -> draft -> approve -> apply.
  - Session window ingest -> checkpoint -> daily report.
  - Read-only vault/context API for managed status, system docs, search, backlinks, and context pack.
- Codex adapters:
  - Session JSONL parsing from local Codex transcript files.
  - 30 minute windowing with empty-slot placeholder generation.
  - Manual JSONL import path from CLI into checkpoint and daily report writes.
  - Cursor-backed JSONL tail sync with replay-offset resume and boundary-anchor validation.
  - Local JSONL attach mode with file-watch wakeups and polling fallback.
  - Codex app-server one-shot thread import path.
- LLM/operator path:
  - OpenAI-compatible provider client.
  - Model discovery and operator-model selection.
  - GPT-5 `/responses` transport for compatible providers.
  - Retry hardening for `429`, `5xx`, and transport EOF.
  - Natural-language agent loop with Lore tool dispatch.
  - Local workspace tools behind `--local-exec`; only `shell_exec` adds a confirmation step.
- TUI:
  - Bubble Tea interactive workbench with conversation lane, context/status lane, pending approvals placeholder, keyboard focus switching, and async agent execution.
  - Text fallback and `--once` mode for non-interactive environments.
- CLI:
  - Preferred `cmd/lore` entrypoint with legacy `cmd/obsidian-harness` compatibility.
  - `status [workdir]`.
  - `bootstrap [workdir]`.
  - `demo-p0a [workdir]`.
  - `demo-p0b [workdir]`.
  - `smoke p0 --workdir <dir>`.
  - `tui --workdir <dir> [--once "<request>"]`.
  - `console --workdir <dir> [--once "<request>"]`.
  - `daemon run --workdir <dir> [--once] [--codex-jsonl <session.jsonl>]`.
  - `mcp [workdir]`.
  - `import-codex-jsonl --workdir <dir> --input <session.jsonl>`.
  - `import-codex-appserver --workdir <dir> --thread <id> -- <codex-app-server-command>`.
  - `sync-codex-jsonl --workdir <dir> --input <session.jsonl>`.
  - `attach-codex-jsonl --workdir <dir> --input <session.jsonl> --once`.

## Verified

Current verification baseline with the repo-managed Go toolchain:

```powershell
.\.tools\go\bin\go.exe test ./...
.\.tools\go\bin\go.exe run ./cmd/lore version
.\.tools\go\bin\go.exe run ./cmd/lore smoke p0 --workdir .\tmp\p0-smoke
.\.tools\go\bin\go.exe run ./cmd/lore tui --workdir .\tmp\p0-smoke --once "show current status"
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness bootstrap .\tmp\demo
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness demo-p0a .\tmp\demo
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness demo-p0b .\tmp\demo
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness status .\tmp\demo
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness mcp .\tmp\demo
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness import-codex-jsonl --workdir .\tmp\import-demo --input <codex-session.jsonl>
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness sync-codex-jsonl --workdir .\tmp\attach-demo --input <codex-session.jsonl>
.\.tools\go\bin\go.exe run ./cmd/obsidian-harness attach-codex-jsonl --workdir .\tmp\attach-demo --input <codex-session.jsonl> --once
```

The `smoke p0`, `demo-p0b`, and process-sink summarization paths require `LORE_LLM_BASE_URL`, `LORE_LLM_API_KEY`, and `LORE_LLM_MODEL` when run manually. Automated tests use fake providers.

## Current Gaps

- No long-running app-server attach loop yet; app-server support is currently one-shot import, while continuous attach is JSONL-based.
- Pending approval queue is still a placeholder in the TUI; draft review/apply exists through CLI and agent actions.
- Attachment refs are surfaced in read APIs, but binary/media extraction is not implemented yet.
- Cost tracking exists at the store/model level; user-facing usage panels and soft warnings still need P1 polish.

## Suggested Next Steps

1. Standardize the P0 smoke command as the user-facing acceptance test for future changes.
2. Add a runtime pending-action queue so shell confirmation and future approvals can surface in the TUI approval pane.
3. Add user-facing usage summaries and soft warnings.
4. Extend attachment/media extraction beyond markdown ref surfacing.
5. Add long-running app-server attach mode if JSONL attach proves insufficient.

## Recent Git Checkpoints

- `33a5595` `feat: add p0 smoke verification command`
- `aaf4ebe` `feat: replace json state store with sqlite runtime store`
- `57e2b30` `fix: align bootstrap agent template with shell confirm flow`
- `dd9f389` `feat: remove keyword gating from local tools`
- `90ddd6a` `feat: polish runtime doc templates`
