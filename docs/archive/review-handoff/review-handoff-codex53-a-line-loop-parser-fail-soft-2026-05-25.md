# Review Handoff: A-Line Loop Parser Fail-Soft Safety

Date: 2026-05-25

## FINAL Finding

No direct `REVIEW-2026-05-25-FINAL.txt` finding ID covers this parser issue.
This is an A-line runtime safety follow-up from the user-reported failure mode:
model output of ordinary JSON such as MCP config was being parsed as a Lore loop
envelope and failed with `unsupported type ""`.

Related FINAL area: A-line safety/runtime guardrails. This slice does not claim to
close S-1/S-2/S-3.

## Scope

Hardens `parseLoopResponse` so non-protocol content is returned as a final user
message, while control-like JSON without a valid loop `type` still hard-fails and
never executes tools.

## Modified Files

- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `internal/console/session_usage_e2e_test.go`
- `cmd/obsidian-harness/main_test.go`
- `scripts/release-gate.ps1`
- `README.md`

## Implementation Notes

- Legal loop envelopes still execute as before:
  - `{"type":"tool_call", ...}`
  - `{"type":"final", ...}`
- Legacy decision JSON still works when no loop `type` is present:
  - `{"action":"show_status"}`
- Non-envelope JSON now becomes `final.message`:
  - `{"mcpServers":{...}}`
- Markdown/prose containing JSON examples now becomes `final.message` instead of
  extracting the example as protocol.
- Control-like JSON without a valid `type` still hard-fails:
  - `{"tool":"vault_read","arguments":{...}}`
- Added `isLoopControlLike` as the explicit safety gate for `tool`, `arguments`,
  and `action` control fields.

## Load-Bearing Tests

- `TestModelAgentRespondTreatsNonEnvelopeJSONAsFinal`
  - Covers top-level MCP config JSON fallback to final output.
  - Asserts no tool call is executed.
- `TestModelAgentRespondTreatsMarkdownJSONExampleAsFinal`
  - Covers prose/markdown with a JSON code example fallback to final output.
  - Asserts no tool call is executed.
- `TestModelAgentRespondRejectsControlLikeJSONWithoutType`
  - Covers `tool`/`arguments` without valid `type`.
  - Asserts parser returns an error and no tool call is executed.
- Existing regression tests still cover:
  - valid loop envelope execution,
  - provider-concatenated leading JSON object handling,
  - legacy `action` fallback,
  - usage propagation on parser hard failures.
- `TestEndToEndUsagePipelineSuccessAndFailureTurns` and
  `TestRunUsageCLIReadsFailedTurnUsageEndToEnd`
  - Updated to use the remaining hard-fail case (`tool`/`arguments` without
    `type`) instead of plain text, because plain text is now a safe final
    response by design.

Release gate addition:

```powershell
go test ./internal/operatoragent -run "Test(ModelAgentRespondTreatsNonEnvelopeJSONAsFinal|ModelAgentRespondTreatsMarkdownJSONExampleAsFinal|ModelAgentRespondRejectsControlLikeJSONWithoutType)$" -count=1 -v
```

## Explicit Non-Scope

- Did not change tool dispatch or tool authorization.
- Did not infer tool calls from legacy/non-envelope JSON.
- Did not change MCP exposed tool surface.
- Did not change persona, TUI, sessionlog, vault governance, or SDK behavior.
- Did not rely on prompt wording to prevent this failure mode.

## Reviewer Focus

- Confirm ordinary config JSON no longer fails with `unsupported type ""`.
- Confirm markdown JSON examples do not execute tools.
- Confirm control-like JSON without `type` remains a hard parser error.
- Confirm valid envelopes and legacy action JSON did not regress.
