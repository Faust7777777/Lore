# Review Handoff: A-Line LLM Profile Persistence

## Finding / Task Reference

- FINAL alignment: `A5` / config startup validation follow-up from `REVIEW-2026-05-25-FINAL.txt`.
- User task: `A-2 LLM profile 持久化 API`.

## Summary

Adds workspace-only LLM profile persistence APIs for future TUI `/model save/use profile` flows. TUI code should call these runtime/config APIs instead of constructing `.lore/config.json` JSON itself.

## Modified Files

- `internal/config/config.go`
- `internal/config/workspace_edit.go`
- `internal/config/workspace_edit_test.go`
- `scripts/release-gate.ps1`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-llm-profile-persistence-2026-05-27.md`

## New API

- `config.LoadEditableConfig(workDir)`
- `config.SaveWorkspaceConfig(workDir, patch)`
- `config.UpsertLLMProfile(workDir, profileName, profile)`
- `config.SetActiveLLMProfile(workDir, profileName)`

## Behavior

- Writes only `<workdir>/.lore/config.json`.
- Preserves existing workspace config sections, including unknown top-level sections.
- Writes via a temp file in the same directory followed by rename.
- Stores only `api_key_env` / `api_key_ref` names, never API key values.
- Keeps layered read semantics unchanged: defaults < user-global < workspace.

## Load-Bearing Tests

- `TestUpsertLLMProfilePreservesWorkspaceConfigAndOmitsSecretValue`
  - Existing `runtime` and `process_sink` sections survive profile writes.
  - Actual `DEEPSEEK_API_KEY` value is not written.
  - Repeated upsert of the same profile is idempotent.
- `TestSetActiveLLMProfileSwitchesResolver`
  - Active profile changes are reflected by `ResolveLLMConfig`.
- `TestSetActiveLLMProfileRejectsUnknownProfile`
  - Active profile cannot point at a missing profile.
- `TestSaveWorkspaceConfigPreservesUnknownTopLevelFields`
  - Unknown workspace config sections survive patch writes.
- `TestLoadEditableConfigRejectsNonObjectWorkspaceConfig`
  - Non-object workspace config fails clearly before writing.

## Validation Run

```powershell
go test ./internal/config -run "Test(UpsertLLMProfile|SetActiveLLMProfile|SaveWorkspaceConfig|LoadEditableConfig|ResolveLLMConfig)" -count=1 -v
go test ./internal/config -count=1
```

## Release Gate

`scripts/release-gate.ps1` now runs the LLM resolver plus workspace persistence guardrails:

```powershell
go test ./internal/config -run "Test(ResolveLLMConfig|UpsertLLMProfile|SetActiveLLMProfile|SaveWorkspaceConfig|LoadEditableConfig)" -count=1 -v
```

## Explicit Non-Scope

- Did not implement TUI profile editing UI.
- Did not implement CLI-first profile commands.
- Did not implement Windows Credential / `api_key_ref` resolution.
- Did not auto-migrate `LORE_LLM_*` env into config.
- Did not change MCP tool surface.
- Did not alter persona candidate state machine.

## Reviewer Focus

- Confirm workspace writes preserve non-LLM config.
- Confirm no API key value can be serialized into `.lore/config.json` through these APIs.
- Confirm active profile changes are visible through the existing resolver.
