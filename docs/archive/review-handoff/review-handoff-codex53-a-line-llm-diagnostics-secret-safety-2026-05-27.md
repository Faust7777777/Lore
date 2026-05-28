# Review Handoff: A-Line LLM Diagnostics Secret Safety

Date: 2026-05-27

## FINAL Finding

Related to `REVIEW-2026-05-25-FINAL.txt` config/runtime hardening area:

- `A5` / model configuration diagnostics and secret-handling follow-up: status,
  TUI status, and session transcripts must expose model source metadata without
  leaking API key values or credential-bearing errors.

This slice builds on the runtime profile boundary from
`review-handoff-codex53-a-line-runtime-llm-profile-boundary-2026-05-27.md`.

## Scope

Hardens visible model diagnostics and sessionlog metadata:

- status/TUI status now shows key status (`key=present` / `key=missing`) next to
  provider/model/source/profile/base_url/api_key_env.
- credential-bearing diagnostic errors containing `Authorization` or `Bearer`
  are redacted before rendering.
- sessionlog `session_meta` records safe model metadata only:
  provider, model, base_url, profile, source.
- sessionlog metadata does not record API key values, API key env names, or auth
  headers.

## Modified Files

- `cmd/obsidian-harness/main_test.go`
- `internal/cli/cli.go`
- `internal/cli/llm_config_view.go`
- `internal/cli/llm_config_view_test.go`
- `internal/config/llm.go`
- `internal/sessionlog/model.go`
- `internal/sessionlog/writer.go`
- `internal/sessionlog/sessionlog_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-llm-diagnostics-secret-safety-2026-05-27.md`

## Implementation Notes

- `config.LLMDiagnostic` now has `KeyStatus`.
- `ResolvedLLMConfig.Diagnostic(err)` derives key status without carrying the
  key value into diagnostics.
- `renderLLMConfigDiagnostics` renders `key=present/missing` and redacts
  credential-bearing errors.
- `configureSessionRecorder` now seeds sessionlog metadata from
  `runtime.ResolveLLMConfig(operator)` instead of the old `LORE_MODEL`-only
  path. `LORE_MODEL` remains a compatibility fallback only when no resolved
  model is available.
- `sessionlog.Meta`, `sessionlog.Event`, and `sessionlog.Summary` carry safe
  model metadata fields. No key or auth-header fields were added.

## Load-Bearing Tests

- `TestRenderLLMConfigDiagnosticsOmitsSecretsAndShowsSource`
  - Confirms model source/profile/api_key_env/key status render without secret
    values.
- `TestRenderLLMConfigDiagnosticsShowsErrorsAndDisabled`
  - Confirms missing key renders as `key=missing` and points to the env var name.
- `TestRenderLLMConfigDiagnosticsRedactsCredentialBearingErrors`
  - Confirms Authorization/Bearer-style errors are replaced with a redaction
    marker.
- `TestRecorderWritesIndexAndRestoresSnapshot`
  - Confirms sessionlog snapshot and summary preserve provider/model/base_url/
    profile/source metadata.
- `TestRunConsoleOnceWritesSessionTranscript`
  - Confirms real CLI transcript contains safe model metadata and does not
    contain test secret, api_key, Authorization, or Bearer strings.

Release gate additions:

```powershell
go test ./internal/cli -run "Test(RenderLLMConfigDiagnostics(OmitsSecretsAndShowsSource|ShowsErrorsAndDisabled|RedactsCredentialBearingErrors)|InteractiveWorkbench(ModelPanelUsesWorkspaceLLMProfileOverEnv|ModelDiscoveryFallbackUsesConfiguredModelAndError|SwitchModelUpdatesPersonaExtractor))$" -count=1 -v
go test ./cmd/obsidian-harness -run "TestRun(SmokeP0|SmokeP0FullIncludesGovernedNoteIntake|TUIOnceShowsResolveReadFinalTaskVisibility|DaemonOnceTriggersDraftAfterStablePlanChange|DaemonOnceSyncsCodexJSONLWhenConfigured|DaemonOnceMissingCodexJSONLRemainsNonFatal|ConsoleOnceWritesSessionTranscript)$" -count=1 -v
```

## Verification Run

Passed locally:

```powershell
go test ./internal/cli -run "Test(RenderLLMConfigDiagnostics(OmitsSecretsAndShowsSource|ShowsErrorsAndDisabled|RedactsCredentialBearingErrors)|InteractiveWorkbench(ModelPanelUsesWorkspaceLLMProfileOverEnv|ModelDiscoveryFallbackUsesConfiguredModelAndError|SwitchModelUpdatesPersonaExtractor))$" -count=1 -v
go test ./internal/sessionlog -run "TestRecorderWritesIndexAndRestoresSnapshot$" -count=1 -v
go test ./cmd/obsidian-harness -run "TestRunConsoleOnceWritesSessionTranscript$" -count=1 -v
go test ./internal/config ./internal/cli ./internal/sessionlog ./cmd/obsidian-harness -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
$env:LORE_LLM_MODEL='gpt-5.4-openai-compact'; powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

Note: local default `LORE_LLM_MODEL=gpt-5.4` is still incompatible with this
machine's current model account for smoke chat completions. The full verify run
above used `gpt-5.4-openai-compact`, which passed.

## Explicit Non-Scope

This slice did not change:

- MCP exposed surface; it remains read + proposal intake only.
- TUI profile editing UI or model panel layout.
- Persona candidate lifecycle/state machine.
- Credential-store integration.
- API key storage. Key values remain env-only and are not written to config,
  sessionlog, status, or TUI status.

Untracked B/TUI work such as `internal/app/persona_log.go` and
`internal/app/persona_summary.go` is not part of this slice and should not be
staged with it.

## Reviewer Focus

- Confirm status/TUI status has enough source metadata for users to diagnose the
  active model without exposing secrets.
- Confirm sessionlog metadata contains only safe model identity fields.
- Confirm diagnostic error redaction does not hide ordinary missing-env errors
  needed for setup.
- Confirm release gate now pins these deterministic checks without adding real
  network/model dependencies to PR gate.
