# Lore

Lore is a local-first, governed Obsidian knowledge-operations agent.

It is built around one rule: important vault changes are proposed first,
reviewed locally, and only then applied. The product surface is the TUI.
The CLI is still supported, but primarily as automation, diagnostics, and
the tool surface that the model can call. MCP is an external-agent intake
surface, not a write proxy.

## What Lore Does

Lore turns an Obsidian workspace into a governed operating system for long-running knowledge work:

- Reads managed core documents, vault notes, context packs, backlinks, drafts, findings, and process history.
- Chats with the operator through a local TUI and model-backed agent loop.
- Creates reviewable drafts instead of directly changing governed documents.
- Lets the operator approve, reject, revise, supersede, and apply drafts locally.
- Mines durable persona-memory candidates from normal chat, then routes accepted candidates through the same draft governance path.
- Imports Codex and external transcript JSONL into process-sink checkpoints and daily reports.
- Exposes read tools and narrow proposal-intake tools to external agents through MCP.
- Tracks LLM usage by purpose and model so chat, persona extraction, and process-sink costs can be separated.

## Product Surfaces

### TUI: Primary Operator Surface

Run the TUI for normal use:

```powershell
go run ./cmd/lore tui --workdir .\tmp\lore-workspace
```

The TUI is intended to cover the real workflow:

- Chat with Lore.
- Inspect runtime status, model identities, and current workdir health.
- Configure and switch model profiles with `/model`.
- Review, approve, reject, and apply drafts.
- Inspect persona candidates with `/candidates`.
- Inspect extraction/model diagnostics with `/errors`.
- Navigate panels with Tab / Shift+Tab and use list/detail shortcuts.

See [`docs/tui-manual-test.md`](docs/tui-manual-test.md) for the current end-to-end manual test script.

### CLI: Automation, Debugging, And Model-Callable Surface

The `lore` binary remains the stable command surface for smoke tests, diagnostics, scripting, and the in-app agent's tool flow.

Common commands:

```powershell
go run ./cmd/lore bootstrap .\tmp\lore-workspace
go run ./cmd/lore status .\tmp\lore-workspace
go run ./cmd/lore console --workdir .\tmp\lore-workspace --once "show current status"
go run ./cmd/lore tui --workdir .\tmp\lore-workspace
go run ./cmd/lore usage --days 7 .\tmp\lore-workspace
go run ./cmd/lore persona summary --workdir .\tmp\lore-workspace
```

Important CLI groups:

- `status`, `bootstrap`, `smoke p0`
- `console`, `tui`
- `daemon run`
- `mcp`
- `draft ...`
- `findings ...`
- `persona candidates ...`
- `persona errors`, `persona summary`
- `usage [--days N] [--json] [workdir]`
- `import-codex-jsonl`, `sync-codex-jsonl`, `attach-codex-jsonl`
- `import-codex-appserver`
- `import-external-jsonl`

The legacy `cmd/obsidian-harness` entrypoint remains for compatibility. New work should prefer `cmd/lore`.

### MCP: External Agent Read + Proposal Intake

Lore's MCP server is stdio-based:

```powershell
lore mcp C:\path\to\lore-workdir
```

MCP v1 allows:

- `initialize`, `ping`, `tools/list`, `tools/call`
- read tools such as managed status, system docs, vault read/search/resolve/backlinks, and context packs
- proposal intake such as `persona_update_propose` and `markdown_note_propose`

MCP v1 forbids:

- shell execution
- generic file write/edit tools
- direct vault markdown writes
- direct writes to governed documents
- draft approve/apply
- runtime policy changes

External agents should read `system_doc_get("agent")` first. See
[`docs/integrations/mcp-client-setup.md`](docs/integrations/mcp-client-setup.md).

## Governed Workspace Model

Lore bootstraps and protects five managed core documents:

- `00-系统/系统说明.md`
- `0-排期/00-系统/文档进度总表.md`
- `03-画像/人物画像.md`
- `agent.md`
- `identity.md`

Document roles:

- `agent.md`: external-first workspace operating manual and shared agent rules.
- `identity.md`: Lore's local self identity; external agents should not adopt it.
- persona document: durable user profile and applied persona updates.
- system document: workspace governance rules.
- progress index: managed document status/progress table.

Governed writes follow this path:

```text
proposal / observed change
        -> draft
        -> local review
        -> approve or reject
        -> controlled apply
        -> audit / findings / process records
```

External agents can submit proposals. Only local Lore/runtime can approve or apply.

The architecture decision record is [`docs/adr-lore-v1-architecture.md`](docs/adr-lore-v1-architecture.md).

## Persona Memory

Lore includes an LLM-based persona candidate pipeline:

```text
TUI / console user turn
        -> background persona extraction
        -> evidence substring validation
        -> candidate store with dedup
        -> operator review
        -> persona_update draft
        -> local approve/apply
```

Key constraints:

- Extraction is asynchronous and must not block the chat turn.
- Candidates require evidence from the user's own text.
- Low-confidence or unverifiable candidates are discarded.
- Candidates are deduplicated before review.
- Draft creation reuses the same governance path as MCP `persona_update_propose`.
- No persona update is auto-approved or auto-applied.

CLI surfaces:

```powershell
lore persona candidates list   --workdir <dir> [--state open|drafted|dismissed] [--json]
lore persona candidates show   <id> --workdir <dir> [--json]
lore persona candidates draft  <id> --workdir <dir> [--retry-rejected]
lore persona candidates dismiss <id> --workdir <dir>
lore persona candidates recover <id> --workdir <dir> --link <draft-id>
lore persona candidates recover <id> --workdir <dir> --force-dismiss
lore persona errors --workdir <dir> [--tail N] [--stage extract|store|parse_warning] [--json]
lore persona summary --workdir <dir> [--fail-on-orphan] [--json]
```

Maintainer overview: [`docs/persona-memory-pipeline.md`](docs/persona-memory-pipeline.md).

## Process Sink And Session Logs

Lore can import and summarize agent transcripts into durable process records:

- Codex JSONL import: `import-codex-jsonl`
- Codex JSONL incremental sync: `sync-codex-jsonl`
- Codex JSONL attach/watch mode: `attach-codex-jsonl`
- Codex app-server import: `import-codex-appserver`
- External transcript JSONL import: `import-external-jsonl`
- Daemon-driven vault scan and process handling: `daemon run`

The process-sink flow creates checkpoints and daily reports. It does not automatically create formal notes or persona updates; durable knowledge should enter through proposal/draft review.

Session logs use an incremental index and streaming search path so transcript search remains bounded as sessions grow.

## Usage And Cost Attribution

`lore usage` reports model calls and tokens by day, purpose, and model:

```powershell
lore usage --days 7 <dir>
lore usage --days 7 --json <dir>
```

Purposes currently include:

- `chat`
- `persona_extract`
- `process_sink`

The report shows:

- daily calls / prompt / completion / total tokens
- purpose breakdown
- per-purpose model detail
- cross-purpose model rollup
- JSON with uncapped `purpose_breakdown.<purpose>.by_model`

This lets operators see whether cost is coming from chat, background persona extraction, or process-sink summarization.

## Configuration

Lore loads config in increasing precedence:

1. Built-in defaults from `config.Default(workDir)`.
2. User-global config at `~/.lore/config.json`.
3. Workspace config at `<workDir>/.lore/config.json`.

Example workspace config:

```json
{
  "llm": {
    "active_profile": "deepseek",
    "profiles": {
      "deepseek": {
        "provider": "deepseek",
        "base_url": "https://api.deepseek.com/v1",
        "model": "deepseek-v4-pro",
        "timeout": 30000000000,
        "api_key_env": "DEEPSEEK_API_KEY"
      }
    }
  }
}
```

Rules:

- API key values are never written to config, session logs, diagnostics, or TUI panels.
- Profiles store the environment variable name, for example `DEEPSEEK_API_KEY`.
- DeepSeek presets use official model names such as `deepseek-v4-pro` and `deepseek-v4-flash`.
- Short provider aliases such as `v4-pro` are rejected by the provider.
- Duration fields in JSON are currently nanoseconds; string durations are not the stable config format yet.

TUI model commands:

```text
/model
/model profiles
/model current
/model use <model-or-profile>
/model test
/model persist <profile>
```

## Workdir Layout

A bootstrapped workdir contains:

```text
<workdir>/
  .lore/
    config.json
  vault/
    00-系统/
    0-排期/
    03-画像/
    agent.md
    identity.md
  state/
    store.db
    logs/
```

Runtime state is stored in SQLite at `state/store.db`. Legacy `state/store.json` migration is supported.

Vault writes use atomic file operations and path/symlink boundary checks. Managed-core and governed targets are not writable through external MCP.

## Repository Layout

- `cmd/lore`: preferred CLI/TUI/MCP entrypoint.
- `cmd/obsidian-harness`: compatibility entrypoint.
- `internal/app`: runtime orchestration facade and operator-facing app methods.
- `internal/bootstrap`: managed workspace templates.
- `internal/config`: layered config, LLM profiles, diagnostics, model identity.
- `internal/console`: operator session loop, persona extraction hook, usage recording.
- `internal/mcp`: stdio MCP server and tool dispatch boundary.
- `internal/model`: shared domain and runtime types.
- `internal/operatoragent`: model-backed loop agent and protocol parser.
- `internal/orchestrator`: draft governance, review/apply, progress sync, read API.
- `internal/persona`: persona extraction model, prompt, parser, dedup helpers.
- `internal/sessionlog`: session transcript writer, index, and streaming search.
- `internal/store`: store interfaces plus memory, JSON legacy, and SQLite backends.
- `internal/tools`: registry-owned tool contracts shared by console/MCP surfaces.
- `internal/tui`: Bubble Tea interactive workbench and renderers.
- `internal/vault`: vault I/O, path safety, search, backlinks, and attachment parsing.
- `sdk/go/lore`: Go SDK for MCP read contracts.
- `docs/`: ADRs, integration guides, plans, handoffs, manual tests.

## Development And Validation

Use the repo-managed Go toolchain when available:

```powershell
.\.tools\go\bin\go.exe test ./...
```

Or system Go:

```powershell
go test ./...
```

Release-oriented checks:

```powershell
.\scripts\release-gate.ps1
.\scripts\release-gate.ps1 -Full
.\scripts\verify.ps1
```

Useful smoke commands:

```powershell
go run ./cmd/lore smoke p0 --workdir .\tmp\p0-smoke
go run ./cmd/lore tui --workdir .\tmp\p0-smoke --once "show current status"
```

CI expectations:

- Pull requests run the deterministic release gate.
- Pushes to `main` run the full gate.
- Optional E2E checks are enabled separately.
- Release gates are expected to leave the repo clean.

## Key Docs

- Architecture: [`docs/adr-lore-v1-architecture.md`](docs/adr-lore-v1-architecture.md)
- MCP setup: [`docs/integrations/mcp-client-setup.md`](docs/integrations/mcp-client-setup.md)
- Persona pipeline: [`docs/persona-memory-pipeline.md`](docs/persona-memory-pipeline.md)
- Persona manual test: [`docs/persona-memory-manual-test.md`](docs/persona-memory-manual-test.md)
- TUI manual test: [`docs/tui-manual-test.md`](docs/tui-manual-test.md)
- Product/developer handoff: [`docs/handoff-full-project-review-2026-05-23.md`](docs/handoff-full-project-review-2026-05-23.md)
- Archived review handoffs: [`docs/archive/review-handoff/`](docs/archive/review-handoff/)

## Non-Goals

- No MCP shell.
- No external-agent direct vault writes.
- No MCP draft approve/apply.
- No automatic persona apply.
- No generic proposal API before narrow proposal tools prove stable.
- No claim that post-scan prevents out-of-band writes; it detects and reconciles them.

Lore is intentionally not a generic "agent can edit anything" system. It is a governed local workspace agent whose main job is to keep the vault useful, reviewable, and recoverable.
