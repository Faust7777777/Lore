# Handoff: B-line Persona Memory Pipeline — Ship Completion

Date: 2026-05-23

Audience: next reviewer / next contributor picking up the B-line.

Status: Backend + CLI + observability + real-LLM acceptance gate
all shipped. No outstanding code blockers. Real-LLM manual test
ready to run; deferred items are non-blocking.

When this document and `git log` disagree, trust `git log`. This
document is a synthesis as of 2026-05-23 and may drift as new
slices land.

## TL;DR

The LLM-based persona memory candidate pipeline is complete. The
operator can:

1. Chat with `lore console`. Persona-relevant facts get mined
   fire-and-forget by an LLM extractor running on each user turn.
2. Inspect mined candidates via `lore persona candidates list /
   show`.
3. Promote a candidate into a `persona_update` draft via
   `lore persona candidates draft <id>`. The draft flows through
   the same governance pipeline external MCP `persona_update_propose`
   uses; no auto-apply.
4. Recover lifecycle scars via `lore persona candidates recover
   --link / --force-dismiss` and retry rejected drafts via
   `draft <id> --retry-rejected`.
5. Diagnose failures via the workdir log surface
   `lore persona errors --tail / --stage / --since` and the
   dashboard `lore persona summary [--fail-on-orphan] [--json]`.

All lifecycle refusal paths print a `hint:` block naming the
concrete follow-up command with the actual candidate / draft ID.

## Surface map

### Console-side write paths

- `lore console <workdir>` — chat shell; spawns fire-and-forget
  persona extraction goroutine after each user turn.
- `lore tui --once <utterance> <workdir>` — same wiring, mirror
  shell.

### Operator-facing read paths

- `lore persona candidates list [--workdir <p>] [--state open|drafted|dismissed] [--limit N]`
- `lore persona candidates show <id> [--workdir <p>]`
- `lore persona errors [--workdir <p>] [--tail N] [--stage extract|store|parse_warning] [--since <dur>]`
- `lore persona summary [--workdir <p>] [--fail-on-orphan] [--json]`
- `lore usage [--days N] [--workdir <p>]` (existing, now with
  `By purpose:` block showing `persona_extract` as its own bucket)

### Operator-facing write paths

- `lore persona candidates dismiss <id>` — Open / Dismissed
  idempotent; refuses Drafted (with hint).
- `lore persona candidates draft <id> [--retry-rejected]` —
  first-time promote or retry on terminal-state linked draft.
- `lore persona candidates recover <id> --link <draft-id>` — mend
  partial-orphan by linking to an existing orphan draft.
- `lore persona candidates recover <id> --force-dismiss` — abandon
  partial-orphan (DedupKey kept as tombstone).

## Contract anchors

| Layer | File | Contract |
|---|---|---|
| `persona` package | `internal/persona/model.go` | `PersonaCandidate`, `PersonaCandidateRecord` (with `DraftID`), `PersonaCandidateState`, `DedupKey` |
| `store` package | `internal/store/store.go` | `PersonaCandidateStore` interface: `UpsertCandidate / GetCandidate / ListCandidatesByState / UpdateCandidateState / LinkCandidateDraft / ClaimCandidateForDraft / ClaimCandidateForRetry` |
| `app` package | `internal/app/persona_candidate.go` | `Runtime.RecordPersonaCandidate / GetPersonaCandidate / ListPersonaCandidates / DismissPersonaCandidate / CreatePersonaDraftFromCandidate / RetryRejectedPersonaDraft / RecoverPersonaCandidateLink / ForceDismissPartialPersonaCandidate` |
| `model` package | `internal/model/runtime.go` | `UsageRecord.Purpose`, `UsageSummary.PurposeBreakdown`, `UsagePurposeStats` (additive on existing frozen shape) |
| `console.Session` | `internal/console/session.go` | `PersonaExtractor / PersonaExtractTimeout / PersonaExtractLogger`, `DrainPersonaExtractions` |
| `Runtime` | `internal/app/runtime.go` | `PersonaExtractor / PersonaExtractTimeout / PersonaExtractLogger / PersonaExtractLogPath()` |

These shapes are stable; future work must add fields, not rename
or restructure.

## Sentinel errors

- `app.ErrPersonaCandidateAlreadyDrafted` — candidate is in any
  Drafted shape; `dismiss` / `draft` refuse.
- `app.ErrPersonaCandidateDismissed` — candidate has been
  dismissed; `draft` refuses (DedupKey is a tombstone).
- `app.ErrPersonaCandidatePartialStateRequired` — `recover` only
  works on `(Drafted, DraftID="")`.
- `app.ErrPersonaCandidateLinkedStateRequired` — `draft --retry-rejected`
  requires `(Drafted, DraftID!="")`.
- `app.ErrPersonaDraftNotTerminalForRetry` — `--retry-rejected`
  refuses while linked draft is pending_review / approved / applied.
- `app.ErrPersonaDraftKindMismatch` — `recover --link` rejects a
  non-persona_update draft.
- `store.ErrConflict` — CAS lost (`ClaimCandidateForDraft` /
  `ClaimCandidateForRetry`); duplicate-prevention won the race.

Every sentinel above has CLI-side hint coverage; see
`docs/archive/review-handoff/review-handoff-b-line-cli-hint-batch-2026-05-23.md`
for the design rules.

## Observability

| Signal | Source | Reader |
|---|---|---|
| LLM cost by purpose | UsageRecord(Purpose=persona_extract) writes in `internal/persona/extractor.go` | `lore usage` |
| Extractor failure | `<StateDir>/logs/persona-extract.log` (B-P9) | `lore persona errors`, `lore persona summary` |
| Parser refusal | same log, `stage=parse_warning` when zero candidates landed (B-P9 v2) | same |
| Store write failure | same log, `stage=store` | same |
| Candidate health snapshot | composite of `ListCandidatesByState` + log read | `lore persona summary` |
| Programmatic dashboard | same composite | `lore persona summary --json` |
| CI health gate | partial-orphan presence | `lore persona summary --fail-on-orphan` |

## Real-LLM manual test

`docs/persona-memory-manual-test.md` is the copy-pasteable
cookbook. 11 numbered steps plus a Step 9a dashboard block. The
"Acceptance criteria" list at the bottom is what passes / fails
the manual test.

`scripts/release-gate.ps1 -PersonaAcceptance` runs the offline
fake-model gate end-to-end (acceptance scaffold activated in
`d4625f6`, fake LLM persona-extractor branch in same commit).

## Frozen review milestones

| Slice | Commit | Headline |
|---|---|---|
| B-P1+P2 | `83192c1` | LLM extractor + parser contract |
| B-P3 | `86082cb` | Three-backend persona_candidate store |
| B-P4 | `87c8a17` | Console / TUI fire-and-forget extraction |
| B-P5+P6 | `688d062` | CAS-based promote + initial CLI |
| B-P7 | `1c8f692` | show displays DraftID |
| B-P8 v1+v2 | `3d9b892` / `a3eebab` / `e688adf` | Lifecycle recovery + CAS round-2 fix + sqlite payload-CAS hardening |
| B-P9 v1+v2 | `4f3aaac` / `eeafd8d` | Workdir log + LORE_LLM_PERSONA_EXTRACT_TIMEOUT env + parse_warning stage |
| A-Gate | `d4625f6` | Acceptance gate activated (cross-boundary) |
| B-P10 | `ccf1940` | list DRAFT column |
| B-P11a | `4ad71d9` | usage purpose breakdown |
| B-P11c | `8103f5f` | `lore persona errors` reader |
| B-P11d batch | `55b55d1` / `ae8a5ec` / `c5e857a` / `4418201` | CLI hint UX across 4 surfaces |
| B-P11e | `b1808ee` / `1b95ff8` / `4916ef3` | `lore persona summary` + `--fail-on-orphan` + `--json` |
| docs | various | per-slice handoffs + pipeline overview + manual test cookbook + index |

Full per-commit detail in `docs/persona-memory-pipeline.md` "Commit
timeline" section.

## What was NOT shipped (intentional)

- **MCP exposure of persona candidates** — A-line / MCP scope.
  Today, only `persona_update_propose` is on MCP; candidates stay
  local.
- **TUI candidate review view** — A-line scope (TUI workbench).
  Today, the CLI is the only operator-facing review surface.
- **Auto approve / auto apply** — violates governance principle
  (proposal-first; reviewer in the loop). Not on any roadmap.
- **DedupKey including source_kind** — design choice; current key
  is `field|value|evidence` after NFKC, source-agnostic. Revisit
  once a second import source materializes; today only `console`
  emits.
- **GC for dismissed candidates** — dismissed acts as a DedupKey
  tombstone preventing re-extraction. Prune semantics would need
  `--release-dedup` opt-in; deferred until volume becomes a problem.
- **Log file rotation for persona-extract.log** — bounded by
  failure count (one line per failure, low thousands per year
  worst case). Add when a workdir crosses ~100k lines.
- **`lore persona errors --follow` streaming** — operators use
  `Get-Content -Wait`. Native follow-mode requires either polling
  or OS-specific watch syscall; deferred.
- **History axis for `lore persona summary`** — current dashboard
  is a snapshot. A "summary over the last 7 days" view would
  require persisting daily snapshots into the audit log; deferred.

## Highest-priority review focuses

For a reviewer with limited time, the order of attention is:

1. **B-P8 v2 CAS** (`a3eebab` + `e688adf`). Two CAS additions
   (`ClaimCandidateForRetry`, sqlite payload-CAS) are the
   load-bearing pieces preventing duplicate persona drafts. The
   in-app + store layer + sqlite payload guards reinforce each
   other; weakening any one re-opens the race. Tests:
   `TestRetryRejectedPersonaDraftConcurrentCallsOnlyOneSucceeds`,
   `TestRetryRejectedPersonaDraftLinkFailureDoesNotDuplicate`,
   `TestClaimCandidateForRetryIsAtomicUnderConcurrency`.

2. **B-P9 v2 parse_warning branch** (`eeafd8d`). The "zero
   candidates, non-empty warnings" branch is what makes parser
   refusals visible. The suppress-on-success rule prevents log
   noise during normal use. Test:
   `TestSessionHandleParserWarningsLoggedWhenZeroCandidates` +
   `TestSessionHandleParserWarningsSuppressedOnSuccess`.

3. **B-P11d hint batch** (`55b55d1`+`ae8a5ec`+`c5e857a`+`4418201`).
   Every lifecycle refusal must name the next command with the
   actual candidate / draft ID. The reviewer should be able to
   read each `render*ErrorHint` function once and confirm every
   sentinel-to-command mapping is correct.

4. **B-P11e summary contract** (`b1808ee` + `1b95ff8` +
   `4916ef3`). The JSON shape under `--json` is a contract; any
   future change must be additive. The `--fail-on-orphan` exit
   code 2 is distinct from 1 (command error) — this distinction
   is what scripts branch on.

5. **A-Gate cross-boundary activation** (`d4625f6`). Cross-boundary
   slice; B-line developer touched A-line files with explicit
   user authorization. Handoff at
   `review-handoff-b-line-cross-boundary-persona-gate-activation-2026-05-21.md`
   lists exactly what was touched and what was NOT.

## Test counts

Approximate as of 2026-05-23:

- `internal/persona`: parser / extractor / model — covered by
  existing P1+P2 tests
- `internal/store`: 3-backend table-driven tests covering
  UpsertCandidate / GetCandidate / ListCandidatesByState /
  UpdateCandidateState / LinkCandidateDraft / ClaimCandidateForDraft
  (16-goroutine concurrent) / ClaimCandidateForRetry (16-goroutine
  concurrent + mismatch + Open refusal + empty inputs)
- `internal/app`: ~25 tests covering CreatePersonaDraftFromCandidate
  happy / idempotent / partial-orphan refusal / dismissed refusal /
  proposal-failure rollback / link-failure-not-duplicate /
  8-goroutine concurrent winner-only / RecoverPersonaCandidateLink
  4 paths / ForceDismissPartialPersonaCandidate 5 paths /
  RetryRejectedPersonaDraft 6 paths + 2 race tests
- `internal/console`: ~10 persona tests covering fire-and-forget
  semantics, drain timeout, logger writer (5 stages), Core
  context, empty-final skip
- `internal/cli`: ~35 tests covering list / show / dismiss / draft /
  recover happy + refusal + hint paths, errors filter combos,
  summary dashboard + --json + --fail-on-orphan, production-path
  failure mirrors for console and TUI shells

Run all:

```
go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./cmd/...
```

Full-fleet sanity has been green continuously since `e688adf`.

## What to do next (if anything)

The pipeline is at a natural pause point. Plausible next directions
(none required):

- **Real-LLM manual test session.** Run the cookbook against
  whatever LLM endpoint the operator uses. Capture any rough
  edges (especially around the prompt-vs-extractor relationship —
  some user utterances will produce zero candidates with no log
  warning if the LLM itself decides nothing was worth mining).
- **MCP exposure of `persona candidates list / show`**. Would
  let an external agent query candidates as a read tool. Stays
  proposal-first because no write surface is added.
- **Persona-extractor prompt tuning** based on real-LLM results.
  The current 5-extract / 5-NEVER prompt is conservative; real
  data may motivate either tightening (more false positives) or
  loosening (too many parse_warning discards).
- **Migrate `persona-extract.log` to UsageRecord** with a
  TokensIn/Out=0 + ErrorReason field. Currently rejected during
  B-P9 design; reconsider only if `lore usage` adds a generic
  filter that would benefit from one canonical store.

If none of the above is needed, the pipeline is complete enough
for daily use.

## Reading order for a new contributor

1. This document.
2. `docs/persona-memory-pipeline.md` — the maintainer overview
   (state machine, frozen contracts, design rationale).
3. `docs/persona-memory-manual-test.md` — what the pipeline
   should actually do from the operator's seat.
4. The B-P5+P6 handoff (committed in `688d062`'s message) for
   the original CAS design.
5. The B-P8 v2 handoff at
   `review-handoff-b-line-p8-candidate-lifecycle-recovery-2026-05-22.md`
   for the lifecycle gymnastics (partial orphan, retry-rejected,
   recover branches).
6. The B-P11d batch handoff at
   `review-handoff-b-line-cli-hint-batch-2026-05-23.md` for the
   hint UX design rules — useful before adding any new CLI
   command that exposes a typed error.

Per-slice handoffs are all under
`docs/archive/review-handoff/review-handoff-b-line-*.md` and the
index at `docs/review-handoff-index.md` covers them alphabetically.
