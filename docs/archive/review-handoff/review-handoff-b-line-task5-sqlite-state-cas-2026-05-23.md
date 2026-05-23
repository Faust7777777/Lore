# Review Handoff: B-line Task 5 — SQLite State Mutation CAS Normalization

Date: 2026-05-23

Owner line: B-line (store).

Slice ID: B-line task 5 (per
`docs/handoff-full-project-review-2026-05-23.md` Recommended
Roadmap R1 item 3 + architect
`hermes-workspace/store-module-handover.md` section 2.2).

## Scope

Three sqlitestore mutation paths still used the same Get-then-
Save pattern that task 3's `UpdateFindingState` and earlier
`ClaimCandidateForDraft` / `ClaimCandidateForRetry` already
replaced:

- `UpdateDraftState`
- `UpdateCandidateState`
- `LinkCandidateDraft`

Under the current `MaxOpenConns=1` setting these were
effectively serialized by the connection pool, but the two-step
shape becomes a race the moment the pool widens (or a future
direct-SQL admin path writes outside the helpers). This slice
normalizes all three around the established single-transaction
pattern.

## Changed files (1)

- `internal/store/sqlitestore/store.go` — three method bodies
  rewritten:
  - `UpdateDraftState` gains an `id == ""` guard plus the
    transactional CAS shape (Begin → SELECT payload → unmarshal
    → mutate → UPDATE WHERE id=? AND state=<observed> →
    RowsAffected==1 → Commit).
  - `UpdateCandidateState` same shape, with a note that the
    legacy-empty-state OR branch from `ClaimCandidateForDraft`
    is reachable here via the observed state being `""` (which
    the WHERE clause matches directly).
  - `LinkCandidateDraft` same shape, with a comment that all
    callers drive from a known prior state so a CAS miss means
    a genuine peer race, not a contract bug.

## Why transactional + CAS even though MaxOpenConns=1

- **Today**: writes are serialized by the connection pool. The
  CAS WHERE clause is a no-op (the SELECT and UPDATE see the
  same row). Behavior is byte-identical to the pre-change
  Get-then-Save.
- **Tomorrow**: any change that widens `MaxOpenConns` exposes
  the previous two-step pattern as a race. A peer's `SaveDraft`
  or another `UpdateDraftState` call between the SELECT and the
  INSERT-ON-CONFLICT would silently overwrite. The CAS guard
  catches that condition and returns `store.ErrConflict` so the
  caller can re-read + decide instead of silently losing data.
- **Operational signal**: a future maintainer reading the
  helper sees the CAS guard and immediately understands the
  transitions are protected. The previous Get-then-Save had no
  such signal; readers had to know `MaxOpenConns=1` to be
  comfortable with it.

The pattern also matches `ClaimCandidateForDraft` /
`ClaimCandidateForRetry` / `UpdateFindingState` so the four
mutation paths read consistently.

## What was NOT touched

- No memory / jsonstore changes. Both backends already serialize
  through a struct-wide mutex; their two-step patterns are
  already race-free for the in-process case.
- No interface change. Method signatures and error contracts
  are unchanged; the only new path is the `ErrConflict` return
  on a CAS miss, which is consistent with the other CAS-bearing
  store methods.
- No app / orchestrator / persona / CLI / MCP / TUI / SDK
  changes.
- No schema migration. The drafts and persona_candidates tables
  already had the `state` column the CAS WHERE clause needs.

## Review focus

- Confirm each method's read-then-write happens inside a single
  `tx.Begin / tx.Commit` block with a deferred rollback. A
  reviewer spotting a stray `s.db.Exec` instead of `tx.Exec`
  inside one of these helpers has a real finding.
- Confirm the WHERE clause references the `state` column
  (separately indexed) rather than the `payload` blob. Using
  payload here would prevent the optimizer from using the
  state-based index that other queries rely on.
- Confirm the WHERE binding is the **observed** state, not the
  **new** state. Binding the new state would make the UPDATE
  always succeed (state column already equals the value we're
  about to set), defeating the CAS.
- Confirm `affected != 1` returns `store.ErrConflict` rather
  than `nil` (no rows changed because the row no longer matches
  the observed state -- the peer raced ahead).

## Why no new race test

A direct in-process race test is not added in this slice:

- The only writer is the single-connection sqlite pool. A
  `SaveDraft` + `UpdateDraftState` sequence trivially passes
  the CAS because the second SELECT sees the SaveDraft result
  and the WHERE clause matches.
- Reproducing the race would require either multi-connection
  setup or low-level transaction race injection, both out of
  scope for this slice and not part of the architect's
  recommendation. Task 3's `UpdateFindingState` test surface
  also relies on the same single-connection reality.
- Full existing sanity (8 packages, all ApplyDraft / ApproveDraft
  / RejectDraft / RequestDraftRevision / SupersedeDraft /
  DismissPersonaCandidate / CreatePersonaDraftFromCandidate /
  RetryRejectedPersonaDraft / RecoverPersonaCandidateLink /
  ForceDismissPartialPersonaCandidate tests) exercise these
  methods and stays green, proving the transaction wrap and
  CAS WHERE clause do not regress the existing
  single-connection contract.

If `MaxOpenConns` is widened in a future infra slice, that slice
should add the multi-connection race test against these four CAS
helpers as part of its acceptance criteria.

## Validation

```powershell
go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./internal/orchestrator/... ./cmd/... -count=1
# all green
```

## Known limitations / future follow-up

- The four CAS-bearing methods now stand alone; the remaining
  mutation methods on sqlitestore (`SaveDraft`,
  `SaveCheckpoint`, `SaveDailyReport`, `AppendAudit`,
  `SaveFinding`, `AppendUsage`, `SaveCursor`,
  `UpsertCandidate`) are append-or-replace by design and do not
  need CAS. Future writes that introduce a transition semantic
  (e.g. a `Persona.RevisedAt` field that must monotonically
  advance) should adopt the same shape.
- `LinkCandidateDraft`'s CAS uses the observed state as the
  WHERE clause. The callers
  (`CreatePersonaDraftFromCandidate`, `RecoverPersonaCandidateLink`,
  `RetryRejectedPersonaDraft`) currently all drive the candidate
  through a known prior state, so the CAS firing means a peer
  raced. If a future caller wants to link unconditionally
  (i.e. force-link regardless of state), the right move is to
  add a sibling helper rather than relax the CAS here.
- A `PRAGMA user_version` schema migration framework is the
  next architect-recommended store improvement (see
  store-module-handover 4.1 item 2). Not required for this
  slice.
