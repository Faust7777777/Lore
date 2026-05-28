# Review Handoff: Persona Summary JSON and LLM Secret Redaction Fixes

Date: 2026-05-28

## Review Findings

Addresses two reviewer findings from 2026-05-28:

- `Reviewer-Medium-1`: persona summary JSON shape regressed by changing
  `extract_log.by_stage.malformed` to `"(malformed)"`.
- `Reviewer-Medium-2`: LLM diagnostic redaction only caught Authorization/Bearer
  and could leak provider errors containing API key / token / secret text; base
  URLs with credentials also needed safe display metadata.

## Scope

This is a review-fix slice across the B persona-summary dashboard and A
runtime/model diagnostics boundary:

- App DTO keeps the machine-facing malformed bucket as `malformed`.
- Human persona summary rendering still displays that bucket as `(malformed)`.
- JSON summary tests now include one malformed log line and assert only the
  stable `malformed` key is present.
- LLM diagnostic redaction now covers `api key`, `api_key=`, `api_key:`,
  `api-key`, `apikey`, `access_token`, `token`, `secret`, `password`, `passwd`,
  and `sk-`, while preserving actionable missing-env-var messages.
- Safe model metadata sanitizes base URLs before status/sessionlog/TUI display
  by removing userinfo, query, and fragment components.

## Modified Files

- `internal/app/persona_extractor.go`
- `internal/app/persona_summary.go`
- `internal/app/persona_summary_test.go`
- `internal/app/runtime.go`
- `internal/cli/cli.go`
- `internal/cli/llm_config_view.go`
- `internal/cli/llm_config_view_test.go`
- `internal/cli/persona_summary.go`
- `internal/cli/persona_summary_test.go`
- `internal/cli/sessionlog_meta_test.go`
- `internal/cli/tui_workbench.go`
- `internal/config/llm.go`
- `internal/config/llm_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-review-fixes-persona-summary-llm-redaction-2026-05-28.md`

## Load-Bearing Tests

- `TestPersonaSummaryBucketsExtractLogByStage`
  - Ensures app DTO uses `ByStage["malformed"]` for malformed log lines.
- `TestRunPersonaSummaryJSONWithLogEntries`
  - Ensures JSON emits `extract_log.by_stage.malformed` and not
    `extract_log.by_stage["(malformed)"]`.
- `TestRenderPersonaSummaryAggregatesLogStages`
  - Ensures human rendering still shows `(malformed)`.
- `TestRenderLLMConfigDiagnosticsRedactsCredentialBearingErrors`
  - Covers Authorization/Bearer, `api key`, `api_key=`, token, and secret error
    shapes.
- `TestRenderLLMConfigDiagnosticsKeepsMissingEnvVarName`
  - Ensures setup diagnostics still name the missing env var.
- `TestSanitizeLLMBaseURLRemovesCredentialBearingParts`
  - Removes userinfo/query/fragment from credential-bearing URLs.
- `TestResolvedLLMConfigDiagnosticSanitizesBaseURL`
  - Ensures diagnostics use sanitized base_url.
- `TestSessionLogMetaSanitizesLLMBaseURL`
  - Ensures sessionlog model metadata cannot store URL userinfo/query secrets.

Release gate now includes these deterministic checks.

## Verification Run

Passed locally:

```powershell
go test ./internal/app -run "TestPersonaSummary(BucketsExtractLogByStage|ReflectsFullStateSeededByFixtureHelpers)$" -count=1 -v
go test ./internal/cli -run "Test(RenderLLMConfigDiagnosticsRedactsCredentialBearingErrors|SessionLogMetaSanitizesLLMBaseURL|RunPersonaSummaryJSONWithLogEntries|RenderPersonaSummaryAggregatesLogStages)$" -count=1 -v
go test ./internal/config -run "Test(SanitizeLLMBaseURLRemovesCredentialBearingParts|ResolvedLLMConfigDiagnosticSanitizesBaseURL)$" -count=1 -v
go test ./internal/config ./internal/app ./internal/cli ./internal/console ./internal/tui ./cmd/obsidian-harness -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```

## Explicit Non-Scope

This slice did not change:

- MCP exposed surface; still read + proposal intake only.
- Persona candidate lifecycle/state machine.
- TUI persona panel layout or interaction.
- Credential-store integration.
- Actual outbound LLM request configuration; base URL sanitization is for
  display/sessionlog metadata, not for mutating request routing.

## Reviewer Focus

- Confirm JSON monitoring consumers can keep reading
  `extract_log.by_stage.malformed`.
- Confirm human CLI still renders `(malformed)` clearly.
- Confirm missing-env diagnostics remain actionable and are not over-redacted.
- Confirm no key, token, secret, Authorization, Bearer, URL userinfo, query, or
  fragment is written to status/sessionlog/TUI display metadata.
