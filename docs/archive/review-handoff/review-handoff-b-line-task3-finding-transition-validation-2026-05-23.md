# Review Handoff: B-line Task 3 — Finding Transition Validation

Date: 2026-05-23

Owner line: B-line (model / store / governance).

Slice ID: B-line task 3 (per
`docs/handoff-full-project-review-2026-05-23.md` section 4
"P0 - Finding State Transitions Are Not Enforced" and
`hermes-workspace/lore-state-machine-audit.md` F-1).

## Scope

P0 closes the only state machine in the codebase that had no
transition guard. Before this commit:

- A resolved Finding could be overwritten back to open silently.
- An ignored Finding could be flipped to resolved silently.
- All three store backends (memory, jsonstore, sqlitestore) had
  the same gap; the audit trail could imply a clean state machine
  while the underlying data permitted invalid histories.
- This was inconsistent with the Draft and PersonaCandidate state
  machines, which both ship with strong guards (`CanTransition`,
  `ClaimCandidateForDraft`, `ClaimCandidateForRetry`).

## Changed files (5)

- `internal/model/finding.go` — new `ValidateFindingTransition`
  + `ErrFindingTerminalState` + `ErrFindingIllegalTransition`.
- `internal/store/memory/store.go` — UpdateFindingState validates
  before mutating under the existing mutex.
- `internal/store/jsonstore/store.go` — same pattern plus a
  previous-value rollback on `persistLocked` failure so disk
  writes never leak a partial transition.
- `internal/store/sqlitestore/store.go` — switched from Get +
  Save to a single transaction with read + validate + CAS-style
  `UPDATE ... WHERE id = ? AND state = ?` + RowsAffected == 1
  check. Adds a `FindingOpenForCAS` constant pinning the WHERE
  clause to the only legal source state.
- `internal/store/finding_transition_test.go` — new file, 8
  3-backend tests + 1 model-level table covering 10 transition
  cases.

## State machine (flat, by design)

```
                 +--> resolved [terminal]
   open ---->----+
                 +--> ignored  [terminal]
```

- `open` is the only legal source state.
- `resolved` and `ignored` are terminal.
- No retry / reopen path. If the operator wants to reverse a
  decision, they file a new Finding; the original stays in the
  audit trail.
- `open -> open` is rejected too, since UpdateFindingState is a
  mutating operation and a no-op should not stamp UpdatedAt.

This matches the architect's audit recommendation and the
existing Daemon code paths (`recordOutOfBandScanFinding` only
emits Open; no daemon code requests a reopen).

## Why ValidateFindingTransition lives in `model`

- The function takes only model types, so the three backends can
  share it without depending on each other.
- The two sentinel errors (`ErrFindingTerminalState`,
  `ErrFindingIllegalTransition`) are caller-facing; placing them
  in `model` lets app / CLI / future MCP layers all `errors.Is`
  them without importing internal store packages.
- This is the same shape used by `internal/persona` for the
  PersonaCandidate state helpers.

## Why sqlite uses a CAS transaction instead of just validating

`MaxOpenConns = 1` makes the single-connection case race-free
today, but the architect's review and the existing
`ClaimCandidateForDraft` / `ClaimCandidateForRetry` patterns
treat the in-process validation as belt-and-suspenders. The
SQL-level guard catches:

- A peer connection (if `MaxOpenConns` ever rises above 1) that
  transitions the row between SELECT and UPDATE.
- A future direct-SQL admin tool that bypasses the Go validation.
- A migration script that mistakenly overwrites state without
  going through the helper.

In all those cases `RowsAffected == 0` and the helper returns
`store.ErrConflict` instead of silently winning the race.

## What was NOT touched

- No app / CLI / persona / MCP / TUI / SDK changes. The Runtime
  `ResolveFinding` / `IgnoreFinding` methods still return
  whatever the backend returns; callers see the typed errors
  when they attempt an illegal transition.
- No new sentinel errors leaked into the app layer.
- No CI / release-gate changes.
- No schema migration. The findings table existed before; only
  the write path is tightened.

## Review focus

- Confirm every backend goes through `ValidateFindingTransition`
  (memory line ~272, jsonstore line ~289, sqlitestore line ~395).
  A reviewer who can spot one of those calls missing has a real
  finding.
- Confirm the sqlite CAS WHERE clause uses
  `string(FindingOpenForCAS)` not a literal `"open"`. The
  constant exists to anchor the link back to
  `ValidateFindingTransition`.
- Confirm jsonstore's previous-value restore. Without it a disk
  write failure would leave the in-memory state ahead of disk.
- Confirm the model-level table test
  `TestValidateFindingTransitionTable` covers every grid cell
  including the `open -> open` rejection and the empty-source
  edge case.

## Validation

```powershell
go test ./internal/store -count=1 -run "TestUpdateFindingState|TestValidateFindingTransition" -v
# 8 backend tests x 3 backends + 1 model table x 10 cases ALL PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./cmd/... -count=1
# all green
```

Load-bearing verified by temporarily removing the
`ValidateFindingTransition` call in the memory backend's
`UpdateFindingState` and confirming
`TestUpdateFindingStateRejectsResolvedToOpen/memory` fails on
"UpdateFindingState should reject resolved -> open", then
restoring.

## Known limitations / future follow-up

- The Daemon's `recordOutOfBandScanFinding` only emits Open
  Findings; no production code path tries an illegal transition
  today. The guard is preventative.
- If a future feature wants "reopen a resolved Finding" semantics
  (e.g. an operator pressing "review again"), the canonical
  shape is a NEW Finding referencing the old one in metadata,
  not a state reversal. Document that explicitly in the daemon
  module handoff if such a feature is proposed.
- A reviewer with the app-layer hat on may want
  `Runtime.ResolveFinding` to surface a friendlier error message
  when the typed error fires (parallel to the persona CLI hint
  batch). Out of scope for task 3 because no CLI command exists
  yet for findings; the app methods are currently called from
  daemon code that handles its own errors.
- ApplyDraft still has its own state/vault consistency gap
  documented in the full project review (task 4). Finding's
  state machine is now the strongest in the codebase; ApplyDraft
  is the next thing to bring up to parity.
