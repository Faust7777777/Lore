# Review Handoff: Out-of-Band Vault Post-Scan

## Scope

Task 11 implements daemon post-scan handling for vault markdown changes made outside Lore.

This is a compensation/governance mechanism, not a prevention boundary:

- External shell/file writes cannot be blocked by Lore.
- Ordinary note changes are audited as out-of-band writes and persisted as open findings.
- Governed changes are audited as governance findings and persisted as open review-needed findings.
- Plan docs keep the existing progress-sync draft path.

## Files Changed

- `internal/app/daemon.go`
- `internal/app/daemon_test.go`
- `internal/app/findings.go`
- `internal/model/finding.go`
- `internal/model/audit.go`
- `internal/store/store.go`
- `internal/store/memory/store.go`
- `internal/store/jsonstore/store.go`
- `internal/store/sqlitestore/store.go`

## Key Semantics

- `ScanVaultChanges` now scans all markdown docs, not only plan docs.
- Initial full-vault observation still primes cursors and creates no audit/finding.
- After the initial baseline is complete, new markdown files without per-file cursors are treated as out-of-band changes instead of being silently primed.
- Changed plan docs still call `ObserveDocumentChange` and create progress drafts.
- Changed ordinary markdown notes create `AuditOutOfBandVaultWrite`, an open `FindingOutOfBandVaultWrite`, and no draft.
- Changed managed core docs create `AuditGovernanceFinding`, an open `FindingGovernanceReviewNeeded`, and no silent self-heal.
- Changed process-sink docs create `AuditGovernanceFinding` plus an open review-needed finding; they are not treated as safe ordinary notes.
- Findings are persisted in memory/json/sqlite stores and linked back to the audit record with `Finding.AuditID`.
- The daemon summary prints finding IDs when it creates findings.
- Local CLI exposes `lore findings list`, `lore findings resolve <id>`, and `lore findings ignore <id>` for review visibility and closure.
- Finding state changes append `AuditFindingStateChange`.

## Boundary Checks

External MCP remains unchanged:

- No `vault_write_low`.
- No shell/workspace write.
- No draft approve/apply/supersede.
- No generic proposal tool.

Task 11 does not expose post-scan via MCP.

## Tests Added

- `TestRuntimeScanVaultChangesAuditsOutOfBandOrdinaryNote`
- `TestRuntimeScanVaultChangesAuditsGovernedCoreOutOfBandChange`
- `TestRuntimeScanVaultChangesAuditsProcessSinkOutOfBandChange`
- `TestRuntimeScanVaultChangesAuditsNewOutOfBandOrdinaryNoteAfterBaseline`
- `TestRuntimeScanVaultChangesAuditsRecreatedGovernedCoreAfterBaseline`
- `TestRuntimeScanVaultChangesAuditsNewProcessSinkAfterBaseline`
- store persistence coverage in json/sqlite store tests.
- CLI coverage for findings list/resolve/ignore.

## Verification

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/app -run "TestRuntimeScanVaultChangesAudits" -count=1 -v
.\.tools\go\bin\go.exe test ./internal/app -run "TestRuntimeScanVaultChangesAuditsNew" -count=1 -v
.\.tools\go\bin\go.exe test ./internal/app -run "TestRuntimeScanVaultChanges|TestVaultDaemonScanSummary" -count=1 -v
.\.tools\go\bin\go.exe test ./internal/model ./internal/app ./internal/store/memory ./internal/store/jsonstore ./internal/store/sqlitestore -count=1
.\.tools\go\bin\go.exe test ./cmd/obsidian-harness -run "TestRunFindings" -count=1 -v
```

Expected: all pass.

## Review Focus

- Ensure ordinary note out-of-band changes do not create or apply drafts.
- Ensure ordinary note out-of-band changes create open findings linked to audit records.
- Ensure baseline-after-new ordinary note and process-sink files are not silently primed.
- Ensure managed core and process-sink out-of-band changes are not silently blessed.
- Ensure managed core and process-sink findings use review-needed severity/state and remain local store state, not MCP tools.
- Ensure local finding resolve/ignore writes state-change audit and does not expose MCP tools.
- Ensure existing plan-doc progress draft behavior is preserved.
- Ensure audit failures are not silently ignored by cursor advancement.
- Ensure no MCP contract or SDK read-only artifact drift was introduced.
