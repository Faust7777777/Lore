# Review Handoff: Agent Template Proposal-Only Writes

## Scope

This change updates the default `agent.md` template so external agents see the proposal-only write boundary during onboarding.

Changed:

- `internal/bootstrap/templates.go`
- `internal/bootstrap/templates_test.go`
- `docs/integrations/mcp-client-setup.md`

Not changed:

- MCP server behavior.
- MCP tool contracts.
- Existing workdir files. `lore bootstrap` still does not overwrite existing managed docs.
- TUI behavior.

## Decisions Captured

- `agent.md` remains the external-first shared operating manual.
- External MCP must not expose direct vault markdown writes.
- Durable markdown content should be submitted through proposal-intake tools.
- `markdown_note_propose` is planned, not implemented in this change.
- When proposal tooling is unavailable, external agents should emit a structured `Markdown Note Candidate` block.

## Review Focus

- Confirm the template does not tell external agents that low-risk direct writing may be added later.
- Confirm the template clearly distinguishes proposal creation from apply.
- Confirm `identity.md` remains Lore self identity and is not external onboarding material.
- Confirm existing workdir migration caveat remains documented.

## Suggested Verification

```powershell
.\.tools\go\bin\go.exe test ./internal/bootstrap -run TestDefaultManagedTemplatesIncludeRuntimeAgentDocs -count=1 -v
```
