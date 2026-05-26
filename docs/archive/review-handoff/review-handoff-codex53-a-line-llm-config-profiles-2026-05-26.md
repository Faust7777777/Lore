# Review Handoff: A-Line LLM Config Profiles

Date: 2026-05-26

## FINAL Finding

Related to `REVIEW-2026-05-25-FINAL.txt` config/runtime hardening area:

- `A5` / `Config startup validation` follow-up: model provider selection is now
  config/profile-backed instead of env-only.

This slice does not claim to close Q-8/TUI model-panel work or any persona state
machine finding.

## Scope

Adds a layered `llm` config section and a shared resolver for operator chat,
persona extraction, and process-sink summarization. Workspace/user-global
profiles take precedence over legacy env-only config. API key values remain in
environment variables only.

## Modified Files

- `README.md`
- `internal/config/config.go`
- `internal/config/loader.go`
- `internal/config/llm.go`
- `internal/config/llm_test.go`
- `internal/app/llm_config.go`
- `internal/app/runtime.go`
- `internal/app/runtime_llm_config_test.go`
- `internal/app/processsink_summarizer.go`
- `internal/app/persona_extractor.go`
- `internal/operatoragent/model.go`
- `internal/cli/cli.go`
- `internal/cli/llm_config_view.go`
- `internal/cli/llm_config_view_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-llm-config-profiles-2026-05-26.md`

## Implementation Notes

- `config.Config` now has `llm.active_profile` and `llm.profiles`.
- Each profile supports:
  - `provider`
  - `base_url`
  - `model`
  - `timeout`
  - `api_key_env`
  - `api_key_ref` reserved for future credential-store support.
- `config.ResolveLLMConfig(workDir, purpose)` is the shared resolver.
- Purposes currently used by runtime:
  - `operator`
  - `persona_extract`
  - `process_sink`
- Resolver precedence:
  - workspace/user-global config profile when any `llm` section is configured;
  - legacy env fallback only when config has no `llm` section;
  - disabled default when neither config nor env is present.
- Profile API key values are never loaded from config. `api_key_env` names the
  env var containing the key. `api_key_ref` returns a clear unsupported error
  unless an env var is also configured.
- `operatoragent.NewFromEnv()` remains as a compatibility path.
- Production console/TUI session construction now uses `runtime.OperatorAgent`,
  which comes from the resolved runtime config.
- Process-sink summarizer and persona extractor now use the same runtime LLM
  resolver instead of directly reading `LORE_LLM_*`.
- `lore status` appends safe model diagnostics: source/profile/provider/model/
  base_url/api_key_env, never the API key value.
- `lore models list [workdir]` now resolves the model endpoint from config first
  and env second.

## Load-Bearing Tests

- `TestResolveLLMConfigUsesWorkspaceProfileOverEnv`
  - Env is set to `api.ikuncode.cc` + `gpt-5.4`; workspace config selects
    DeepSeek; resolver returns workspace source and DeepSeek values.
- `TestResolveLLMConfigFallsBackToEnvWhenNoProfile`
  - Preserves legacy env-only behavior when no `llm` config exists.
- `TestResolveLLMConfigRequiresProfileAPIKeyEnv`
  - Rejects config profiles that would require storing API keys in config.
- `TestResolveLLMConfigReportsDisabledDefault`
  - Confirms no config/no env is a disabled default, not an error.
- `TestOpenRuntimeUsesWorkspaceLLMProfileOverGenericEnv`
  - Full runtime wiring acceptance: generic env points at ikuncode/gpt-5.4, but
    workspace profile points at fake DeepSeek. The operator request hits the
    fake DeepSeek endpoint with model `deepseek-chat` and `DEEPSEEK_API_KEY`.
- `TestRenderLLMConfigDiagnosticsOmitsSecretsAndShowsSource`
  - Ensures status diagnostics show source/profile/model but not API key values.

Release gate additions:

```powershell
go test ./internal/config -run "TestResolveLLMConfig" -count=1 -v
go test ./internal/app -run "TestOpenRuntimeUsesWorkspaceLLMProfileOverGenericEnv$" -count=1 -v
```

## Verification Run

Passed:

```powershell
go test ./internal/config ./internal/app ./internal/operatoragent -count=1
go test ./internal/config -run "TestLoad(AppliesWorkspaceOverrides|WorkspaceOverridesUserGlobal|DiagnosticOrder)|TestResolveLLMConfig" -count=1 -v
go test ./internal/app -run "TestOpenRuntimeUsesWorkspaceLLMProfileOverGenericEnv" -count=1 -v
go test ./internal/operatoragent -run "TestLoadEnvConfig|TestNewDefaultReturnsUnavailableAgentWhenConfigMissing" -count=1 -v
git diff --check -- <A-line LLM config files>
```

Blocked by unrelated dirty worktree:

- `go test ./internal/cli` currently fails before reaching this slice because
  `internal/cli/tui_workbench.go` has an unrelated incomplete model-panel edit
  with a syntax error.
- `go test ./internal/tui` is also blocked by unrelated TUI model-panel changes
  (`handleModelPanelResult` / `handleModelPanelKeys` missing).
- Therefore full `release-gate.ps1` / `verify.ps1` should wait until the TUI
  dirty line is either completed or separated.

## Explicit Non-Scope

This slice did not change:

- MCP exposed write/apply/shell surface.
- Persona candidate state machine or draft governance semantics.
- TUI product behavior or model-panel UI. Existing TUI model-panel dirty files
  are not part of this slice.
- Process-sink prompt semantics.
- SDK behavior.
- OS credential-store integration; `api_key_ref` is reserved only.

## Reviewer Focus

- Confirm workspace config profile beats legacy env config for actual requests.
- Confirm API key values are never rendered in status diagnostics or stored in
  config structs intended for status output.
- Confirm `operatoragent.NewFromEnv()` remains compatible for older callers.
- Confirm production console/TUI sessions use `runtime.OperatorAgent` rather
  than calling `operatoragent.NewDefault()` directly.
