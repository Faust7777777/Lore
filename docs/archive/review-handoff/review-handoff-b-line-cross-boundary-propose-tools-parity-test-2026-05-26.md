# Handoff: B-line cross-boundary — propose-tools parity test — 2026-05-26

Audience: A-line operatoragent reviewer + next B-line contributor.

Trigger: follow-up to the cross-boundary propose-tools wiring shipped in
`9eefb6f` / `6aff287`. The wiring slice closed the bug (lore in-app TUI
agent had no draft-creation tool) but left two A-line review surfaces
flagged in its handoff:

1. Operator agent prompt assumptions — *resolved by inspection*: the
   "Available tools" enumeration in `internal/operatoragent/model.go:816`
   builds dynamically by `strings.Join(names, ", ")`. No hardcoded count
   or fixed list to update; the two newly-surfaced tools are auto-picked-
   up. No A-line edits required.
2. Audit / governance link — out-of-scope for this commit; needs live
   exercise on a real harness, not a unit test. Left as the next
   follow-up.

This commit addresses a third drift risk that was implicit in the wiring
slice: nothing currently fails fast if a future B-line edit re-hardcodes
proposal-tool metadata in `DescribeTools` and lets the console surface
diverge from the MCP surface schema.

Cross-boundary protocol followed (per memory rule):
1. Authorization: continuing under the same session-scoped
   "你能修就给它修上" + "都看看" the operator gave for the wiring slice.
2. Minimal-surface change: 1 file touched (test-only, B-line scope).
3. This handoff.
4. Commit message tagged `cross-boundary (B-line)`.

## TL;DR

| What | Detail |
|---|---|
| Commit | `9ce7e76` |
| Touched | 1 file / +37 |
| Tests | `go test ./internal/console/ -count=1` green |
| Surface change | None — only adds a regression guard |

## What changed

`internal/console/tool_runtime_test.go` — new
`TestToolRuntimeProposalToolsDescribeMatchesRegistryContract`. For every
Tool surfaced by `runtime.ProposalTools()`, asserts:

- The console `DescribeTools` entry exists.
- Its `Description` is byte-equal to the registry `Tool.Description()`.
- Its rendered `Arguments` hint mentions every required field name.

The test reuses the `fakeRuntime.ProposalTools()` path that
`TestToolRuntimeProposalToolsExposeAndDispatch` already exercises — same
`tools.RegisterProposal` registry the MCP server uses at
`internal/mcp/server.go:174`. So if anyone diverges either surface, the
console side fails here first.

## Why this and not a bigger parity harness

A full schema-equality test between MCP `tools/list` JSON and console
`DescribeTools` would over-fit. The two surfaces deliberately render
different shapes: MCP emits a JSON Schema object while console emits a
single-line example hint string. The behaviour that matters for governance
is "the LLM, on either surface, sees the same name, the same intent
sentence, and is told about the same required fields." This test pins
exactly those three.

## A-line review surface

Nothing new. This commit is a regression guard, not a contract change.

The earlier flag #1 from `review-handoff-b-line-cross-boundary-console-
propose-tools-2026-05-25.md` is **resolved**: I read
`internal/operatoragent/model.go:780-829`. The "Available tools" prompt
section at line 816 reads:

```go
builder.WriteString("\n\nAvailable tools (use exact names; keep calls minimal):\n- ")
builder.WriteString(strings.Join(names, ", "))
```

`names` is built from the input the agent loop hands the prompt builder
each turn — i.e. whatever `DescribeTools` just returned. No A-line code
counts or enumerates tools statically. The only static reference to a
tool name in the prompt is `vault_write_low` at model.go:789 inside a
governance rule (a specific direct-write boundary), not a tool list, and
unchanged by this slice.

The earlier flag #2 (audit-field parity vs MCP-created drafts) remains
open. It cannot be unit-tested cleanly because the audit columns are
populated by the harness — a real one, not the `fakeProposalHarness` —
and the parity question is "do the audit rows look the same when the
draft was created from console vs MCP." Right test for that is an end-
to-end exercise where both surfaces hit the real `Harness.Propose*`
method and we inspect the audit log. Left for the next live session.

## Boundary statement

Did NOT touch:
- A-line vault symlink S-1.
- A-line runtimeDocs S-2 isolation.
- A-line sessionlog S-3 lock-order documentation.
- TUI Q-8 context-background audit.
- A-line `internal/operatoragent/` (read only — the prompt is dynamic
  and needs nothing).
- SDK / MCP server.
- Any non-test source file.

## Reading order for a new contributor

1. This document.
2. `review-handoff-b-line-cross-boundary-console-propose-tools-2026-05-25.md`
   (the wiring slice this guard protects).
3. `internal/console/tool_runtime_test.go::TestToolRuntimeProposalToolsDescribeMatchesRegistryContract`.
4. `internal/tools/builtin_proposal.go:20` — surface declaration the
   guard pins to.

## Follow-ups (not in scope here)

- A-line: live exercise audit-field parity vs MCP-created drafts (flag
  #2 from the prior handoff — still open).
- B-line: when adding new SurfaceConsole tools to `internal/tools/`,
  consider whether the dynamic-source pattern from the wiring slice
  should expand to cover any of the remaining 18 hardcoded tool
  definitions in `DescribeTools`.
