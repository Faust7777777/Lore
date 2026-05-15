# Review Handoff: Corrupt Session Index Guard

## Change

Added `TestListRecentRejectsCorruptIndex` in `internal/sessionlog/sessionlog_test.go`.

## Reason

Missing `index.json` is recoverable derived state and is rebuilt from transcripts. A present but corrupt `index.json` should remain an explicit failure, not be silently overwritten. This test locks that boundary after the missing-index rebuild change.

## Behavior Locked

- `ListRecent` returns an error when `index.json` exists but contains invalid JSON.
- The corrupt index file is not overwritten during that failed read.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/sessionlog -run TestListRecentRejectsCorruptIndex -count=1 -v
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm this complements, rather than weakens, missing-index rebuild behavior.
- Confirm corrupt index remains visible to the caller.
