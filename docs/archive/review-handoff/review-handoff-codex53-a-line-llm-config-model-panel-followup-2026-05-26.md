# Review Handoff: A-Line LLM Config Model-Panel Follow-up

## Finding / Task Reference

- FINAL alignment: `A5` / config startup validation follow-up from `REVIEW-2026-05-25-FINAL.txt`.
- User task: A-line model config foundation, specifically the acceptance that workspace `llm` profile selection must beat legacy env settings.

## Summary

This follow-up closes production-path drift in the interactive `/model` panel:

- The operator runtime already used `config.ResolveLLMConfig(...)`.
- `lore models list` already used `config.ResolveLLMConfig(...)`.
- The interactive TUI model panel still called `operatoragent.LoadEnvConfig()` directly, so a workspace profile pointing at DeepSeek could be ignored while `LORE_LLM_*` pointed at another endpoint.

The panel now resolves the operator LLM through the same config/profile-backed resolver as the rest of runtime-facing model selection.

Reviewer follow-up included in this slice:

- `/model use <name>` now rebuilds the session persona extractor and `PersonaExtractModelInfo` for the switched model, so chat and fire-and-forget persona extraction do not silently diverge.
- Model discovery fallback now uses the configured model when the current session model is empty and records the discovery error in `ModelInfo.TestStatus` instead of rendering an empty row.

## Modified Files

- `internal/cli/tui_workbench.go`
- `internal/cli/tui_workbench_test.go`
- `internal/app/persona_extractor.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-llm-config-model-panel-followup-2026-05-26.md`

## Behavior Changes

- `interactiveWorkbenchDriver.DiscoverModels`, `SwitchModel`, and `TestModel` now call `config.ResolveLLMConfig(workDir, config.LLMPurposeOperator)`.
- Workspace/user-global `llm` profile config now takes precedence over legacy `LORE_LLM_*` env for interactive model discovery, switching, and testing.
- `/status` inside the interactive workbench now includes the same safe config/model diagnostics as the text-fallback TUI status path.
- `app.Runtime.BuildPersonaExtractorForOperatorModel` builds a session-scoped persona extractor from the resolved operator profile plus the selected model name, and attaches usage attribution to the runtime usage store.
- `/model use` first constructs the new chat client and persona extractor binding, then drains existing persona extraction briefly and swaps the session chat agent / persona extractor / model-info together. If persona binding fails, the old session state stays intact.

## Security / Secret Boundary

- API key values remain in environment variables and are passed only to the model client.
- Rendered model panel entries expose only `KeyStatus` (`OK` / `missing`), never the API key value.
- Status diagnostics continue to show `api_key_env` / `api_key_ref` names only, never secret values.
- Sessionlog metadata records only the model name, not provider credentials.
- Persona extraction failure logs continue to receive only provider/model/base_url fields; API keys are not added.

## Load-Bearing Tests

- `TestInteractiveWorkbenchModelPanelUsesWorkspaceLLMProfileOverEnv`
  - Sets generic env to a fake ikuncode-style endpoint.
  - Writes workspace `.lore/config.json` pointing at a fake DeepSeek endpoint.
  - Asserts `/model` discovery hits the workspace endpoint exactly once and never hits the env endpoint.
  - Asserts discovered model rows report `source=workspace`, `provider=deepseek`, and `KeyStatus=OK`.
- `TestInteractiveWorkbenchModelDiscoveryFallbackUsesConfiguredModelAndError`
  - Forces `/models` discovery failure with an unavailable session agent.
  - Asserts the fallback row uses configured `deepseek-chat`, not an empty model name.
  - Asserts the discovery error is exposed through `TestStatus`.
- `TestInteractiveWorkbenchSwitchModelUpdatesPersonaExtractor`
  - Opens a real runtime with a workspace DeepSeek profile.
  - Calls `/model use` through `SwitchModel("deepseek-reasoner")`.
  - Asserts both chat `session.Agent` and `session.PersonaExtractModelInfo` switch to `deepseek-reasoner`.
- Existing `TestOpenRuntimeUsesWorkspaceLLMProfileOverGenericEnv`
  - Continues to cover the actual operator request path.

## Validation Run

```powershell
go test ./internal/cli -run "TestInteractiveWorkbench(ModelPanelUsesWorkspaceLLMProfileOverEnv|ModelDiscoveryFallbackUsesConfiguredModelAndError|SwitchModelUpdatesPersonaExtractor)$" -count=1 -v
go test ./internal/app ./internal/cli ./internal/console ./internal/tui -count=1
.\scripts\release-gate.ps1 -SkipDiffCheck
.\scripts\verify.ps1
git diff --check
```

## Release Gate

Added targeted gate:

```powershell
go test ./internal/cli -run "TestInteractiveWorkbench(ModelPanelUsesWorkspaceLLMProfileOverEnv|ModelDiscoveryFallbackUsesConfiguredModelAndError|SwitchModelUpdatesPersonaExtractor)$" -count=1
```

## Explicit Non-Scope

- Did not add or change MCP tool surface.
- Did not add direct write/apply/shell capability.
- Did not implement Windows Credential / `api_key_ref`.
- Did not implement persistent `/model` profile editing or TUI provider/key setup.
- Did not change model-panel visual rendering or TUI presentation copy.
- Did not alter persona candidate state machine or draft governance.
- Did not touch B-line persona log model-tag tests; those remain a separate dirty slice if present in the worktree.

## Reviewer Focus

- Confirm the interactive `/model` path no longer uses `operatoragent.LoadEnvConfig()`.
- Confirm `/model use` updates both the chat agent and the session persona extractor/model-info.
- Confirm model-discovery failure cannot render an empty current-model row.
- Confirm no API key value can be rendered or written to sessionlog through this path.
- Confirm release gate remains deterministic and does not require a real model.
