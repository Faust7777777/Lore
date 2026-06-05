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
| **llm** | **P1-7** (function-calling) | `591712a` | The legacy `/chat/completions` path now sends `req.Tools` (chat function-tool shape, `tool_choice=auto`) and parses `tool_calls` back, returning them via `ChatCompletionResponse.ToolCalls`; a content-less tool-call turn no longer errors. Mirrors the Responses helpers. Mocked-endpoint test. **(Done on follow-up: I took this on once authorized — the wire format is standard and unit-testable without a live endpoint.)** |
| cmd (test) | flaky watcher test | `ce26ccb` | De-flaked `TestRunCodexAttachLoopSyncsOnWatcherEventBeforePoll` (60s poll / 30s deadline) — it was intermittently failing under full-module load, which would break the Release Gate. |

## Deferred to owner — verified diagnosis, NOT a safe cross-boundary quick fix

### llm
- **P1-7 — DONE `591712a`** (see Fixed table). Originally deferred as a feature;
  taken on once authorized since the chat tools/`tool_calls` wire format is
  standard and unit-testable with a mocked endpoint.
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
- **P2-31 / P2-32 — NOT bugs (verified 2026-06-05).** `tools/call` deliberately
  returns BOTH a pretty-printed `content[].text` (the human/LLM-readable result,
  `server.go:155-162`) AND `structuredContent` (the machine-parseable result,
  `:164`). So `MarshalIndent` (P2-32) is intentional readability for the text
  channel, not overhead — switching to `Marshal` would regress it; and
  `structuredContent` (P2-31) is the intended machine path (a current MCP field,
  not "non-standard"). No action — both are correct as-is.

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
