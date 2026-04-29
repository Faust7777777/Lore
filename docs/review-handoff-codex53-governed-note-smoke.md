# Review Handoff: Governed Markdown Note Intake Smoke

## Scope

This change adds a business-level smoke for the external-agent governed note intake flow.

It does not add any MCP write/apply/supersede tools.

## Product Flow Covered

The smoke covers:

1. Bootstrap managed workspace.
2. Create a `markdown_note_write` proposal.
3. Verify proposal creation does not write the target note.
4. Load local review state.
5. Approve and apply locally through Lore runtime.
6. Verify the target markdown note is written only after local apply.
7. Verify audit contains draft create/state/apply events.
8. Verify external MCP `tools/list` still includes `markdown_note_propose` but excludes direct write/apply/supersede tools.

## Implementation Notes

- Added `Runtime.SmokeGovernedMarkdownNoteIntake`.
- Added `GovernedNoteSmokeResult`.
- The smoke is deterministic and offline; it does not call the model provider.
- The MCP boundary assertion lives in `internal/app/smoke_test.go` so production app code does not depend on the MCP package.

## Review Focus

- Confirm proposal creation checks target absence before local apply.
- Confirm local apply writes the expected ordinary markdown note.
- Confirm the smoke does not rely on real model calls.
- Confirm MCP boundary check rejects `vault_write_low`, shell, workspace write/edit, draft approve/apply/supersede, and generic proposal submit.
- Confirm this smoke does not weaken existing P0 smoke.

## Verification

Passed:

```powershell
.\.tools\go\bin\go.exe test ./internal/app -run "TestRuntimeSmokeGovernedMarkdownNoteIntake|TestRuntimeSmokeP0" -count=1 -v
```
