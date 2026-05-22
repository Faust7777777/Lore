# Review Handoff: B-P9 Persona Extraction Observability

Date: 2026-05-22

Owner line: B-line (persona memory candidate pipeline, business / data)

Commit: `4f3aaac feat(app,console,cli): observe persona extraction failures via workdir log`

## Scope

Closes the silent-swallow gap the P4 fire-and-forget goroutine left
at `internal/console/session.go:670`: extractor / parser / store
failures returned without leaving any trace, so operators running
real-world chat sessions could not diagnose "why are there zero
candidates after this conversation".

The fix is operator-pull observability (tail a workdir log) rather
than CLI-render observability (extend `lore usage`), matching the
0.5-day scope budgeted for B-P9.

## Changed files (5)

- `internal/console/session.go` — new `PersonaExtractLogger
  io.Writer` field on Session; goroutine writes one line per
  failure with helper `logPersonaExtractError(stage, sessionID,
  err)`.
- `internal/console/session_persona_test.go` — 5 new tests covering
  extractor failure logging, store failure logging, success path
  staying silent, nil-logger path safe, and timeout (slow LLM) path
  logging the deadline signal.
- `internal/app/runtime.go` — new `Runtime.PersonaExtractLogger
  io.Writer` and `Runtime.PersonaExtractTimeout time.Duration`
  fields. `OpenRuntime` opens
  `<StateDir>/logs/persona-extract.log` and reads
  `LORE_LLM_PERSONA_EXTRACT_TIMEOUT` env. `Runtime.Close` releases
  the file handle.
- `internal/app/runtime_persona_observability_test.go` — new file,
  6 tests covering env parse success / absent / invalid / negative
  paths, log file creation + write round-trip to disk, Close
  releasing the OS handle so a second OpenRuntime in the same
  workdir succeeds on Windows.
- `internal/cli/cli.go` — `RunConsoleCommand` and `RunTUICommand`
  wire `Session.PersonaExtractLogger` and `Session.PersonaExtractTimeout`
  from the runtime alongside the existing PersonaExtractor.

## Log line format

```
<RFC3339Nano UTC>\tstage=<extract|store>\tsession=<id>\terror=<quoted>\n
```

- `stage=extract` covers extractor.Extract failures (LLM error,
  context deadline, parser refusal — the extractor wraps all three
  in its returned err so the message is self-describing).
- `stage=store` covers `runtime.RecordPersonaCandidate` failures
  (sqlite write error, dedup race surfacing as conflict, etc.).
- Tab-delimited so a plain awk/grep/cut pipeline can parse it
  without a structured log dependency.

## Operator usage

```powershell
Get-Content "$workdir\state\logs\persona-extract.log" -Tail 20
```

Or to bump the per-call timeout while a slow provider is in use:

```powershell
$env:LORE_LLM_PERSONA_EXTRACT_TIMEOUT = "30s"
lore console $workdir
```

Unset / invalid / negative env values silently fall back to the
existing 8-second default — by design, a malformed env should never
break the chat.

## What was NOT touched

- No `internal/store/*` interface changes.
- No `internal/model.UsageRecord` schema changes — earlier draft
  proposal A in design discussion (write Purpose=persona_extract_error
  records) was rejected because surfacing it through `lore usage`
  would have required extending UsageSummary semantics and changing
  three backends; the file-log path is lighter and matches the
  scope budget. UsageRecord changes are deferred to B-P11a (which
  did the additive PurposeBreakdown work later, on a different
  scope).
- No MCP, TUI, or CI changes.

## Review focus

- Confirm the log path placement (`<StateDir>/logs/persona-extract.log`)
  is consistent with where the rest of the workdir state files
  live. The current `Config.Paths.StateDir` is the right anchor;
  putting the log under vault/ would be wrong (it is operational,
  not knowledge content).
- Confirm "file open failure is non-fatal" is acceptable: if the
  log file cannot be opened (permissions, disk full), extraction
  still runs silently rather than refusing to boot. The intentional
  trade-off is observability degrades gracefully; the test
  `TestRuntimeCloseReleasesPersonaExtractLogFile` partially covers
  this by exercising re-open after close, but not the
  permission-denied path.
- Confirm the nil-logger safe path: `logPersonaExtractError`
  no-ops when `s.PersonaExtractLogger` is nil so unit tests and
  embedders without observability stay dependency-free. Verified
  by `TestSessionHandleNilLoggerSurvivesFailure`.
- Confirm the LORE_LLM_PERSONA_EXTRACT_TIMEOUT env contract is
  defensive: invalid / negative / unset all coerce to zero so the
  Session falls through to defaultPersonaExtractTimeout (8s).

## Validation

```powershell
go test ./internal/console -count=1 -run "TestSessionHandle.*Logger|TestSessionHandleExtractionTimeout" -v
# 5 tests PASS

go test ./internal/app -count=1 -run "TestOpenRuntimePersonaExtractTimeout|TestOpenRuntimeOpensPersonaExtractLogFile|TestRuntimeCloseReleasesPersonaExtractLogFile" -v
# 6 tests PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... -count=1
# all green
```

Load-bearing verified by temporarily removing the
`logPersonaExtractError("store", ...)` call in session.go and
confirming `TestSessionHandleStoreFailureWritesLoggerLine` fails on
`stage=store` missing, then restoring.

## Known limitations / future follow-up

- No rotation. Every failure adds one line; busy workdirs running
  for months could accumulate. Practical worst case (one failure
  per chat turn for a permanently broken provider, 200 turns/day,
  365 days) is ~73k lines / ~10 MB — not a concern for a single
  user. A `--rotate-after Nmb` flag is the natural addition if it
  ever becomes a problem.
- No `lore persona errors` reader command. Operators use
  `Get-Content -Tail N` or `awk` directly. A native CLI command
  could ship as B-P11c if the file path becomes ergonomically
  unfriendly for the PM persona.
- ~~Parser warnings (`internal/persona/model.go:100`) are still
  inside the parser's returned result and not propagated to the
  log.~~ Addressed in the round-2 fix below.

## Round 2 fix (reviewer-flagged blocker)

Reviewer found that the original B-P9 log only covered the
`extractor.Extract(ctx, input)` returning err path. The most common
"zero candidates after a chat" outcome is parser-side: paraphrased
evidence_quote, low confidence, empty evidence, or NEVER-extract
rule matches make the parser drop candidates and emit
`result.Warnings` while returning `(result, nil)`. The original
session.go branch saw err==nil, processed `result.Candidates`
(which was empty), and left no log line -- defeating the operator's
diagnostic goal.

Fix: when `len(result.Candidates) == 0` and `result.Warnings` is
non-empty, emit one `stage=parse_warning` log line per warning.
Suppressed on the success path (at least one candidate landed) so
the parser's incidental sibling-discard warnings do not flood the
log during normal operation -- the diagnostic value of the log file
comes from focusing on the zero-candidate case.

Tests added:

- `TestSessionHandleParserWarningsLoggedWhenZeroCandidates`: scripted
  extractor returns empty Candidates + 2 Warnings, asserts both
  warnings land as `stage=parse_warning` lines.
- `TestSessionHandleParserWarningsSuppressedOnSuccess`: scripted
  extractor returns 1 Candidate + 1 sibling-discard Warning,
  asserts no `parse_warning` line in the log; the kept candidate
  still lands in the store.

Load-bearing verified by temporarily removing the
`parse_warning` block in `launchPersonaExtraction` and confirming
`TestSessionHandleParserWarningsLoggedWhenZeroCandidates` fails on
`stage=parse_warning count = 0, want 2`, then restoring.
