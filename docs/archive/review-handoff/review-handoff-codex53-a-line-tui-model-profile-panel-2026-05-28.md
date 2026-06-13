# Review Handoff: A-line TUI Model Profile Panel Closure (2026-05-28)

## Scope

A-line model configuration follow-up. This slice closes an incomplete TUI model-profile panel change that left `internal/tui` and `internal/cli` unbuildable.

FINAL review finding reference: N/A. This is not one of the 2026-05-25 FINAL security/runtime findings; it is a continuation of the A-line LLM profile persistence/model-panel work.

Concurrent dirty note: while closing the model-profile slice, `internal/tui/interactive_workbench.go` also contained a partial persona-candidate panel insertion. This handoff includes the minimal compile-safe bridge/render/stub repairs needed to preserve that work, but it does not claim to finish the persona candidate TUI product surface or change the app/runtime persona candidate state machine.

## Intent

Make the TUI model surface consume the already-shipped runtime LLM profile APIs instead of requiring users to hand-edit `.lore/config.json` for the common preset flow.

The covered behaviors are deterministic and do not require real network/model calls in PR gate:

- `/model profiles` lists workspace LLM profiles.
- profile rows can be selected to persist the active workspace profile and hot-switch the current session.
- `n` in the model panel opens the built-in preset picker.
- creating a DeepSeek preset writes only `api_key_env`, never an API key value.
- profile/preset panels render key env names and key presence status without exposing secret values.

## Modified Files

- `internal/tui/interactive_workbench.go`
  - extends the model panel state machine for model/profile/preset modes.
  - adds async commands for listing profiles, creating profiles from presets, and persisting active profiles.
  - ensures profile row selection goes through the profile persistence path instead of treating the profile row as a raw model name.
  - preserves the concurrent persona-candidate panel hooks and fixes focus-cycle/list-result plumbing so the package compiles.

- `internal/tui/interactive_render.go`
  - renders model profile and preset picker views.
  - avoids reading raw secret values in the renderer; preset display shows only the key env var name.
  - adds a minimal persona-candidate panel renderer for the concurrent candidate panel hooks.

- `internal/cli/tui_workbench.go`
  - implements the new TUI driver methods via runtime config APIs.
  - maps `app.LLMProfileView` to key-free `tui.ModelInfo` rows.
  - creates profiles from config presets without writing API key values.
  - persists the active profile and hot-switches the current session agent plus persona extractor binding.
  - adds a minimal persona-candidate TUI driver bridge using existing app runtime methods; no new state-machine path is introduced.
  - satisfies the expanded diagnostics driver surface (`ListErrors`) without adding a new log/state source.

- `internal/cli/tui_workbench_test.go`
  - adds real driver tests for preset creation and profile persistence/hot-switch behavior.

- `internal/tui/panel_state_test.go`
  - adds deterministic TUI state-machine tests for profile persistence and preset creation.
  - adds render no-secret regression coverage.
  - adds deterministic `/errors` diagnostics coverage for hint rendering, empty state, and obvious secret redaction.
  - updates focus-cycle expectations for the concurrent candidate panel focus state.

- `internal/tui/approval_state_test.go`
  - updates the approval test stub to satisfy the expanded workbench driver interface.

- `scripts/release-gate.ps1`
  - adds deterministic CLI/TUI model-profile guardrails.

- `README.md`
  - updates release-gate coverage wording for LLM profile/model-panel guardrails.

- `docs/tui-manual-test.md`
  - adds a readable UTF-8 manual checklist for model profile setup, persona candidate review, draft approval/apply, and error diagnostics.
  - remains a manual smoke document; it is not wired into CI.

## Load-bearing Tests

Targeted tests run:

```powershell
go test ./internal/cli -run "Test(RenderLLMConfigDiagnostics(OmitsSecretsAndShowsSource|ShowsErrorsAndDisabled|RedactsCredentialBearingErrors|KeepsMissingEnvVarName)|SessionLogMetaSanitizesLLMBaseURL|InteractiveWorkbench(ModelPanelUsesWorkspaceLLMProfileOverEnv|ModelDiscoveryFallbackUsesConfiguredModelAndError|SwitchModelUpdatesPersonaExtractor|CreateProfileFromPresetWritesWorkspaceConfigWithoutSecret|PersistActiveProfileUpdatesWorkspaceAndSession))$" -count=1 -v

go test ./internal/tui -run "Test(ApprovalFlow_|InteractiveWorkbenchViewDoesNotRefreshContent|RenderInteractiveConversationShowsTaskSteps|RenderTaskStepsArgSummary|RenderTaskStepsTruncatesObservation|RenderTaskStepsErrorStep|RenderTaskStepsNonErrorLastOutputNotShown|FindingsOffsetUsesFindingsPanelHeight|SinkOffsetUsesSinkPanelHeight|ApprovalOffsetUsesApprovalPanelHeight|ModelPanel(ProfilesPersistSelectedProfile|CreateProfileFromPreset)|ModelProfileAndPresetRenderDoNotLeakSecrets)" -count=1 -v

go test ./internal/tui ./internal/cli -count=1

.\scripts\release-gate.ps1 -SkipDiffCheck

.\scripts\verify.ps1

git diff --check
```

What these tests specifically protect:

- workspace profile selection updates both persisted config and current in-session model.
- persona extraction binding switches with the chat model when a profile is persisted.
- preset creation writes the profile model/base/api_key_env but not the API key value.
- profile row selection does not fall through to raw model-name switching.
- render output does not include obvious secret markers or test secret values.
- release gate covers the new deterministic model-profile guardrails and still passes after the candidate focus-state insertion.
- release gate covers `/errors` diagnostics rendering through deterministic TUI tests.
- full `verify.ps1` stays green with the concurrent usage/candidate dirty work present.

## Boundaries Not Touched

- No MCP tool surface changes; external MCP remains read + proposal intake only.
- No persona candidate state-machine changes.
- Persona candidate TUI hooks are only compile/panel bridge repairs for concurrent dirty work; no new app/runtime persona candidate lifecycle semantics were added.
- No approval pane semantics changes.
- No runtime config schema changes; this consumes existing runtime/config APIs.
- No real model/network requirement added to PR gate.

## Reviewer Focus

- Confirm profile row `enter/u` behavior should be workspace-persistent + session hot switch, not session-only.
- Confirm `openai-compatible` preset remaining custom-field constrained is acceptable for this slice; complete custom field editing can be a separate TUI product slice.
- Confirm render/status surfaces expose only key env var names, not API key values.
- Confirm release-gate additions are deterministic and do not require real provider credentials.
- Confirm the compile-only persona-candidate/error-panel bridge is acceptable to keep as part of this mixed dirty recovery, or split it before commit if reviewer wants a stricter A-only patch.
- Confirm `docs/tui-manual-test.md` matches the actual current TUI labels before treating it as a user-facing QA script.
