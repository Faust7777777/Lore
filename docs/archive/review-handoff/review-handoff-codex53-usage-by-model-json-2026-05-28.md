# Review Handoff: Usage By-Model Breakdown + JSON Surface (2026-05-28)

## Scope

Concurrent usage/cost-observability slice found in the dirty worktree while closing the A-line TUI profile panel. This handoff documents the usage changes so review has a stable entry point; it does not claim ownership of TUI persona candidate behavior.

FINAL review finding reference: N/A. This is a B/observability continuation of the usage purpose breakdown work, not one of the 2026-05-25 FINAL security/runtime findings.

## Intent

Extend `lore usage` from purpose-level cost visibility to purpose + provider/model visibility.

The operator need is concrete: after `/model` hot-switch and persona extraction split, a single purpose such as `chat` can consume multiple providers/models in the same window. The top-line usage report should remain compact, but scripted consumers need the full uncapped breakdown.

## Modified Files

- `internal/model/runtime.go`
  - extends `UsagePurposeStats` with `ByModel`.
  - documents empty purpose fallback to `chat` and missing provider/model fallback to `unknown`.

- `internal/store/usage_aggregate.go`
  - centralizes `UsagePurposeBucket`, `UsageModelBucket`, and `AccumulateUsageBreakdown`.
  - keeps memory/json/sqlite stores on the same bucketing rules.

- `internal/store/memory/store.go`
- `internal/store/jsonstore/store.go`
- `internal/store/sqlitestore/store.go`
  - populate purpose and by-model breakdowns during `SummarizeUsage`.

- `internal/store/usage_purpose_breakdown_test.go`
  - covers backend parity across memory/json/sqlite.
  - covers legacy empty purpose folding into `chat`.
  - covers provider/model fallback and nested totals.

- `internal/cli/cli.go`
  - adds `lore usage --json`.
  - renders top-five by-model rows under each purpose for human output.
  - emits uncapped `purpose_breakdown.<purpose>.by_model` in JSON output.

- `internal/cli/usage_render_test.go`
  - covers human by-model rendering, top-five cap, and JSON shape.

## Load-bearing Tests

Commands run:

```powershell
go test ./internal/cli ./cmd/obsidian-harness -count=1
go test ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1
.\scripts\release-gate.ps1 -SkipDiffCheck
.\scripts\verify.ps1
git diff --check
```

What matters:

- human `lore usage` stays bounded by showing only the top five models under each purpose.
- JSON output is one line and keeps the full uncapped `by_model` map for downstream dashboards.
- all three stores share the same fallback behavior via `store.AccumulateUsageBreakdown`.
- full repo verification stays green with this usage surface present.

## Boundaries Not Touched

- No MCP tool surface changes.
- No new billing writes or usage record creation paths.
- No TUI Cost/Usage panel implementation.
- No API key or credential exposure in usage output.
- No persona candidate lifecycle/state-machine change.

## Reviewer Focus

- Confirm empty purpose folding to `chat` is the desired legacy behavior.
- Confirm `unknown`, `provider/unknown`, and `unknown/model` bucket names are acceptable for dashboards.
- Confirm human output top-five cap plus JSON uncapped map is the right split.
- Confirm `lore usage --json` shape is acceptable before other consumers depend on it.
