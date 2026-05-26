# Handoff: B-line Task #44 — model-tag log column tests

Date: 2026-05-26
Audience: DeepSeek reviewer.
Status: tests landed and green; no production code changed.

## Why this slice exists

Operator commit `04bb31c feat(tui): /model interactive panel for
model discovery, test, and hot-switch` introduced workspace LLM
profiles plus the persona-extract log model-tag columns (`model=`,
`base_url="..."`), but shipped without round-trip / backward-compat
coverage on the new columns and without integration coverage proving
the workspace profile reaches the persona-extract and process-sink
subsystems.

Task #44 closes that gap. The point is to pin the contract so a
future refactor cannot silently break observability or cost
attribution.

## What was added

Four test files touched, eleven new tests, zero production-code
changes (the wiring under test is what operator commit 04bb31c
already shipped).

### `internal/cli/persona_errors_test.go` (+4 tests)

- `TestParsePersonaLogLineRoundTripsModelTagColumns` —
  Constructs a log line in the writer format documented at
  `internal/console/session.go` `logPersonaExtractError` (see lines
  779-800) and asserts `parsePersonaLogLine` recovers all six
  fields (timestamp, stage, session, error, model, base_url).
  Writer's `%q` quoting on error/base_url is round-tripped through
  `unquoteSafe`; model stays plain.
- `TestParsePersonaLogLineParsesLegacyFourColumnFormat` —
  Backward-compat: a pre-B-line 4-column line still parses with
  `parsed=true`, `model=""`, `baseURL=""`.
- `TestParsePersonaLogLineHandlesModelOnlyTail` —
  Asymmetric column: `model=` present, `base_url=` absent (resolved
  profile populated `PersonaExtractModelInfo.Model` but not BaseURL).
- `TestEmitPersonaExtractErrorsJSONIncludesModelAndBaseURL` —
  Downstream JSON surface: `lore persona errors --json` exposes the
  new columns via `model` and `base_url` keys with `omitempty`
  semantics so legacy entries don't emit `"model":""`.

### `internal/console/session_persona_test.go` (+2 tests)

- `TestSessionHandleExtractionFailureStampsModelTagColumns` —
  With `Session.PersonaExtractModelInfo` populated, a failed
  extraction lands a log line carrying both `\tmodel=<name>` and
  `\tbase_url="<url>"` columns AFTER the legacy four columns. Pins
  column ordering so awk-style readers that ignore fields[4:] keep
  working.
- `TestSessionHandleExtractionFailureOmitsModelTagWhenInfoEmpty` —
  Zero `PersonaExtractModelInfo` keeps the legacy four-column shape
  (no `\tmodel=` or `\tbase_url=` suffix). Guards against a future
  change that always emits the columns regardless of whether the
  identity is known.

### `internal/app/runtime_llm_config_test.go` (+5 tests)

- `TestOpenRuntimePersonaExtractIdentityReflectsWorkspaceProfile` —
  With a workspace profile `active_profile: deepseek` plus generic
  `LORE_LLM_*` env set, `Runtime.PersonaExtractProvider` /
  `PersonaExtractModel` / `PersonaExtractBaseURL` reflect the
  workspace profile (`deepseek` / `deepseek-chat` / server URL) and
  NOT the env defaults. Also asserts no `secret`-like substring
  leaks into those fields — the persona log lives in workdir.
- `TestOpenRuntimePersonaExtractIdentityHotSwitchesAcrossReopen` —
  Opens the same workDir twice with two different active profiles
  (`deepseek-chat` → `kimi-k2`) and asserts the identity fields
  change accordingly. This is the closest the current code gets to
  a "hot switch" for the persona-extract subsystem; see "Known
  gaps" below for the in-process variant operator commit 04bb31c
  deliberately left unwired.
- `TestOpenRuntimeProcessSinkSummarizerUsesWorkspaceProfile` —
  Drives `runtime.ProcessSinkSummarizer.SummarizeCheckpoint` end-
  to-end through an httptest server and asserts the request hits
  `/chat/completions` with the workspace `model: deepseek-chat` and
  `Authorization: Bearer deepseek-secret`. Confirms the workspace
  profile reaches the process-sink subsystem; env defaults do not
  leak into the request body.
- `TestDefaultProcessSinkSummarizerEmbedsResolvedProviderAndModel` —
  Focused unit complement: given a `config.ResolvedLLMConfig`,
  `defaultProcessSinkSummarizer` produces a
  `*modelProcessSinkSummarizer` with matching Provider/Model.
- `TestPersonaExtractIdentityRespectsResolveErrorAndEnabledGate` —
  Unit test for the `personaExtractIdentity` helper. A non-nil
  resolveErr or `Enabled=false` must yield `""` so the writer omits
  the optional columns instead of stamping a stale identity onto
  every failure line.

Plus one shared helper, `writeWorkspaceLLMProfile`, that
materialises a workspace `.lore/config.json` with one active
profile. Used by the three OpenRuntime-based tests to keep the
fixture body short.

## What this contract pins

Two surfaces and the bridge between them:

1. **Writer** (`internal/console/session.go::logPersonaExtractError`):
   format is `<RFC3339Nano>\tstage=<s>\tsession=<id>\terror=<%q>[\tmodel=<plain>\tbase_url=<%q>]`.
   Tests in `console/session_persona_test.go` lock this verbatim,
   including the "omit tail columns when ModelInfo is empty"
   semantics.
2. **Reader** (`internal/cli/persona_errors.go::parsePersonaLogLine`):
   handles 4-column legacy lines and 5/6-column tagged lines
   uniformly. JSON emit uses `omitempty` so legacy entries keep
   their original shape on the wire.
3. **Plumbing** (`internal/app/runtime.go::OpenRuntime`): workspace
   profile flows into `Runtime.PersonaExtractProvider` /
   `PersonaExtractModel` / `PersonaExtractBaseURL`, and also into
   the `*modelProcessSinkSummarizer{provider, model}` fields that
   ultimately tag every `UsageRecord` from `Purpose=process_sink`.

If any of these three drift, one of the eleven new tests fails.

## Known gaps (deliberate, not bugs)

- **In-process hot-switch via `/model use` does NOT propagate to
  persona-extract or process-sink subsystems.** Operator commit
  04bb31c wires `/model use` to replace `session.Agent` directly
  but leaves `Runtime.PersonaExtractor` and
  `Runtime.ProcessSinkSummarizer` (plus the identity snapshots) on
  the values OpenRuntime computed. The hot-switch tested by this
  slice is the "reopen runtime with different active_profile" form
  — operators who switch via the TUI panel will still see the
  original profile on persona-extract.log and in process-sink
  `UsageRecord` until next runtime open. Closing this gap is a
  future B-line slice (would need a `Runtime.RebindLLM(purpose,
  cfg)` seam plus TUI wiring); not in scope for task #44.
- **No real-LLM integration test added.** The B-line acceptance
  gate covered the live path during the original ship; this slice
  is hermetic so CI stays fast and deterministic.

## How to verify

```sh
go vet ./internal/cli/... ./internal/app/... ./internal/console/...
go test ./internal/cli/ -run "PersonaLogLine|PersonaExtractErrorsJSON" -v
go test ./internal/console/ -run "ModelTag|FourColumn|StampsModel|OmitsModelTag" -v
go test ./internal/app/ -run "PersonaExtractIdentity|HotSwitches|WorkspaceProfile|EmbedsResolved|RespectsResolve" -v
go test ./internal/cli/ ./internal/app/ ./internal/console/
```

All eleven new tests pass. Whole-package suites stay green
(`cli` 6.7s, `app` 9.3s, `console` 1.1s on the development host).

## Review focus suggestions for DeepSeek

1. Is the round-trip test sufficient as a contract pin, or should
   the format constants be lifted into a shared package so writer
   and reader assert against the same source-of-truth string?
2. The `writeWorkspaceLLMProfile` helper duplicates the existing
   JSON literal in `TestOpenRuntimeUsesWorkspaceLLMProfileOverGenericEnv`.
   Worth refactoring the older test onto the new helper, or leave
   the old one intact to avoid touching working code?
3. `TestOpenRuntimePersonaExtractIdentityHotSwitchesAcrossReopen`
   reopens the same workdir twice — verify the sqlite store and
   persona-extract.log file handles release cleanly on Windows
   (relevant per CLAUDE.md note about LocalSystem/LocalService).
   `TestRuntimeCloseReleasesPersonaExtractLogFile` already
   exercises the file-handle close; the new test is a slightly
   different stressor.
4. The "known gaps" section explicitly calls out the in-process
   hot-switch limitation. Confirm this is the right level of
   documentation for now, or whether a `// TODO(b-line)` marker in
   `runtime.go` would be more discoverable.
