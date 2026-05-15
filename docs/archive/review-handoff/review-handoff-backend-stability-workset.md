# Review Handoff: Backend Stability and Working Set

Reviewer: DeepSeek
Scope owner: Codex
Date: 2026-04-24

## Scope

This handoff covers backend/runtime-facing changes outside the TUI presentation lane:

- session-level working set memory for short follow-up requests
- status priority order alignment
- draft domain transition validation

It intentionally does not cover:

- `internal/tui/interactive_*` visual or interaction changes
- OpenAI native tool calling, which is covered by `docs/review-handoff-native-tools.md`
- audit record schema expansion, which remains pending and should be reviewed separately

## Files To Review

Working set memory:

- `internal/operatoragent/agent.go`
- `internal/operatoragent/model.go`
- `internal/operatoragent/model_test.go`
- `internal/console/session.go`
- `internal/console/session_test.go`

Status priority:

- `internal/model/status.go`
- `internal/model/status_test.go`

Draft transition validation:

- `internal/domain/drafts/draft.go`
- `internal/domain/drafts/draft_test.go`
- `internal/orchestrator/harness.go`

## Intent


### Managed Core Alias Routing

`system_doc_get` now accepts stable aliases for managed core docs, including Chinese aliases such as `画像` and `人物画像` for `persona`.

Native OpenAI tool schemas now preserve the runtime tool description instead of replacing it with a generic `Lore tool <name>` string, so the model can see deterministic mappings like `persona = 人物画像` when deciding which tool to call.

### Vault Resolve

Added `vault_resolve` as a read-only vault-search tool for arbitrary user notes. It resolves a natural-language note reference into `unique`, `ambiguous`, or `not_found` using strict path/title scoring.

Default uniqueness thresholds live in `config.Vault.Resolve`: score >= 0.9 and lead over the second match >= 0.3. The tool does not read file content and does not bypass `vault_read`; callers should only read `selected_path` when status is `unique`.
### Working Set Memory

Fix the bad follow-up UX where Lore can list or mention a vault path, then fail to resolve a short follow-up like `读取`.

The chosen design is not keyword gating. It mirrors Codex/OpenCode-style agent context:

- remember recently referenced vault markdown paths in `console.Session`
- pass them into `operatoragent.Context.WorkingSet`
- render them in the next loop prompt as `Current working set`
- let the model choose the appropriate tool naturally

The working set is session-memory only. It is not persisted, does not bypass runtime policy, and does not directly execute tools.

### Status Priority

Align `MergeStatus()` with the frozen status precedence:

`blocked > adapter_disconnected > conflict > unauthorized > unsupported > error > ok`

This makes the most actionable state win when several outcomes are merged.

### Draft Transition Validation

Move draft state transitions closer to the domain contract before store mutation.

`domain/drafts` now exposes transition aliases and validation helpers, and `orchestrator.Harness` calls them before state updates for review/apply/conflict flows.

## Important Semantics

- Working set items only accept relative vault markdown paths.
- Absolute paths, drive-letter paths, non-markdown paths, and empty paths are ignored.
- Working set is bounded to the latest 8 remembered items.
- Duplicate paths are de-duplicated case-insensitively and moved to the newest position.
- Paths can be learned from tool arguments or assistant JSON/text output.
- Working set context is advisory prompt context only; runtime tools still enforce permissions and governance.
- Draft transition validation returns the existing public `ErrDraftNotReady` shape from orchestrator for invalid transitions.
- `DraftRevisionRequested` is intentionally still present; whether it should remain a persisted state or become only a review action is a product decision, not part of this fix.

## Verification Already Run

```powershell
.\.tools\go\bin\go.exe test ./internal/console ./internal/operatoragent ./internal/model ./internal/domain/drafts ./internal/orchestrator -count=1
```

Previous full test run reported passing before this handoff was written:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
```

Please re-run full tests after review if any code changes are requested.

## Review Focus

- Check whether `WorkingSet` belongs in `operatoragent.Context` or should be wrapped in a more generic context package later.
- Check that workset extraction cannot accidentally capture local filesystem paths or MCP/tool implementation details.
- Check that short follow-ups are solved through context, not keyword whitelists or command hacks.
- Check that status priority matches the frozen order exactly.
- Check that draft transition validation catches illegal transitions before store mutation.
- Check that orchestrator error mapping remains user-stable.

## Manual Smoke Suggested

Use the demo vault:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tmp\start-lore.ps1 -Mode tui
```

Scenario:

1. Ask: `看看人物画像`
2. Then ask: `读取`
3. Expected: Lore uses the working-set path such as `03-画像/人物画像.md` and calls `vault_read`, instead of asking for the path again.

Console equivalent if TUI is not needed:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tmp\start-lore.ps1 -Mode console -Once "看看人物画像"
```

The two-turn behavior is better tested in TUI or an interactive console because the working set is session memory.

## Known Pending Work Not In This Patch

- Structured audit fields from decision #18: `result_status`, `correlation_id`, `actor_type`, `actor_id`.
- Pending approval/action queue for shell confirmation and future runtime confirmations.
- Product decision on whether `request_revision` remains a persisted draft state.
- Persistent session memory is intentionally not added.
