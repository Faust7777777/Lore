# Review Handoff: A-Line Tool Result Context Bound

Date: 2026-05-23

## Scope

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`

## Reading Gate

Read before implementation:

- `AGENTS.md`
- `docs/handoff-full-project-review-2026-05-23.md`
- `docs/review-handoff-index.md`
- `docs/archive/OPUS_COLLAB_GUARDRAILS.md`
- A-line release gate / SDK handoffs under `docs/archive/review-handoff/`
- `C:\Users\15892\Desktop\docs\hermes-workspace\tui-module-handoff.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-console-llm-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-orchestrator-operatoragent-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-sdk-handover.md`
- `C:\Users\15892\Desktop\docs\hermes-workspace\lore-infra-handover.md`

## Summary

Bounds tool-result content before it is re-injected into subsequent model messages.

Before this change, `TurnStep.ObservationExcerpt` was bounded for UI/session display, but the complete tool result was still appended to the model conversation through `buildToolResultPrompt`. A large `vault_read` or search result could therefore inflate the next model request even though the visible progress excerpt was safe.

This change adds a separate model-context cap:

- `maxToolResultPromptBytes = 4096`
- success tool results are rune-safely truncated before model re-injection
- tool error text is also bounded, including the existing "error + content" shape
- invalid UTF-8 tool content is replaced before model re-injection

## Boundary

- No MCP surface change.
- No persona governance change.
- No TUI rendering/state-machine change.
- No SDK change.
- `TurnStep.ObservationExcerpt` remains independently bounded for UI/session consumers.

## Review Focus

- Confirm truncation happens in `buildToolResultPrompt`, not only in UI excerpts.
- Confirm the raw tool result tail is not sent into the second model request in the new fake-model regression test.
- Confirm this does not alter tool dispatch or external MCP exposure.

## Validation

```powershell
go test ./internal/operatoragent -run "TestModelAgentRespond(BoundsToolResultReinjectionIntoModelContext|TurnStepTruncatesLongObservation|TurnStepRedactsBinaryObservation|AppendsTurnStepPerToolCall)|TestBuildObservationExcerptRuneBoundaryTruncation" -count=1 -v
go test ./internal/operatoragent ./internal/console ./cmd/obsidian-harness -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
