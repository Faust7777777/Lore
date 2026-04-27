# External MCP Client Setup

Lore exposes its read-only vault surface as a local MCP server. External agents can connect to it over stdio and call the same tool contract used by the Go SDK.

## Scope

- Transport: stdio only.
- Command shape: `lore mcp <workdir>`.
- Protocol methods: `initialize`, `ping`, `tools/list`, `tools/call`.
- Tool surface: read-only tools only. Lore does not expose shell execution, writable vault mutation, TUI actions, or runtime policy changes through MCP.
- Contract source: `docs/contracts/mcp-sdk-tools-v0.json`.
- Go SDK reference: `sdk/go/lore/README.md`.

## Prerequisites

1. Build or locate a `lore` executable that external clients can run.
2. Bootstrap the target workdir before connecting:

   ```powershell
   lore bootstrap C:\path\to\lore-workdir
   ```

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

Keep `trust` false unless you have reviewed the exact tool surface. Lore is read-only, but the external client still controls whether to ask before tool execution.

## Available Tools

The SDK-facing contract is frozen in `docs/contracts/mcp-sdk-tools-v0.json`.

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

Standard argument names should be preferred:

- `vault_list`: use `dir`; server still accepts deprecated `path` as an alias.
- `vault_search_text`: use `dir`; server still accepts deprecated `path` as an alias.
- `vault_resolve`: use `dir`; server still accepts deprecated `path` as an alias.
- `context_pack`: use `target_path`; server still accepts deprecated `path` as an alias.

## Smoke Tests

Repository-level SDK E2E smoke builds the real CLI, bootstraps a temporary workdir, starts `lore mcp <workdir>`, and exercises `Ping`, `ManagedStatus`, `VaultResolve`, and `ContextPack`:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1 -E2E"
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

Compare the client's generated arguments against `docs/contracts/mcp-sdk-tools-v0.json`. For SDK-style calls, prefer `dir` and `target_path` over deprecated `path` aliases.

## Current Limits

- No HTTP, SSE, or WebSocket transport in v0.
- No writable MCP tools.
- No cross-client persistent memory is enabled by MCP. The external agent receives tool results in its own conversation context; Lore does not automatically share that context with other agents.
- No MCP resources or prompts are exposed yet; tools are the only public surface.

## External References

- Model Context Protocol local server setup: <https://modelcontextprotocol.io/docs/tutorials/use-local-mcp-server>
- Claude Code MCP setup: <https://code.claude.com/docs/en/mcp>
- OpenCode MCP server setup: <https://opencode.ai/docs/mcp-servers>
- Gemini CLI MCP server setup: <https://google-gemini.github.io/gemini-cli/docs/tools/mcp-server.html>
