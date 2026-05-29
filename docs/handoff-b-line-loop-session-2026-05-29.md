# B-line review handoff — `/loop 迭代` session (2026-05-29)

Audience: reviewer (DeepSeek) + B-line owner.
Branch: local `main`, **26 commits ahead of `origin/main`, NOT pushed.**
HEAD: `f88f38f`.

Verification: a fresh `git worktree` checked out at HEAD (i.e. the
reviewable batch with the uncommitted TUI/A-line WIP excluded) passes
`go build ./...`, `go vet ./...`, and `go test ./...` — **every package
`ok`, zero failures**.

---

## TL;DR — what to look at, in priority order

1. **Production bug fix — `2ac9974`** (§3). One-line fix to a corrupted
   progress-index table header that broke table re-detection. Highest
   review priority because it changes runtime behavior.
2. **The `lore usage` feature** (§2). The only other production code in
   this batch (`cli.go`, `model`, `store`). Detailed contract doc:
   `docs/handoff-b-line-usage-by-model-2026-05-28.md`.
3. **Open decision needed from owner** (§4). A sibling corruption at
   `harness.go:1116` I deliberately did **not** fix — I don't know the
   intended heading text.
4. Everything else (§5) is **test-only** coverage hardening — no
   production behavior change.

Of the 26 commits: **3 feat + 1 fix** touch production code; 16 are
test-only; 6 are docs. Production diff is concentrated and small (see §1).

---

## 1. Production code touched (the only files to scrutinize for behavior)

| File | Δ | What |
|---|---|---|
| `internal/cli/cli.go` | +257 | `lore usage` report: per-model detail, `--json`, cross-purpose rollup, purpose %-share, hidden-tail token total |
| `internal/store/usage_aggregate.go` | +70 (new) | Shared bucketing helper: `UsagePurposeBucket`, `UsageModelBucket`, `AccumulateUsageBreakdown` |
| `internal/model/runtime.go` | +27 | `UsagePurposeStats.ByModel` field + contract docs |
| `internal/store/sqlitestore/store.go` | +18 | `SummarizeUsage` pulls provider/model from payload; calls shared helper |
| `internal/store/{memory,jsonstore}/store.go` | +6 each | `SummarizeUsage` calls shared helper |
| `internal/orchestrator/harness.go` | +1/-1 | **Bug fix** — corrupted progress-index header (§3) |

All other changed files are `_test.go` or docs.

---

## 2. Feature: `lore usage` cost attribution

Shipped incrementally (commits `8bf9a22`, `01fbf00`, `752eada`, `b491d45`,
`7c30189`, `5356db6`, `5f5d017` + doc commits). Full contract — bucketing,
fallbacks, JSON shape, reconciliation invariant — lives in
**`docs/handoff-b-line-usage-by-model-2026-05-28.md`** (kept current; its
"Follow-ups" section logs every refinement in this batch).

Summary of operator-facing output (`lore usage`):
- Top-line DAY table (unchanged) + `Today Usage` tail.
- `By purpose:` block — each purpose's calls/tokens **+ its token-weighted
  share** (e.g. `= 750 tokens (75%)`), with indented per-model detail
  (top-5 by tokens, `... N more model(s), T tokens` for the truncated
  tail).
- `By model (all purposes):` rollup when >1 purpose — total spend per
  provider/model regardless of purpose.
- `--json` emits the **uncapped** structured shape (the human caps/%/
  rollup are display-only; JSON consumers fold for themselves).

Data-quality dependency: the buckets are only correct if billers stamp
`Purpose`/`Provider`/`Model`. `5f5d017` adds the missing guard so all
three billers (chat / persona-extract / process-sink) are symmetrically
tested for `Purpose` stamping.

---

## 3. PRODUCTION BUG FIX — `2ac9974` (review priority)

**Symptom:** `internal/orchestrator/harness.go:progressIndexTableBlock`
emitted the progress-index table **header as mojibake** (corrupted bytes,
`闁哄倸娲…`) instead of the intended `| 文档 | 类型 | 状态 | 最近更新 |`
(document / type / status / last-updated). The expected header is defined
(correctly, via `\u` escapes) in `isProgressHeaderLine`.

**Impact:** the generated header did not byte-match what
`isProgressHeaderLine` recognizes, so once a progress-index table was
created, `findProgressTable` could never find it again. The next upsert
for the same doc therefore **appended a second (also-garbled) table**
instead of updating the row in place — silently duplicating/garbling the
index.

**Why it was latent:** `progressIndexTableBlock` had **0% test coverage**
(the existing upsert tests only exercised the already-has-a-valid-table
path). The coverage-driven sweep (§5) is what surfaced it.

**Proof:** added two regression tests (`harness_test.go`) — both RED
before the fix, GREEN after:
- `TestProgressIndexTableBlockHeaderIsRecognized` — the generated header
  must round-trip through `isProgressHeaderLine`.
- `TestUpsertProgressIndexRowReplacesSameDocInsteadOfDuplicating` — a
  same-doc re-upsert must keep a single table.

**Fix:** one line — restored the header to the correct UTF-8 columns
(verified at the byte level; the terminal/tooling mangles Chinese
display, so I checked `.encode("utf-8").hex()` rather than trusting the
render). Diff is surgical: 1 production line + the two tests.

Reviewer note: the fix landed as raw UTF-8 Chinese rather than the file's
usual `\u`-escape convention (my tooling kept interpreting typed `文`
as the character). It is functionally identical and byte-matches
`isProgressHeaderLine`. If you prefer `\u` escapes for corruption-
resistance, that's a trivial follow-up — flagging it for your call.

---

## 4. OPEN — needs owner decision: `harness.go:1116`

The **same corruption affected a second line**: the `## ` section heading
inserted by `upsertProgressIndexRow` when appending a table to a doc that
already has non-table content. It is mojibake **and** has a mangled
newline (`…硵n\n` — a literal `n` before the newline).

I did **not** fix it because **I don't know the intended heading text**
(unlike the table header, which is fully determined by
`isProgressHeaderLine`). The core re-detection bug is already resolved by
`2ac9974`; this residual is a garbled section title on a narrower path.

**Action needed:** tell me the intended heading text (and confirm the
trailing should be `\n\n`), and I'll fix + test it. A repo-wide check
found these were the **only two** corrupted raw-CJK lines in
`harness.go`; the rest use `\u` escapes and are intact.

---

## 5. Test-coverage hardening (test-only — no production change)

Coverage-driven sweep across B-line packages. Each commit adds tests
only; each guards behavior where a regression would be a real bug
(governance state machines, billing data quality, parser robustness) —
not trivial getters.

| Package | Coverage | Commit | What was guarded |
|---|---|---|---|
| `domain/processsink` | 73.5% → 85.7% | `9c9d0f1` | LLM summary-override daily rollup; empty-content → placeholder checkpoint |
| `domain/drafts` | 75.9% → 93.1% | `f6006b1` | `New` required-field validation; wrong-state / missing-actor lifecycle guards |
| `persona` | 77.1% → 92.7% | `ba9e1c8`, `14029dd` | `extractJSONObject` code-fence/prose lenience; `NormalizeCandidateState`, `NewCandidateID` |
| `app/findings.go` | ~0% → ~97% | `5101641` | Resolve/Ignore state change + audit record + rune-safe audit-ID |
| `app` import identity | — | `1aa2af6`, `5f5d017` | External-import override+normalization; process-sink `Purpose` stamping |
| `domain/docclass` | 83.6% → 92.5% | `86da720` | `NewClassifier` rule validation; path normalization (backslash, `.`/`./`, `..`, case) |
| `orchestrator` | 81.8% → ~83% | `8ff31d1`, `f5ea0fc` | Draft review-action `ErrDraftNotReady` precheck; `SupersedeDraft` input/kind validation |
| `adapter/codexjsonl` | 74.3% → 80.4% | `572e0f4`, `f88f38f` | Timestamp parsing forms (`parseUnixish` 17.6%→100%); `sanitizeAgentID` slug normalization |

---

## 6. Commit list (oldest → newest)

```
8bf9a22 feat(model,store,cli): break usage totals down by model
470eef6 docs(b-line): handoff for usage by-model breakdown
01fbf00 feat(cli): show hidden-model token total in usage report tail
1bc990b docs(b-line): note hidden-model tail-token total in usage handoff
752eada test(cli): guard cross-day by_model usage aggregation
06be052 docs(b-line): record cross-day by_model guard test in usage handoff
b491d45 test(cli): guard usage idle-window output on both surfaces
0b3fec0 docs(b-line): record idle-window usage guards in handoff
7c30189 feat(cli): show each purpose's share of total tokens in usage report
62ffd72 docs(b-line): record purpose token-share in usage handoff
5f5d017 test(app): assert process-sink usage stamps Purpose
0cb4f6b docs(b-line): note process-sink Purpose guard in usage handoff
5356db6 feat(cli): roll up total spend per model across purposes
65a0ee8 docs(b-line): document cross-purpose model rollup in usage handoff
9c9d0f1 test(processsink): cover summary-override rollup and placeholder checkpoint
f6006b1 test(drafts): cover creation validation and lifecycle guards
ba9e1c8 test(persona): cover extractJSONObject output-lenience branches
14029dd test(persona): cover candidate-state default and ID generation
5101641 test(app): cover finding state-change governance and audit
1aa2af6 test(app): cover external-import identity override and normalization
2ac9974 fix(orchestrator): repair corrupted progress-index table header   <-- production fix
8ff31d1 test(orchestrator): guard draft review-action state precheck
f5ea0fc test(orchestrator): guard SupersedeDraft input and kind validation
86da720 test(docclass): cover rule validation and path normalization edges
572e0f4 test(codexjsonl): cover transcript timestamp parsing forms
f88f38f test(codexjsonl): cover sanitizeAgentID slug normalization
```

---

## 7. Boundaries & process

- **B-line only.** No A-line (LLM config / model profiles), TUI, MCP, or
  CI-gate files were modified. The working tree carries other lines'
  uncommitted WIP (13 entries under `internal/tui`, `internal/cli/
  tui_workbench*`, `README.md`, `scripts/release-gate.ps1`, and untracked
  TUI handoff docs); none of it was staged, reverted, or touched.
- **Per-subtask commits**, each verified green at commit time; staged by
  explicit file path (never `git add -A`).
- **Not pushed** — awaiting this review + CI per the B-line workflow.
- One release-gate subitem for the usage feature is staged as *text* in
  the usage handoff doc (§"Release-gate subitem") for the CI-file owner,
  because `scripts/release-gate.ps1` is dirty with A-line WIP.

## 8. Suggested review focus

1. `2ac9974` — confirm the header fix and the round-trip/idempotency
   tests are sound (§3). Decide raw-UTF-8 vs `\u`-escape form.
2. `harness.go:1116` — supply the intended heading text (§4).
3. `cli.go` usage rendering — the only sizeable production diff; check the
   cross-purpose rollup aggregation and the divide-by-zero guard on the
   purpose %-share.
4. `usage_aggregate.go` — the shared bucketing + fallback rules
   (empty Purpose → chat; empty provider/model → unknown) used by all
   three store backends.
