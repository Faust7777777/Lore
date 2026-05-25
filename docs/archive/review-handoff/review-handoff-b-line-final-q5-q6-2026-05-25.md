# Handoff: B-line FINAL Q-5 / Q-6 — 2026-05-25

Audience: next reviewer / next B-line contributor.

Source: REVIEW-2026-05-25-FINAL.txt section 3 "代码质量缺陷",
items Q-5 and Q-6. Operator instruction: keep this slice
deliberately small and separate from the Q-1/Q-2/Q-3/Q-4 cli.go
split so each finding lands in a reviewable commit.

## TL;DR

| FINAL # | Headline | Commit |
|---|---|---|
| Q-5 | `renderPersonaRecoverErrorHint` default branch now uses `errors.Is(err, store.ErrNotFound)` instead of `strings.Contains(err.Error(), "not found")` | `f7aaeb3` |
| Q-6 | `emitPersonaSummaryJSON.last_entry` formatted as RFC3339Nano so persona summary and persona candidates JSON paths agree on timestamp precision | `f7aaeb3` |

Both changes ride in the same commit (`f7aaeb3`) because each
is a one-line behavior-preserving edit and the two together are
still well under the threshold where review noise dominates.

## What Q-5 changed

Before:
```go
default:
    if strings.Contains(err.Error(), "not found") {
        // ...
    }
```

After:
```go
default:
    if errors.Is(err, store.ErrNotFound) {
        // ...
    }
```

`renderPersonaRecoverErrorHint` lives in
`internal/cli/persona_candidates.go`. The default branch fires when
the error is neither `persona.ErrCandidateInvalidState` nor
`persona.ErrCandidateAlreadyResolved` nor a draft-shape mismatch.
Confirmed sentinel reaches this branch:

- `runtime.RecoverPersonaCandidateLink` (`internal/app/persona_candidate.go:364-373`)
  returns `store.ErrNotFound` directly via
  `r.Store.PersonaCandidates().GetCandidate` and `r.Harness.GetDraft`.
- `runtime.ForceDismissPartialPersonaCandidate` returns the same
  sentinel via the same store call.
- `store.ErrNotFound = errors.New("store: not found")`
  (`internal/store/store.go:12`) is the canonical "row missing"
  signal across the runtime; nothing wraps it on this path so
  `errors.Is` is sufficient (no `errors.As` / unwrap chain needed).

Why this matters: string-matching on `err.Error()` for "not found"
was a stringly-typed contract that any future error message
phrasing change could silently break. The sentinel is the
authoritative shape; the hint is now coupled to it instead of to
prose.

## What Q-6 changed

Before:
```go
out.ExtractLog.LastEntry = latest.UTC().Format(time.RFC3339)
```

After:
```go
out.ExtractLog.LastEntry = latest.UTC().Format(time.RFC3339Nano)
```

`emitPersonaSummaryJSON` lives in `internal/cli/persona_summary.go`.
The persona candidates JSON path (`personaCandidateOut` /
`newPersonaCandidateOut` in `internal/cli/persona_candidates.go`)
already uses RFC3339Nano for `created_at`, `updated_at`,
`drafted_at`, `dismissed_at`. The summary path was the lone
holdout; downstream consumers piping `lore persona summary --json`
into `lore persona candidates list --json` saw two timestamp shapes
in the same pipeline.

Behavioral note: Go's `time.RFC3339Nano` omits trailing zero
fractional digits. A whole-second timestamp formatted via
RFC3339Nano renders identically to RFC3339
(e.g. `2026-05-23T02:30:00Z`), so existing fixtures with
whole-second timestamps remain valid. Sub-second timestamps now
get nanos; previously they were silently truncated to seconds.

## Files changed (f7aaeb3)

- `internal/cli/persona_candidates.go` (+1/-1 line in
  `renderPersonaRecoverErrorHint`; +1 import line for
  `obsidian-harness/internal/store`)
- `internal/cli/persona_summary.go` (+1/-1 line in
  `emitPersonaSummaryJSON`)

Total diff: 3 insertions / 2 deletions.

## Load-bearing test rationale

- `TestRunPersonaSummaryJSONWithLogEntries`
  (`internal/cli/persona_summary_test.go:324-353`) seeds three
  whole-second log entries (`2026-05-23T01:00:00Z`,
  `02:00:00Z`, `02:30:00Z`) and asserts
  `last_entry == "2026-05-23T02:30:00Z"`. RFC3339Nano of a
  whole-second time renders that exact string, so the assertion
  still holds — Q-6 is backwards-compatible for the existing
  fixture set without modification.
- `TestRenderPersonaRecoverErrorHintNotFound` (the persona
  candidates command tests) drives the default branch with a
  `store.ErrNotFound` return; the swap from `strings.Contains`
  to `errors.Is` keeps the same hint emission, so the existing
  assertion on stderr substring matching still passes.
- Verified end-to-end: `go test ./internal/cli/... -count=1` →
  `ok obsidian-harness/internal/cli 7.453s`.
- Verified full repo: `go build ./...` clean.

## Boundary statement (B-line scope only)

This slice did NOT touch:

- A-line vault symlink S-1 (`internal/vault/query.go`,
  `internal/orchestrator/readapi.go`, `internal/orchestrator/harness.go`).
- A-line runtimeDocs S-2 isolation
  (`internal/operatoragent/model.go`).
- A-line sessionlog S-3 lock-order documentation
  (`internal/sessionlog/index.go`, `writer.go`).
- TUI Q-8 context-background audit (`internal/tui/...`).
- SDK frame-size limits (any `internal/mcp` / `internal/sdk`
  surface).

Each of those has its own architect-driven handoff under
`docs/archive/review-handoff/review-handoff-codex53-a-line-*-2026-05-25.md`.

## What is intentionally NOT done in this slice

- **Q-7..Q-16** (the rest of the FINAL P2/P3 list). Out of
  scope; handled when their owning surface changes for unrelated
  reasons.
- Sub-second-precision regression test for the new RFC3339Nano
  output. The behavior is well-known stdlib semantics, and the
  whole-second fixture is sufficient evidence the format swap
  preserves existing contract; adding a sub-second fixture now
  would test Go's stdlib, not our code.

## Push readiness

One B-line commit added on `main`:

```
f7aaeb3 refactor(cli): tighten persona error sentinel + JSON timestamp precision
```

Working tree still carries A-line edits (S-1/S-2/S-3 plus
README/release-gate touches and four codex53 a-line handoff
docs) — those are NOT part of this commit. The push decision
belongs to the human operator; this document only records that
the B-line work is bench-clean and does not introduce a push
blocker.

## Reading order for a new contributor

1. This document.
2. The Q-1/Q-2/Q-3/Q-4 sibling handoff:
   `review-handoff-b-line-final-q1-q4-2026-05-25.md`.
3. `REVIEW-2026-05-25-FINAL.txt` section 3 items Q-5 and Q-6.
4. The commit diff: `git show f7aaeb3`.
5. The two functions touched:
   - `internal/cli/persona_candidates.go` →
     `renderPersonaRecoverErrorHint` (default branch).
   - `internal/cli/persona_summary.go` →
     `emitPersonaSummaryJSON` (last_entry field).
6. The sentinel definition:
   `internal/store/store.go:12` (`ErrNotFound`).

## FINAL Q-1..Q-6 closure

With this commit, FINAL section 3 items Q-1 through Q-6 are all
landed on `main`:

| # | Status | Commit |
|---|---|---|
| Q-1 | landed | `bd19935` |
| Q-2 | landed | `5609a80` |
| Q-3 | landed | `5609a80` |
| Q-4 | landed | `5609a80` |
| Q-5 | landed | `f7aaeb3` |
| Q-6 | landed | `f7aaeb3` |

Q-7 onward is deliberately deferred — none of those findings
intersects with surfaces B-line currently owns or has reason to
touch this cycle.
