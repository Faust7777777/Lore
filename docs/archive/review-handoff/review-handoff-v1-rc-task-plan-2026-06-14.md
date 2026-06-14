# Task Plan: Lore V1 Development

Archived for V1 RC review on 2026-06-14.

Source: root-level `task_plan.md` scratch plan from the V1 RC stabilization
session. It keeps the detailed phase breakdown for reviewer context. Some
checklist items were superseded after archive capture; use
`docs/v1-rc-status-2026-06-14.md` as the current status source.

## Goal
Implement the user's PM schedule for Lore development, starting with the highest-value V1 path and continuing across turns until the full schedule is complete.

## Current Phase
Phase 13 partially complete locally; two commits are pending remote push because GitHub SSH/HTTPS connectivity is timing out. Remaining Phase 13 items require a configured model endpoint, then Phase 14 RC cut/handoff.

## Phases

### Phase 1: Current State & Gap Audit
- [x] Inspect current branch, git status, and line-ending noise.
- [x] Recover previous documentation findings.
- [x] Map the PM schedule to current implemented code and tests.
- **Status:** complete

### Phase 2: V1.0 Beta Acceptance Gate
- [x] Identify existing deterministic tests/smokes for governed markdown intake, persona candidate acceptance, external transcript import, MCP boundary, and usage.
- [x] Add or update a single V1 Beta acceptance entrypoint without real model dependency.
- [x] Verify it exercises the product's core governance promises.
- **Status:** complete

### Phase 3: Operator Queue / Inbox Surface
- [x] Audit current `lore inbox`, findings, drafts, and persona candidate aggregation.
- [x] Fill the smallest missing CLI/runtime gap before TUI work.
- **Status:** complete

### Phase 4: TUI Review Workflows
- [x] Audit current TUI panels for drafts, candidates, findings, usage, and model status.
- [x] Implement next missing TUI-backed workflow only after runtime/CLI contracts are stable.
- **Status:** complete

### Phase 5: Governance Hardening
- [x] Review conflict, audit failure, apply failure, and post-scan finding semantics against V1 release criteria.
- [x] Add regression tests and implementation for any missing hard blocker.
- **Status:** complete

### Phase 6: External Contract Freeze
- [x] Lock live MCP v1 vs SDK v0 documentation and contract verification.
- [x] Ensure examples and onboarding docs match implemented tools.
- **Status:** complete

### Phase 7: TUI Pending Action Queue Narrow Slice
- [x] Confirm current TUI approval pane already supports draft list/detail approve/reject/apply.
- [x] Identify the real remaining gap: shell confirmations live only in `console.Session.PendingShellCommand`, not in the TUI approval pane.
- [x] Add a session pending-action projection for shell confirmation.
- [x] Render pending shell action in the TUI approval pane and route `a/r` through the existing confirm/cancel session input path.
- [x] Add release-gate coverage.
- **Status:** complete

### Phase 8: TUI Usage Visibility
- [x] Add TUI view-model fields for today's usage summary and soft-warning threshold.
- [x] Load today's usage in `loadWorkbenchViewModel` through a narrow optional `SummarizeUsage` interface.
- [x] Render compact Today Usage totals, purpose rows, model rows, and soft-budget warning in the TUI status panel.
- [x] Add targeted CLI/TUI tests and V1 beta release-gate coverage.
- [x] Commit and push `819000c feat: surface usage summary in tui`.
- **Status:** complete

### Phase 9: Post-Scan Reconciliation Decision
- [x] Product decision: V1 reconciliation is persisted open findings surfaced through operator queue / inbox / TUI findings review.
- [x] Defer automatic conflict markers or draft creation for governed out-of-band edits; that is a richer post-V1 reconciliation path.
- [x] Add acceptance coverage proving a post-scan governed finding enters `OperatorQueue`.
- [x] Add acceptance coverage proving resolving the finding removes it from `OperatorQueue`.
- [x] Add V1 beta release-gate coverage.
- [x] Commit and push `beca9e3 test: gate post-scan finding reconciliation`.
- **Status:** complete

### Phase 10: External Transcript V1 Boundary
- [x] Product decision: keep Lore external transcript import process-sink-only for V1.
- [x] Defer automatic persona extraction from imported external transcripts until it has an explicit opt-in design and abuse/fact-governance review.
- [x] Make release gate explicitly pin `TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly`.
- [x] Run release gates, commit, and push `a5b8deb test: pin external transcript import boundary`.
- **Status:** complete

### Phase 11: RC Documentation and Status Cleanup
- [x] Decide V1 RC should be split into four remaining phases: docs/status cleanup, gate/flake cleanup, manual smoke, and RC cut/handoff.
- [x] Avoid editing older status/handoff docs that currently show unrelated line-ending noise.
- [x] Add a canonical RC status document with current scope, completed slices, remaining phases, known caveats, deferred items, and estimate.
- [x] Verify the documentation diff and commit it as `2958690 docs: record v1 rc phase plan`.
- [ ] Push pending because GitHub SSH 443 and HTTPS connectivity timed out.
- **Status:** complete locally; push pending

### Phase 12: RC Gate and Flake Cleanup
- [x] Reproduce `TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds` flake with `-count=50`.
- [x] Identify root cause: claim-conflict losers can recheck after the winner links a draft and incorrectly return as idempotent nil-success.
- [x] Add deterministic regression coverage for conflict-after-peer-linked.
- [x] Fix `CreatePersonaDraftFromCandidate` so claim-conflict Drafted rechecks return `ErrPersonaCandidateAlreadyDrafted`, while ordinary later retries remain idempotent through the fast pre-check.
- [x] Verify package tests, repeated flaky test, V1 beta gate, and default release gate.
- [x] Commit locally as `908ff35 fix: make persona candidate claim conflicts deterministic`.
- [ ] Push pending because GitHub SSH 443 and HTTPS connectivity timed out.
- **Status:** complete locally; push pending

### Phase 13: Manual Smoke and Operator Workflow
- [x] Build a temporary `lore-smoke` binary under `tmp/v1-rc-smoke-2026-06-14-phase13`.
- [x] Bootstrap a disposable workdir and verify managed core docs/status render.
- [x] Verify empty `inbox --json` and `usage` render.
- [x] Run `demo-p0a` and confirm an applied governed draft appears in draft list.
- [x] Simulate governed out-of-band persona edit, run daemon one-shot baseline, then run a second one-shot scan to create a governance finding.
- [x] Verify the finding enters `inbox --json`.
- [x] Resolve the finding and verify `inbox --json` returns to zero action items.
- [ ] Re-run model-dependent smoke after configuring `LORE_LLM_BASE_URL` and `LORE_LLM_API_KEY`: external transcript import, `smoke p0` P0-B, and `tui --once`.
- **Status:** partially complete; model-dependent items pending

## Estimate
- V1 release-candidate scope: about 2-3 focused working days remaining.
- Full expanded scope including external transcript persona extraction, long-running app-server attach, and media/binary extraction: about 1-2 weeks.

## Decisions Made
| Decision | Rationale |
|---|---|
| Start with V1.0 Beta acceptance gate | The schedule's first milestone is proving the core product loop; many capabilities already exist but need a repeatable gate. |
| Do not touch TUI first | TUI is Opus-owned and should consume stable runtime/CLI contracts; backend acceptance comes first. |
| Work in current checkout for now | The repo has many line-ending-only modified files; no linked worktree is active. Avoid creating a new worktree without user consent. |
| Use Claude as a bounded subagent | User explicitly asked to use Claude; assign it disjoint file ownership and review its changes before integrating. |
| Do not use Claude for Phase 8 | User later said Claude has other work; Phase 8 was implemented directly by Codex. |
| V1 post-scan reconciliation is open findings | The implemented product already audits out-of-band changes, persists findings, surfaces them in queue/TUI, and supports resolve/ignore. Auto-draft/conflict artifacts would expand governance semantics and are deferred. |
| V1 external transcript import stays process-sink-only | Imported external-agent transcripts can contain third-party or stale claims. Persona extraction should remain local-chat-only until there is an explicit opt-in governance design. |

## Errors Encountered
| Error | Attempt | Resolution |
|---|---|---|
| Existing planning files described the completed documentation-review task | 1 | Replaced `task_plan.md` with a development-focused plan while preserving key findings in `findings.md`. |
| PowerShell/Go/Claude CLI invocation errors during setup | 1 | Logged details in `progress.md`; continue with corrected command forms. |

## Notes
- Current branch: `b-line/audit-followups-2026-06-04`.
- `git diff --ignore-space-at-eol --stat` is empty; tracked modifications appear to be line-ending noise.
- `findings.md` still contains product/documentation findings from the previous review pass.
- First TDD red: `/usr/bin/go test ./cmd/obsidian-harness -run TestReleaseGateHasV1BetaAcceptanceSwitch -count=1 -v` failed because `release-gate.ps1` lacks `[switch]$V1BetaAcceptance`.
- V1 Beta acceptance gate now exists as `scripts/release-gate.ps1 -V1BetaAcceptance`. It runs a focused deterministic set across MCP boundary, governed markdown intake, persona candidate acceptance, external transcript process-sink import, inbox/operator queue, and usage purpose/model reporting.
- `lore inbox --json` now has direct command-path test coverage via `TestRunInboxJSONCommand`.
- External transcript import now has `TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly`, pinning that import writes only process-sink artifacts and creates no drafts, findings, persona candidates, or ordinary notes.
- Phase 4 used Claude CLI with `--model claude-opus-4-8` as a code-writing subagent. It identified findings list-view quick actions as the smallest TUI workflow gap and implemented `x=resolve` / `i=ignore` from the findings list, guarded to open findings only.
- The default release gate's TUI guardrail regex now includes `FindingsListQuick(ResolveAction|IgnoreAction|ActionNoOpOnNonOpenState)`.
- Phase 5 used Claude CLI with stdout/stderr redirected to `tmp/claude-phase5-governance.log` and structured report written to `tmp/claude-phase5-governance-report.md`.
- Phase 5 fixed `markDraftConflicted` so a conflict-state persist failure is no longer hidden behind a clean `store.ErrConflict`; the returned error now preserves `errors.Is(store.ErrConflict)` and wraps the underlying persist failure.
- The default release gate now includes an `orchestrator apply failure-semantics guardrails` group covering `TestMarkDraftConflictedSurfacesStatePersistFailure`.
- Phase 6 used Claude CLI with stdout/stderr redirected to `tmp/claude-phase6-contract-freeze.log` and structured report written to `tmp/claude-phase6-contract-freeze-report.md`.
- Phase 6 added `TestClientSetupDocAvailableToolsMatchV1Contract`, pinning `docs/integrations/mcp-client-setup.md` Available Tools to `docs/contracts/mcp-tools-v1.json`.
- Phase 6 review also fixed the MCP integration example tests so Windows-style example paths such as `C:\path\to\lore.exe` validate correctly when tests run under Linux/WSL.
- The default release gate now includes an `MCP onboarding docs and example configs` group covering external client examples and the new client setup doc contract test.
- Phase 8 adds TUI status-panel usage visibility using existing `Runtime.SummarizeUsage`, without schema changes or CLI behavior changes.
- Phase 8 verification passed:
  - `/usr/bin/go test ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1`
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -V1BetaAcceptance -SkipDiffCheck`
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -SkipDiffCheck`
- Phase 9 pins V1 post-scan reconciliation to open findings surfaced through queue/TUI, with richer conflict/draft artifacts deferred.
