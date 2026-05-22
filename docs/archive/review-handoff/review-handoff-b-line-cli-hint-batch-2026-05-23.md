# Review Handoff: B-line CLI Hint Batch

Date: 2026-05-23

Owner line: B-line (persona memory candidate pipeline, business / data)

Slice IDs: B-P11d (informal — incremental UX polish over the
B-P5+P6 / B-P8 / B-P11c CLI surfaces, no new feature)

## Scope

Four CLI polish commits batched into one handoff for review:

| Commit | Surface | Change |
|---|---|---|
| `55b55d1` | `lore persona candidates list` | reject `--limit < 0` so the store contract's "<=0 = no cap" stops surprising operators who typed a negative limit |
| `ae8a5ec` | `lore persona candidates dismiss <id>` | when the candidate is Drafted, append a hint pointing at `recover --force-dismiss` or `lore draft reject` |
| `c5e857a` | `lore persona candidates draft <id>` | 4 typed-error hints (dismissed / partial-orphan / retry-rejected-on-open / non-terminal-linked) |
| `4418201` | `lore persona candidates recover <id> --link / --force-dismiss` | typed-error hints for non-partial / kind-mismatch / not-found |

Single thread of work: every lifecycle refusal (state guard,
kind guard, lookup miss) that the persona CLI surfaces now ends
with a concrete `lore ...` command the operator can copy/paste,
with the actual candidate / draft ID baked in.

## Why batch

Each individual commit is small (5-50 LOC plus 1-4 tests) and the
design contract is shared:

- the helper sits in `internal/cli/cli.go` next to the surface it
  decorates (no global hint router);
- the helper signature is `func render<Surface>ErrorHint(stderr
  io.Writer, candidateID string, ..., err error)` so the call
  site is one line: `render<Surface>ErrorHint(stderr, ..., err)`;
- inside the helper, a top-level `switch` with one `errors.Is`
  case per typed error means a new error type slots in by adding
  a case, not by rewiring the call site;
- each hint line is plain text indented with two leading spaces,
  matching the existing `parsePersonaErrorsFlags` validation
  message style;
- the candidate / draft ID is interpolated into the example
  command so the operator does not have to copy the ID separately.

A reviewer who accepts one helper's design accepts all four. Hence
the batched handoff.

## When to add a new hint

Add a hint when ALL of the following hold:

1. The error returned by the runtime is a typed sentinel (or wraps
   one such), so `errors.Is(err, sentinel)` is a stable check.
2. There is a concrete `lore ...` command (or a small enumerated
   set of them) that addresses the failure condition.
3. The operator could plausibly hit the failure during normal use
   — not just from a malformed input that should have been caught
   in flag parsing.

Do NOT add a hint when:

- The error is generic (`fmt.Errorf("open runtime: %v", err)`) and
  the next step depends on operator context.
- The follow-up requires the operator to read code or a doc rather
  than type another command (those belong in
  `docs/persona-memory-manual-test.md` cookbook, not the CLI).
- The hint would just paraphrase the error message verbatim
  ("the linked draft is rejected" → "because the linked draft is
  rejected, you may want to ... rejected drafts") — that adds
  noise without information.

## Where to put a hint

Co-locate the helper with the subcommand handler in `cli.go`:

- A subcommand with one typed error → keep the hint inline in the
  case branch (the `dismiss` case in ae8a5ec).
- A subcommand with two or more typed errors → extract a
  `render<Surface>ErrorHint` helper (the `draft` / `recover`
  helpers in c5e857a / 4418201).

The helper takes `io.Writer` not `*os.File` so tests can capture
output into `bytes.Buffer`. The helper does NOT return a value
— the caller has already printed the base error line and returns
exit 1 right after; the helper just appends.

## Testing pattern

For each typed-error case, write one test that:

1. Sets up the workdir state required to trigger that specific
   error (e.g. seed an Open candidate to hit
   `ErrPersonaCandidateDismissed`, force partial-orphan to hit
   `ErrPersonaCandidateAlreadyDrafted` on the draft surface).
2. Calls `Run` with the command line that should produce the
   error.
3. Asserts a non-zero exit and a list of stderr substrings
   covering both the underlying error and the hint fragments.

The substring list MUST include the actual candidate / draft ID
where it appears in the hint, not a fixture constant. That way
a future refactor of the hint format that drops the ID
interpolation fails loudly.

## What was NOT touched

- No store interface change.
- No app interface change (no new sentinel errors).
- No persona prompt / parser / extractor change.
- No MCP / TUI / CI change.
- No new flags beyond the validation tightening on `--limit`.

## Validation

```powershell
go test ./internal/cli -count=1 -run "TestRunPersonaCandidates.*Hint|TestRunPersonaCandidatesListRejectsNegativeLimit" -v
# 10 hint tests + 1 validation test PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./cmd/... -count=1
# all green
```

Manual smoke against a fresh workdir reproduces the hints:

```powershell
$work = "$env:USERPROFILE\Desktop\hint-smoke"
Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
lore status $work
# Seed an Open candidate via your usual chat flow, capture its id
# as $pc. Then:
lore persona candidates dismiss --workdir $work nonexistent
lore persona candidates draft   --workdir $work $pc
lore persona candidates dismiss --workdir $work $pc   # after promote
lore persona candidates recover --workdir $work --link draft-bogus $pc
lore persona candidates recover --workdir $work --force-dismiss $pc
# Each failure now ends with a "hint:" block naming the next command.
```

## Known limitations / future follow-up

- The `ErrNotFound` hint in the recover surface uses an
  `err.Error()` substring match (`"not found"`) because the app
  layer wraps both candidate-not-found and draft-not-found into
  the same `store.ErrNotFound` sentinel. A future slice could
  introduce distinct sentinels (`app.ErrCandidateNotFound`,
  `app.ErrDraftNotFound`) so the recover helper can `errors.Is`
  on each and skip the dual-hint output.
- The hint format is plain text. A future `lore` JSON output mode
  (`--format json`) would need a parallel hint shape — an array
  of `{command, description}` rather than free text. Not pursued
  because no JSON consumer exists today.
- No hint on the `lore persona errors` surface yet; its only
  failure mode is "log path inaccessible" which surfaces as a
  generic OS error and does not have a clean follow-up command
  to point at. Revisit if a typed error materializes there.
- The pattern is persona-specific. Other CLI subgroups (`lore
  draft`, `lore findings`, `lore status`) could adopt the same
  helper shape for their own typed errors — a follow-up slice
  could codify a `internal/cli/hints.go` shared file if more
  surfaces start carrying typed sentinels.
