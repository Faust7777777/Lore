# Review Handoff: MCP External Client Setup

## Scope

Added `docs/integrations/mcp-client-setup.md` as the first user-facing integration guide for external agents connecting to Lore over MCP.

Follow-up added copyable example configs under `docs/integrations/examples/` and tests in `internal/mcp/client_examples_test.go`.

This follow-up adds `scripts/smoke-mcp-stdio.ps1`, a raw stdio smoke that sends MCP frames directly to `lore mcp` without the Go SDK.

This does not change MCP server behavior, SDK behavior, or TUI behavior. The only executable changes are verification assets: example-config tests, the raw stdio smoke script, and `verify.ps1 -E2E` wiring.

## Facts To Verify

- Command shape is `lore mcp [workdir]` from `internal/cli/cli.go`; the integration guide recommends an explicit workdir for external clients.
- MCP methods are `initialize`, `ping`, `tools/list`, and `tools/call` from `internal/mcp/server.go`.
- Process auth uses server-side `LORE_MCP_API_KEY` / `OBSIDIAN_HARNESS_MCP_API_KEY` and client-side `LORE_CLIENT_KEY` / `OBSIDIAN_HARNESS_CLIENT_KEY`.
- Tool surface is read-only and matches `docs/contracts/mcp-sdk-tools-v0.json`.
- E2E smoke command is `powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1 -E2E"`; it runs the SDK E2E and the raw stdio smoke.
- Raw MCP stdio smoke command is `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\smoke-mcp-stdio.ps1`.

## Review Focus

- Check that every concrete client config uses stdio and starts `lore mcp` with an explicit workdir, even though the CLI accepts `lore mcp [workdir]`.
- Check that the doc does not imply writable tools, shell execution, HTTP/SSE/WS support, MCP resources, or prompts.
- Check that external-client syntax is framed as client-specific config, while the Lore contract source remains the local artifact.
- Check that auth wording does not confuse `LORE_MCP_API_KEY` with `LORE_CLIENT_KEY`.
- Check Windows JSON path escaping and PowerShell examples.
- Check that example config files stay aligned with the inline snippets and still start `lore mcp <explicit-workdir>`.
- Check that `scripts/smoke-mcp-stdio.ps1` does not use the SDK path and only exercises MCP protocol frames.

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

Targeted:

```powershell
.\.tools\go\bin\go.exe test ./internal/mcp -run TestExternalClientExamples -count=1 -v
.\.tools\go\bin\go.exe test ./internal/mcp -run TestOpenCodeExample -count=1 -v
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\smoke-mcp-stdio.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\smoke-mcp-stdio.ps1 -MCPAPIKey smoke-key -ClientKey smoke-key
```

Optional E2E:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1 -E2E"
```
