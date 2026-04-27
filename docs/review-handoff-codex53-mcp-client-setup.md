# Review Handoff: MCP External Client Setup

## Scope

Added `docs/integrations/mcp-client-setup.md` as the first user-facing integration guide for external agents connecting to Lore over MCP.

This is docs-only. It does not change MCP server behavior, SDK behavior, or TUI behavior.

## Facts To Verify

- Command shape is `lore mcp [workdir]` from `internal/cli/cli.go`; the integration guide recommends an explicit workdir for external clients.
- MCP methods are `initialize`, `ping`, `tools/list`, and `tools/call` from `internal/mcp/server.go`.
- Process auth uses server-side `LORE_MCP_API_KEY` / `OBSIDIAN_HARNESS_MCP_API_KEY` and client-side `LORE_CLIENT_KEY` / `OBSIDIAN_HARNESS_CLIENT_KEY`.
- Tool surface is read-only and matches `docs/contracts/mcp-sdk-tools-v0.json`.
- SDK E2E smoke command is `powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1 -E2E"`.

## Review Focus

- Check that every concrete client config uses stdio and starts `lore mcp <workdir>`.
- Check that the doc does not imply writable tools, shell execution, HTTP/SSE/WS support, MCP resources, or prompts.
- Check that external-client syntax is framed as client-specific config, while the Lore contract source remains the local artifact.
- Check that auth wording does not confuse `LORE_MCP_API_KEY` with `LORE_CLIENT_KEY`.
- Check Windows JSON path escaping and PowerShell examples.

## External Syntax Sources Used

- Model Context Protocol local server setup: <https://modelcontextprotocol.io/docs/tutorials/use-local-mcp-server>
- Claude Code MCP setup: <https://code.claude.com/docs/en/mcp>
- OpenCode MCP server setup: <https://opencode.ai/docs/mcp-servers>
- Gemini CLI MCP server setup: <https://google-gemini.github.io/gemini-cli/docs/tools/mcp-server.html>

## Verification

Run:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```

Optional E2E:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1 -E2E"
```
