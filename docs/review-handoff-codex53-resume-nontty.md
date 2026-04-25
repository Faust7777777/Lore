# 5.3 Codex Review Handoff: Non-TTY Resume Behavior

## Scope

This change makes `--resume` deterministic in non-interactive contexts.

Changed files:
- `internal/cli/cli.go`
- `internal/cli/session_resume_test.go`

## Behavior

- `--resume-id <id>` remains the explicit scripted resume path.
- `--resume` without `--resume-id` now requires interactive stdin.
- If stdin is not a terminal, Lore prints the recent session list and returns an error telling scripts to use `--resume-id <id>`.
- No transcript is started and no old transcript is loaded when that non-TTY `--resume` error path is taken.

## Review Focus

- Confirm `configureSessionRecorder` checks interactivity before calling `chooseRecentSession`.
- Confirm non-TTY `--resume` does not block waiting for scanner input.
- Confirm `--resume-id` behavior is unchanged.
- Confirm this is CLI/session behavior only; no TUI presentation files are part of this change.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/cli ./internal/sessionlog ./cmd/obsidian-harness -count=1
```