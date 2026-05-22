# Review Handoff: B-P11c `lore persona errors` Reader Command

Date: 2026-05-23

Owner line: B-line (persona memory candidate pipeline, business / data)

Slice ID: B-P11c

## Scope

Closes the operator-facing side of the B-P9 observability loop. B-P9
wired the writer (`<StateDir>/logs/persona-extract.log` populated by
the console fire-and-forget goroutine); P9 round-2 added the
`stage=parse_warning` branch. Reading the file still required the
operator to compute the workdir-relative path and call
`Get-Content -Tail N` manually. B-P11c adds a native CLI subcommand
so the operator can type:

```powershell
lore persona errors --workdir $work --tail 20
lore persona errors --workdir $work --stage parse_warning
lore persona errors --workdir $work --since 1h
```

without knowing the file layout.

## Changed files

- `internal/app/runtime.go` — new exported method
  `Runtime.PersonaExtractLogPath()` so the CLI gets the same path
  `openPersonaExtractLog` uses without duplicating the layout
  constant.
- `internal/cli/cli.go` — `runPersonaCommand` now dispatches to
  `candidates` (existing) or `errors` (new). `runPersonaErrorsCommand`
  + `parsePersonaErrorsFlags` + `parsePersonaLogLine` +
  `renderPersonaExtractErrors` implement the read path. Help text
  updated to list both subgroups.
- `internal/cli/persona_errors_test.go` — new file, 11 tests.
- `internal/cli/persona_command_test.go` —
  `TestRunPersonaWithoutSubcommandShowsUsage` updated to expect the
  reworked usage line (asserts both `candidates` and `errors`
  surfaces are mentioned).

## CLI surface

```
lore persona errors [flags]

  --workdir <path>          workdir containing vault/ and state/ (defaults to cwd)
  --tail N                  show only the last N matching lines (default 20, 0 = no limit)
  --stage <name>            filter by stage: extract, store, parse_warning (empty = all)
  --since <duration>        only show lines newer than this Go duration string (e.g. 1h, 30m)
```

Output shape:

```
Persona Extraction Errors
=========================
Source: C:\...\state\logs\persona-extract.log
Filter: tail=20, stage=parse_warning

2026-05-22T10:00:00Z	stage=parse_warning	session=lore-cli-xxx	error="..."
...

3 of 7 entries shown (filtered from 12 total).
```

Lines are printed verbatim from the file (no re-formatting) so what
the operator sees in `lore persona errors` is byte-identical to
what they would see in `Get-Content -Tail`. Malformed lines (future
schema, partial writes from a crashed flush) are still shown when
no filter is applied; stage / since filters skip them because parse
failed. Dropping them silently would hide real signal.

## What was NOT touched

- No new state in the persona store; this is read-only over the
  log file.
- No change to the writer side (B-P9 format unchanged).
- No new dependencies. `os.ReadFile` reads the whole file because
  the file is bounded by failure count, not turn count -- a busy
  workdir running for months stays under ~10 MB even in the worst
  case (already covered in the P9 handoff).
- No MCP, TUI, or CI changes.
- No persona / app / store interface changes beyond
  `Runtime.PersonaExtractLogPath()` which is a thin getter.

## Why a Runtime method rather than a free function

`PersonaExtractLogPath` lives on `*Runtime` because the path is a
function of `cfg.Paths.StateDir`, and `Runtime` is the canonical
holder of resolved config in the CLI's plumbing. Exposing a free
`app.PersonaExtractLogPath(stateDir)` would force the CLI to
import `config` and re-resolve workdir → StateDir, duplicating
the work `OpenRuntime` already did. The Runtime method also keeps
the magic `"logs/persona-extract.log"` constant private to
`internal/app` so a future relocation only needs to update that
file.

## Why not extend `lore usage` instead

`lore usage` answers "what cost token". `lore persona errors`
answers "what failed to produce a token-billed extraction".
Merging them would force the operator to disambiguate two
unrelated questions in one report and would require synthesizing
zero-token UsageRecord rows for failures -- an option B-P9
explicitly rejected for keeping the UsageRecord schema clean.

## Review focus

- Confirm the filter-then-tail order is correct: filter first
  (stage + since), then keep last N. The other order
  (tail-then-filter) would surface zero rows when the last N are
  all-extract and the operator filtered for parse_warning.
  Verified by `TestRenderPersonaExtractErrorsStageFilter` (filter
  drops 2 of 3, tail leaves all 1).
- Confirm `--since` uses `time.Now().UTC()` cutoff and the log
  timestamps are parsed as RFC3339Nano. Mixed-locale workdirs
  should never see a one-off due to timezone drift.
  `renderPersonaExtractErrors` accepts `now time.Time` as a
  parameter so tests inject a deterministic clock; production
  callers pass `time.Now().UTC()`.
- Confirm the missing-log-file branch returns exit 0 with a
  friendly message rather than exit 1 with an OS error. A workdir
  that has never run extraction (or that was just bootstrapped)
  is a normal state, not a CLI failure.

## Validation

```powershell
go test ./internal/cli -count=1 -run "TestRenderPersonaExtract|TestRunPersonaErrors|TestRunPersonaWithoutSubcommand" -v
# 11 tests PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./cmd/... -count=1
# all green
```

Load-bearing verified by temporarily routing the `errors`
subcommand to the default unknown-subcommand error and confirming
`TestRunPersonaErrorsEndToEnd` fails on
`stderr = "persona: unknown subcommand \"errors\"\n"`, then
restoring.

## Known limitations / future follow-up

- No `--follow` / `--tail -f` streaming mode. Operators tail in
  real-time today via `Get-Content -Wait`; matching that would
  require either polling the file or using the OS-specific watch
  syscall. Out of scope.
- Output is single-stream stdout; no JSON / NDJSON output mode
  for piping into downstream analytics. Add `--format json` if a
  consumer ever materializes.
- `--since` accepts a duration only, not an absolute timestamp.
  `--after 2026-05-22T10:00:00Z` is a natural extension once a
  reviewer asks for it.
- Lines are read entirely into memory before filtering. For a
  realistic workdir this is bounded (one line per failure, low
  thousands per year), but a stream-based reader would be a
  cheap upgrade if a workdir ever crosses 100k lines.
