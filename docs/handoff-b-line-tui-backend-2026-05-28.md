# Handoff: B-line backend for TUI-first persona review

Date: 2026-05-28
Audience: next reviewer (DeepSeek) and the TUI line picking up the
persona-review panel.

Status: B-1 / B-2 / B-3 / B-5 shipped; B-4 partially shipped (the
runtime accessor for resolved LLM identity already exists from the
A-line commit `443b3bd feat(runtime): expose llm profile boundary
for tui`; B-line side closed only the doc-correction half).

## TL;DR

B line is now positioned as a backend layer the TUI can call
without re-implementing state-machine logic or re-reading the
persona-extract.log file. Five additive pieces:

| Piece | Where | What changed |
|---|---|---|
| Action availability helper | `internal/app/persona_actions.go` | New `PersonaCandidateActions` struct + `Runtime.PersonaCandidateActions(rec)` method covering open / drafted-orphan / drafted-linked (pending, terminal, approved) / dismissed. |
| TUI candidate DTO | `internal/app/persona_view.go` | New `PersonaCandidateView` (flat, JSON-stable shape with `Actions` embedded) + `Runtime.ListPersonaCandidateViews` / `Runtime.GetPersonaCandidateView`. |
| Persona-log reader lifted to app | `internal/app/persona_log.go` | New `PersonaExtractErrorEntry` / `PersonaErrorsFilter` / `PersonaExtractErrorsResult` + `ReadPersonaExtractErrors` / `Runtime.ListPersonaExtractErrors` / `ParsePersonaExtractLogLine`. CLI delegates. |
| Persona summary lifted to app | `internal/app/persona_summary.go` | New `PersonaSummaryView` + `Runtime.PersonaSummary` aggregating candidate counts + ExtractLog buckets. CLI delegates. |
| Fixture helpers | `internal/app/persona_candidate_test.go` | `seedDraftedPersonaCandidate` / `seedPartialOrphanPersonaCandidate` / `seedRejectedLinkedPersonaCandidate` / `seedDismissedPersonaCandidate` cover every dashboard state through the Runtime public API. |

The CLI surface (`lore persona candidates …` / `errors` / `summary`)
keeps its commands, exit codes, and JSON shape unchanged; cli now
internally consumes the app DTOs instead of duplicating parse / read
logic.

## Why this slice exists

The next persona-review surface is interactive TUI, not CLI. Today the
state machine for "what buttons should be live on this candidate"
lives in cli error-message hints, and the persona-extract log is
parsed inside cli. Both block a TUI panel from being a thin renderer
over a clean backend; this slice closes those gaps.

Specifically, the TUI now has:

1. A typed action-availability bundle (`PersonaCandidateActions`) so
   it can render `[Draft] [Dismiss] [Recover] [Retry]` button states
   without string-matching CLI error messages.
2. A list/get DTO (`PersonaCandidateView`) with `Actions` embedded so
   a single roundtrip returns everything a row + button column needs.
3. A reader for the persona-extract log (`PersonaExtractErrorsResult`)
   so the "why no candidates" diagnostic panel does not need to read
   files directly.
4. A dashboard view (`PersonaSummaryView`) for the headline
   buckets + extract-log freshness signal.
5. Reusable test fixtures so TUI tests can stand up any combination
   of candidate states in one call against `Runtime`.

## What is NOT in scope

- No TUI rendering. The TUI line owns the actual panel layout.
- No production seed surface. Fixture builders live in `*_test.go`
  files (same-package, no build tag) so they do not enter
  production. Cross-package consumers (TUI tests) will write
  package-local helpers that call the same Runtime methods.
- No write-side persistence of LLM profiles — `UpsertLLMProfile` /
  `SetActiveLLMProfile` already shipped on the A line at
  `443b3bd feat(runtime): expose llm profile boundary for tui`.
- No `UsagePurposeStats.ByModel` extension. The brief asked for
  "usage summary 按 purpose/model 可查"; today's `UsagePurposeStats`
  carries Calls / PromptTokens / CompletionTokens per purpose but
  not per model. Adding a `ByModel` map is its own design + store
  migration; flagged below as a follow-up.
- No CLI rewrites beyond delegation. CLI commands are unchanged in
  surface behaviour; they just now route through the new app
  methods. Migration from the legacy record-returning methods
  (`Runtime.ListPersonaCandidates` etc.) is deferred — the new
  view-returning methods live alongside them per the "parallel new"
  decision the operator picked when scoping this slice.

## Commit map

| Commit | Slice piece |
|---|---|
| `366fe43 feat(app): add PersonaCandidateActions helper` | B-2 |
| `6480965 feat(app): add PersonaCandidateView DTO for TUI list/get` | B-1 |
| `1af5083 feat(app): add persona log + summary readers for TUI dashboards` | B-3 phase A |
| `7a87ced refactor(cli): delegate persona log + summary reads to app` | B-3 phase B |
| `eb0450b docs(handoff): correct B-#44 errata about persona-extract hot-switch` | B-4 doc fix |
| `5b8762b test(app): add persona candidate fixture helpers for all states` | B-5 |

This handoff covers all six.

## Hot-switch product semantics (confirmed)

`/model use` already hot-switches the chat agent AND the persona
extractor through `app.Runtime.BuildPersonaExtractorForOperatorModel`
+ `cli/tui_workbench.go::SwitchModel`. Process-sink summarizer stays
on the workspace active profile by design — it represents the
"trusted summarization model" boundary the operator pins per
workspace, not the per-chat model the operator may swap mid-session.

The corrected B-#44 errata document captures this; the original B-#44
handoff had it backwards. See
`docs/handoff-b-line-task44-model-tag-tests-2026-05-26.md` for the
correction.

For TUI display: call `runtime.LLMIdentity(purpose)` per purpose to
show what model each subsystem is using. The returned `LLMIdentity`
is **key-free** by construction — it carries provider / model /
base_url / source / profile and the `api_key_env` / `api_key_ref`
NAMES (never the secret value), so the whole struct is safe to
render directly. BaseURL is sanitised through
`config.SanitizeLLMBaseURL` so URL-embedded credentials, if any,
do not leak.

`runtime.ResolveLLMConfig(purpose)` still exists for internal
callers that need the API key (e.g., to rebuild an LLM client),
but should never reach TUI / dashboard surfaces. The A-line commit
`443b3bd feat(runtime): expose llm profile boundary for tui` added
`ResolveLLMConfig`; the key-free `LLMIdentity` view landed
separately in the post-slice fix-up.

## Verification

```sh
go vet ./internal/cli/... ./internal/app/... ./internal/console/...
go test ./internal/app/ -run "PersonaCandidate(Actions|View)|PersonaSummary|PersonaExtractError|ParsePersona|ReadPersona" -v
go test ./internal/cli/ -run "PersonaSummary|PersonaExtractError|PersonaErrors" -v
go test ./internal/app/ ./internal/cli/ ./internal/console/
```

All packages green at slice close. Net diff for this slice: roughly
+800 added (test coverage, new types, fixture helpers), −430 removed
(deduplicated cli parsing).

## Follow-ups for the next slice

1. **TUI persona-review panel.** Backend ready; panel layout owned
   by TUI line. Brief shape: list rows from
   `ListPersonaCandidateViews(state, limit)`, detail page from
   `GetPersonaCandidateView(id)`, button availability straight from
   `view.Actions.CanX` + tooltips from `view.Actions.XReason`.
2. **`UsagePurposeStats.ByModel`.** Today the dashboard knows
   "process-sink had N calls this day" but not which models produced
   them. Adding a `ByModel map[string]UsagePurposeStats` is the
   natural shape; needs a store aggregator change and a JSON-shape
   bump on `lore usage` output.
3. **Drop the record-returning methods.** Once TUI is on
   `*View` exclusively and any internal callers migrate, the
   `ListPersonaCandidates` / `GetPersonaCandidate` (record return)
   methods can be retired. Not urgent — they exist for automation
   consumers that want raw store shape.
