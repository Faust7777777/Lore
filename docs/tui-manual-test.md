# TUI Manual Test Script

This script verifies the intended end-user path: configure models from the TUI, chat, let persona extraction run in the background, review generated candidates, and apply a governed persona draft. It is a manual smoke checklist, not a CI gate.

## Prerequisites

- Go is available, or use the bundled `./.tools/go/bin/go.exe` on Windows.
- A disposable workspace path, for example `tmp/tui-manual-workspace`.
- `DEEPSEEK_API_KEY` is set if testing the DeepSeek preset against the real provider.

## 1. Bootstrap A Workspace

```powershell
go run ./cmd/lore bootstrap tmp/tui-manual-workspace
```

Expected:

- `vault/`, `state/`, and `.lore/config.json` are created.
- No API key value is written to the workspace.

## 2. Launch The TUI

```powershell
go run ./cmd/lore tui --workdir tmp/tui-manual-workspace
```

Expected:

- The TUI opens with conversation, status/context, review, and supporting panels.
- The input box is focused.

## 3. Check Model Status

Look at the status/context area.

Expected:

- A model/status section shows Chat, Persona, and Sink identities when resolvable.
- Each row includes provider/model/source/profile where available.
- It shows the API key env var name and present/missing state, never the key value.
- If Chat and Persona are using different models, the warning is visible.

## 4. Create A DeepSeek Profile From The TUI

1. Type `/model` and press Enter.
2. Press `s` to switch to profile view.
3. Press `n` to open the preset picker.
4. Select `deepseek` and press Enter.

Expected:

- A `deepseek` workspace profile is created.
- The profile uses model `deepseek-v4-pro`.
- The profile stores `api_key_env=DEEPSEEK_API_KEY` only.
- The actual API key value is not displayed and not written to `.lore/config.json`.

## 5. Test The Profile

1. Keep the cursor on the `deepseek` profile row.
2. Press `t`.

Expected:

- If `DEEPSEEK_API_KEY` is valid, the row reports OK.
- If the key is missing or invalid, the error points to the env var or provider/model issue without printing the key.

## 6. Use The Profile In The Current Session

1. Keep the cursor on the `deepseek` profile row.
2. Press `u` or Enter.

Expected:

- The current chat agent switches to `deepseek-v4-pro`.
- The persona extractor binding switches with the session model.
- The status panel reflects the new session model.

## 7. Persist The Profile As Workspace Default

1. Keep the cursor on the `deepseek` profile row.
2. Press `p`, or type `/model persist deepseek`.
3. Restart the TUI.

Expected:

- The workspace active profile is `deepseek`.
- After restart, the TUI resolves the model from workspace config, not only from environment fallback.

## 8. Chat And Trigger Persona Extraction

Type a stable self-description, for example:

```text
我是学生，每周三晚上会复盘英语听力错题，也在学习编程。
```

Expected:

- Lore answers the chat normally.
- Persona extraction runs asynchronously and does not block the turn.
- If extraction fails, the error is available through diagnostics rather than interrupting the chat turn.

## 9. Review Persona Candidates

1. Type `/candidates`, or Tab to the Persona Candidates panel.
2. Select a candidate and press Enter.

Expected:

- List view shows candidate ID/state/field/proposed value at a glance.
- Detail view shows evidence quote, reason, confidence, source, observed time, and linked draft ID when present.
- Long text is clipped or wrapped; secrets are not rendered.

## 10. Create A Draft From A Candidate

1. Select an open candidate.
2. Press `d`.

Expected:

- The candidate transitions to drafted.
- A `persona_update` draft appears in the review panel.
- No direct write to the persona document occurs yet.

## 11. Approve And Apply The Draft

1. Move to the Reviewable Drafts panel.
2. Open the persona update draft.
3. Press `a` to approve.
4. Press `p` to apply.

Expected:

- Approval and apply are separate actions.
- Apply writes the governed persona update only after approval.
- The final message shows the target path.

## 12. Check Error Diagnostics

Type `/errors`.

Expected:

- If no recent errors exist, the panel shows an empty state.
- If errors exist, entries show time, stage, model, sanitized base URL, and error text.
- Authorization headers, API keys, tokens, passwords, and URL credentials are not shown.

## 13. Wrong Model Name Smoke

1. Type `/model use nonexistent-model-x`.
2. Type `/errors`.

Expected:

- The model switch fails safely.
- Diagnostics point to the model/provider issue.
- The TUI does not leak credentials.

## Keyboard Reference

| Key | Context | Action |
| --- | --- | --- |
| Tab | Global | Cycle focus forward |
| Shift+Tab | Global | Cycle focus backward |
| Ctrl+C / Ctrl+Q | Global | Quit |
| Esc | Detail/edit modes | Back or close detail |
| j/k or Down/Up | Lists | Move cursor |
| Enter | Lists | Open detail or use selected item |
| a | Draft detail | Approve pending draft |
| r | Draft detail | Reject pending draft |
| p | Approved draft detail | Apply approved draft |
| d | Candidate list/detail | Draft candidate |
| x | Candidate list/detail | Dismiss candidate |
| r | Candidate list/detail | Recover supported orphan candidate |
| n | Model profiles | New profile from preset |
| s | Model panel | Switch profiles/models view |
| p | Profiles view | Persist profile as workspace active |
| t | Model/profile row | Test connection |
| e | Model panel | Type a model name manually |

## Commands

| Command | Description |
| --- | --- |
| `/model` | Open model panel |
| `/model profiles` | Open profile view |
| `/model current` | Show current session model |
| `/model use <name>` | Hot-switch current session model |
| `/model test` | Test current model/profile |
| `/model persist <profile>` | Set workspace active profile |
| `/candidates` | Refresh persona candidates |
| `/errors` | Show recent diagnostics |
| `/status` | Show runtime status |
| `/drafts` | List drafts |
| `/refresh` | Reload panels |
| `/quit` or `/exit` | Exit TUI |
