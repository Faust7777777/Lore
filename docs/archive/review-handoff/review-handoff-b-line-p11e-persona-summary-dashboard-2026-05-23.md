# Review Handoff: B-P11e `lore persona summary` Dashboard

Date: 2026-05-23

Owner line: B-line (persona memory candidate pipeline, business / data)

Commit: `b1808ee feat(cli): lore persona summary one-page dashboard`

## Scope

Adds `lore persona summary` as the third subgroup under `lore
persona` (after `candidates` and `errors`). Combines the two
answers an operator already could get separately
(`candidates list --state ...` × 3, `errors --tail N`) into a
single read-only dashboard. Used by:

- the manual test cookbook's new Step 9a as a single-glance health
  check during a real-chat session;
- regression triage when something looks off post-extraction
  (one command vs. four).

## Changed files (2)

- `internal/cli/cli.go` — new dispatch case + `runPersonaSummaryCommand`
  + `renderPersonaSummary` helper. Top-level `lore persona` usage
  line lists `summary` alongside `candidates` and `errors`.
- `internal/cli/persona_summary_test.go` — new file, 7 tests
  covering empty workdir / log-aggregation / empty-log-file /
  end-to-end / partial-orphan visibility / usage line / workdir
  echo.

## CLI surface

```
lore persona summary [--workdir <p>]
```

No other flags. The dashboard is a snapshot, not a windowed report
(the cost-split windowing belongs to `lore usage`).

Output shape:

```
Persona Memory Summary
======================
Workdir: <abs path>

Candidates:
  open         N
  drafted      N (linked M, partial-orphan K)
  dismissed    N

Extract log:
  source: <path>
  total entries: N
  by stage:
    extract           N
    parse_warning     N
    store             N
    (malformed)       N         # only when non-zero
  last entry: <RFC3339 or "—">
```

The log section distinguishes:

- "(no log file yet)" — file does not exist; a fresh workdir or
  one that has never run extraction.
- "(empty)" — file exists but contains no entries; extraction ran
  and never failed in this workdir.
- a numbered breakdown — at least one failure has been logged.

Stage counts sort alphabetically so successive `summary` runs
produce identical output (`map` iteration order is otherwise
unspecified in Go).

## Why `renderPersonaSummary` is a pure helper

The CLI command opens a runtime to resolve `cfg.Paths.StateDir`,
then calls `renderPersonaSummary(stdout, workDir, logPath, openN,
draftedLinked, draftedOrphan, dismissedN)` with concrete numbers.
This split lets the unit tests drive the renderer with synthetic
counts + a seeded log fixture without any OpenRuntime / sqlite
overhead. The end-to-end tests still exercise the runtime path so
the wiring stays under coverage; the split is for keeping the
log-format / stage-sort / partial-orphan-split tests fast and
deterministic.

## Frozen-output considerations

The output format is intentionally human-first. A future caller
that wants to script against the dashboard (alerting, monitoring)
should drive `--format json` once it exists, not parse the human
output. No JSON shape is shipped today because there is no
consumer; the existing read-only listing commands have the same
status.

## What was NOT touched

- No `internal/app/*` interface change. The runtime's
  `ListPersonaCandidates(state, 0)` and `PersonaExtractLogPath()`
  are already public; `summary` is a thin orchestrator.
- No store change. Read-only.
- No persona / MCP / TUI / CI change.

## Review focus

- Confirm the snapshot-not-windowed scope is correct. A `--days N`
  flag would force the dashboard to either re-query the runtime
  for past states (which it does not track) or to aggregate the
  log over a window (which `lore persona errors --since` already
  does). Keeping `summary` snapshot-only avoids that overlap.
- Confirm the partial-orphan count is taken from
  `record.DraftID == ""` after listing State=Drafted, not from a
  new store query. Matches `renderPersonaCandidateList`'s DRAFT
  column logic so the dashboard and the list cannot disagree.
- Confirm the malformed-line bucket is visible. A line that
  `parsePersonaLogLine` cannot decode lands in `(malformed)` so
  the operator notices schema drift; it does not roll into the
  totals invisibly.
- Confirm the empty-vs-missing-log distinction renders for both
  branches. A fresh `lore status <workdir>` produces neither file
  nor entries, while a workdir that ran extraction once with
  zero failures produces an empty file.

## Validation

```powershell
go test ./internal/cli -count=1 -run "TestRenderPersonaSummary|TestRunPersonaSummary|TestRunPersonaWithoutSubcommandMentionsSummary" -v
# 7 tests PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./cmd/... -count=1
# all green
```

Sample dashboard against a workdir mid-test:

```
Persona Memory Summary
======================
Workdir: C:\Users\15892\Desktop\lore-manual-test

Candidates:
  open         2
  drafted      1 (linked 1, partial-orphan 0)
  dismissed    0

Extract log:
  source: C:\...\state\logs\persona-extract.log
  total entries: 1
  by stage:
    parse_warning    1
  last entry: 2026-05-23T01:42:11Z
```

## Known limitations / future follow-up

- No `--format json` mode. Add when a monitoring consumer asks.
- No history axis: the dashboard answers "what does this workdir
  look like right now". A future "summary over the last 7 days"
  view would require persisting daily snapshots, which is a
  separate slice.
- No --quiet / --check flags returning exit codes based on
  thresholds (e.g. exit 1 when partial-orphan > 0). Could ship
  if CI / health-check users materialize.
- The malformed-line bucket name `(malformed)` is intentionally
  bracketed to sort distinct from valid stage names; if a future
  stage rename collides ("malformed" used as a real stage), the
  bracketing keeps them separate.
