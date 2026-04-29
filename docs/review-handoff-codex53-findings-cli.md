# Review Handoff: Local Findings CLI

## Scope

This change makes persisted post-scan findings visible and closable through local Lore CLI commands.

It does not expose findings through MCP and does not create automatic remediation drafts.

## Files Changed

- `internal/app/findings.go`
- `internal/cli/cli.go`
- `internal/model/audit.go`
- `cmd/obsidian-harness/main_test.go`

## Key Semantics

- `lore findings list [--workdir <dir>] [--limit N]` lists persisted findings.
- `lore findings resolve [--workdir <dir>] <id>` marks a finding resolved.
- `lore findings ignore [--workdir <dir>] <id>` marks a finding ignored.
- Resolve/ignore append `AuditFindingStateChange` with the finding ID as the correlation ID.
- This is local runtime governance only. MCP remains read + proposal intake and still does not expose finding mutation, direct write, shell, or draft apply.

## Verification

Run:

```powershell
.\.tools\go\bin\go.exe test ./cmd/obsidian-harness -run "TestRunFindings" -count=1 -v
.\.tools\go\bin\go.exe test ./internal/app ./internal/store/... ./internal/model -count=1
.\.tools\go\bin\go.exe test ./internal/mcp -run "TestMCPV1ExposesOnlyReadAndProposalTools|TestExternalMCPDoesNotExposeDirectWrites" -count=1 -v
```

## Review Focus

- Confirm finding state changes create audit records.
- Confirm listing/closing findings is local CLI only.
- Confirm no MCP tool contract drift.
- Confirm command output does not imply that resolving a finding rewrites vault content.
