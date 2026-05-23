# Review Handoff: B-line Task 6 — Config Validate Pass

Date: 2026-05-23

Owner line: B-line (config / infra).

Slice ID: B-line task 6 (per
`docs/handoff-full-project-review-2026-05-23.md` section 4 P2
"Config and Vault Infra Need Hardening" + architect
`hermes-workspace/lore-infra-handover.md`).

## Scope

Adds an explicit `Config.Validate()` method covering operator-
facing invariants, wired into `LoadWithOptions` so a malformed
user-global or workspace override fails at runtime open rather
than leaking into a daemon callback / vault write / scheduler
tick.

The rule set is deliberately small and operator-facing: every
rule maps to either a path the downstream code dereferences
blindly, a window/duration that must be non-negative for the
scheduler, or a string time that downstream parsing assumes is
"HH:MM". Policy-style rules (port range, AuditLogDir must be
inside WorkDir, etc.) are intentionally omitted -- this slice
is validation for typos, not policy.

## Changed files (3)

- `internal/config/config.go` — new `Validate` method on
  `Config`; new unexported `hhmmPattern` regexp for the two
  string-time fields.
- `internal/config/loader.go` — `LoadWithOptions` calls
  `cfg.Validate()` as the last step before returning, preserving
  the diagnostics list so the operator still sees which layers
  loaded successfully when validation rejects the merged result.
- `internal/config/validate_test.go` — new file, 8 test
  functions covering the legal default, every reject path, and
  the LoadWithOptions integration.

## Validate rule table

| Section | Field | Rule |
|---|---|---|
| Paths | work_dir | non-empty |
| Paths | vault_root | non-empty |
| Paths | state_dir | non-empty |
| Paths | audit_log_dir | non-empty |
| Paths | process_sink_dir | non-empty |
| Vault | debounce_window | >= 0 |
| Vault | temp_suffix | non-empty |
| Vault.Resolve | unique_score_threshold | [0, 1] |
| Vault.Resolve | unique_score_margin | [0, 1] |
| Runtime | max_event_queue | > 0 |
| Runtime | max_draft_queue | > 0 |
| Runtime | max_memory_mb | > 0 |
| Runtime | inspect_at | matches HH:MM (24h) |
| ProcessSink | checkpoint_every | >= 0 |
| ProcessSink | daily_rollup_at | matches HH:MM |
| ProcessSink | retention_days | >= 0 |
| Usage | soft_warning_tokens | >= 0 |

Each rule's error message names the offending field so the
operator can grep their config file directly.

## Why LoadWithOptions instead of a separate Validate-on-runtime
hook

- The runtime opens via `app.OpenRuntime` -> `config.LoadWithOptions`.
  Adding Validate inside the loader keeps the contract at one
  place (loader = source of merged config, validated). Any
  caller that uses `LoadWithOptions` gets validation for free.
- The package-level `Default(workDir)` skips Validate because
  it is the canonical baseline; the test
  `TestValidateDefaultPasses` is the regression guard that the
  baseline itself remains legal.
- Diagnostics are still returned even when Validate fails, so a
  troubleshooting operator can see which layers were actually
  applied. The Validate error tells them which field broke the
  merge.

## What was NOT touched

- No new dependency. `regexp` is stdlib.
- No `Config` field rename or type change. The Validate method
  observes the existing fields.
- No `Default(workDir)` change. The shipped defaults already
  satisfy every rule; the
  `TestValidateDefaultPasses` regression guard locks that in.
- No app / store / persona / orchestrator / CLI / MCP / TUI /
  SDK changes outside the LoadWithOptions tail call.

## Review focus

- Confirm Validate is called as the **last** step before
  LoadWithOptions returns. Calling it earlier (between layers)
  would reject an intermediate state that the next layer would
  have fixed.
- Confirm the diagnostics slice is returned in the Validate-
  failure path too, so an operator running the loader for
  troubleshooting can see which layers loaded successfully even
  when the final merge fails validation.
- Confirm hhmmPattern correctly accepts 00:00 / 23:59 and
  rejects 24:00 / single-digit hour. The boundary test pins
  both directions.
- Confirm Validate does NOT mutate the receiver. The current
  body only reads fields; if a future rule wants to normalize
  (e.g. lowercase the proactive_mode string), it should do that
  in a separate Normalize method called before Validate, not
  silently inside Validate.

## Validation

```powershell
go test ./internal/config -count=1 -v
# 8 test functions, all subtests PASS

go test ./internal/store/... ./internal/app/... ./internal/cli/... ./internal/console/... ./internal/orchestrator/... ./internal/config/... ./cmd/... -count=1
# all 9 packages green
```

The existing `TestOpenRuntimeWithConfigOptionsIsolatesUserGlobal`
keeps passing because the malformed-config-file branch surfaces
the JSON parse error before Validate ever runs.

## Known limitations / future follow-up

- **Human-readable duration parsing** ("500ms", "30m") is NOT in
  this slice. The architect handoff lists this as a P2 nice-to-
  have alongside the validation pass. Implementing it requires
  either a `humanDuration` wrapper type (changes every field's
  type and breaks downstream consumers) or a custom Decoder
  path (changes how the layer JSON is unmarshalled and affects
  test fixtures). Both are larger refactors than the validation
  pass; recorded here so a future contributor can scope it
  independently.
- **Environment variable overrides** for operational knobs (the
  architect's other P2 item) are also deferred. The current
  three-layer system (default / user-global / workspace) covers
  every documented use case today; a fourth env-override layer
  would slot in cleanly under the same `applyLayerFile` shape
  if one materialises.
- **Cross-field consistency rules** (e.g. AuditLogDir should be
  inside StateDir, ProcessSinkDir should be inside VaultRoot)
  are intentionally NOT enforced. The current Validate is for
  typos; adding cross-field policy would risk false positives
  for valid non-default layouts (e.g. AuditLogDir on a separate
  disk).
- **Vault atomic write durability** (the third architect P2
  item: lack of fsync, no cross-device rename fallback) lives
  in `internal/vault` and stays B-line task 7 if it gets one.
