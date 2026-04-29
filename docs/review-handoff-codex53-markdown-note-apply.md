# Review Handoff: Markdown Note Draft Apply

## Scope

- Extends local `ApplyDraft` to support approved `DraftKindMarkdownNoteWrite` drafts.
- Adds new-file base-version handling through `DraftBaseVersionNewFile = "new"`.
- Keeps external MCP unchanged: no draft approve/apply and no direct write tool.

## Boundary To Review

- Pending markdown-note drafts cannot apply.
- New-file markdown-note drafts only apply if the target still does not exist.
- Existing-note drafts only apply if the target hash still matches the proposal base version.
- Apply revalidates the target path and class; forged core/persona/progress/process-sink/plan paths must fail before writing.
- Apply revalidates the JSON payload and requires payload `target_path` to match `Draft.Target.Path`.
- The note content is written as the reviewed markdown content with a trailing newline; no frontmatter is injected in this MVP.
- Invalid payloads leave the draft in `approved` state; conflicts transition the draft to `conflicted`.

## Verification

- `go test ./internal/orchestrator -run "TestApplyMarkdownNoteDraft" -count=1 -v`
- `go test ./internal/orchestrator ./internal/app ./internal/console ./internal/operatoragent -count=1`
- `go test ./internal/mcp -run "TestExternalMCPDoesNotExposeDirectWrites|TestMCPV1ExposesOnlyReadAndProposalTools" -count=1 -v`
