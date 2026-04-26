# Review Handoff: Sessionlog Large JSONL Lines

## Change

`sessionlog.Load` now uses the explicit `MaxJSONLLineBytes` scanner limit instead of a hard-coded 1 MiB line limit.

Changed files:

- `internal/sessionlog/model.go`
- `internal/sessionlog/writer.go`
- `internal/sessionlog/sessionlog_test.go`

## Reason

Transcript events can contain long assistant messages or pasted user content. A 1 MiB JSONL scanner limit can make resume/load fail with `bufio.Scanner: token too long`, even though the transcript file is otherwise valid.

## Behavior

- The current max JSONL event line is `16 MiB`.
- Corrupted-line handling is unchanged.
- Transcript history still keeps only the last 20 conversation turns in the restored snapshot.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/sessionlog -run TestLoadSupportsLargeJSONLLines -count=1 -v
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm `16 MiB` is acceptable as a P0/P1 transcript line cap.
- Confirm the test exceeds the old 1 MiB limit and validates full content restoration.
- Confirm no TUI or SDK behavior changed.
