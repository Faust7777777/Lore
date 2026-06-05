# Development Status

Updated: 2026-06-05

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

## Recent Additions Since April Baseline

- External MCP v1 now derives its live tool surface from the registry and remains read + proposal intake only; direct write/apply/shell tools are still excluded.
- Governed intake supports persona update proposals and markdown note proposals as pending drafts, with local review/apply/supersede and CoreContext injected as user-role vault context for review.
- Daemon post-scan records out-of-band vault writes and governed-document changes as persisted findings for later local review.
- Task/Turn visibility is active: operator-agent responses carry visible turn steps, console stores current-turn steps, session logs persist them, and TUI renders bounded task steps without exposing chain-of-thought.
- Persona memory candidate pipeline exists on the local path: LLM extraction runs asynchronously, candidates are persisted, evidence is validated, usage is recorded under `persona_extract`, and CLI supports list/show/dismiss/draft/recover.
- Usage summaries normalize records by local calendar day and show per-purpose breakdowns through `lore usage` and status output.
- Release gates now separate PR-safe fake-provider checks from full model-backed checks, with persona acceptance available as an explicit opt-in gate.

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
- Usage tracking has CLI/status visibility and a soft daily-budget warning (`usage.soft_warning_tokens`, surfaced as a nudge in `lore status`); richer TUI usage panels are still not implemented.
- Persona candidates do not update `人物画像.md` directly; they still require explicit draft creation and the existing local review/apply path.
- External transcript import is not yet connected to persona candidate extraction; the stable path is still the local console/TUI conversation flow.

## Suggested Next Steps

1. Stabilize persona memory candidate governance, especially retry/recover edge cases and manual draft promotion UX.
2. Add a runtime pending-action queue so shell confirmation and future approvals can surface in the TUI approval pane.
3. Add product-level usage soft warnings and richer usage visibility where it helps decision-making.
4. Add read-only TUI visibility for persona candidates after the CLI/storage path stays stable.
5. Extend attachment/media extraction beyond markdown ref surfacing.
6. Add long-running app-server attach mode if JSONL attach proves insufficient.

## Recent Git Checkpoints

- `33a5595` `feat: add p0 smoke verification command`
- `aaf4ebe` `feat: replace json state store with sqlite runtime store`
- `57e2b30` `fix: align bootstrap agent template with shell confirm flow`
- `dd9f389` `feat: remove keyword gating from local tools`
- `90ddd6a` `feat: polish runtime doc templates`
