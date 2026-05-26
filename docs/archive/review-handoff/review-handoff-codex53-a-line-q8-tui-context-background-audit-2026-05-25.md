# Review Handoff: Q-8 TUI executeLine context.Background Audit

Date: 2026-05-25
Finding: Q-8 (P2) from REVIEW-2026-05-25-FINAL.txt

## Scope

- `internal/tui/interactive_workbench.go`
- `internal/cli/tui_workbench.go`
- Bubble Tea v1.3.5 framework source (go pkg mod cache)

## Reading Gate

- `REVIEW-2026-05-25-FINAL.txt` Q-8
- `docs/handoff-full-project-review-2026-05-23.md`
- `docs/archive/review-handoff/review-handoff-codex53-a-line-tui-view-purity-2026-05-23.md`

## Finding (verbatim from FINAL)

> Q-8: TUI `executeLine` 用 `context.Background()`，与 context 传播趋势矛盾

## Audit Trail

### What executeLine does

```go
// interactive_workbench.go:292-301
func (m *interactiveWorkbenchModel) executeLine(line string) tea.Cmd {
    return func() tea.Msg {
        if contextDriver, ok := m.driver.(InteractiveWorkbenchContextDriver); ok {
            update, err := contextDriver.ExecuteContext(context.Background(), line, m.lastOutput)
            return interactiveResultMsg{update: update, err: err}
        }
        update, err := m.driver.Execute(line, m.lastOutput)
        return interactiveResultMsg{update: update, err: err}
    }
}
```

This runs inside a Bubble Tea `tea.Cmd` goroutine. The `context.Background()` is the origin context for the full call chain:

```
executeLine (tea.Cmd goroutine)
  → contextDriver.ExecuteContext(context.Background(), ...)
    → session.HandleContext(ctx, ...)
      → loopAgent.RespondContext(ctx, ...)
        → a.client.ChatCompletion(turnCtx, ...)  // LLM HTTP call
        → callToolWithContext(turnCtx, ...)       // tool dispatch
```

### Why context.Background() is used

Bubble Tea v1.3.5 Cmd signature:

```go
// bubbletea@v1.3.5/tea.go:66
type Cmd func() Msg
```

Cmd accepts no parameters. There is no mechanism for Bubble Tea to pass a context into a Cmd closure.

Bubble Tea's own comment on this limitation (tea.go:350-354):

```go
// Don't wait on these goroutines, otherwise the shutdown
// latency would get too large as a Cmd can run for some time
// (e.g. tick commands that sleep for half a second). It's not
// possible to cancel them so we'll have to leak the goroutine
// until Cmd returns.
```

### What happens on Ctrl+C

1. User presses Ctrl+C
2. `tea.KeyMsg{String: "ctrl+c"}` → `tea.Quit`
3. `tea.Quit` emits `QuitMsg` → event loop returns
4. `Program.Run()` deferred `p.cancel()` fires → `p.ctx.Done()` closes
5. `handleCommands` goroutine sees `p.ctx.Done()` and returns, stops dispatching new Cmds
6. **But** any already-running Cmd goroutine (like executeLine) continues until the LLM call returns
7. When the LLM call returns, `p.Send(msg)` attempts to send on the closed channel — the message is dropped

### Why TUI managing its own context would not help

A TUI-owned `context.WithCancel` could be cancelled on Ctrl+C. The cancel signal would propagate through `ExecuteContext → HandleContext → RespondContext → ChatCompletion`, causing the in-flight HTTP call to return with `context.Canceled`.

However this **changes the semantics of Ctrl+C** from "quit the program" to "cancel the current turn and continue running". The user's intent on Ctrl+C is ambiguous — it could mean either. This is a UX design decision, not a code fix.

### Current protection layers

Even with `context.Background()`, the call chain has multiple safeguards:

1. **RespondContext entry check** (`model.go:273`): `turnCtx.Err()` checked before any work
2. **Per-step context check** (`model.go:308`): `turnCtx.Err()` checked each loop iteration
3. **Tool call context check** (`model.go:806`): `ctx.Err()` checked before tool dispatch
4. **LLM client timeouts**: OpenAI client has HTTP transport timeouts (retry on 429/5xx, transport EOF handling)
5. **Tool result truncation** (`model.go:1056-1072`): 4KB limit prevents memory runaway
6. **Loop guards** (`model.go:339-343`): repeated/alternating tool loop detection

### Alternative approaches considered

1. **TUI-owned cancel context**: Would work technically but changes Ctrl+C semantics. Requires UX spec.
2. **Bubble Tea WithContext**: `tea.WithContext(ctx)` exists but sets the Program's lifecycle context. Cmd closures still can't access it. Would need to store the context in the model struct and read it from the closure — possible but non-idiomatic.
3. **Turn timeout via context.WithTimeout**: Could wrap the Background with a 60s timeout. This would prevent hangs without changing Ctrl+C semantics. But timeout should be configurable, which is a product decision.

## Design Judgment

**No code change at this time.** `context.Background()` is the correct choice given:

1. Bubble Tea's Cmd signature has no context parameter — framework limitation
2. Ctrl+C semantics are "quit", not "cancel turn" — user intent alignment
3. Multiple safeguard layers already exist in the call chain
4. LLM provider client has its own timeout/retry protection
5. Goroutine leak on quit is documented Bubble Tea behavior (tea.go:353)

If a future UX decision is made to support "cancel current turn" (e.g. Ctrl+C during `running=true` cancels the turn but keeps the program running), the implementation path is:

1. Add a `turnCancel context.CancelFunc` field to `interactiveWorkbenchModel`
2. Create `context.WithCancel(context.Background())` when starting a turn
3. Cancel it on Ctrl+C when `m.running == true`
4. Re-create on next turn start
5. Add test: cancel context during simulated slow LLM call → verify turn aborts cleanly

## Boundary

- TUI context audit only — no code changes
- No persona CLI changes (Q-1)
- No vault symlink fix (S-1)
- No runtimeDocs isolation (S-2)
- No lock order annotation (S-3)
- View() remains pure render (protected per e0f1c6f)

## Files Touched

None (audit only, this handoff doc is the deliverable)

## Load-bearing Tests

Existing tests that verify the related behavior:

- `TestInteractiveWorkbenchViewDoesNotRefreshContent` — View() purity
- `TestRunTUIOnceShowsResolveReadFinalTaskVisibility` — end-to-end TUI turn execution
- `TestApprovalFlow` — panel interaction during turn
- `panel_state_test.go` — all panel offset/cursor/state tests

## Validation

```powershell
go test ./internal/tui -count=1 -run "ViewDoesNotRefresh|ApprovalFlow|SinkOffset|FindingsOffset|ApprovalOffset"
go test ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1
.\scripts\release-gate.ps1 -SkipDiffCheck
```
