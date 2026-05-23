# Review Handoff: A-Line Operator Prompt Version Guard

Date: 2026-05-23

## Scope

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `scripts/release-gate.ps1`
- `README.md`

## Summary

Adds lightweight prompt versioning and critical-rule tests for operatoragent prompts.

New code:

- `operatorDecisionPromptVersion = "operator-decision-v1"`
- `operatorLoopPromptVersion = "operator-loop-v1"`
- `PromptVersions()` returns the current decision/loop prompt versions for diagnostics and tests.
- `systemPrompt()` and `loopSystemPrompt()` now include their prompt version string in the prompt body.

New guardrail:

- `TestOperatorPromptVersionsAndCriticalRules` locks prompt versions plus critical governance/runtime instructions:
  - decision prompt must return one JSON action object and reject background jobs;
  - loop prompt must keep tool_call/final envelope forms;
  - loop prompt must preserve assistant-history warning;
  - loop prompt must preserve CoreContext trust split;
  - loop prompt must expose local_exec mode and runtime docs section when applicable.

## Boundary

- No MCP surface change.
- No tool/runtime behavior change.
- No TUI or persona behavior change.
- The only model-visible prompt change is the explicit version line.

## Release Gate

`TestOperatorPromptVersionsAndCriticalRules` is now part of the targeted operatoragent release gate. README release-gate wording was updated.

## Validation

```powershell
go test ./internal/operatoragent -run "TestOperatorPromptVersionsAndCriticalRules|TestLoopSystemPromptIncludesFileInspectionDiscipline|TestModelAgentRespondPrompt" -count=1 -v
go test ./internal/operatoragent ./internal/console -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
