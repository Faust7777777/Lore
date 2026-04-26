# Review Handoff: Sessionlog Missing Index Rebuild

## Change

`sessionlog.loadIndex` now rebuilds `index.json` from existing `*.jsonl` transcripts when the index file is missing.

Changed files:

- `internal/sessionlog/index.go`
- `internal/sessionlog/sessionlog_test.go`

## Reason

Session transcripts are the primary data. `index.json` is derived state used by `sessions list`, `sessions search`, and resume selection. If the index file is deleted or not materialized, existing transcripts should still be discoverable.

## Behavior

- Missing `index.json` triggers a rebuild from same-directory `*.jsonl` files.
- Rebuilt summaries come from `Load`, so title, turn count, timestamps, model, and agent fields follow transcript semantics.
- The rebuilt index is written back to disk.
- Corrupt or unreadable `index.json` still returns an error; this change does not silently overwrite a damaged index.

## Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/sessionlog -run TestListRecentRebuildsMissingIndex -count=1 -v
.\scripts\verify.ps1
```

Both passed locally.

## Review Focus

- Confirm `rebuildIndex` cannot recursively call `loadIndex` through `Load`.
- Confirm missing index recovery is appropriate while corrupt index remains explicit failure.
- Confirm `Search` benefits through `ListRecent` after rebuild.
