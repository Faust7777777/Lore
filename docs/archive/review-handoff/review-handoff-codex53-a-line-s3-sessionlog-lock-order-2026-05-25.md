# Review Handoff: A-Line S-3 Sessionlog Lock Order Comment

Date: 2026-05-25

## FINAL Finding

Addresses `REVIEW-2026-05-25-FINAL.txt` finding:

Canonical finding ID: `S-3` / `SessionLog lock-order comment` / `P1`.

- `S-3: SessionLog 锁顺序注释 (P1)`

## Scope

Documents the lock-order invariant introduced by the sessionlog per-root index
lock work.

## Modified Files

- `internal/sessionlog/writer.go`
- `internal/sessionlog/index.go`

## Implementation Notes

Added comments at both lock entry points:

- `Recorder.appendAndIndex`
  - states `Recorder.mu` must be acquired before the root-level index lock.
- `acquireIndexLock`
  - states the root-level index lock is below `Recorder.mu`.
  - warns not to call recorder methods while already holding an index lock.

No runtime logic changed in this slice.

## Load-Bearing Tests

No new tests by design. The FINAL finding requested a code-comment slice only.

Existing lock/index behavior remains covered by:

```powershell
go test ./internal/sessionlog -run "Test(ConcurrentRecordersPreserveIndexEntries|RecorderWritesIndexAndRestoresSnapshot|ResumeRefreshesIndexTurnCount)" -count=1 -v
```

## Explicit Non-Scope

- Did not change sessionlog locking behavior.
- Did not add cross-process file locking.
- Did not change recorder append/index semantics.
- Did not touch TUI, persona, MCP, or vault governance.

## Reviewer Focus

- Confirm the documented order is `Recorder.mu -> indexLocks[root]`.
- Confirm no new lock acquisition path was added.
