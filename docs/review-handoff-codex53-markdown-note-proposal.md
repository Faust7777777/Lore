# Review Handoff: Markdown Note Proposal Intake

## Scope

- Adds `model.MarkdownNoteProposal`, `MarkdownNoteProposalResult`, `DraftKindMarkdownNoteWrite`, and `DraftBaseVersionNewFile`.
- Adds `Harness.ProposeMarkdownNote`, which creates a pending draft only.
- Reuses a shared low-governance markdown target validator for local `WriteLowRiskNote` and markdown note proposals.

## Boundary To Review

- Proposal creation must not write the target markdown file.
- Managed core docs, persona, progress, agent, identity, plans, process-sink paths, hidden paths, path traversal, drive paths, non-md files, and oversized fields must be rejected.
- `Draft.Target.Class` is intentionally `note` even when the classifier returns `unknown`; this records the governed semantic target for ordinary notes.
- New-file drafts use `DraftBaseVersionNewFile = "new"` instead of an empty base version.

## Verification

- `go test ./internal/model -count=1`
- `go test ./internal/orchestrator -run "TestProposeMarkdownNote|TestWriteLowRiskNoteRejectsGovernedPaths|TestWriteLowRiskNoteWritesNoteAndAudits" -count=1 -v`
