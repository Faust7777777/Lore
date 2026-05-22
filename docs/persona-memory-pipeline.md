# Persona Memory Candidate Pipeline (B-line)

This document is the maintainer-facing overview of the LLM-based
persona memory candidate pipeline shipped across slices B-P1 through
B-P11c. Each individual slice has its own review-handoff under
`docs/archive/review-handoff/review-handoff-b-line-*`; this file
exists so a new contributor (or a future reviewer) can read one page
and understand the moving parts without reconstructing the timeline
from `git log`.

When `git log` and this document disagree, trust `git log` and update
this document.

## Purpose

Mine durable personal facts ("I major in economics", "I review
English listening errors on Wednesday evenings") from the operator's
ordinary chat with Lore, surface them as reviewable persona update
proposals, and route every accepted proposal through the same
governance pipeline the external MCP `persona_update_propose` tool
uses. The operator stays in control: nothing is auto-applied to the
persona document.

## Data flow (end-to-end)

```
console / TUI user turn
        |
        v
  loop-agent.Respond (returns Final)
        |  Handle persists turn into session log
        |
        v
  launchPersonaExtraction (fire-and-forget goroutine)
        |
        |--- LLM call (LORE_LLM_*, system prompt
        |     "extract candidate persona facts")
        |       |
        |       +-- parser.go validates NFKC-substring,
        |             confidence >= medium, current vs. proposed,
        |             emits result.Candidates + result.Warnings
        |
        |--- on err           -> stage=extract log line
        |--- on zero+warnings -> stage=parse_warning log lines
        |--- on each kept     -> RecordPersonaCandidate (UpsertCandidate)
        |                          -> store dedup on DedupKey
        |                          -> on store err -> stage=store log line
        |
        v
  store.PersonaCandidates: rows in State=Open
        |
        v
  operator runs `lore persona candidates list`
        |
        |--- show, dismiss, draft, recover, retry-rejected
        |
        v
  CreatePersonaDraftFromCandidate
        |
        |--- ClaimCandidateForDraft (atomic Open -> Drafted, DraftID="")
        |--- Harness.ProposePersonaUpdate (same path the MCP tool uses)
        |       |
        |       +-- creates Draft (Kind=persona_update, State=pending_review)
        |       +-- emits Event(EventDraftCreated) + AuditDraftCreated
        |
        |--- augmentPersonaDraftSummary (evidence/reason/conf/conflict)
        |--- LinkCandidateDraft (Drafted, DraftID=newID)
        |
        v
  operator reviews via `lore draft review/approve/reject/supersede`
        |
        |  rejected/expired/superseded -> candidate stays Drafted with
        |                                  DraftID pointing at the dead
        |                                  draft until retry-rejected
        |                                  mints a fresh one.
        |
        |  approved + applied -> persona document mutates;
        |                         candidate stays Drafted with the
        |                         applied draft's ID as audit anchor.
        |
        v
  persona document under managed core (governance-checked write)
```

## CLI surface

```
lore persona candidates list   [--workdir <p>] [--state open|drafted|dismissed] [--limit N]
lore persona candidates show   <id> [--workdir <p>]
lore persona candidates dismiss <id> [--workdir <p>]
lore persona candidates draft  <id> [--workdir <p>] [--retry-rejected]
lore persona candidates recover <id> [--workdir <p>] --link <draft-id>
lore persona candidates recover <id> [--workdir <p>] --force-dismiss

lore persona errors [--workdir <p>] [--tail N] [--stage extract|store|parse_warning] [--since <duration>]

lore usage [--days N] [--workdir <p>]
   - daily DAY / CALLS / PROMPT / COMPLETION / TOTAL table
   - "By purpose:" block aggregating across the window
   - persona_extract appears as its own bucket
```

`lore persona candidates draft <id>` is the standard promote. With
`--retry-rejected` it gates on the linked draft being in a terminal
state (rejected / expired / superseded) and mints a fresh draft via
ClaimCandidateForRetry.

`lore persona candidates recover` is the partial-orphan recovery
surface (State=Drafted, DraftID=""). `--link` mends, `--force-dismiss`
abandons.

## State machine

```
                  +------- LLM extract -------+
                  v                            |
              [ Open ]                          (Upsert with
                  |  ClaimCandidateForDraft       same DedupKey
                  v   (CAS: Open -> Drafted)      returns existing)
       +------ [ Drafted, DraftID="" ]
       |          |                              ^
       |          | LinkCandidateDraft (success) |
       |          v                              |
       |       [ Drafted, DraftID=X ]            |
       |          |                              |
       |  retry-rejected when                    |
       |  Draft(X).State terminal:               |
       |  ClaimCandidateForRetry                 |
       |  (CAS: DraftID=X -> "")                 |
       |          |                              |
       |  recover --force-dismiss                |
       |  (partial only)                         |
       v          v                              |
      [ Dismissed ]                              |
                                                 |
                  recover --link <draft-id>  ----+
                  (partial only, kind=persona_update check)
```

`DedupKey = NormalizeText(field) + "|" + NormalizeText(value) + "|" + NormalizeText(evidence)`
where `NormalizeText` is NFKC + lowercase + whitespace collapse. The
key does NOT include `source_kind` or `SourceSessionID` -- two
sessions reporting the same fact collapse to one candidate. See
"Design rationale" below.

## Failure modes & recovery

| Symptom | Likely cause | Operator action |
|---|---|---|
| `lore persona candidates list` empty after a chat | LLM call failed, parser discarded all, or store write error | `lore persona errors --tail 20` to disambiguate |
| `extractor: model request failed: ... deadline exceeded` | Slow / hung LLM provider | Bump `LORE_LLM_PERSONA_EXTRACT_TIMEOUT=30s` and retry |
| `parser refused: evidence_quote not in user_text` | LLM paraphrased the user's words | Repeat the utterance verbatim, or accept the candidate is unprovable |
| `draft <id>` returns `ErrPersonaCandidateAlreadyDrafted` (DraftID empty) | Prior LinkCandidateDraft failed -- partial orphan | `lore draft list` to find the orphan, then `recover --link <id>` or `recover --force-dismiss` |
| `draft <id> --retry-rejected` returns `ErrPersonaDraftNotTerminalForRetry` | Linked draft still pending_review | Review the existing draft first; retry only after the reviewer's decision |
| `draft <id>` returns `ErrPersonaCandidateDismissed` | Candidate was force-dismissed | Repeat the underlying user turn so a fresh DedupKey lands |
| Drafts table has two persona_update drafts for one candidate | Should not happen post-B-P8 round 2. Investigation needed. | `git log internal/store/sqlitestore/store.go` to confirm the CAS guard is intact; file a regression |

## Observability

- **Success path**: each persona_extract call writes a
  `UsageRecord(Purpose=persona_extract)`. `lore usage` surfaces the
  daily count and tokens in the "By purpose:" block.
- **Failure path**: each failure writes one line to
  `<StateDir>/logs/persona-extract.log`:
  ```
  <RFC3339Nano UTC>\tstage=<extract|store|parse_warning>\tsession=<id>\terror="..."\n
  ```
  Read via `lore persona errors`. The log file is append-only and not
  rotated by Lore itself.
- **Stages**:
  - `extract` — `extractor.Extract(ctx, input)` returned err (LLM
    transport error, context deadline, parser hard refusal).
  - `parse_warning` — `result.Warnings` non-empty AND
    `result.Candidates` empty (only emitted when zero candidates
    landed, to keep the log focused on diagnostic cases).
  - `store` — `runtime.RecordPersonaCandidate` returned err.

## Frozen contracts

These shapes are stable; future slices must add fields rather than
rename or restructure.

- `persona.PersonaCandidate` (Field / ProposedValue / CurrentValue /
  EvidenceQuote / Reason / Confidence / Conflict / SourceKind /
  SourceSessionID / ObservedAt). Adding fields is fine; renaming
  breaks the JSON store and the sqlite payload column.
- `persona.PersonaCandidateRecord` (ID, State, DedupKey, Candidate,
  CreatedAt, UpdatedAt, DraftID).
- `persona.PersonaCandidateState` values:
  `open` (or empty for legacy rows) / `drafted` / `dismissed`. A new
  transient state like "retrying" would require touching every
  state-switch in `internal/app/persona_candidate.go` and the three
  store backends; B-P8 round 2 deliberately reused the
  `(Drafted, "")` partial-orphan shape instead.
- `store.PersonaCandidateStore` interface methods:
  `UpsertCandidate / GetCandidate / ListCandidatesByState /
  UpdateCandidateState / LinkCandidateDraft / ClaimCandidateForDraft /
  ClaimCandidateForRetry`.
- `model.UsageRecord` (frozen additive). `Purpose` accepts new
  string values; `PurposeBreakdown` on `UsageSummary` keys by the
  same strings.
- Persona extract log line format (tab-delimited, four fields). A
  reader (`lore persona errors`) parses these by field position;
  reordering would break the reader.

## Design rationale

### Why DedupKey includes evidence but not source_kind

Two operators repeating the same fact from different sessions should
collapse to one candidate (the fact does not become more true when
said twice). Source attribution lives on the candidate record itself
for audit, not in the dedup space. If a future import source emits
the same fact with paraphrased evidence, it will produce a separate
candidate -- that is the conservative outcome (the operator can
dismiss the extra). The alternative (key on field + value only)
would let a low-quality early candidate block a higher-quality later
one with richer evidence.

### Why the partial-orphan shape

`Drafted` with `DraftID=""` is the in-between state both
`LinkCandidateDraft` failure and `ClaimCandidateForRetry` produce.
Reusing one shape -- rather than introducing a fourth state like
`Retrying` -- means:

- `recover --link` and `recover --force-dismiss` work for both
  retry-mid-flight and link-failure-after-propose without branching.
- The `ErrPersonaCandidateLinkedStateRequired` guard at the top of
  `RetryRejectedPersonaDraft` automatically refuses a blind retry
  when the candidate is in this shape, regardless of which path
  put it there.
- No state-switch in the three store backends needs to learn a new
  enum value.

### Why ClaimCandidateForDraft AND ClaimCandidateForRetry

Two distinct CAS preconditions:

- For-Draft: `state == Open` and `state == ""` (legacy compat).
  Output: `(Drafted, "")`.
- For-Retry: `state == Drafted` and `DraftID == expectedDraftID`.
  Output: `(Drafted, "")`.

Folding them into one method (`ClaimCandidate(id, expectedState,
expectedDraftID, now)`) would make every call site juggle two
preconditions. Two narrow methods keep each call site at one
intent.

### Why a workdir log file rather than `lore usage`-style records

Failures often have no tokens to bill (transport error, parser
discard). Synthesizing `UsageRecord(TokensIn=0, TokensOut=0,
ErrorReason=...)` rows would either pollute the cost view with
zero-token rows or force a schema split. The file is cheaper, the
read path (`lore persona errors`) is operator-friendly enough, and
the two surfaces have orthogonal questions: `lore usage` answers
"what cost token", `lore persona errors` answers "what failed".

### Why the acceptance gate is opt-in

`scripts/release-gate.ps1 -PersonaAcceptance` runs a real-fake-LLM
end-to-end exercise of the console -> extractor -> CLI flow. It is
not on the PR path because (a) it takes ~3 seconds even on cached
machines, (b) the fake-model gate is a property test, not a
contract test that all PRs need to pass, and (c) A-line owns the
release-gate cadence. Default PRs hit the lighter gates; nightly
or pre-release runs include `-PersonaAcceptance`.

## Known limitations / deferred slices

These were observed during B-P1 through B-P11c but intentionally
not addressed because the cost/benefit did not justify a slice yet.
A future maintainer (or reviewer) considering them should consult
the linked review-handoff for the original analysis.

- **No log rotation** for `<StateDir>/logs/persona-extract.log`. A
  single user's worst-case growth (one failure per turn for a
  permanently broken provider) is bounded around 10 MB / year, so
  rotation was punted.
  See `review-handoff-b-line-p9-extraction-observability-2026-05-22.md`.
- **No `lore usage --by-day-and-purpose` matrix**. Single-user
  workdirs do not have a per-day attribution need that justifies
  the table-width cost. See P11a handoff.
- **No DedupKey-versus-source_kind redesign**. The current key
  collapses cross-source duplicates, which is correct for the
  console-only world today. Re-examine once a second import
  source materializes.
- **No GC for dismissed candidates**. They are dedup tombstones, so
  pruning them would let the same fact re-emerge. A `--release-dedup`
  semantics is needed before any prune command can ship.
- **No `--follow` (streaming) mode for `lore persona errors`**.
  Operators use `Get-Content -Wait` if they need it.
- **No parallel acceptance scaffold for the draft/recover path**.
  The current acceptance gate stops at list + show + no auto-draft.
  Extending it to exercise `draft` / `recover` / `retry-rejected`
  is A-line follow-up; see
  `review-handoff-b-line-cross-boundary-persona-gate-activation-2026-05-21.md`
  for the original boundary note.

## Commit timeline

| Commit | Slice | Headline |
|---|---|---|
| `83192c1` | B-P1+P2 | LLM extractor contract |
| `86082cb` | B-P3 | Three-backend persona_candidate store |
| `87c8a17` | B-P4 | Console / TUI fire-and-forget extraction |
| `688d062` | B-P5+P6 | Store CAS for promote + CLI list/show/dismiss/draft |
| `1c8f692` | B-P7 | show displays linked DraftID |
| `3d9b892` | B-P8 v1 | Lifecycle recovery (recover --link / --force-dismiss / draft --retry-rejected) |
| `4f3aaac` | B-P9 v1 | Workdir log + LORE_LLM_PERSONA_EXTRACT_TIMEOUT env |
| `d4625f6` | A-Gate | Acceptance gate activated (cross-boundary) |
| `ccf1940` | B-P10 | list adds DRAFT column |
| `4ad71d9` | B-P11a | lore usage purpose breakdown |
| `a3eebab` | B-P8 v2 | ClaimCandidateForRetry CAS (closes duplicate-race) |
| `eeafd8d` | B-P9 v2 | parse_warning logging when zero candidates |
| `e688adf` | B-P8 v2 | sqlite payload-CAS tighten (followup) |
| `8103f5f` | B-P11c | lore persona errors reader |

Plus per-slice handoffs under
`docs/archive/review-handoff/review-handoff-b-line-*` and the
cross-boundary A-Gate handoff at
`review-handoff-b-line-cross-boundary-persona-gate-activation-2026-05-21.md`.

## When to update this document

- New persona pipeline slice lands → add a row to the commit
  timeline and update the relevant section (state machine, CLI
  surface, observability).
- A frozen contract gains a new field → list it under "Frozen
  contracts" with the additive note.
- A known limitation is addressed → move it from "Known
  limitations" to the appropriate prose section.
- A design decision is reversed → update "Design rationale" with
  the new direction and link the slice that reversed it.
