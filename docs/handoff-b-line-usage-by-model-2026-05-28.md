# Handoff: B-line usage totals broken down by model

Date: 2026-05-28
Audience: next reviewer (DeepSeek) + TUI line building the cost/status panel.

Status: model / store / cli shipped (commit `8bf9a22 feat(model,store,cli):
break usage totals down by model`). Docs in this file. One release-gate
subitem is staged here as text for the CI-file owner to add (see below) —
not applied directly because `scripts/release-gate.ps1` is currently dirty
with A-line / TUI WIP and CI is outside the B-line boundary.

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
with a `... N more model(s)` line beyond that. The DAY table and Total
line are unchanged so an operator who only wants totals still reads
them at a glance.

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
memory/json/sqlite via the existing `usageBackends` table) + 3 cli
tests (human model detail, top-5 cap, JSON by_model).

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
  models per purpose per day) so no cap is needed yet.
