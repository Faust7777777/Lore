# Review Handoff: Sessions CLI P0 Acceptance Coverage

## Change

Added CLI-level tests for `sessions list/search/show` behavior in `internal/cli/sessions_command_test.go`.

## Coverage

- `sessions list` empty state returns exit 0 and prints `No sessions found.`
- `sessions search` empty state returns exit 0 and prints `No sessions found.`
- `sessions show` with a missing transcript returns non-zero and a clear `sessions show:` error.
- `sessions list` and `sessions search` fail explicitly on corrupt `index.json`.
- `sessions show` is read-only: it does not modify `index.json` or the transcript JSONL.

## Reason

This locks the P0 sessions CLI acceptance boundary at the user command layer, not just the `sessionlog` package layer.

## What Did Not Change

- No production code changed.
- No SDK, TUI, runtime governance, or agent behavior changed.
- Tests create transcripts directly through `sessionlog`, avoiding model/LLM dependency.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/cli -run 'TestSessions(ListEmptyState|SearchEmptyState|ShowMissingTranscriptFails|CommandsRejectCorruptIndex|ShowIsReadOnly)' -count=1 -v
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm exit code expectations match the P0 acceptance criteria.
- Confirm corrupt index remains explicit failure at CLI level.
- Confirm `sessions show` remains read-only and does not trigger resume/replay behavior.
