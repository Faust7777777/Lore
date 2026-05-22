# Review Handoff: B-P8 Candidate Lifecycle Recovery + Retry-Rejected

Date: 2026-05-22

Owner line: B-line (persona memory candidate pipeline, business / data)

Commit: `3d9b892 feat(persona,app,cli): candidate lifecycle recovery + retry-rejected`

## Scope

Closes the lifecycle dead-ends the P5+P6 commit message acknowledged
as "operator must reconcile manually": (a) partial orphan
`State=Drafted, DraftID==""` from a failed `LinkCandidateDraft`, and
(b) the rejected/expired/superseded draft that leaves the candidate
stuck pointing at a dead draft with no way to retry.

## Changed files (4)

- `internal/app/persona_candidate.go` — 3 new methods, 4 new sentinel
  errors, extracted `buildPersonaProposalFromCandidate` helper shared
  with `CreatePersonaDraftFromCandidate`, new
  `isTerminalDraftStateForRetry` predicate.
- `internal/app/persona_candidate_test.go` — 15 new tests across the
  three methods including happy paths, every guard refusal, and
  idempotent retries.
- `internal/cli/cli.go` — new `recover` subcommand with mutually
  exclusive `--link <draft-id>` / `--force-dismiss` modes; existing
  `draft <id>` gains `--retry-rejected` flag.
- `internal/cli/persona_command_test.go` — 6 new CLI tests covering
  recover paths, retry-rejected paths, and mutex-flag rejection.

## What the three new methods do

| Method | Required state | Effect |
|---|---|---|
| `RecoverPersonaCandidateLink(id, draftID, now)` | State=Drafted, DraftID="" | Verify draft exists + kind=persona_update, then LinkCandidateDraft |
| `ForceDismissPartialPersonaCandidate(id, now)` | State=Drafted, DraftID="" | Transition to Dismissed (DedupKey tombstone preserved) |
| `RetryRejectedPersonaDraft(id, now)` | State=Drafted, DraftID!=""; linked draft in {Rejected, Expired, Superseded} | New propose + LinkCandidateDraft overwrites old DraftID; original draft left untouched as audit |

## New sentinel errors

- `ErrPersonaCandidatePartialStateRequired` — recover targets only partial scar
- `ErrPersonaCandidateLinkedStateRequired` — retry-rejected requires linked draft
- `ErrPersonaDraftNotTerminalForRetry` — linked draft still PendingReview/Approved/Applied
- `ErrPersonaDraftKindMismatch` — `--link` target is not persona_update

## What was NOT touched

- No `internal/store/*` interface changes — `LinkCandidateDraft` already
  supported overwrite from B-P5+P6, no schema change needed.
- No mutation of the generic `Harness.RejectDraft` / `SupersedeDraft`
  hook — candidate lifecycle is expressed via explicit recovery
  commands rather than implicit cross-kind callbacks. (User
  explicitly rejected the implicit-hook design before B-P8 started.)
- No MCP, TUI, or CI changes.

## Review focus

- Confirm the three guard refusals form a complete partition:
  every (state, DraftID) tuple either falls into a clear refusal
  branch or a clear success path.
- Confirm `RetryRejectedPersonaDraft` cannot race a still-pending
  reviewer: the `isTerminalDraftStateForRetry` predicate is
  load-bearing, verified by temporary revert in the commit.
- Confirm the LinkCandidateDraft-failure error message in
  `RetryRejectedPersonaDraft` (lines ~285) gives operators enough
  context to reconcile manually without losing the new DraftID.
- Confirm `buildPersonaProposalFromCandidate` extraction does not
  drift between `CreatePersonaDraftFromCandidate` and
  `RetryRejectedPersonaDraft` (a future change to the proposal
  shape would have to update both paths via the helper).

## Validation

```powershell
go test ./internal/app -count=1 -run "RecoverPersonaCandidate|ForceDismissPartial|RetryRejectedPersonaDraft" -v
# 15 tests PASS

go test ./internal/cli -count=1 -run "RunPersonaCandidatesRecover|RunPersonaCandidatesDraftRetryRejected" -v
# 6 tests PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... -count=1
# all green
```

Load-bearing verified by temporarily removing the terminal-state
guard in `RetryRejectedPersonaDraft` and confirming
`TestRetryRejectedPersonaDraftRejectsWhenDraftStillPendingReview` +
`TestRetryRejectedPersonaDraftRejectsWhenDraftApproved` both fail,
then restoring.

## Known limitations / future follow-up

- Concurrent `RetryRejectedPersonaDraft` calls on the same candidate
  rely on `LinkCandidateDraft`'s overwrite semantics; if reviewers
  want CAS-level guarantees on retry too, a `ClaimCandidateForRetry`
  helper would be the natural addition (parallel to
  `ClaimCandidateForDraft` from P5+P6).
- No GC for stranded orphan drafts — operator uses `lore draft
  list` + manual `RejectDraft` to clear them. Could grow a `--purge`
  / sweep command if the orphan count becomes a real burden.

## Round 2 fix (reviewer-flagged blocker)

Reviewer found that `RetryRejectedPersonaDraft` as initially shipped
reopened the duplicate-draft race that P5+P6 explicitly closed: two
concurrent callers could both pass the terminal-state check at
persona_candidate.go:492 and both `ProposePersonaUpdate`, with
`LinkCandidateDraft` silently overwriting the DraftID. A blind retry
after a `LinkCandidateDraft` failure could also create a second
draft because the candidate still pointed at the old terminal
DraftID.

Fix: new store-level CAS `ClaimCandidateForRetry(id,
expectedDraftID, now)` that atomically clears the candidate's
DraftID iff it equals expectedDraftID, leaving the candidate in
(Drafted, "") -- the partial-orphan shape that the
`ErrPersonaCandidateLinkedStateRequired` top-guard already refuses.
The CAS shipped in all three backends; `RetryRejectedPersonaDraft`
calls it after the terminal-state check and before
`ProposePersonaUpdate`. Propose-failure path restores the prior
link via `LinkCandidateDraft(priorDraftID)` so a transient propose
error does not strand the candidate.

Tests added:

- `internal/store/persona_candidate_test.go`:
  `TestClaimCandidateForRetryClearsDraftIDWhenMatched`,
  `TestClaimCandidateForRetryRejectsMismatchedDraftID`,
  `TestClaimCandidateForRetryRejectsOpenCandidate`,
  `TestClaimCandidateForRetryReturnsNotFoundForMissingID`,
  `TestClaimCandidateForRetryRejectsEmptyInputs`,
  `TestClaimCandidateForRetryIsAtomicUnderConcurrency` (16
  goroutines, exactly one CAS winner). All run table-driven across
  memory / json / sqlite.
- `internal/app/persona_candidate_test.go`:
  `TestRetryRejectedPersonaDraftConcurrentCallsOnlyOneSucceeds` (8
  goroutines through `barrierClaimStore.ClaimCandidateForRetry`,
  exactly one winner, drafts list has 2 entries = original rejected
  + 1 retry),
  `TestRetryRejectedPersonaDraftLinkFailureDoesNotDuplicate`
  (`linkFailingPersonaStore` makes LinkCandidateDraft fail; first
  retry leaves candidate in partial-orphan with orphan draft logged,
  second blind retry hits `ErrPersonaCandidateLinkedStateRequired`,
  total persona_update drafts in store = 2).
- The `linkFailingPersonaStore` and `barrierClaimStore` mocks gained
  `ClaimCandidateForRetry` methods that delegate / barrier in
  parallel to the existing `ClaimCandidateForDraft` shape.

Load-bearing verified by temporarily removing the
`ClaimCandidateForRetry` call in `RetryRejectedPersonaDraft` and
confirming both new app-layer race tests fail (concurrent test:
all 8 goroutines win; link-failure test: candidate.DraftID not in
partial-orphan shape), then restoring.
