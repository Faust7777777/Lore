# Review Handoff: A-Line Sessionlog Index Lock

Date: 2026-05-25

## Scope

Closes the next sessionlog long-running resilience gap: multiple active
recorders in the same Lore process could update the same `index.json` through
independent `load -> modify -> save` cycles.

Touched files:

- `internal/sessionlog/index.go`
- `internal/sessionlog/sessionlog_test.go`
- `scripts/release-gate.ps1`
- `README.md`

## Why

`Recorder.mu` protects one recorder instance only. It does not protect two
sessions, two resumed handles, or command paths in the same process writing the
same sessionlog root. Because `index.json` is a shared derived file, the
critical section is root-wide:

```text
loadIndex -> merge summary -> saveIndex
```

Without a root-level lock, two writers can both load an older index, then the
later save can drop the earlier writer's session summary.

## Implementation

- Added a package-level root-keyed mutex registry in `index.go`.
- `loadIndex`, `rebuildIndex`, `saveIndex`, and `upsertIndex` now serialize
  index access for the same normalized root.
- Internal unlocked helpers keep the public functions from deadlocking:
  - `loadIndexUnlocked`
  - `rebuildIndexUnlocked`
  - `saveIndexUnlocked`
- The lock is intentionally process-local. It is a guard for concurrent
  recorders in one Lore process, not a cross-process file lock.

## Test Coverage

`TestConcurrentRecordersPreserveIndexEntries` starts 24 recorders for the same
sessionlog root concurrently. Each recorder writes a user/assistant turn. The
test then asserts:

- every session is present in `ListRecent`;
- every session has `TurnCount == 1`;
- no concurrent save dropped another writer's entry.

The existing missing-index rebuild and corrupt-index rejection tests still
cover the read/rebuild boundaries.

## Release Gate

`scripts/release-gate.ps1` now includes
`TestConcurrentRecordersPreserveIndexEntries` in the sessionlog group.

`README.md` release-gate wording now says "concurrent incremental index
updates".

## Verification

Recommended reviewer commands:

```powershell
go test ./internal/sessionlog -run "TestConcurrentRecordersPreserveIndexEntries" -count=1 -v
go test ./internal/sessionlog -count=1
.\scripts\release-gate.ps1 -SkipDiffCheck
```

## Review Focus

- Confirm `upsertIndex` holds one root-level lock across the whole
  load/merge/save sequence.
- Confirm `loadIndex` missing-index rebuild and direct `saveIndex` callers are
  also protected.
- Confirm this does not claim cross-process locking; that remains a separate
  durability topic if Lore later supports multiple OS processes writing one
  sessionlog root simultaneously.
