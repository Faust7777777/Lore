# Review Handoff: MCP Markdown Note Proposal

## Scope

- Adds external MCP tool `markdown_note_propose`.
- Keeps external MCP proposal-only: no `vault_write_low`, no shell, no generic workspace write/edit, no draft approve/apply, no supersede/refine.
- Adds a 1 MiB JSON-RPC frame cap in `readMessage` before body allocation.
- Updates external MCP client setup docs with the live `markdown_note_propose` contract and example.

## Contract

Required:

- `target_path`
- `title`
- `content`
- `source_kind`
- `evidence`
- `reason`
- `source`
- `observed_at`

Optional:

- `task_context`
- `course`
- `topic`
- `dedupe_key`

`tags` and `related_paths` intentionally remain absent from the MCP v1 contract.

## Verification

- `go test ./internal/mcp ./internal/orchestrator -run "TestMarkdownNote|TestMCPV1ExposesOnlyReadAndProposalTools|TestExternalMCPDoesNotExposeDirectWrites" -count=1 -v`
