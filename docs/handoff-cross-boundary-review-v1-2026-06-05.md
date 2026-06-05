# Cross-boundary handoff: review-v1 out-of-boundary fixes (2026-06-05)

The B-line owner (me) was authorized by the user on 2026-06-05 to fix the
out-of-boundary findings from `review-v1.txt` across all lines. Commits tagged
`[cross-boundary]` reference this doc; the owning line should review.

**Scope discipline applied.** I fixed the *clear, safe, self-contained* bugs
across lines. I did **not** implement substantial features, large refactors,
config-design decisions, or transport-specific changes in domains I don't own
— doing so blind (without the owner's context or a real test endpoint) risks
subtle breakage. Those are handed off below with a verified diagnosis so the
owner can act fast. I also touched **no WIP files** (the TUI line's
`internal/tui/*`, `internal/cli/tui_workbench.go`, `README.md`,
`scripts/release-gate.ps1`).

## Fixed (3)

| Line | Finding | Commit | What |
|---|---|---|---|
| MCP | **P1-15** (security) | `30b1e59` | `validateProcessAuth` now uses `crypto/subtle.ConstantTimeCompare` instead of `==` (which leaks the key prefix length via timing). Test added. |
| adapter | **P1-9** (OOM) | `f1972a7` | `LoadTail` reads via `io.LimitReader(file, 64MB)`; the returned offset advances so >cap tails are consumed across calls — no data loss, bounded memory. |
| console | **P2-29** (dead code) | `22876ae` | Removed ~182 lines of commented-out `hasExplicitLocalWorkIntent` code. Zero behavior change. |

## Deferred to owner — verified diagnosis, NOT a safe cross-boundary quick fix

### llm
- **P1-7 (legacy chat path ignores tools / drops `tool_calls`).** Confirmed: the
  legacy `chatCompletionRequestPayload` has **no `Tools` field**, so `req.Tools`
  is silently dropped (the model is never told about tools) and the response's
  `tool_calls` are not parsed; `chatCompletionOnce` even errors on empty content
  (would reject a content-less tool-call turn). Fix = a *feature*: marshal
  `req.Tools` into the legacy payload, parse `message.tool_calls` from the
  response, return them via the existing `ChatCompletionResponse.ToolCalls`, and
  stop erroring when a tool call carries no text. Needs the OpenAI `/chat/completions`
  tools wire format + a real-endpoint test — owner's call.
- **P2-16** (`NewClient` doesn't validate empty `Model`): trivial, but two
  callers live in WIP `tui_workbench.go` I can't verify — owner should add
  `if cfg.Model == "" { return err }` once those are confirmed to set Model.
- **P2-17** (`usesResponsesAPI` hardcodes `gpt-5.4*`): config-design (where would
  the capability map come from?). Owner.

### operatoragent
- **P1-8** (native vs text-mode tool-call handling duplicated ~70 lines): a
  refactor with no behavior change; risky to extract without owning the agent
  loop. Owner.
- **P1-5** (`Decide()` takes no caller ctx): ctx threading through the agent
  loop; involved. (The B-line process-sink summarizer half of P1-5 is already
  fixed: `2755f0e`.) Owner.
- **P2-13/P2-14** (hardcoded model prefixes / `maxLoopSteps=8`): config-design.
  Owner.

### cli (structure)
- **P1-11** (160-line `Run()` switch → command registry), **P1-12** (bootstrap
  contradictory "0 docs" output), **P1-13** (hardcoded Chinese bootstrap paths →
  derive from config), **P2-30** (workdir-resolve duplication): a coherent cli
  cleanup the cli owner should do together. (`cli.go` is not WIP-dirty, but these
  are structural choices, not bugs.)
- **P2-38** (`inferProvider` heuristic): in WIP `tui_workbench.go` — skip.

### mcp
- **P2-31** (`structuredContent` non-standard field), **P2-32** (`MarshalIndent`
  vs `Marshal`): minor; may be intentional (client compat / readable logs). Owner.

### console
- **P1-14** (24-method `Runtime` interface → split per ISP): a large refactor
  touching every mock. Owner.
- **P2-26/P2-27** (history cap 20 / workingset cap 8): config-design. Owner.
- **P2-28** (`BuildCoreContext` error discarded at session.go:715): **NOT a bug**
  — it is documented graceful degradation (comment at L709-714: launch extraction
  with empty context rather than drop the turn); L428 already checks the error.
  No change needed.

### adapter
- **P2-18** (`codexappserver.readEnvelope` no I/O deadline): the fix needs the
  underlying transport (stdio pipe vs net.Conn) to set a read deadline — transport
  knowledge the codexappserver owner has. Owner.

### sessionlog
- **P1-10** (open/close file per append; History truncated to 20 on Load): a
  buffered-writer / handle-lifetime change (perf) + a context-window policy
  choice. Owner.
- **P2-19** (`indexLocks` map never pruned): a real but small leak (bounded by
  distinct session paths, single-user); safe pruning needs refcounting to avoid
  evicting an in-use lock — race-prone to do casually. Owner.

### config
- **P2-20** (plaintext secret in config file): an ops/design concern, true of any
  file-config secret. Owner.
- **P2-21** (no top-level `Config.Validate`): additive, but the value is in calling
  it at load time — a wiring decision. Owner.

### tui — SKIPPED (active WIP)
P2-22 (Esc quits), P2-23 (clipboard error swallowed), P2-24 (per-frame lipgloss
alloc) live in `internal/tui/*` which has uncommitted WIP. The TUI owner should
take these to avoid clobbering in-progress work.

## Corrected in the triage (review was wrong) — no action for anyone
P1-6 (no fd-leak), P2-10 (param misread), P2-34 (classifier already covers
schedule/weekly). See `docs/review-v1-triage-2026-06-04.md`.
