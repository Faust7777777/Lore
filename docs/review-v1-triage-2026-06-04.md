# Triage of review-v1.txt against current code (2026-06-04)

Source report: `review-v1.txt` (external "Lore 项目综合审阅报告", dated 2026-06-03,
~57 findings P0-1…P2-39). It was written **before** the 23 B-line commits on
`b-line/audit-followups-2026-06-04`. Each finding here was **re-verified against
the live `internal/` tree at HEAD `402806c`** (line numbers in the report had
drifted; corrected locations below). Verification was done by an independent
sub-agent reading the actual current code, then curated against my own context.

## Verdict summary (57 findings)

| Verdict | Count | Meaning |
|---|---|---|
| VALID-UNFIXED | 31 | real, still present |
| PARTIALLY-VALID | 13 | real but nuanced / partly addressed |
| DESIGN-CHOICE | 7 | intentional + defensible, not a bug |
| INCORRECT | 3 | report misreads the code |
| ALREADY-FIXED | 1 | already resolved |

Of the 31 VALID-UNFIXED, the large majority are **out-of-boundary** (other
lines: operatoragent / llm / tui / mcp / sdk / config / sessionlog / adapter /
console / cli-structure). The B-line actionable set is small and listed first.

## B-line fix plan (what I will change)

Prioritised, in-boundary, genuinely valid:

| # | Finding | Issue | Current location | Effort |
|---|---|---|---|---|
| 1 | P1-3, P2-11 **+2 unreported siblings** | byte-slice truncation can split a multibyte (Chinese) rune → invalid UTF-8 | `orchestrator/readapi.go` `excerpt`; `app/processsink_summarizer.go` `truncateForSummary`; `orchestrator/harness.go` `summarizeContent` (`preview[:80]`); `vault/query.go` `trimPreview` (`value[:180]`) | trivial |
| 2 | P1-2 | `broker.Publish` failures are fully silent (no log, no health-mark) — gap vs "audit is first-class" | `orchestrator/harness.go` (9 sites) | trivial |
| 3 | P2-6 | `UsageRecord.Purpose` accepts any string → typo creates a junk bucket in `lore usage` | `app/usage.go` `RecordUsage` (normalise/validate) | trivial |
| 4 | P1-1 | draft ID = `draft-<UnixNano>` can collide under multi-entry concurrency | `orchestrator/harness.go` (draft-ID mint sites) | small |

**Deferred / owner-judgment (in-boundary but not auto-fixing this pass):**
- **P2-5 + P2-37** — the dead `drafts.Draft` struct API (`New`/`SubmitForReview`/
  `Approve`/`Apply`/`AllowsDirectWrite`, and the unreachable `DraftCreated`
  state it alone produces). Confirmed dead by both this review and my earlier
  note (`e2b03fb`). Deletion removes a whole domain API + its test file — an
  owner-level call (it may be a deliberate DDD/SDK extension point), so I flag
  it rather than delete unilaterally. The `model.DraftCreated` enum value stays
  (store/JSON compat); `CanTransition`/`ValidateTransition` are live and stay.
- **P2-2** — apply-time TOCTOU (read-hash → atomic-write window). Real but
  narrow (baseVersion guard + atomic write + single-user). A re-hash-before-
  rename fix is medium and touches the atomic writer; deferred.
- **P0-2** — no concurrent-ApplyDraft test. Valid test gap; medium. Candidate
  for a follow-up (the optimistic-lock path is correct by inspection but
  unexercised under real goroutine contention).
- **P1-4 / P2-7** — `internal/runtime` InMemoryBroker partial-delivery on
  backpressure + publish-vs-close race. B-line-adjacent infra; largely defanged
  now that all publishers are best-effort. Worth a concurrent test + lazy-close;
  deferred (and `internal/runtime` is shared infra — coordinate before changing).

## Defended — DESIGN-CHOICE (not changing; rationale)

- **P0-1** `ApplyDraft` no vault rollback on state-update failure — intentional:
  the atomic vault write is the committed effect; a blind retry trips the
  baseVersion guard → `conflicted` (safe fallback); a **critical Finding**
  preserves the "actually applied" signal (`recordApplyStateFailFinding`).
  Covered by `apply_state_fail_test.go`. Residual = no auto-reconcile CLI
  command (a feature, not a bug).
- **P2-1** `renderMarkdownNoteDraft` ignores `current` — `markdown_note_write`
  is whole-note authoring; the human approves the full content, and the
  baseVersion guard ensures it was written against the reviewed version. Not a
  silent merge-loss.
- **P2-8** process-sink store-then-file ordering — the store is the queryable
  source of truth; the `.md` is a derived render; the next checkpoint/rollup for
  the same window re-renders (upsert by window key). Not data loss.
- **P2-4** dual `LORE_MCP_AGENT_ID` / `OBSIDIAN_HARNESS_MCP_AGENT_ID` env vars —
  intentional backward-compat.
- **P2-15** vault symlink read-vs-write asymmetry — intentional security stance
  (never write through a link; tolerate reading real files linked under root).
- **P2-33** persona prompt has no few-shot — deliberately terse; an enhancement,
  not a defect.
- **P2-35** only 2 `FindingKind`s — exactly the two detection sources that exist;
  open string enum, trivially extensible.

## Corrected — INCORRECT (report misreads the code)

- **P1-6** "file-handle leak: `defer file.Close()` inside the walk callback holds
  all fds open." **Wrong** — the `defer` is scoped to the per-file callback
  closure that `WalkDir` invokes once per file, so each file closes when its own
  callback returns: at most one fd open at a time (`vault/query.go` SearchText /
  FindBacklinks). No leak.
- **P2-10** "`UpdateDependencies(modelAvailable, false)` hardcodes persona=false."
  **Wrong** — the second param is `adapterConnected`, correctly `false` for a
  plain vault-daemon tick (import/sync paths pass `true`). There is no "persona"
  dependency in the health probe.
- **P2-34** "classifier missing `schedule/` / `weekly`." **Wrong** — `schedule`
  is present as `PathContains "/schedule/"` + `FileNameContains "schedule"`;
  `weekly` matches via `FileNameContains "week"` (substring). Plus `roadmap`,
  `sprint`, `plan`, `plans/`.

## Already fixed

- **P2-25** mixed-language TUI footer (`a=同意 r=拒绝 …`) — current footers are
  all-English. (Out-of-boundary anyway.)

## Out-of-boundary handoff (for A-line / TUI / infra owners)

Verified-present, NOT mine to fix — routed to the owning line:

- **llm**: P1-7 (`chatCompletionOnce` drops `tool_calls`), P2-16 (`NewClient`
  doesn't validate empty Model), P2-17 (`usesResponsesAPI` hardcodes model names).
- **operatoragent**: P1-8 (duplicated native/text tool-call handling), P1-5
  (`Decide()` no caller ctx), P2-13 (`isLikelyOperatorModel` hardcoded prefixes),
  P2-14 (`maxLoopSteps=8`).
- **tui / cli-ui**: P1-5 (tui_workbench/interactive_workbench ctx), P2-22 (Esc
  quits), P2-23 (clipboard error swallowed), P2-24 (per-frame lipgloss alloc),
  P2-38 (`inferProvider` heuristic).
- **cli structure**: P1-11 (160-line `Run()` switch), P1-12 (bootstrap
  contradictory output), P1-13 (hardcoded Chinese bootstrap paths), P2-30
  (workdir-resolve duplication).
- **mcp**: P1-15 (no default auth, no constant-time compare — security), P2-31
  (`structuredContent`), P2-32 (`MarshalIndent`).
- **console**: P1-14 (24-method Runtime interface / ISP), P2-26 (history cap 20),
  P2-27 (workingset cap 8), P2-28 (`BuildCoreContext` error discarded), P2-29
  (~180 lines commented-out dead code).
- **adapter**: P1-9 (`LoadTail` no size cap → OOM), P2-18 (`readEnvelope` no I/O
  deadline).
- **sessionlog**: P1-10 (open file per append; History cap 20), P2-19
  (`indexLocks` map never pruned).
- **config**: P2-20 (plaintext secret in config), P2-21 (no top-level
  `Config.Validate`).

P1-15 (MCP auth) is the highest-severity out-of-boundary item — flagged to the
MCP owner as a release blocker per the report's own "阻断点 4".
