# Review Handoff: Resume CLI P0 Acceptance Coverage

## Change

Added resume behavior tests in `internal/cli/session_resume_test.go`.

## Coverage

- `--resume` with non-TTY stdin lists recent sessions and fails with `--resume-id` guidance.
- `--resume-id <id>` works in non-interactive mode without printing an interactive prompt.
- `--resume` with no prior sessions returns an explicit `no previous sessions found` error.
- Missing `--resume-id` target fails and does not start a fresh session.

## Reason

This locks the P0 resume acceptance boundary: scripted runs should use `--resume-id`, interactive selection should require TTY, and failure paths must not silently create new transcripts.

## What Did Not Change

- No production code changed.
- TUI and SDK behavior are unchanged.
- Tests exercise `configureSessionRecorder` directly to avoid model/LLM dependency.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/cli -run 'TestConfigureSessionRecorderResume' -count=1 -v
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm non-TTY `--resume` cannot block on input.
- Confirm `--resume-id` restores history and emits only a resume notice.
- Confirm failed resume paths leave `session.Recorder` nil.
