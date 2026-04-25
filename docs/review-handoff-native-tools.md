# Review Handoff: Responses Native Tools

Reviewer: DeepSeek
Scope owner: Codex
Date: 2026-04-24

## Scope

This handoff covers only the backend/operator-agent changes for OpenAI-compatible `Responses API` native tool calling.

It intentionally does not cover TUI presentation changes in `internal/tui/interactive_*`.

## Files To Review

- `internal/llm/openai/client.go`
- `internal/llm/openai/client_test.go`
- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `internal/operatoragent/tool_schema.go`

## Intent

- Send Lore tools through the `Responses API` native `tools` field for `gpt-5.4`.
- Parse native `function_call` items from `Responses API` output into `ToolCall` records.
- Execute native tool calls in the existing Lore loop before falling back to text JSON parsing.
- Keep Lore loop semantics to one tool call per loop step.
- Avoid old JSON-mode retry fallback when native tools fail.

## Important Semantics

- `parallel_tool_calls` is sent as `false` when tools are present.
- If the provider still returns more than one native tool call, Lore returns an explicit error and executes none.
- Tool schemas are intentionally loose and `strict:false` for provider compatibility.
- `operatoragent` still retains text JSON parsing for non-tool final responses and legacy text tool-call responses.
- `shell_exec` confirmation behavior is unchanged.

## Verification Already Run

```powershell
.\.tools\go\bin\go.exe test ./internal/llm/openai ./internal/operatoragent -count=1
.\.tools\go\bin\go.exe test ./... -count=1
```

Manual provider smoke was also run successfully with:

```powershell
.\.tools\go\bin\go.exe run ./cmd/lore console --workdir .\tmp\chat-demo --once "show current workspace status briefly"
```

## Review Focus

- Check that `strict:false` is actually serialized and not accidentally normalized back to `true`.
- Check that `parallel_tool_calls:false` appears only when tools are present.
- Check that multiple native tool calls are rejected before any runtime tool executes.
- Check that text JSON parsing remains as a normal parse path, not as a hidden retry fallback after API errors.
- Check that prompt/tool description compression does not remove necessary runtime governance constraints.

## Known Adjacent Changes

There are concurrent TUI changes in:

- `internal/tui/interactive_workbench.go`
- `internal/tui/interactive_render.go`
- `internal/tui/interactive_render_test.go`

Those belong to the TUI review lane and should not be mixed with this backend review unless a test failure crosses layers.
