# Review Handoff: Chat History As Real Messages

## Scope

This change fixes weak short-follow-up context in the local Lore chat agent and adds the first history-window split.

It changes only:

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`

It does not change session persistence, resume behavior, MCP, SDK, TUI rendering, CoreContext, or v1 architecture docs.

## Problem

`Session.History` was stored in memory, but `ModelAgent.Respond` did not send it as role-aware chat history.

Before this change, the model request was effectively:

- `system`: loop system prompt
- `user`: current request plus a textual `Recent conversation` block

Each history line was flattened through `oneLine(..., 220)`, which weakened role continuity and made short follow-ups like `给` easy to misinterpret or repeat.

## Change

`ModelAgent.Respond` now builds initial loop messages as:

- `system`: loop system prompt
- optional older-history summary as a `user` message
- recent `user` / `assistant` turns from `ctx.History`
- `user`: current request plus working set context

The current user prompt no longer embeds `Recent conversation`.

Unknown or empty history roles are skipped.

Current limits:

- recent full chat history: `maxLoopRecentHistoryMessages = 6`
- older summarized history window: `maxLoopSummaryHistoryMessages = 14`

The older summary still uses flattened text, but only for turns outside the recent strong-context window. `oneLine` now truncates by rune, not byte, so Chinese text cannot be split into invalid UTF-8.

The older summary is intentionally not a `system` message because it is derived from untrusted prior chat content. It is labeled as untrusted chat context and sent with user priority.

Tool loop behavior is unchanged: synthetic assistant tool-call messages and tool-result user messages are still appended after the initial message list during the loop.

## External Pattern Check

A short comparison pass against OpenCode, Codex CLI, Gemini CLI, and Aider found the same practical pattern:

- keep recent conversation as role-aware messages;
- summarize or compact older history only;
- keep tool call/result adjacency intact when structured tool history is available;
- avoid treating provider-specific compaction as the only solution.

Lore currently stores `Session.History` as final user/assistant turns, not full structured tool call/result parts. This change therefore fixes normal short-follow-up continuity first. Structured persisted tool history is a separate future improvement.

## Review Focus

- Confirm recent history is sent as real `openai.Message` items with `Role: "user"` and `Role: "assistant"`.
- Confirm older history is summarized only when it falls outside the recent-history window.
- Confirm older history summary is not sent as `system` or `developer`.
- Confirm working set context still stays in the current user prompt.
- Confirm native tool-call loop message ordering remains stable.
- Confirm rune-based truncation avoids invalid UTF-8 for Chinese text.
- Confirm the system prompt warns that previous assistant messages are user-facing history while the current turn must still return a JSON envelope.
- Confirm this does not imply cross-session memory unless the caller has restored `ctx.History`.
- Confirm TUI files are unrelated and should not be staged with this change.

## Tests

Added:

- `TestModelAgentRespondSendsRecentHistoryAsChatMessages`
- `TestModelAgentRespondSummarizesOlderHistoryAndKeepsRecentMessagesFull`
- `TestModelAgentRespondPromptWarnsAssistantHistoryIsNotOutputFormat`
- `TestOneLineTruncatesChineseAsValidUTF8`

Regression checks run:

```powershell
.\.tools\go\bin\go.exe test ./internal/operatoragent -run "TestModelAgentRespondSendsRecentHistoryAsChatMessages|TestModelAgentRespondSummarizesOlderHistoryAndKeepsRecentMessagesFull|TestModelAgentRespondPromptWarnsAssistantHistoryIsNotOutputFormat|TestOneLineTruncatesChineseAsValidUTF8|TestModelAgentRespondRunsNativeToolCallThenFinal" -count=1 -v
.\.tools\go\bin\go.exe test ./internal/operatoragent -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ".\scripts\verify.ps1"
```

All passed.
