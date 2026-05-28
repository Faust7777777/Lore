# Review Handoff: A-Line Runtime LLM Profile Boundary

Date: 2026-05-27

## FINAL Finding

Related to `REVIEW-2026-05-25-FINAL.txt` config/runtime hardening area:

- `A5` / config-runtime model configuration follow-up: TUI `/model` must consume
  runtime-backed LLM profile resolution instead of reconstructing production
  model config in the shell.

This slice follows `review-handoff-codex53-a-line-llm-config-profiles-2026-05-26.md`
and `review-handoff-codex53-a-line-llm-config-model-panel-followup-2026-05-26.md`.
It does not claim to implement the TUI profile editing UX.

## Scope

Adds a runtime LLM profile boundary for TUI-first model configuration:

- `app.Runtime` now exposes safe profile/list/resolve/write/build methods for
  the TUI shell.
- `/model use` remains session-scoped hot switch.
- workspace profile writes remain persistent via runtime APIs.
- TUI driver no longer needs to build production operator/persona bindings from
  raw config details when running against `app.Runtime`.

## Modified Files

- `internal/config/llm.go`
- `internal/config/llm_test.go`
- `internal/app/llm_config.go`
- `internal/app/persona_extractor.go`
- `internal/app/runtime.go`
- `internal/app/runtime_llm_config_test.go`
- `internal/cli/tui_workbench.go`
- `internal/cli/tui_workbench_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-runtime-llm-profile-boundary-2026-05-27.md`

## Implementation Notes

- Added explicit profile resolution:
  - `config.ResolveLLMProfileConfig(...)`
  - `config.ResolveLLMProfileConfigWithOptions(...)`
  - `config.ResolveLLMProfileConfigFromLoaded(...)`
- Added runtime-facing APIs:
  - `Runtime.ResolveLLMConfig(purpose)`
  - `Runtime.ListLLMProfiles()`
  - `Runtime.UpsertLLMProfile(profileName, profile)`
  - `Runtime.SetActiveLLMProfile(profileName)`
  - `Runtime.BuildOperatorAgentForModel(profileName, modelName)`
  - `Runtime.BuildPersonaExtractorForModel(profileName, modelName)`
- Runtime profile writes reload only LLM wiring and preserve the existing
  runtime/harness/store lifecycle.
- `OpenRuntimeWithConfigOptions` stores its load options so tests remain
  isolated from real user-global config during profile reloads.
- Runtime profile edits always write and reload the canonical workspace file:
  `<workdir>/.lore/config.json`.
- TUI model switch now builds operator and persona bindings before mutating the
  session, avoiding the prior partial-switch risk if persona construction fails.
- TUI still has a fallback path for non-`app.Runtime` test stubs; production
  `app.Runtime` goes through runtime methods.

## Load-Bearing Tests

- `TestResolveLLMProfileConfigUsesRequestedProfile`
  - Active profile is `deepseek`; explicit `kimi` resolution still returns the
    requested Kimi endpoint/model/key env.
- `TestRuntimeLLMProfilePersistenceReloadsActiveProfile`
  - Runtime upserts two workspace profiles, switches active profile, reloads LLM
    wiring, updates persona-extract model identity, and lists sorted profiles.
- `TestRuntimeBuildOperatorAndPersonaForModelUsesNamedProfile`
  - Runtime builds both chat agent and persona extractor from a named profile,
    overriding only the session model.
- `TestInteractiveWorkbenchSwitchModelUpdatesPersonaExtractor`
  - Existing model switch test now also asserts `/model use` is session-scoped:
    a fresh runtime still reads the workspace active profile model.

Release gate additions:

```powershell
go test ./internal/config -run "Test(ResolveLLM(Config|ProfileConfig)|UpsertLLMProfile|SetActiveLLMProfile|SaveWorkspaceConfig|LoadEditableConfig|LLMProfileFromPreset)" -count=1 -v
go test ./internal/app -run "Test(OpenRuntimeUsesWorkspaceLLMProfileOverGenericEnv|RuntimeLLMProfilePersistenceReloadsActiveProfile|RuntimeBuildOperatorAndPersonaForModelUsesNamedProfile)$" -count=1 -v
go test ./internal/cli -run "TestInteractiveWorkbench(ModelPanelUsesWorkspaceLLMProfileOverEnv|ModelDiscoveryFallbackUsesConfiguredModelAndError|SwitchModelUpdatesPersonaExtractor)$" -count=1 -v
```

## Verification Run

Passed locally:

```powershell
go test ./internal/config ./internal/app ./internal/cli -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
$env:LORE_LLM_MODEL='gpt-5.4-openai-compact'; powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

Note: the first full `verify.ps1` attempt with this machine's default
`LORE_LLM_MODEL=gpt-5.4` failed in the model-backed smoke with:
`The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account.`
The same endpoint/key passed with `gpt-5.4-openai-compact`, so this is an
external model-account compatibility issue rather than a unit/runtime failure.

## Explicit Non-Scope

This slice did not change:

- MCP exposed surface; it remains read + proposal intake only.
- Persona candidate lifecycle/state machine.
- TUI profile editing UI or popup layout.
- TUI presentation rendering files.
- SDK behavior.
- Credential-store integration; API key values still come from env vars only.

Untracked B/TUI work such as `internal/app/persona_actions.go` is not part of
this slice and should not be staged with it.

## Reviewer Focus

- Confirm `Runtime` is the production boundary for TUI `/model` config and
  binding construction.
- Confirm `/model use` hot-switches only the current session and does not mutate
  workspace active profile.
- Confirm runtime profile persistence reloads operator/process-sink/persona LLM
  wiring without leaking API key values into status/session metadata.
- Confirm non-`app.Runtime` test stubs remain supported without expanding the
  production config surface in the TUI shell.
