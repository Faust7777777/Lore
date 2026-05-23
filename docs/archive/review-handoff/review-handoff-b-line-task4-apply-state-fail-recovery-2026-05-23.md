# Review Handoff: B-line Task 4 — ApplyDraft Write/State Recovery

Date: 2026-05-23

Owner line: B-line (orchestrator / governance / store).

Slice ID: B-line task 4 (per
`docs/handoff-full-project-review-2026-05-23.md` section 4
"P0 - ApplyDraft Has a Vault/State Consistency Window").

## Scope

P0 closes the governance correctness gap on the apply path. The
state-machine audit confirmed `Harness.ApplyDraft` had a narrow
window where the vault file was written via
`vault.WriteFileAtomic` but the follow-up `UpdateDraftState`
could fail, leaving the vault changed while the draft stayed in
the Approved state with no audit signal.

This commit adds a recovery hook that emits a governance Finding
on the write-success / state-fail path so the operator can see
the inconsistency in `lore findings list` and reconcile manually.

## Changed files (2)

- `internal/orchestrator/harness.go` — `ApplyDraft` now branches
  on the `transitionDraftState` error. When the transition
  fails AFTER the write completed, it calls a new
  `recordApplyStateFailFinding` helper and returns a wrapped
  error pointing the operator at `lore findings list`.
  Helpers added next to ApplyDraft: `recordApplyStateFailFinding`
  (builds the Finding, swallows SaveFinding errors), `findingID`
  (prefix + UnixNano, mirrors auditID).
- `internal/orchestrator/apply_state_fail_test.go` — new file
  with a `stateUpdateFailingStore` + `failingDraftStore` test
  fixture and two cases: the failure-injection happy path and a
  regression guard that the normal happy path does NOT emit the
  finding.

## The recovery path

```
                   write vault.WriteFileAtomic
                          |
                          v
                  store.UpdateDraftState (state=applied)
                       /   \
                  err?  yes \--> recordApplyStateFailFinding
                              \--> return wrapped error pointing at
                                   `lore findings list`
                       no  --> recordAudit + return applied draft
```

The Finding shape:

```
Kind:       FindingGovernanceReviewNeeded
Severity:   FindingSeverityCritical
State:      FindingOpen
Target:     draft.Target
Title:      "ApplyDraft wrote vault but state update failed"
Summary:    "Draft <id> vault write completed for <path> but the
             store could not persist State=applied. Operator
             must verify file content matches the proposal and
             reconcile manually."
Detail:     "Underlying state-update error: <err>"
Source:     "apply_draft_state_fail"
Metadata:
  draft_id    <draft.ID>
  draft_kind  <draft.Kind>
  vault_path  <absolute vault path>
  target_path <draft.Target.Path>
  state_error <err.Error()>
```

The error returned to the caller:

```
apply: vault write succeeded but draft state update failed: <err>;
a governance Finding was emitted, run `lore findings list` to inspect
```

`errors.Is` against the original `<err>` still works so callers
that need to branch on the root cause (e.g. retry on transient
sqlite busy) can.

## Why Finding-emit instead of a new "applying" state

The architect handoff listed two options:

1. Emit a Finding / audit signal on the write-success/state-fail
   path. (Adopted here.)
2. Flip to a prepare/commit protocol where the draft is marked
   `apply_write_succeeded` (or similar intermediate state)
   before the write, then transitioned to `applied` after.

Option 1 was chosen because:

- It preserves the existing 9-state Draft state machine. No
  `CanTransition` table change, no domain-layer schema rev.
- It reuses the Finding state machine that task 3 just hardened,
  so a critical Finding in this new path can never be silently
  swallowed back to a non-terminal state.
- The Finding lands in the existing `lore findings list`
  dashboard the operator already uses; no new surface to learn.
- A future slice can layer Option 2 on top if real-world data
  shows that Finding-based recovery is not strong enough.
- Option 2 requires either a new DraftKind transition table or
  a write-side flag on the existing Approved state, both of
  which change the draft contract that external MCP callers
  observe via `lore draft list`.

## What was NOT touched

- No `internal/model/draft.go` change. The Draft state machine
  is unchanged.
- No `internal/store/*` interface change. SaveFinding is a
  pre-existing method.
- No CLI / persona / MCP / TUI / SDK change.
- No CI / release-gate change.

## Review focus

- Confirm the helper is called only when the state update fails
  AFTER the vault write succeeded. Specifically: not before the
  write (target validation, baseVersion mismatch, patch error
  all bail out before WriteFileAtomic), and not on the
  conflicted path (markDraftConflicted runs from
  readDraftTargetForApply, before any write).
- Confirm the wrapped error preserves the underlying state-update
  error via `%w` so `errors.Is` works.
- Confirm the Finding is saved before the wrapped error is
  returned. A reverse order would leave a race where a caller
  could see the error but not yet see the Finding.
- Confirm SaveFinding errors are intentionally swallowed (with
  a code comment explaining why) rather than masked into the
  apply error. The operator already has the inconsistent vault
  state to deal with; a double error would not help.

## Validation

```powershell
go test ./internal/orchestrator -count=1 -run "TestApplyDraft" -v
# 4 tests PASS: Emits, HappyPathDoesNotEmit, UpsertsProgressRow, DetectsConflict

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./internal/orchestrator/... ./cmd/... -count=1
# all green
```

Load-bearing verified by temporarily replacing the
`recordApplyStateFailFinding` call + wrapped error with a bare
`return model.Draft{}, err` and confirming
`TestApplyDraftEmitsGovernanceFindingWhenStateUpdateFails` fails
on `missing "vault write succeeded but draft state update
failed"`, then restoring.

## Known limitations / future follow-up

- The Finding-based recovery is observability, not idempotence.
  An operator who retries `lore draft apply` after this Finding
  fires will hit the baseVersion guard (the file hash no longer
  matches the draft's stored baseVersion) and the second attempt
  marks the draft Conflicted. That is the conservative outcome
  (no double-write), but the operator now has BOTH the Finding
  AND a Conflicted draft to clean up. A future polish could
  detect "draft is approved + Finding apply_draft_state_fail
  exists for this draft" and transition the draft to Applied
  directly without re-writing the file.
- The Finding ID format uses UnixNano, which is microsecond-unique
  in practice but theoretically collidable under extreme
  parallelism. A future slice could use a UUID instead. Same
  caveat applies to auditID; punted because the existing usage
  hasn't tripped over collisions.
- The architect's prepare/commit alternative (Option 2 above) is
  the stronger long-term solution if multi-step apply pipelines
  become common (e.g. write + index + notify). Today the only
  multi-step is "write file + update state", which the Finding
  recovery covers adequately.
- The test fixture is local to the orchestrator package; the
  generic `stateUpdateFailingStore` pattern could be promoted
  to a shared test-support package if other store-failure tests
  start materializing.

## Round 2 fix (reviewer-flagged Medium)

Reviewer found a Medium issue on the first version of this
slice: `recordApplyStateFailFinding` swallowed `SaveFinding`
errors and the caller always claimed a Finding had been emitted,
which would be a false recovery signal in the worst case where
both stores were failing (e.g. shared sqlite handle, disk full).
Commit `68e265e` addresses this.

Changes:

- `recordApplyStateFailFinding` now returns the SaveFinding
  error. Its responsibility is just to build and save; reporting
  the outcome is the caller's.
- `ApplyDraft` branches on the SaveFinding result:
  - success: the existing message points the operator at
    `lore findings list`.
  - failure: a wrapped message includes BOTH errors and tells
    the operator "no automatic recovery record exists" so they
    know they must reconcile by hand without the audit anchor.
  - `errors.Is` against the original state-update error still
    works in both branches via the `%w` wrap.
- New test `TestApplyDraftSurfacesFindingSaveFailureInError`
  drives the double-failure scenario with a `failingFindingStore`
  whose SaveFinding returns the injected error. Asserts both
  errors appear in the apply error, the "no automatic recovery
  record exists" line is present, and the store-side findings
  table is genuinely empty.
- `failingFindingStore` proxies read / list / update calls through
  to the inner store; only `SaveFinding` is rejected. Compile-time
  guard `var _ store.FindingStore = (*failingFindingStore)(nil)`
  catches future interface additions at build time.

Load-bearing verified by temporarily reverting the branch on
SaveFinding's result (always claim a Finding was emitted) and
confirming the new test fails on the "ALSO failed to emit
governance Finding" substring check, then restoring.
