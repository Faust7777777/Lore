# Review Handoff: A-Line Sessionlog Streaming Search

Date: 2026-05-25

## Scope

Hardens sessionlog transcript search for long-running sessions by replacing
whole-file transcript reads with bounded streaming scans.

Touched files:

- `internal/sessionlog/writer.go`
- `internal/sessionlog/sessionlog_test.go`
- `scripts/release-gate.ps1`
- `README.md`

## Why

`Search()` already uses the session index for ordering, but transcript-content
matching previously called `os.ReadFile` for every candidate transcript. Long
sessions could force large allocations just to answer whether a query appears
in a transcript.

## Implementation

- `transcriptContains` now opens the transcript and scans line-by-line with
  `bufio.Scanner`.
- The scanner uses the existing `MaxJSONLLineBytes` limit so search and
  transcript loading share the same line-size boundary.
- Existing missing/unreadable transcript behavior is preserved: search treats
  that transcript as non-matching.

## Locked Behavior

- Title/session-ID matching still happens before transcript scanning.
- Search still returns summaries in recent-index order.
- Large JSONL lines within `MaxJSONLLineBytes` remain searchable.
- Search does not expose transcript contents; it only returns matching
  summaries.

## Release Gate

`scripts/release-gate.ps1` now includes:

```powershell
go test ./internal/sessionlog -run "Test(Search(MatchesTranscriptContent|ScansLargeTranscriptLines))$" -count=1 -v
```

`README.md` release-gate wording now mentions streaming search.

## Verification

Run during implementation:

```powershell
go test ./internal/sessionlog -count=1
.\scripts\release-gate.ps1 -SkipDiffCheck
```

## Review Focus

- Confirm search no longer loads entire transcript files.
- Confirm scanner buffer uses the established sessionlog line cap.
- Confirm unreadable transcripts remain non-fatal for search.
