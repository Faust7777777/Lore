# Persona Memory Manual Test Cookbook

Step-by-step manual test of the persona memory candidate pipeline
with a real LLM. Aimed at the product owner (single-user). Targets
B-P1 through B-P11c end-to-end. Designed to be copy-pasted into
PowerShell on Windows; equivalents for macOS/Linux are noted where
they differ.

When this cookbook and the pipeline overview at
`docs/persona-memory-pipeline.md` disagree, trust the pipeline
overview (it is the contract anchor) and update this cookbook.

## Flag ordering

All `lore persona candidates` subcommands accept flags either before
OR after the positional candidate / draft ID. Both of these work
identically:

```powershell
lore persona candidates show --workdir $work --json $id
lore persona candidates show --workdir $work $id --json   # same thing
```

The cookbook examples below use flag-first for readability; the
ID-first form is fine when you are pasting an ID from a previous
output.

## 0. Prerequisites

- A working `lore` build on PATH (`go build -o lore ./cmd/lore` if
  not already done).
- An OpenAI-compatible LLM endpoint reachable from the box. Either:
  - `LORE_LLM_BASE_URL`, `LORE_LLM_API_KEY`, `LORE_LLM_MODEL`, or
  - the legacy `OBSIDIAN_HARNESS_LLM_*` triple.
- About 10 minutes of human attention and ~10 LLM calls of budget.
- PowerShell 5.1+ on Windows. The same commands work in pwsh and in
  bash with minor variable-syntax tweaks (use `$env:VAR` in
  PowerShell, `VAR` in bash).

## 1. Set up an isolated workdir

```powershell
$work = "$env:USERPROFILE\Desktop\lore-manual-test"
Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $work | Out-Null

# Confirm env is wired (replace with your own values; the LORE_*
# triple is preferred since OBSIDIAN_HARNESS_LLM_* is legacy):
$env:LORE_LLM_BASE_URL  = "<your endpoint>"
$env:LORE_LLM_API_KEY   = "<your key>"
$env:LORE_LLM_MODEL     = "<your model>"   # gpt-5.4 etc.

# Bootstrap the vault / store layout.
lore status $work
```

**Expected**: status output prints the managed core layout, with no
drafts and no audit entries yet. If it errors with "LLM is not
configured", the env vars above did not take.

## 2. Mine a few persona candidates with a real chat

Open an interactive console session and say 3-4 sentences that
contain durable personal facts. Each fact should be self-contained
("I major in economics") rather than a verb phrase ("I'm reviewing").
The extractor needs a noun-form claim it can lift verbatim.

```powershell
lore console $work
# Inside the prompt, type these one at a time:
#   > 我每周三晚上都会复盘英语听力错题。
#   > 我习惯早上六点起床写代码。
#   > 我比较抗拒频繁切换工具栈。
# Press Ctrl+D (or type /exit) to close.
```

**Expected**: each turn returns Lore's normal response promptly
(extraction is async; the chat is not blocked). The shell prints a
"draining persona extractions" line at exit if the goroutines were
still mid-flight.

## 3. Inspect the mined candidates

```powershell
lore persona candidates list --workdir $work
```

**Expected**: a Persona Candidates table with State=open and one row
per stable fact you said (probably 2-3 rows). Each row shows ID,
FIELD (e.g. `study_routine`), PROPOSED (the extracted value),
CONFIDENCE, CONFLICT (`no`), SOURCE (`console`), and DRAFT (`-`
since none has been promoted yet).

**If the list is empty**, the LLM either timed out, the parser
discarded all candidates, or the store write failed. Diagnose with:

```powershell
lore persona errors --workdir $work --tail 20
```

You should see one of:

- `stage=extract` — the LLM call itself failed. If
  `context deadline exceeded`, bump the timeout:
  `$env:LORE_LLM_PERSONA_EXTRACT_TIMEOUT = "30s"` and repeat
  Step 2.
- `stage=parse_warning` — the LLM produced candidates that the
  parser refused (paraphrased evidence, low confidence, no match
  against your text). Repeat the utterance more literally.
- `stage=store` — sqlite write failed. Rare; usually disk full.

## 4. Look at one candidate in detail

Pick the first ID from the list (it starts with `pc-`):

```powershell
$id = "<paste the pc-... ID here>"
lore persona candidates show --workdir $work $id
```

**Expected**: a Persona Candidate block with State=open, the Field
/ Proposed value / Evidence / Reason / Confidence / Source /
Observed at lines populated, and NO `Draft:` line (the candidate
is not yet linked to a draft).

The Evidence line should contain a verbatim substring of what you
said. If it has been paraphrased, treat the candidate as suspect.

## 5. Promote a candidate to a draft

```powershell
lore persona candidates draft --workdir $work $id
```

**Expected**: an Updated block with `Action: draft`,
`State:  drafted`, and `Draft:  draft-<unix-nano>
(pending_review; use lore draft review draft-...)`.

Confirm the draft exists in the review queue:

```powershell
lore draft list --workdir $work
```

**Expected**: at least one `persona_update` draft in
`pending_review`.

Confirm `show` now displays the link:

```powershell
lore persona candidates show --workdir $work $id
```

**Expected**: same block as Step 4 plus a `Draft: draft-<id>` line
right after `State`.

## 6. Verify idempotent retry

Re-run the promote on the same id:

```powershell
lore persona candidates draft --workdir $work $id
```

**Expected**: the same `Draft:` id as Step 5 (not a new one), and
`lore draft list` still shows exactly one `persona_update` draft.
If you see two, file a regression: the P5+P6 ClaimCandidateForDraft
CAS or the P8 round-2 ClaimCandidateForRetry CAS has broken.

## 7. Reject the draft, then retry-rejected

```powershell
$draftID = "<paste the draft-... ID from Step 5>"
lore draft review --workdir $work $draftID    # observe the proposal
lore draft reject --workdir $work $draftID
```

**Expected**: the draft state transitions to `rejected`.

Now reissue the draft via the retry-rejected path:

```powershell
lore persona candidates draft --workdir $work --retry-rejected $id
```

**Expected**: a NEW `Draft:` id, distinct from the rejected one. The
old draft stays in store as `rejected` (audit history). The
candidate's DraftID now points at the new draft.

```powershell
lore draft list --workdir $work
```

**Expected**: exactly two `persona_update` drafts:
1. the original (state `rejected`)
2. the retry (state `pending_review`)

## 8. Exercise dismiss on a different candidate

Pick another `pc-...` id from `list` and dismiss it without
drafting:

```powershell
$id2 = "<another pc-... id>"
lore persona candidates dismiss --workdir $work $id2
lore persona candidates list --workdir $work --state dismissed
```

**Expected**: the dismissed candidate appears with `State:
dismissed`. `list --state open` no longer shows it. The DedupKey
remains, so repeating the source utterance in a new chat session
will NOT re-emerge as a new candidate (the dismissed row is a
dedup tombstone).

## 9. Exercise partial-orphan recovery (optional, requires manual store edit)

This branch is rarely hit in normal use but is the operator's
escape hatch when `LinkCandidateDraft` fails after a successful
proposal. To simulate, use a sqlite client to manually clear a
drafted candidate's DraftID, then:

```powershell
# Force the partial-orphan shape on a drafted candidate (using
# sqlite3 or the Go runtime directly), then:
lore persona candidates show --workdir $work $partialID
# Expect: State=drafted, no Draft: line (DraftID empty).

# Locate the orphan draft via `lore draft list` and recover:
lore persona candidates recover --workdir $work --link <orphan-draft-id> $partialID
# OR abandon entirely:
lore persona candidates recover --workdir $work --force-dismiss $partialID
```

**Expected** after `recover --link`: the candidate is back in
`drafted` with the supplied DraftID populated.

**Expected** after `recover --force-dismiss`: the candidate is
`dismissed`, DraftID stays empty.

## 9a. One-page dashboard

```powershell
lore persona summary --workdir $work
```

**Expected**: a `Persona Memory Summary` block listing candidate
counts per state (with `drafted` split into `linked` and
`partial-orphan`), then an `Extract log:` section with total
entries, per-stage counts (`extract` / `store` / `parse_warning`,
plus `(malformed)` for any lines that fail to parse), and the
RFC3339 timestamp of the most recent entry. A fresh workdir shows
all zeros and `(no log file yet)`. Use this command at any point
during the test to get a single-glance health check without
running `list` three times plus `errors --tail`.

For a scriptable health check that returns a distinct exit code
when an orphan is present:

```powershell
lore persona summary --workdir $work --fail-on-orphan
# exit 0 when clean
# exit 2 when at least one partial-orphan candidate exists
# exit 1 on real command error (open runtime, list query, etc.)
```

The dashboard always prints first; the exit-code signal is
appended via stderr so the operator can see the numbers
regardless of the script's branch. Suitable for CI gates or
periodic health pings.

For programmatic consumers (monitoring exporters, alert pipes)
the dashboard also has a stable JSON shape:

```powershell
lore persona summary --workdir $work --json | ConvertFrom-Json
# stdout: {"workdir": "...", "candidates": {"open": ..., "drafted_linked": ..., ...}, "extract_log": {...}}
```

The two flags compose: `--json --fail-on-orphan` keeps stdout
parseable while stderr + exit code carry the check signal, so a
script can `jq` the snapshot AND branch on the exit code in one
invocation.

## 10. Look at the cost split

```powershell
lore usage --workdir $work
```

**Expected**: a daily table with one row for today, followed by a
"By purpose:" block that lists `chat`, `persona_extract`, and
possibly `process_sink`, each with their own call count + token
totals. The sum of the buckets reconciles against the top-line
Total.

## 11. Look at all error log lines

```powershell
lore persona errors --workdir $work --tail 50
```

**Expected**: if any extraction failed during the session, those
lines appear here. If no failures occurred, the output ends with
"No matching entries" or "No log file yet".

Useful filters:

- `--stage extract` — LLM transport / context errors
- `--stage store` — sqlite write errors
- `--stage parse_warning` — parser refusal
- `--since 10m` — only the last 10 minutes
- `--tail 5` — only the most recent 5
- `--json` — stable-shape object for monitoring consumers
  (`{log_path, filter, summary, entries[]}`)

## Acceptance criteria

The manual test passes when:

- Step 2 + 3 produced at least one open candidate per chat turn
  that named a noun-form fact (some turns may legitimately produce
  zero because the parser correctly rejected paraphrase / low
  confidence — verify via Step 11).
- Step 5 transitioned the candidate to `drafted` with a fresh
  `draft-<unix-nano>` id and added a `persona_update` row to
  `lore draft list`.
- Step 6 produced no duplicate drafts.
- Step 7 produced exactly two persona_update drafts: original
  rejected + retry pending_review.
- Step 8 left the dismissed candidate in `lore persona candidates
  list --state dismissed` and absent from `--state open`.
- Step 10 shows `persona_extract` as a distinct bucket in `lore
  usage`.
- No CLI command returned an unhandled panic or a misleading error
  (e.g. "open runtime" for a missing log file).

If any criterion fails, capture the workdir + the persona-extract
log lines + the failing command output and file a regression.

## Troubleshooting

| Symptom | Likely cause | Action |
|---|---|---|
| Step 2 chat itself errors out | LORE_LLM_* env not set or unreachable endpoint | `Get-ChildItem env:LORE_*`; ping the endpoint URL manually |
| Step 3 list is empty but Step 2 chat worked | extractor timed out, parser discarded, or store error | `lore persona errors --tail 20` to disambiguate |
| Step 5 draft errors with `ErrPersonaCandidateAlreadyDrafted` | candidate is in partial-orphan from a prior crashed run | follow Step 9 to recover |
| Step 7 retry-rejected errors with `ErrPersonaDraftNotTerminalForRetry` | linked draft is still pending_review or already approved | review/reject it first, then retry |
| `lore usage` shows huge token spend on persona_extract | extractor calling LLM on every turn even when no facts | check the prompt — short-circuit utterances ("ok", "?") should produce zero candidates with no log warnings; if every utterance fires, file a bug |
