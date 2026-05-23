# Review Handoff: A-Line Sessionlog Index Atomicity

Date: 2026-05-23

## Scope

- `internal/sessionlog/index.go`
- `internal/sessionlog/sessionlog_test.go`
- `scripts/release-gate.ps1`
- `README.md`

## Summary

Hardens sessionlog index writes by replacing direct `os.WriteFile(index.json)` with same-directory atomic write via `vault.WriteFileAtomic`.

New behavior:

- `saveIndex` writes `index.json.tmp` first and then renames it over `index.json`.
- Stale temp files are overwritten by the next save.
- Successful saves leave no temp file behind.
- Existing corrupt `index.json` behavior remains unchanged: `loadIndex` still surfaces corrupt index errors and does not silently rebuild/overwrite.

## Boundary

- No transcript JSONL format change.
- No session summary model change.
- No CLI output change.
- No TUI/operator/MCP/persona change.

## Regression Test

- `TestSaveIndexReplacesAtomicallyAndCleansTempFile`

The test plants a stale `index.json.tmp`, writes two sessions, verifies the temp file is removed, and verifies both sessions remain indexed newest-first.

## Release Gate

The sessionlog release-gate group now includes the atomic index test. README release-gate wording was updated.

## Validation

```powershell
go test ./internal/sessionlog -run "Test(SaveIndexReplacesAtomicallyAndCleansTempFile|ListRecent|RecorderWritesIndex)" -count=1 -v
go test ./internal/sessionlog ./internal/cli ./cmd/obsidian-harness -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-gate.ps1 -SkipDiffCheck
```
