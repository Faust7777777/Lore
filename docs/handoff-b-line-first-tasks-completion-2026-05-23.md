# Handoff: B-line First Tasks Completion — 2026-05-23

Audience: next reviewer / next B-line contributor.

Status: All 6 ordered first-tasks from
`docs/handoff-full-project-review-2026-05-23.md` section 8
"B Line first tasks" are shipped. Task 7 (archive 10 Hermes
docs) is intentionally NOT done because the handoff conditions
it on explicit user request.

When this document and `git log` disagree, trust `git log`.
This is a synthesis as of 2026-05-23 and may drift as new
slices land.

## TL;DR

| Task | Headline | Code commit | Doc commit |
|---|---|---|---|
| 1 | Persona CLI accepts flags before OR after positional ID | `9c828eb` | `8d80843` |
| 2 | Persona docs note both flag orderings | bundled in `8d80843` | — |
| 3 | Finding state transitions validated across 3 backends (P0) | `7b509cd` | `f6a44d0` |
| 4 | ApplyDraft emits governance Finding on write-success/state-fail (P0) | `f41d5ed` | `54e89db` |
| 4r2 | ApplyDraft tailors error when SaveFinding ALSO fails (reviewer Medium) | `68e265e` | `3440e8f` |
| 5 | SQLite state mutation methods go transactional + CAS | `447fb3c` | `1750284` |
| 6 | Config Validate pass on runtime open | `c785fa6` | `be3305b` |
| 7 | Archive Hermes docs | NOT DONE — gated on explicit request |

Full sanity green across 9 packages
(`internal/store ./internal/store/jsonstore ./internal/store/sqlitestore ./internal/app ./internal/cli ./internal/console ./internal/orchestrator ./internal/config ./cmd/...`).

## Files reviewers should open first

1. This document.
2. `docs/handoff-full-project-review-2026-05-23.md` — the
   architect-driven plan that defined the 7 tasks.
3. Per-task handoffs under `docs/archive/review-handoff/`:
   - `review-handoff-b-line-task1-persona-flag-ordering-2026-05-23.md`
   - `review-handoff-b-line-task3-finding-transition-validation-2026-05-23.md`
   - `review-handoff-b-line-task4-apply-state-fail-recovery-2026-05-23.md`
     (read the "Round 2 fix" section for the SaveFinding-fail path)
   - `review-handoff-b-line-task5-sqlite-state-cas-2026-05-23.md`
   - `review-handoff-b-line-task6-config-validate-2026-05-23.md`
4. The per-slice handoffs each have a "Review focus" section
   highlighting the load-bearing assertions for that task.

## Highest-priority review focuses

If a reviewer has limited time, the order of attention is:

1. **Task 4 round-2** (`68e265e`). The error-message branching
   on `SaveFinding`'s result is the only place in the codebase
   that has to be honest about a double-failure scenario. The
   `failingFindingStore` fixture and the `errors.Is` chain are
   the load-bearing pieces.
2. **Task 3 Finding transitions** (`7b509cd`). The sqlite CAS
   guard mirrors task 5's pattern; reviewer should verify the
   `FindingOpenForCAS` constant is the only legal source state
   in the WHERE clause, and that the model-level table test
   covers every illegal pair.
3. **Task 5 SQLite CAS normalization** (`447fb3c`). Three
   methods (`UpdateDraftState`, `UpdateCandidateState`,
   `LinkCandidateDraft`) now use the same CAS pattern as
   `ClaimCandidateForDraft` and `UpdateFindingState`. The
   WHERE binding is the **observed** state captured by the
   in-transaction SELECT, not the new state.
4. **Task 1 flag ordering** (`9c828eb`). The
   `reorderFlagsBeforePositionals` helper is unscoped (works
   for any flag.FlagSet), but the call sites are persona-
   specific. Other CLI subgroups (`lore draft`, `lore findings`,
   `lore status`) keep stdlib flag.Parse behavior.
5. **Task 6 Config Validate** (`c785fa6`). The rule set is
   deliberately small and operator-facing; cross-field policy
   rules are intentionally NOT enforced.

## Cumulative test stats

Approximate as of `be3305b`:

- `internal/config` — Validate happy + 8 reject categories +
  `LoadWithOptions` integration (2 cases).
- `internal/store` — 3-backend table-driven Finding transitions
  (8 cases × 3 backends + 1 model-level table × 10 cases).
- `internal/orchestrator` — ApplyDraft happy path + failure
  injection (Finding emit success) + double-failure (Finding
  emit ALSO fails) + existing conflict / supersede / approve /
  reject tests.
- `internal/cli` — 4 new `*WorksWithIDFirst` flag-ordering
  tests + the pre-existing persona command suite.
- Plus all pre-existing tests across the 9 packages.

## What was intentionally NOT shipped

- **Task 7 Hermes docs archive.** The full-project-review
  handoff sections 5 and 8 conditioned this on the operator
  asking for it. Until that request lands, the 10 Hermes
  markdown files at `C:\Users\15892\Desktop\docs\hermes-workspace`
  stay outside the repo.
- **Human-readable duration parsing** (`"500ms"` JSON form).
  The architect noted it alongside config validation; deferring
  it kept task 6 small. A `humanDuration` wrapper type would
  change every consumer's field type.
- **Environment variable override layer.** Three-layer (default
  / user-global / workspace) covers documented uses today.
- **Cross-field config consistency rules** (e.g. AuditLogDir
  must be inside StateDir). Validate is for typos, not policy.
- **Vault atomic write durability** (fsync, cross-device rename
  fallback). Architect P2 item; lives in `internal/vault` and
  is a natural next slice for the B-line.
- **Vault search returning more than first match per file** and
  **backlinks beyond wiki-link patterns**. Both shipped 2026-05-25.
  See `docs/archive/review-handoff/review-handoff-b-line-vault-search-and-backlinks-2026-05-25.md`.
- **TUI persona candidate review panel.** A-line scope, listed
  in the architect handoff Recommended Roadmap R3.

## Push readiness

The architect handoff also flagged a parallel A-line concern:
`main...origin/main` is ahead by ~30 commits and includes A-line
SDK / TUI work. None of the B-line first-tasks commits introduce
a push blocker -- the worktree is clean, `git diff --check` is
clean, full sanity is green, and the only unstaged file
(`docs/handoff-full-project-review-2026-05-23.md`) is the
architect's review baseline that was already on disk before this
work started.

The push decision still belongs to the human operator; this
document only records that B-line work does not add new blockers.

## What to do next (if anything)

Optional, in roughly decreasing value:

1. **Real-LLM manual test.** Run
   `docs/persona-memory-manual-test.md` against a real LLM
   endpoint. The persona pipeline + the just-shipped state-
   machine + config-validation hardening should all hold.
2. **Vault atomic write durability** (architect P2 item under
   "Config and Vault Infra Need Hardening"). Add fsync to
   `vault.WriteFileAtomic` and a fallback for cross-device
   renames (`syscall.EXDEV`). Failure-injection test would
   parallel task 4's failingFindingStore shape.
3. **Human-readable duration parsing** (deferred task 6
   follow-up). Adds a `humanDuration` wrapper type so config
   files can say `"500ms"` instead of `500000000`. Caller-side
   churn is non-trivial.
4. **Task 7 archive Hermes docs.** Wait for explicit request;
   architect handoff is clear that this is operator-gated.

If none of the above is needed, B-line is at a natural pause
point for review.

## Reading order for a new contributor

1. This document.
2. `docs/handoff-full-project-review-2026-05-23.md` (the
   architect plan).
3. Per-task handoffs in the order listed under "Files reviewers
   should open first" above.
4. `docs/persona-memory-pipeline.md` (the maintainer overview
   for the persona pipeline shipped earlier).
5. `docs/persona-memory-manual-test.md` (the operator cookbook).
