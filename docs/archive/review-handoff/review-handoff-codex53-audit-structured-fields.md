# Review Handoff: Structured Audit Fields

## Scope

This change closes the model-level audit contract gap from decision #18 by adding structured audit fields while preserving existing persisted payloads and legacy callers.

Changed files:

- `internal/model/audit.go`
- `internal/model/audit_test.go`
- `internal/runtime/auditor.go`
- `internal/store/memory/store.go`
- `internal/store/jsonstore/store.go`
- `internal/store/sqlitestore/store.go`
- `internal/orchestrator/readapi_test.go`

## Contract

`model.AuditRecord` now includes:

- `actor_type`
- `actor_id`
- `result_status`
- `correlation_id`

The existing `actor` field is intentionally retained for compatibility with old JSON state, existing tests, and callers that still write compact actor strings such as `mcp:lore-agent`.

## Normalization

`model.NormalizeAuditRecord` fills missing structured fields:

- Empty `actor` becomes `system`.
- `actor` values containing `type:id` split into `actor_type` and `actor_id`.
- Non-prefixed actors use the same value for both type and id.
- Empty `result_status` defaults to `ok`.
- Empty `correlation_id` defaults to the audit record ID.

Normalization is applied in two places:

- `runtime.Auditor.Record`, covering normal orchestrator/business paths.
- Each store `AppendAudit`, covering tests, future direct store users, and transactional SQLite audit writes.

## Compatibility

The new JSON fields use `omitempty`. Older stored records without these fields still unmarshal. New records get the structured fields before persistence.

## Tests

Added/updated coverage:

- `TestNormalizeAuditRecordFillsStructuredFields`
- `TestNormalizeAuditRecordPreservesExplicitStructuredFields`
- `TestVaultReadAuditPrefersLoreAgentIdentity` now asserts `actor_type`, `actor_id`, `result_status`, and non-empty `correlation_id` from a real orchestrator read audit path.

Verification run:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
```

Result: pass.

## Review Focus

Please check:

- Whether defaulting `correlation_id` to `AuditRecord.ID` is acceptable for P0, or whether callers should provide higher-level request/session IDs later.
- Whether `result_status=ok` as the default is acceptable for current success-only audit call sites.
- Whether store-level normalization is desirable as a defensive contract, or if you prefer keeping normalization only at `runtime.Auditor.Record`.
