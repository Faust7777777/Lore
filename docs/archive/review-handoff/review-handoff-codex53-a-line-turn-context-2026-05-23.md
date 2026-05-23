# Review Handoff: A-Line Turn Context Cancellation

Date: 2026-05-23

## Scope

- `internal/operatoragent/agent.go`
- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `internal/console/session.go`
- `internal/console/session_test.go`
- `internal/console/tool_runtime.go`
- `internal/console/tool_runtime_test.go`
- `internal/cli/tui_workbench.go`
- `internal/tui/interactive_workbench.go`

## Summary

Adds a compatible turn-level `context.Context` path through the local operator loop.

New behavior:

- `ModelAgent.Respond` remains source-compatible and delegates to `RespondContext(context.Background(), ...)`.
- `ContextLoopAgent` lets console callers pass a per-turn context into loop agents without changing the legacy `LoopAgent` interface.
- `ContextToolRuntime` lets operatoragent dispatch tools with the same turn context when a runtime supports it.
- `console.Session.Handle` remains source-compatible and delegates to `HandleContext(context.Background(), ...)`.
- `interactiveWorkbenchDriver.ExecuteContext` calls `Session.HandleContext`, and TUI shell dispatch uses the optional context-aware driver interface.
- Local git tools derive their command timeout context from the turn context instead of an independent background root.

Cancellation handling:

- Pre-cancelled turns return before model calls or session history writes.
- Cancellation during a model call is propagated through the LLM client context.
- Cancellation during a context-aware tool call terminates the turn with `TurnStopToolError` and preserves already-billed model usage via `UsageError`.
- Console tool runtime rejects already-cancelled contexts before dispatch.

## Boundary

- No MCP surface change.
- No persona candidate state-machine change.
- No draft governance path change.
- No TUI presentation change.
- Existing `LoopAgent`, `ToolRuntime`, `Session.Handle`, and `InteractiveWorkbenchDriver.Execute` call sites remain compatible.

## Regression Tests

- `TestModelAgentRespondContextBeforeModelCallCancelled`
- `TestModelAgentRespondContextDuringModelCallCancelled`
- `TestModelAgentRespondContextDuringToolCallCancelled`
- `TestSessionHandleContextCancelledBeforeLoopAgentDoesNotRecordTurn`
- `TestToolRuntimeCallToolContextCancelledBeforeDispatch`

## Validation

```powershell
go test ./internal/operatoragent -run "RespondContext|Context|Cancelled" -count=1 -v
go test ./internal/console -run "HandleContext|CallToolContext" -count=1 -v
go test ./internal/operatoragent ./internal/console ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
