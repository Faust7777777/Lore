# Review Handoff: A-Line LLM Profile Presets

## Finding / Task Reference

- FINAL alignment: `A5` / config startup validation follow-up from `REVIEW-2026-05-25-FINAL.txt`.
- User task: `A-3 模型 profile 预设`.

## Summary

Adds deterministic built-in LLM profile presets for future TUI-first model setup. Presets are local templates only: they do not call provider APIs and they do not migrate `LORE_LLM_*` env into workspace config.

## Modified Files

- `internal/config/llm_preset.go`
- `internal/config/llm_preset_test.go`
- `README.md`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-llm-profile-presets-2026-05-27.md`

## Presets

- `deepseek`
  - provider: `deepseek`
  - base URL: `https://api.deepseek.com/v1`
  - model: `deepseek-v4-pro`
  - key env: `DEEPSEEK_API_KEY`
- `deepseek-fast`
  - provider: `deepseek`
  - base URL: `https://api.deepseek.com/v1`
  - model: `deepseek-v4-flash`
  - key env: `DEEPSEEK_API_KEY`
- `openai-compatible`
  - provider: `openai-compatible`
  - caller must provide `base_url`, `model`, and `api_key_env`

## New API

- `config.BuiltinLLMProfilePresets()`
- `config.LLMProfileFromPreset(presetName, overrides)`
- `config.UpsertLLMProfileFromPreset(workDir, profileName, presetName, overrides)`

## Security / Secret Boundary

- Presets contain only provider metadata and env var names.
- API key values are never read or written by preset generation.
- `UpsertLLMProfileFromPreset` writes only workspace config through the A-2 persistence path.

## Load-Bearing Tests

- `TestLLMProfileFromPresetDeepSeekUsesOfficialFullModelNames`
  - Ensures DeepSeek presets use `deepseek-v4-pro` / `deepseek-v4-flash`, not rejected short names like `v4-pro`.
- `TestLLMProfileFromPresetOpenAICompatibleRequiresUserFields`
  - Ensures generic OpenAI-compatible setup requires explicit base/model/key env.
- `TestUpsertLLMProfileFromPresetWritesCompleteProfileWithoutEnvMigration`
  - Sets legacy `LORE_LLM_*` env and verifies preset writing does not copy those values into workspace config.
  - Verifies resolver uses the written preset plus `DEEPSEEK_API_KEY`.
- `TestLLMProfileFromPresetRejectsUnknownPreset`
  - Ensures unknown preset errors include available preset names.

## Validation Run

```powershell
go test ./internal/config -run "Test(LLMProfileFromPreset|UpsertLLMProfileFromPreset|UpsertLLMProfile|SetActiveLLMProfile|SaveWorkspaceConfig|LoadEditableConfig|ResolveLLMConfig)" -count=1 -v
```

## Release Gate

`scripts/release-gate.ps1` now includes preset tests in the deterministic config gate:

```powershell
go test ./internal/config -run "Test(ResolveLLMConfig|UpsertLLMProfile|SetActiveLLMProfile|SaveWorkspaceConfig|LoadEditableConfig|LLMProfileFromPreset)" -count=1 -v
```

## Explicit Non-Scope

- Did not implement TUI profile editing UI.
- Did not implement CLI-first profile commands.
- Did not call provider APIs for model discovery.
- Did not auto-migrate legacy `LORE_LLM_*` env.
- Did not implement Windows Credential / `api_key_ref` resolution.
- Did not change MCP tool surface.

## Reviewer Focus

- Confirm DeepSeek model names are the full official names.
- Confirm generic `openai-compatible` cannot write an incomplete profile.
- Confirm preset generation does not leak env key values into config.
