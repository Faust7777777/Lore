# Review Handoff: B-P11a Usage Purpose Breakdown

Date: 2026-05-22

Owner line: B-line (persona memory candidate pipeline, business / data)

Commit: `4ad71d9 feat(model,store,cli): lore usage shows per-purpose breakdown`

## Scope

Closes the success-path visibility gap that B-P9 explicitly deferred:
B-P9 surfaced extraction failures via the workdir log file, but
successful `persona_extract` calls were lumped into the top-line
`lore usage` Calls / Tokens with no way to disaggregate. Operators
running real-world chat sessions could not see what fraction of
their LLM spend went to chat vs persona_extract vs process_sink.

## Changed files (7)

- `internal/model/runtime.go` — `UsageSummary` gains
  `PurposeBreakdown map[string]UsagePurposeStats` (json omitempty).
  New `UsagePurposeStats` struct with `Calls / PromptTokens /
  CompletionTokens` + `TotalTokens()` accessor.
- `internal/store/sqlitestore/store.go` — `SummarizeUsage` switches
  from SQL aggregate to per-row scan + Go-side group-by. Decodes
  `payload` for each row to extract Purpose; unmarshal failures
  fall into the empty-purpose bucket so a corrupt row never breaks
  the report.
- `internal/store/memory/store.go` — `SummarizeUsage` mirrors the
  same group-by pattern in plain Go.
- `internal/store/jsonstore/store.go` — same pattern.
- `internal/store/usage_purpose_breakdown_test.go` — new file,
  3-backend table covering breakdown fill, nil-on-empty-day,
  empty-purpose-bucket-collapses. Asserts the sum-of-buckets ==
  top-line totals reconciliation contract on every backend.
- `internal/cli/cli.go` — `renderUsageReport` aggregates breakdowns
  across the queried window and prints a "By purpose:" block under
  the daily table. Alphabetical Purpose order so output is
  deterministic.
- `internal/cli/usage_render_test.go` — new file, 4 pure-render
  tests covering breakdown display, alphabetical ordering, hidden
  block when breakdown is nil, "unspecified" label for empty
  Purpose, multi-day window aggregation.

## Design note: per-row scan vs json_extract

sqlite's `SummarizeUsage` was previously a single SQL aggregate
(`COUNT(*), SUM(prompt_tokens), SUM(completion_tokens)`). B-P11a
switches to `SELECT prompt_tokens, completion_tokens, payload` +
Go-side decode + Go-side group-by. The alternative was a SQL-level
`json_extract(payload, '$.purpose')` aggregate, which would be
cleaner but introduces a dependency on SQLite's JSON1 extension
not previously used in this repo.

The day's record count stays small (one row per LLM call, low
hundreds even on a busy day), so the extra payload-decode cost is
negligible compared to introducing a new SQLite feature dependency.
If the daily record count ever crosses ~10k, json_extract becomes
the right migration; the new test surface (3-backend table) would
catch any drift between the SQL aggregate and the Go aggregate at
that point.

## What was NOT touched

- UsageRecord schema unchanged (the existing `Purpose` field was
  already added when the constant was first introduced; B-P11a
  only adds the per-day aggregate shape).
- Existing `lore usage` top-line `DAY / CALLS / PROMPT / COMPLETION
  / TOTAL` table format unchanged. The "By purpose:" block is
  appended; older test substring asserts remain green.
- No MCP, TUI, or CI changes.
- No persona / app / store interface changes beyond the
  `UsageSummary` struct addition.

## Empty Purpose handling

Records with `Purpose == ""` (pre-Purpose-field legacy data, or any
future caller that forgets to set it) collapse into the `""` bucket
and render in the CLI under the label `unspecified` rather than a
blank cell. Backends MUST NOT drop them silently or roll them into
`chat` — the empty bucket is the honest answer. The 3-backend test
`TestSummarizeUsageBreakdownEmptyPurposeCollapses` enforces this.

## Review focus

- Confirm the sum-of-buckets reconciliation contract is correct:
  `sum(bucket.Calls) == summary.Calls`, same for tokens. If any
  backend's group-by drifts from the top-line aggregate, the
  reconciliation assertion in
  `TestSummarizeUsageFillsPurposeBreakdown` catches it on every
  backend.
- Confirm the sqlite payload-decode-failure path is acceptable:
  corrupt row top-line increments from the SQL columns but
  Purpose stays `""`. This is intentionally lossy on Purpose but
  preserves total spend visibility. An alternative would be to
  return an error from `SummarizeUsage`; the choice here is
  graceful degradation.
- Confirm the alphabetical sort in `renderUsageReport` is stable
  enough for downstream consumers. `map` iteration is unspecified
  in Go so the sort is load-bearing for any test or pipeline that
  compares diff'd output across runs.
- Confirm `PurposeBreakdown` left nil (not empty map) on a no-data
  day so JSON encodings omit the field via omitempty.

## Validation

```powershell
go test ./internal/store -count=1 -run "TestSummarizeUsage" -v
# 3-backend table: 9 subtests PASS

go test ./internal/cli -count=1 -run "TestRenderUsageReport" -v
# 4 tests PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./cmd/... -count=1
# all green
```

Load-bearing verified by temporarily removing the
`PurposeBreakdown` assignment in sqlite `SummarizeUsage` and
confirming the sqlite subtest of
`TestSummarizeUsageFillsPurposeBreakdown` fails on "PurposeBreakdown
buckets = 0, want 3", then restoring.

## Known limitations / future follow-up

- The breakdown aggregates across the entire `--days N` window,
  not per-day. A `--by-day-and-purpose` matrix would clutter the
  table without obvious value for a single user; if a team
  workspace lands and per-day attribution matters, this is the
  natural extension.
- `lore usage` still does not surface
  `persona_extract_error` records because B-P9 chose the file-log
  path instead of writing UsageRecord rows for failures. The two
  observability surfaces are intentionally separate: `lore usage`
  shows what cost token, the persona-extract.log shows what
  failed. Merging them would require giving every failure a
  synthetic UsageRecord with TokensIn/Out=0 + ErrorReason field,
  which was the design discussion option A rejected during B-P9.
- No drill-down per Purpose × Session. Today the breakdown is one
  bucket per Purpose; a future `lore usage --purpose persona_extract
  --by-session` could surface which sessions burned the most
  extraction tokens.
