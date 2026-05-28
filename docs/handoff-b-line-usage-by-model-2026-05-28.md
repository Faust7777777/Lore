# Handoff: B-line usage totals broken down by model

Date: 2026-05-28
Audience: next reviewer (DeepSeek) + TUI line building the cost/status panel.

Status: model / store / cli shipped (commit `8bf9a22 feat(model,store,cli):
break usage totals down by model`). Docs in this file. One release-gate
subitem is staged here as text for the CI-file owner to add (see below) —
not applied directly because `scripts/release-gate.ps1` is currently dirty
with A-line / TUI WIP and CI is outside the B-line boundary.

Follow-ups (2026-05-29):
- `01fbf00 feat(cli): show hidden-model token total in usage report tail`
  — the human report's truncation line now also discloses the summed
  tokens of the dropped models (see "Human output" below).
- `752eada test(cli): guard cross-day by_model usage aggregation` — adds
  a two-day test that locks the by_model cross-window merge (one model
  billed on both days must sum; a day-unique model must still appear)
  that the human report and TUI dashboard both rely on. No prod change.
- `b491d45 test(cli): guard usage idle-window output on both surfaces` —
  locks the no-usage path (fires on `totalCalls == 0`, since the command
  always passes one zero summary per window day): the human report says
  "No usage recorded." with no table/Total rows, and `--json` emits a
  valid zero-state object with `purpose_breakdown` omitted (not null /
  `{}`) so an idle-day poll never breaks a consumer. No prod change.
- `7c30189 feat(cli): show each purpose's share of total tokens in usage
  report` — each `By purpose` row now ends with its token-weighted share
  (e.g. `(75%)`), rounded to nearest and divide-by-zero-safe. Human-only;
  the JSON shape is unchanged.
- `5f5d017 test(app): assert process-sink usage stamps Purpose` —
  data-quality foundation: the report's buckets are only correct if
  billers stamp `Purpose`. The process-sink usage test checked
  Provider/Model but not `Purpose`, unlike the persona extractor test;
  now all three billers (chat / persona / process-sink) are symmetrically
  guarded so a dropped label can't silently misbucket spend as `chat`.

Model / store contract unchanged. JSON shape unchanged (the `7c30189`
share is human-report-only).

## TL;DR

`lore usage` and the TUI cost dashboard can now answer "chat /
persona_extract / process_sink each spent on which models". The
per-purpose breakdown gained a nested `by_model` map keyed
`provider/model`.

| Layer | File | What changed |
|---|---|---|
| model | `internal/model/runtime.go` | `UsagePurposeStats.ByModel map[string]UsagePurposeStats` (`json:"by_model,omitempty"`). Existing Calls/PromptTokens/CompletionTokens unchanged. |
| store (shared) | `internal/store/usage_aggregate.go` (new) | `UsagePurposeBucket`, `UsageModelBucket`, `AccumulateUsageBreakdown` — one place for the purpose/model bucketing + fallbacks. |
| store (backends) | `memory` / `jsonstore` / `sqlitestore` `store.go` | SummarizeUsage now calls the shared helper; sqlite also pulls Provider/Model from the decoded payload. |
| cli | `internal/cli/cli.go` | `lore usage --json` (new flag) emits `purpose_breakdown.<p>.by_model`; human report adds indented per-model detail capped at top 5. |

## Contract details

### Bucketing + fallbacks (in `internal/store/usage_aggregate.go`)

- **Purpose key**: empty/absent Purpose folds into `chat`. Rationale:
  before the Purpose field existed the operator agent was the only
  biller, so an unlabeled call is a chat call. **This is a deliberate,
  non-additive change** to `purpose_breakdown`: the old empty-string
  (`""`) bucket no longer appears; legacy rows merge into `chat`. The
  store contract test was rewritten accordingly
  (`TestSummarizeUsageBreakdownEmptyPurposeFoldsIntoChat`, replacing the
  old `...EmptyPurposeCollapses`). This was an explicit product call
  (see commit message), not an accident.
- **Model key**: `provider/model` (e.g. `deepseek/deepseek-v4-pro`).
  Both parts empty → `unknown`; a single empty part is filled with
  `unknown` so the slash form stays parseable (`unknown/orphan-model`,
  `deepseek/unknown`).
- **Reconciliation**: for any purpose bucket, the sum of its `by_model`
  leaves equals the purpose-level Calls/Prompt/Completion, which in turn
  sum to the top-line `UsageSummary` totals. Tests assert this per
  backend.

### `lore usage --json` shape (new)

```json
{
  "window_days": 7,
  "totals": {"calls": N, "prompt_tokens": N, "completion_tokens": N, "total_tokens": N},
  "purpose_breakdown": {
    "chat": {
      "calls": N, "prompt_tokens": N, "completion_tokens": N, "total_tokens": N,
      "by_model": {
        "deepseek/deepseek-v4-pro": {"calls": N, "prompt_tokens": N, "completion_tokens": N, "total_tokens": N}
      }
    }
  },
  "days": [
    {"day": "2026-05-21", "calls": N, "prompt_tokens": N, "completion_tokens": N, "total_tokens": N}
  ]
}
```

`purpose_breakdown` is aggregated across the window (mirrors the human
"By purpose" block) and carries the **uncapped** `by_model` map. The
`days` array preserves per-day top-line for time-series rendering.

### Human output

Default `lore usage` adds indented per-model lines under each "By
purpose" row, sorted by total tokens descending, capped at the top 5
with a `... N more model(s), T tokens` line beyond that — `T` is the
summed `TotalTokens()` of the hidden tail (commit `01fbf00`), so a
truncated report still discloses how much spend it folds away rather
than just how many models. Each `By purpose` row also ends with its
token-weighted share of the window (e.g. `= 750 tokens (75%)`, commit
`7c30189`) so cost concentration reads at a glance; the percent rounds
to nearest and is divide-by-zero-safe when the window total is zero (a
zero-token call still counts toward Calls). The DAY table and Total
line are unchanged so an operator who only wants totals still reads
them at a glance. The `--json` surface is untouched: it already emits
the uncapped `by_model` map and carries no percent field (consumers
compute their own), so scripting consumers never saw the cap, the tail
summary, or the share.

## TUI cost-dashboard wiring

The TUI does not need the CLI. Call `runtime.SummarizeUsage(day)` per
day, then merge `summary.PurposeBreakdown[purpose].ByModel` across the
window (same shape the CLI's `aggregateUsageBreakdown` builds). Each
leaf is a `model.UsagePurposeStats` with `Calls / PromptTokens /
CompletionTokens` and a `TotalTokens()` accessor. Combine with
`runtime.LLMIdentity(purpose)` (from the prior slice) to show "this
purpose is configured for model X, and over the last N days it actually
billed models X (n calls) + Y (m calls)" — the latter catches drift
when `/model use` hot-switched mid-window.

## Release-gate subitem (for the CI-file owner to add)

`scripts/release-gate.ps1` is dirty with A-line/TUI WIP and CI is
outside the B-line boundary, so this block is provided as text rather
than applied. Add alongside the other `Invoke-GoGate` blocks:

```powershell
    Invoke-GoGate `
        -Label "usage by-model aggregation guardrails" `
        -Package "./internal/store" `
        -Run "Test(SummarizeUsageByModel(SplitsSamePurposeAcrossModels|DoesNotMixAcrossPurposes|FallsBackToUnknown)|SummarizeUsageBreakdownEmptyPurposeFoldsIntoChat)$"

    Invoke-GoGate `
        -Label "lore usage by-model CLI guardrails" `
        -Package "./internal/cli" `
        -Run "Test(EmitUsageJSONIncludesByModelBreakdown|RenderUsageReportShowsModelDetailUnderPurpose|RenderUsageReportCapsModelDetailAtTopFive)$"
```

## Verification

```sh
go build ./...
go test ./internal/model/ ./internal/store/... ./internal/cli/ -count=1
go test ./internal/store/ -run "SummarizeUsage" -v
go test ./internal/cli/ -run "Usage" -v
```

All green at slice close. New tests: 4 store contract tests (across
memory/json/sqlite via the existing `usageBackends` table) + 8 cli
tests (human model detail, top-5 cap incl. hidden-tail token sum, JSON
by_model, cross-day by_model aggregation, two idle-window guards [human
"No usage recorded." + `--json` zero-state], and two purpose token-share
guards [normal percent + zero-token divide guard]).

## Boundaries honored

- No TUI panel code (TUI line owns it; this is backend + CLI).
- No persona candidate state machine change.
- No LLM profile write change.
- No broad release-gate change; only the additive usage subitem above,
  left for the CI-file owner.

## Follow-ups

- Apply the release-gate subitem once `scripts/release-gate.ps1` is
  back to a clean base (or have the CI owner fold it into their next
  commit).
- If `by_model` cardinality ever grows unbounded (many models per
  workday), consider a store-side cap; today the maps stay small (a few
  models per purpose per day) so no cap is needed yet. The human report
  already bounds its own output (top 5 + a tail-token summary line), so
  this would only be about store/JSON memory, not display noise.
