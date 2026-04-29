# External MCP Client Setup

Lore exposes its governed vault surface as a local MCP server. External agents can connect to it over stdio, use read tools, and submit narrow proposal-intake requests for Lore review.

## Scope

- Transport: stdio only.
- Command shape: `lore mcp [workdir]`. External client configs should pass an explicit workdir.
- Protocol methods: `initialize`, `ping`, `tools/list`, `tools/call`.
- Tool surface: read tools plus proposal intake. Lore does not expose shell execution, writable vault mutation, TUI actions, draft approve/apply, or runtime policy changes through MCP.
- SDK v0 read-only contract source: `docs/contracts/mcp-sdk-tools-v0.json`.
- Go SDK v0 reference: `sdk/go/lore/README.md`.

The Go SDK v0 typed methods remain read-only. MCP proposal intake is available through `tools/call` for clients that use the raw tool surface. Current external clients must not assume direct vault-write tools exist.

External MCP is an intake surface, not a write surface. Ordinary markdown notes that should be written through Lore must be submitted through proposal-intake tools such as `markdown_note_propose`; local Lore reviews, creates revised drafts when needed, approves, and applies them.

## Prerequisites

1. Build or locate a `lore` executable that external clients can run.
2. Bootstrap the target workdir before connecting:

   ```powershell
   lore bootstrap C:\path\to\lore-workdir
   ```

   `lore bootstrap` creates missing managed core documents, but it does not overwrite existing `agent.md`, `identity.md`, persona, system, or progress documents. Existing workdirs should be manually or otherwise governed-updated to the current `agent.md` and `identity.md` templates before external agent onboarding.

3. Prefer absolute paths in external client config. Many MCP clients launch servers from their own working directory.
4. If process auth is enabled, set matching keys:

   - Server expected key: `LORE_MCP_API_KEY` or `OBSIDIAN_HARNESS_MCP_API_KEY`.
   - Client provided key: `LORE_CLIENT_KEY` or `OBSIDIAN_HARNESS_CLIENT_KEY`.

If `LORE_MCP_API_KEY` is not set, Lore accepts local MCP clients without a client key.

For stdio clients, both variables are read by the spawned `lore mcp` child process. The expected key can be inherited from the environment that launches the external client, or set in the client config alongside the client key. The invariant is simple: if an expected key is present, the client key must match it. Do not commit real keys into shared config files.

## Generic Stdio MCP Config

Use this shape for clients that accept an `mcpServers` map with stdio process config:

```json
{
  "mcpServers": {
    "lore": {
      "command": "C:\\path\\to\\lore.exe",
      "args": ["mcp", "C:\\path\\to\\lore-workdir"],
      "env": {
        "LORE_CLIENT_KEY": "same-value-as-LORE_MCP_API_KEY-if-enabled"
      }
    }
  }
}
```

If the client already inherits the needed Lore auth environment variables, the `env` block can be omitted.

Copyable example files live under `docs/integrations/examples/`:

- `generic-mcp.json`
- `claude-desktop-mcp.json`
- `claude-code-project.mcp.json`
- `opencode.jsonc`
- `gemini-cli-settings.json`

## External Agent Onboarding

After connecting, an external agent should first read the workspace operating manual:

```json
{
  "name": "system_doc_get",
  "arguments": {
    "name": "agent"
  }
}
```

`agent.md` is the external-first shared operating manual. It tells external agents how to work through Lore, which documents are governed, and how to hand off changes that require Lore review.

Do not default-read `identity.md` for external onboarding. `identity.md` is Lore's local self identity; an external agent must not adopt it as its own identity.

For existing workdirs, confirm `agent.md` contains the current external-first operating manual. Bootstrapping does not replace an older `agent.md` that already exists.

When task-relevant, use additional read tools:

- `system_doc_get("persona")` for user long-term profile context.
- `system_doc_get("system")` for workspace governance rules.
- `system_doc_get("progress")` for managed document status.
- `context_pack` for a task or target document.

### Persona Update Handoff

If an external agent observes a stable user profile fact, education fact, long-term preference, or a conflict with the persona document, it must not edit `人物画像.md` directly.

MCP proposal intake includes `persona_update_propose`. That tool creates a pending `persona_update` draft and returns `draft_created`, `draft_id`, `target`, and `review_required: true`. Creating a proposal does not update the persona document and does not apply a draft.

Arguments:

- `field`
- `current_value` (optional)
- `proposed_value`
- `evidence`
- `reason`
- `confidence`: `low`, `medium`, or `high`
- `source`
- `observed_at`: RFC3339 timestamp or `YYYY-MM-DD`

When a client cannot call `persona_update_propose`, the external agent should include this block in its final response:

```text
Persona Update Candidate:
- field:
- current_value:
- proposed_value:
- evidence:
- reason:
- confidence: low|medium|high
- source: external_agent
- observed_at:
- action: request_lore_review
```

Both `persona_update_propose` and this fallback are handoffs to local Lore. They are not persona writes and are not draft apply.

### Markdown Note Handoff

If an external agent produces a classroom note, meeting note, development summary, or other durable markdown content that should enter the vault through Lore, it must not expect a direct vault-write MCP tool.

MCP proposal intake includes `markdown_note_propose`. That tool creates a pending markdown-note draft only; local Lore must review the content with persona, weakness, system, and progress context, create a revised/superseding draft when needed, approve, and apply locally.

Arguments:

- `target_path`: ordinary markdown note path, for example `03-notes/inbox/ecommerce-platforms.md`.
- `title`
- `content`: full proposed markdown content.
- `source_kind`: `class`, `meeting`, `development`, `conversation`, `research`, or `other`.
- `evidence`
- `reason`
- `source`
- `observed_at`: RFC3339 timestamp or `YYYY-MM-DD`.
- `task_context` (optional)
- `course` (optional)
- `topic` (optional)
- `dedupe_key` (optional)

Example request:

```json
{
  "name": "markdown_note_propose",
  "arguments": {
    "target_path": "03-notes/inbox/ecommerce-platforms.md",
    "title": "E-commerce Platforms",
    "content": "# E-commerce Platforms\n\n- Marketplaces coordinate buyers and sellers.",
    "source_kind": "class",
    "evidence": "class transcript discussed marketplace coordination",
    "reason": "durable class note for later review",
    "source": "external_agent",
    "observed_at": "2026-04-28T10:30:00+08:00",
    "task_context": "class note extraction",
    "course": "E-commerce",
    "topic": "platforms",
    "dedupe_key": "ecommerce-platforms-2026-04-28"
  }
}
```

Expected result:

```json
{
  "status": "draft_created",
  "draft_id": "draft-...",
  "target": "03-notes/inbox/ecommerce-platforms.md",
  "review_required": true
}
```

Creating this proposal does not write the note and does not apply a draft. If a client cannot call `markdown_note_propose`, the external agent should include this block in its final response:

```text
Markdown Note Candidate:
- title:
- target_path:
- source_kind: class|meeting|development|conversation|research|other
- evidence:
- reason:
- content:
- source: external_agent
- observed_at:
- action: request_lore_review
```

Existing workdirs may still have an older `agent.md` without this markdown-note proposal guidance. Update that managed document through a governed local process before relying on external-agent onboarding.

## External Transcript Import

MCP is for live context reads and proposal intake. Bulk chat transcript ingestion is a CLI/process-sink flow, not an MCP tool.

For external-agent transcripts, write NDJSON using Lore's external transcript JSONL schema. This is not an automatic adapter for arbitrary third-party JSONL formats.

Use one optional session metadata row and message rows:

```json
{"type":"session_meta","agent_id":"Claude Code","session_id":"class-1"}
{"timestamp":"2026-04-22T09:05:00+08:00","role":"user","text":"summarize the class"}
{"timestamp":"2026-04-22T09:35:00+08:00","role":"assistant","phase":"final","text":"class summary ready"}
```

Then import it:

```powershell
lore import-external-jsonl --workdir C:\path\to\lore-workdir --input C:\path\to\external.jsonl
```

This creates process-sink checkpoints and daily reports using the existing 30-minute window pipeline. It does not create formal notes, does not update persona, and does not submit markdown proposals. If the transcript contains durable knowledge that should enter the vault, use `markdown_note_propose` separately.

Current external transcript import is one-shot only. Incremental attach/sync remains Codex-specific for now.

## Claude Desktop

Claude Desktop uses `claude_desktop_config.json` with an `mcpServers` object. The config file is normally opened from Settings -> Developer -> Edit Config.

Windows path:

```text
%APPDATA%\Claude\claude_desktop_config.json
```

macOS path:

```text
~/Library/Application Support/Claude/claude_desktop_config.json
```

Example:

```json
{
  "mcpServers": {
    "lore": {
      "command": "C:\\path\\to\\lore.exe",
      "args": ["mcp", "C:\\path\\to\\lore-workdir"],
      "env": {
        "LORE_CLIENT_KEY": "same-value-as-LORE_MCP_API_KEY-if-enabled"
      }
    }
  }
}
```

Restart Claude Desktop after editing the config. If the server does not appear, check the Claude MCP logs:

```text
%APPDATA%\Claude\logs
```

## Claude Code

Claude Code can add a local stdio server from the CLI:

```powershell
claude mcp add --transport stdio --env LORE_CLIENT_KEY=same-value-as-LORE_MCP_API_KEY-if-enabled lore -- C:\path\to\lore.exe mcp C:\path\to\lore-workdir
```

For a project-scoped checked-in config, use `.mcp.json`:

```json
{
  "mcpServers": {
    "lore": {
      "command": "C:\\path\\to\\lore.exe",
      "args": ["mcp", "C:\\path\\to\\lore-workdir"],
      "env": {
        "LORE_CLIENT_KEY": "${LORE_CLIENT_KEY:-}"
      }
    }
  }
}
```

Use `/mcp` inside Claude Code or `claude mcp list` to verify the connection.

## OpenCode

OpenCode configures local MCP servers under the `mcp` object in `opencode.jsonc`.

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "lore": {
      "type": "local",
      "command": ["C:\\path\\to\\lore.exe", "mcp", "C:\\path\\to\\lore-workdir"],
      "enabled": true,
      "environment": {
        "LORE_CLIENT_KEY": "same-value-as-LORE_MCP_API_KEY-if-enabled"
      }
    }
  }
}
```

OpenCode exposes MCP tools alongside built-in tools. If context size becomes a problem, disable the Lore MCP globally and enable it only for the agent that needs vault read access.

## Gemini CLI

Gemini CLI reads MCP server definitions from `settings.json` under `mcpServers`.

```json
{
  "mcpServers": {
    "lore": {
      "command": "C:\\path\\to\\lore.exe",
      "args": ["mcp", "C:\\path\\to\\lore-workdir"],
      "env": {
        "LORE_CLIENT_KEY": "$LORE_CLIENT_KEY"
      },
      "timeout": 30000,
      "trust": false
    }
  }
}
```

Keep `trust` false unless you have reviewed the exact tool surface. Lore does not expose direct vault-write MCP tools, but it does expose proposal intake, and the external client still controls whether to ask before tool execution.

## Available Tools

The SDK-facing read-only contract is frozen in `docs/contracts/mcp-sdk-tools-v0.json`. Raw MCP `tools/list` also includes proposal-intake tools that are not part of the Go SDK v0 typed read-only methods.

| Tool | Purpose |
| --- | --- |
| `managed_status` | Return managed mode and core document status. |
| `system_doc_get` | Read a managed core document by name. |
| `vault_read` | Read a markdown document from the vault. |
| `vault_list` | List vault files under a directory. |
| `vault_search_text` | Search vault text content. |
| `vault_resolve` | Resolve a natural-language note reference into unique, ambiguous, or not_found. |
| `vault_backlinks` | Find backlinks to a vault document. |
| `doc_classify` | Classify a vault document by current rules. |
| `context_pack` | Build a read-only context pack for a task or target document. |
| `persona_update_propose` | Create a pending persona_update draft for Lore review; does not write or apply the persona document. |
| `markdown_note_propose` | Create a pending markdown-note draft for Lore review; does not write the note and does not apply a draft. |

Standard argument names should be preferred:

- `vault_list`: use `dir`; server still accepts deprecated `path` as an alias.
- `vault_search_text`: use `dir`; server still accepts deprecated `path` as an alias.
- `vault_resolve`: use `dir`; server still accepts deprecated `path` as an alias.
- `context_pack`: use `target_path`; server still accepts deprecated `path` as an alias.

## Smoke Tests

Repository-level E2E smoke builds the real CLI and starts `lore mcp <workdir>`. It runs both the Go SDK E2E path and the raw stdio MCP frame smoke:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1 -E2E"
```

The raw stdio smoke can also be run directly. It sends MCP JSON-RPC frames to `lore mcp` without using the Go SDK:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\smoke-mcp-stdio.ps1
```

Against an existing binary and workdir:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\smoke-mcp-stdio.ps1 -Lore C:\path\to\lore.exe -WorkDir C:\path\to\lore-workdir
```

To include process auth in the smoke:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\smoke-mcp-stdio.ps1 -MCPAPIKey smoke-key -ClientKey smoke-key
```

For default verification without E2E:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```

## Troubleshooting

`mcp authentication failed: client key mismatch`

Set `LORE_CLIENT_KEY` to exactly the same value as `LORE_MCP_API_KEY`, or unset `LORE_MCP_API_KEY` for local unauthenticated use.

`open runtime` or managed status failures

Run `lore bootstrap <workdir>` first and confirm the client config points at that same workdir.

Client reports command not found

Use an absolute path to `lore.exe`. On Windows, remember to escape backslashes in JSON strings.

Tools are not discovered

Confirm the client started the stdio process, then check client-specific MCP logs. Lore writes protocol responses to stdout; diagnostic errors go to stderr.

Tool calls fail with bad arguments

For SDK-style read calls, compare the client's generated arguments against `docs/contracts/mcp-sdk-tools-v0.json` and prefer `dir` and `target_path` over deprecated `path` aliases. For proposal intake, inspect the live `tools/list` schema from the MCP server.

## Current Limits

- No HTTP, SSE, or WebSocket transport in v0.
- No direct vault-write MCP tools.
- Proposal intake can create pending drafts only; it cannot apply drafts or update governed documents.
- No cross-client persistent memory is enabled by MCP. The external agent receives tool results in its own conversation context; Lore does not automatically share that context with other agents.
- No MCP resources or prompts are exposed yet; tools are the only public surface.

## External References

- Model Context Protocol local server setup: <https://modelcontextprotocol.io/docs/tutorials/use-local-mcp-server>
- Claude Code MCP setup: <https://code.claude.com/docs/en/mcp>
- OpenCode MCP server setup: <https://opencode.ai/docs/mcp-servers>
- Gemini CLI MCP server setup: <https://google-gemini.github.io/gemini-cli/docs/tools/mcp-server.html>
