# Review Handoff: Draft Audit Correlation IDs

## Scope

This change improves audit traceability for the draft lifecycle without changing draft state semantics or persisted store layout.

Changed files:

- `internal/orchestrator/harness.go`
- `internal/orchestrator/harness_test.go`

## Behavior

Draft lifecycle audit records now set `AuditRecord.CorrelationID` to the draft ID:

- `AuditDraftCreated`
- `AuditDraftStateChange`
- `AuditDraftApplied`

This makes create/review/apply events for one draft queryable as a single audit chain. Other audit records still use the normalization fallback from `model.NormalizeAuditRecord`, which defaults missing correlation IDs to the audit record ID.

## Non-Goals

- No draft state transitions changed.
- No TUI code changed.
- No new store schema or migration required; audit payload remains JSON.
- No change to the legacy `actor` field compatibility path.

## Tests

`TestBootstrapAndDraftLifecycle` now verifies that the created, state-change, and applied audit records for a draft share `CorrelationID == draft.ID`.

Verification commands:

```powershell
.\.tools\go\bin\go.exe test ./internal/orchestrator -run TestBootstrapAndDraftLifecycle -count=1
.\.tools\go\bin\go.exe test ./... -count=1
```

## Review Focus

Please check whether using draft ID as correlation ID is the right P0 granularity. A future request/session correlation layer can still be added separately without changing this draft-local chain.
