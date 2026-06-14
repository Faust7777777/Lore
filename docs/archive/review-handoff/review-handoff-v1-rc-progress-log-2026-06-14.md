# Progress Log

Archived for V1 RC review on 2026-06-14.

Source: root-level `progress.md` scratch log from the V1 RC stabilization
session. This preserves command history, phase notes, and transient failures.
Some entries describe state before later commits or pushes; use
`docs/v1-rc-status-2026-06-14.md` as the current status source.

## Session: 2026-06-13

### Phase 1: Scope & Inventory
- **Status:** complete
- **Started:** 2026-06-13
- Actions taken:
  - Initialized planning files for Lore product documentation review.
  - Ran planning session catchup; no output indicated no recoverable previous context.
  - Read repository `AGENTS.md` and inventoried `docs/` tree.
  - Identified 183 documentation files under `docs/`.
  - Read current product definition docs: `README.md`, `DEVELOPMENT_STATUS.md`, `docs/adr-lore-v1-architecture.md`, `docs/compact-handoff-lore-v1.md`, and `docs/review-handoff-index.md`.
  - Read MCP client setup, SDK ADR/README, integration examples, and both MCP contract artifacts.
  - Read external governed-intake plan through completion.
  - Read current and archived handoffs for proposal-only MCP, persona proposal, markdown note proposal/apply/supersede, CoreContext, governed-note smoke, out-of-band post-scan, findings CLI, external transcript import, B-line persona pipeline, TUI backend, usage by model, and cross-boundary review.
  - Read task/turn, session resume, persona acceptance, config enforcement, failure semantics, full-project review, and open-source harness comparison docs.
  - Rechecked MCP/SDK contract names with `jq -r '.[].name'`.
- Files created/modified:
  - `task_plan.md`
  - `findings.md`
  - `progress.md`

### Phase 2: Core Product Model
- **Status:** complete
- Key result:
  - Lore is a local-first governed Obsidian knowledge-operations harness. Its durable product value is proposal/review/apply governance, auditability, recovery, and context supply around a managed vault.

### Phase 3: Integration Surfaces
- **Status:** complete
- Key result:
  - External MCP is stdio read + proposal intake only. Live contract has 11 tools; SDK typed contract has 9 read-only tools.
  - External transcript import is CLI/process-sink one-shot, not MCP and not formal note/persona mutation.

### Phase 4: Feature Lines & Current State
- **Status:** complete
- Key result:
  - Persona, usage, task/turn, session resume, markdown proposal, CoreContext, post-scan, and findings flows have documented shipped slices and remaining owner-boundary/deferred items.

### Phase 5: Synthesis
- **Status:** complete
- Result:
  - Final answer prepared with current contracts, workflows, boundaries, shipped state, gaps, and recommended product questions.

## Test Results
| Test | Input | Expected | Actual | Status |
|------|-------|----------|--------|--------|

## Error Log
| Timestamp | Error | Attempt | Resolution |
|-----------|-------|---------|------------|
| 2026-06-13 | Inventory command had mismatched shell quote | 1 | Logged error; continue with narrower commands instead of repeating exact command |
| 2026-06-13 | `jq '.tools[].name'` failed because MCP contract JSON root is an array | 1 | Re-ran as `jq '.[].name'` and recorded the correct tool names |
| 2026-06-13 | `powershell.exe -File .\\scripts\\release-gate.ps1` from WSL did not resolve the relative path correctly | 1 | Use `wslpath -w` for Windows PowerShell paths or avoid PowerShell for static tests |
| 2026-06-13 | `powershell.exe` did not reject the unknown `-V1BetaAcceptance` switch and started the default gate | 1 | Killed the accidental gate process and switched to a static Go test for script contract |
| 2026-06-13 | Used Windows-style Go path from WSL | 1 | Found `/usr/bin/go` and `/mnt/c/Users/15892/.local/go/bin/go.exe`; use `/usr/bin/go` from WSL |
| 2026-06-13 | Claude `--allowedTools` consumed the prompt argument in non-interactive mode | 1 | Feed prompt through stdin with `claude --print` |

## 5-Question Reboot Check
| Question | Answer |
|----------|--------|
| Where am I? | Complete |
| Where am I going? | Final response |
| What's the goal? | Detailed Chinese product understanding of Lore integration/product docs |
| What have I learned? | Product mission, governance boundaries, integration contracts, current implementation/gaps, and MCP proposal-only direction |
| What have I done? | Created planning files, read current integration/handoff docs, updated findings, and verified contract tool names |

## Session: 2026-06-14

### Post-push continuation / next schedule slice discovery
- Pushed commit `9b35d22 test: add v1 acceptance gate hardening` to `origin/b-line/audit-followups-2026-06-04`.
- Rechecked remaining worktree:
  - 63 tracked files are whitespace / line-ending noise only (`git diff --ignore-space-at-eol --quiet` exit 0; `git diff -w --quiet` exit 0).
  - `findings.md`, `progress.md`, and `task_plan.md` remain local planning scratch files.
- Re-read `DEVELOPMENT_STATUS.md`, `docs/adr-lore-v1-architecture.md`, and `docs/compact-handoff-lore-v1.md`.
- Product gap correction:
  - Status docs still say the TUI pending approval queue is a placeholder.
  - Current code has already advanced beyond that: `internal/tui` can list reviewable drafts, show draft detail, approve/reject pending drafts, and apply approved drafts.
  - The still-open schedule item is more accurately: add a unified runtime/session pending-action queue so shell confirmations and future approval actions are visible in the TUI approval pane instead of only existing as `console.Session.PendingShellCommand`.
- Current candidate next slice:
  - Add a narrow pending-action projection for shell confirmation.
  - Feed it through `loadWorkbenchViewModel`.
  - Render it in the right-bottom approval area alongside/above draft approvals.
  - Let TUI keys approve/cancel the shell action by sending the existing confirmation words through `Session.HandleContext`, preserving current shell execution semantics.
- Implementation result:
  - Added `tui.PendingActionInfo` and `WorkbenchViewModel.PendingActions`.
  - Added `console.Session.PendingActions()` to expose a read-only shell confirmation projection.
  - `loadWorkbenchViewModel` now copies pending actions from the session and records `Snapshot.PendingActions`.
  - TUI approval pane renders a `Pending Actions` section for shell confirmations.
  - Approval pane keys route `a` to `confirm` and `r` to `cancel` through the existing `InteractiveWorkbenchDriver.Execute` path, so shell confirmation semantics remain centralized in `console.Session.HandleContext`.
  - Default release gate now includes both the TUI render/state-machine tests and the CLI workbench view-model projection test.
- Verification:
  - First TDD red:
    - `/usr/bin/go test ./internal/tui -run 'TestRenderApprovalPaneWithPendingShellAction|TestApprovalFlowPendingShellAction' -count=1` failed because pending action type/view model fields did not exist.
    - `/usr/bin/go test ./internal/cli -run 'TestLoadWorkbenchViewModelIncludesPendingShellAction' -count=1` failed because the view model had no pending action projection.
    - `/usr/bin/go test ./cmd/obsidian-harness -run 'TestReleaseGateIncludesPendingActionApprovalPaneCoverage' -count=1 -v` failed until release gate coverage was added.
  - `/usr/bin/go test ./internal/tui ./internal/cli ./internal/console ./cmd/obsidian-harness -count=1` passed.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -SkipDiffCheck` passed and printed `[gate] release gate passed`.

### Phase 2: V1.0 Beta Acceptance Gate
- **Status:** complete
- Actions taken:
  - Added `TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly` to pin that external transcript import remains process-sink only: checkpoint/report paths must live under `Config.Paths.ProcessSinkDir`, and the import must not create drafts, findings, persona candidates, or ordinary `03-notes` content.
  - Added `TestRunInboxJSONCommand` to cover the real `lore inbox --json <workdir>` command path, including CLI dispatch, flag parsing, runtime open, and stable empty-array JSON output.
  - Added `TestReleaseGateHasV1BetaAcceptanceSwitch` as a static script contract for the release gate.
  - Added `-V1BetaAcceptance` and `Invoke-V1BetaAcceptanceGate` to `scripts/release-gate.ps1`.
  - Made `-V1BetaAcceptance` focused: it runs the V1 Beta deterministic acceptance set and returns without executing the default full release gate.
- Verification:
  - `/usr/bin/go test ./internal/app -run 'TestRuntimeImportExternalTranscriptJSONL' -count=1 -v` passed.
  - `/usr/bin/go test ./cmd/obsidian-harness -run 'TestRunInboxJSONCommand|TestReleaseGateHasV1BetaAcceptanceSwitch' -count=1 -v` passed.
  - `/usr/bin/go test ./cmd/obsidian-harness -run 'TestRunImportExternalJSONL|TestRunPersonaMemoryCandidateAcceptanceScaffold' -count=1 -v` passed.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -V1BetaAcceptance -SkipDiffCheck` passed and printed `[gate] release gate passed`.

### Phase 3: Operator Queue / Inbox Surface
- **Status:** complete
- Key result:
  - Existing runtime aggregation and internal CLI render/JSON tests were already present.
  - The missing direct command-path coverage is now filled by `TestRunInboxJSONCommand`.

### Next
- Phase 4 starts with auditing TUI review surfaces for drafts, persona candidates, findings, process-sink, usage/model status, and deciding the smallest runtime-backed TUI workflow gap.

### Phase 4: TUI Review Workflows
- **Status:** complete
- Subagent:
  - Invoked Claude CLI with `--model claude-opus-4-8`, `--permission-mode bypassPermissions`, and a bounded no-commit task.
  - Claude took several minutes with no intermediate stdout, then returned exit code 0 and a final report.
- Actions taken:
  - Audited TUI review surfaces.
  - Identified the smallest asymmetry: findings detail view allowed `x=resolve` / `i=ignore`, but findings list view only allowed navigation and detail entry.
  - Added findings list-view quick actions: `x` resolves and `i` ignores the selected open finding.
  - Added contextual findings list footer hint: `x=resolve i=ignore` only for open selected findings.
  - Added tests:
    - `TestFindingsListQuickResolveAction`
    - `TestFindingsListQuickIgnoreAction`
    - `TestFindingsListQuickActionNoOpOnNonOpenState`
  - Added those tests to the default release gate's TUI guardrail regex.
- Verification:
  - `/usr/bin/go test ./internal/tui -count=1 -v` passed.
  - `/usr/bin/go vet ./internal/tui` passed.
  - `/usr/bin/go test ./internal/tui -run 'Test(ApprovalFlow_|InteractiveWorkbenchViewDoesNotRefreshContent|RenderInteractiveConversationShowsTaskSteps|RenderTaskStepsArgSummary|RenderTaskStepsTruncatesObservation|RenderTaskStepsErrorStep|RenderTaskStepsNonErrorLastOutputNotShown|FindingsOffsetUsesFindingsPanelHeight|FindingsListQuick(ResolveAction|IgnoreAction|ActionNoOpOnNonOpenState)|SinkOffsetUsesSinkPanelHeight|ApprovalOffsetUsesApprovalPanelHeight|ModelPanel(ProfilesPersistSelectedProfile|CreateProfileFromPreset)|ModelProfileAndPresetRenderDoNotLeakSecrets|ErrorsCommand(RendersDiagnosticsWithHintsAndNoSecrets|EmptyState))' -count=1 -v` passed.

### Next
- Phase 5 starts with governance hardening: conflict semantics, audit/apply failure handling, and post-scan finding behavior against V1 release criteria.

### Phase 5: Governance Hardening
- **Status:** complete
- Subagent:
  - Invoked Claude CLI with `--model claude-opus-4-8`, stdout/stderr redirected to `tmp/claude-phase5-governance.log`.
  - Required Claude to write a structured report to `tmp/claude-phase5-governance-report.md`.
  - Claude wrote the report before stdout flushed, which confirmed the file-output workflow is useful for long-running calls.
- Audit result:
  - Most mutation paths already followed the failure semantics documented in `docs/review-b-line-failure-semantics-2026-06-04.md`.
  - The smallest missing hardening gap was `markDraftConflicted`: conflict detection attempted `State=conflicted`, but if that state persist failed, the old path returned only `store.ErrConflict` and hid the persist failure.
- Actions taken:
  - Added `TestMarkDraftConflictedSurfacesStatePersistFailure`.
  - Updated `markDraftConflicted` so the error still satisfies `errors.Is(err, store.ErrConflict)` while also wrapping the underlying state persist error and telling the operator the draft still shows approved.
  - Added a default release gate group: `orchestrator apply failure-semantics guardrails`.
- Verification:
  - `/usr/bin/go test ./internal/orchestrator -run 'Test(ApplyDraftEmitsGovernanceFindingWhenStateUpdateFails|ApplyDraftSurfacesFindingSaveFailureInError|MarkDraftConflictedSurfacesStatePersistFailure)$' -count=1 -v` passed.
  - `/usr/bin/go test ./internal/orchestrator -run 'TestMarkDraftConflictedSurfacesStatePersistFailure|TestApplyMarkdownNoteDraftDetects|TestApplyDraft|TestApplyDraftSerializes|TestApplyPersona' -count=1 -v` passed.
  - `/usr/bin/go test ./internal/orchestrator -count=1` passed.
  - `/usr/bin/go vet ./internal/orchestrator` passed.
  - `/usr/bin/go test ./internal/app -count=1` initially hit a one-off failure in `TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds`; the same test then passed with `-count=10`, and a full `/usr/bin/go test ./internal/app -count=1` rerun passed.
  - `/usr/bin/go test ./cmd/obsidian-harness -run 'TestReleaseGateHasV1BetaAcceptanceSwitch' -count=1` passed.

### Next
- Phase 6 starts with external contract freeze: lock live MCP v1 vs SDK v0 docs/contract verification and align examples/onboarding docs with implemented tools.

### Phase 6: External Contract Freeze
- **Status:** complete
- Subagent:
  - Invoked Claude CLI with `--model claude-opus-4-8`, stdout/stderr redirected to `tmp/claude-phase6-contract-freeze.log`.
  - Required Claude to write a structured report to `tmp/claude-phase6-contract-freeze-report.md`.
- Audit result:
  - Live MCP v1 contract artifact and SDK v0 README contract coverage already existed.
  - The smallest freeze gap was the live MCP onboarding doc: `docs/integrations/mcp-client-setup.md` listed v1 tools in an Available Tools table, but no test pinned that table to `docs/contracts/mcp-tools-v1.json`.
- Actions taken:
  - Added `TestClientSetupDocAvailableToolsMatchV1Contract`.
  - Review found full `internal/mcp` failed under Linux/WSL because example config commands use Windows paths like `C:\path\to\lore.exe`; fixed `assertStdioLoreCommand` to normalize backslashes before basename extraction.
  - Added default release gate group `MCP onboarding docs and example configs`.
- Verification:
  - `/usr/bin/go test ./internal/mcp -run 'TestClientSetupDocAvailableToolsMatchV1Contract' -count=1 -v` passed.
  - `/usr/bin/go test ./internal/mcp -run 'Test(ExternalClientExamplesStartLoreMCP|OpenCodeExampleStartsLoreMCP|ClientSetupDocAvailableToolsMatchV1Contract)$' -count=1 -v` passed.
  - `/usr/bin/go test ./internal/mcp -count=1` passed after the path-normalization fix.
  - `cd sdk/go/lore && /usr/bin/go test -run TestREADMEArgumentTableMatchesContractArtifact ./...` passed.

### Next
- All planned phases are complete. Remaining work is release verification, review of the combined diff, then commit/push when requested.

### Release Verification
- **Status:** complete
- Review result:
  - No blocking issue found in the Phase 2-5 diff.
  - Review did find one Phase 6-relevant example-test issue: Windows-style example commands (`C:\path\to\lore.exe`) failed validation under Linux/WSL because `filepath.Base` did not split backslashes. Fixed by normalizing `\` to `/` before basename extraction in `assertStdioLoreCommand`.
- Verification:
  - `git diff --check -- cmd/obsidian-harness internal/app internal/orchestrator internal/tui internal/mcp scripts task_plan.md progress.md` passed.
  - `/usr/bin/go test ./internal/mcp -count=1` passed.
  - `/usr/bin/go test ./internal/orchestrator -count=1` passed.
  - `/usr/bin/go test ./internal/tui -count=1` passed.
  - `/usr/bin/go test ./internal/app -count=1` passed.
  - `/usr/bin/go test ./cmd/obsidian-harness -run 'TestReleaseGateHasV1BetaAcceptanceSwitch' -count=1` passed.
  - `cd sdk/go/lore && /usr/bin/go test -run TestREADMEArgumentTableMatchesContractArtifact ./...` passed.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -V1BetaAcceptance -SkipDiffCheck` passed.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -SkipDiffCheck` passed and printed `[gate] release gate passed`.

### Phase 8: TUI Usage Visibility
- **Status:** complete
- Subagent:
  - None. User asked not to use Claude for now.
- Actions taken:
  - Added `TodayUsage` and `UsageSoftWarningTokens` to `tui.WorkbenchViewModel`.
  - Added a narrow optional `usageSummaryRuntime` interface in the CLI TUI driver so `loadWorkbenchViewModel` can call `SummarizeUsage(day)` without widening `console.Runtime`.
  - Rendered compact Today Usage visibility in the TUI status panel:
    - total calls/tokens;
    - prompt/completion split;
    - top purpose rows;
    - top model rows per purpose;
    - soft-budget warning when today's total crosses `usage.soft_warning_tokens`.
  - Added tests:
    - `TestLoadWorkbenchViewModelIncludesTodayUsage`
    - `TestRenderInteractiveStatusShowsTodayUsageBreakdown`
    - `TestReleaseGateIncludesTUIUsageVisibilityCoverage`
  - Added V1 beta release-gate groups:
    - `V1 beta TUI usage visibility`
    - `V1 beta TUI usage rendering`
  - Committed and pushed `819000c feat: surface usage summary in tui`.
- Verification:
  - First TDD red:
    - `/usr/bin/go test ./internal/cli -run TestLoadWorkbenchViewModelBuildsSnapshotFromRuntimeAndSession -count=1` failed on missing `WorkbenchViewModel.TodayUsage`.
    - `/usr/bin/go test ./internal/tui -run TestRenderInteractiveStatusShowsTodayUsageBreakdown -count=1` failed on missing `TodayUsage` / `UsageSoftWarningTokens`.
    - `/usr/bin/go test ./cmd/obsidian-harness -run TestReleaseGateIncludesTUIUsageVisibilityCoverage -count=1` failed because `release-gate.ps1` lacked the TUI usage gate.
  - `/usr/bin/go test ./internal/cli -run TestLoadWorkbenchViewModelIncludesTodayUsage -count=1` passed.
  - `/usr/bin/go test ./internal/tui -run TestRenderInteractiveStatusShowsTodayUsageBreakdown -count=1` passed.
  - `/usr/bin/go test ./cmd/obsidian-harness -run TestReleaseGateIncludesTUIUsageVisibilityCoverage -count=1` passed.
  - `/usr/bin/go test ./internal/tui ./internal/cli ./cmd/obsidian-harness -count=1` passed.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -V1BetaAcceptance -SkipDiffCheck` passed and printed `[gate] release gate passed`.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -SkipDiffCheck` passed and printed `[gate] release gate passed`.
- Push:
  - Plain `git push` failed because SSH to `github.com:22` timed out.
  - Retried via `ssh://git@ssh.github.com:443/Faust7777777/Lore.git`, which succeeded:
    - `d66cb95..819000c HEAD -> b-line/audit-followups-2026-06-04`.
  - `git ls-remote origin b-line/audit-followups-2026-06-04` confirmed remote SHA `819000c9174f244bbc90614372504017f5195ac5`.

### Phase 9: Post-Scan Reconciliation Decision
- **Status:** complete
- Subagent:
  - None. User asked not to use Claude for now.
- Product decision:
  - For V1, post-scan reconciliation is the persisted open finding path:
    - scan detects out-of-band governed/process-sink/ordinary note changes;
    - Lore writes audit records and open findings;
    - `OperatorQueue` / `lore inbox` / TUI findings surface the item;
    - operator resolves or ignores it locally.
  - Automatic conflict markers or generated drafts for governed out-of-band changes are deferred. They would expand governance semantics beyond the current release candidate and are not needed to close the V1 loop.
- Actions taken:
  - Added `TestRuntimePostScanFindingReconcilesThroughOperatorQueue`.
  - Added `TestReleaseGateIncludesPostScanReconciliationCoverage`.
  - Added V1 beta release-gate group `V1 beta post-scan reconciliation`.
  - Added the same test to the default `governed smoke, daemon watcher, and post-scan guardrails` group.
  - Committed and pushed `beca9e3 test: gate post-scan finding reconciliation`.
- Verification so far:
  - First TDD red:
    - `/usr/bin/go test ./cmd/obsidian-harness -run TestReleaseGateIncludesPostScanReconciliationCoverage -count=1` failed because `release-gate.ps1` lacked `V1 beta post-scan reconciliation`.
  - `/usr/bin/go test ./internal/app -run TestRuntimePostScanFindingReconcilesThroughOperatorQueue -count=1 -v` passed.
  - `/usr/bin/go test ./cmd/obsidian-harness -run TestReleaseGateIncludesPostScanReconciliationCoverage -count=1` passed.
  - `/usr/bin/go test ./internal/app -run TestRuntimePostScanFindingReconcilesThroughOperatorQueue -count=1` passed.
  - `/usr/bin/go test ./internal/app ./cmd/obsidian-harness -count=1` was attempted twice; both runs were blocked by the pre-existing flaky `TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds` (`winners = 3` then `winners = 2`). The failing test was isolated with `/usr/bin/go test ./internal/app -run TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds -count=10`, which passed.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -V1BetaAcceptance -SkipDiffCheck` passed and printed `[gate] release gate passed`.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -SkipDiffCheck` passed and printed `[gate] release gate passed`.
- Push:
  - First 443 push attempt timed out.
  - Second 443 push succeeded:
    - `819000c..beca9e3 HEAD -> b-line/audit-followups-2026-06-04`.

### Phase 10: External Transcript V1 Boundary
- **Status:** complete
- Subagent:
  - None. User asked not to use Claude for now.
- Product decision:
  - For V1, Lore external transcript JSONL import remains process-sink-only.
  - Automatic persona extraction from imported external transcripts is deferred. The imported transcript can contain third-party, stale, or low-trust claims; turning it into persona memory needs an explicit opt-in design and governance review.
  - Estimate:
    - V1 release-candidate scope: about 2-3 focused working days remaining.
    - Expanded scope including external transcript persona extraction, long-running app-server attach, and media/binary extraction: about 1-2 weeks.
- Actions taken so far:
  - Added `TestReleaseGatePinsExternalTranscriptProcessSinkOnlyBoundary`.
  - Changed V1 beta external transcript runtime gate from broad `TestRuntimeImportExternalTranscriptJSONL` to explicit full test names, including `TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly`.
  - Committed and pushed `a5b8deb test: pin external transcript import boundary`.
- Verification so far:
  - First TDD red:
    - `/usr/bin/go test ./cmd/obsidian-harness -run TestReleaseGatePinsExternalTranscriptProcessSinkOnlyBoundary -count=1` failed because `release-gate.ps1` did not include the full `TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly` name.
  - `/usr/bin/go test ./internal/app -run '(TestRuntimeImportExternalTranscriptJSONLWritesCheckpointsAndRollup|TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly|TestRuntimeImportExternalTranscriptJSONLRequiresInput)$' -count=1` passed.
  - `/usr/bin/go test ./cmd/obsidian-harness -run TestReleaseGatePinsExternalTranscriptProcessSinkOnlyBoundary -count=1` passed after the script regex used full test names.
  - `/usr/bin/go test ./cmd/obsidian-harness ./internal/app -run '(TestReleaseGatePinsExternalTranscriptProcessSinkOnlyBoundary|TestRuntimeImportExternalTranscriptJSONLWritesCheckpointsAndRollup|TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly|TestRuntimeImportExternalTranscriptJSONLRequiresInput)$' -count=1` passed.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -V1BetaAcceptance -SkipDiffCheck` passed and printed `[gate] release gate passed`.
  - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -SkipDiffCheck` passed and printed `[gate] release gate passed`.
- Push:
  - First 443 push attempt timed out.
  - Second 443 push succeeded:
    - `beca9e3..a5b8deb HEAD -> b-line/audit-followups-2026-06-04`.

### Phase 11: RC Documentation and Status Cleanup
- **Status:** complete locally; push pending
- Subagent:
  - None. User asked not to use Claude for now.
- Product decision:
  - V1 RC should have four remaining phases:
    - Phase 11: RC docs and status cleanup.
    - Phase 12: RC gate and flake cleanup.
    - Phase 13: manual smoke and operator workflow.
    - Phase 14: RC cut and handoff.
  - Practical V1 RC estimate remains two to three focused working days.
  - Expanded backlog remains one to two weeks and should stay post-RC unless scope changes.
- Actions taken so far:
  - Avoided editing `DEVELOPMENT_STATUS.md` and `docs/compact-handoff-lore-v1.md` because they are already modified with unrelated line-ending noise in this checkout.
  - Added `docs/v1-rc-status-2026-06-14.md` as the current canonical RC planning snapshot.
  - Committed `2958690 docs: record v1 rc phase plan`.
- Push:
  - Two attempts to push over `ssh://git@ssh.github.com:443/Faust7777777/Lore.git` timed out.

### Phase 12: RC Gate and Flake Cleanup
- **Status:** complete locally; push pending
- Subagent:
  - Claude Opus was invoked with `--model claude-opus-4-8` and a bounded no-commit task for the flaky test.
  - It remained silent, wrote no report, and was terminated before completion. The main agent completed and reviewed the fix.
  - During review, a silent Claude edit had weakened the existing concurrent test assertion; that hunk was reverted before commit.
- Root cause:
  - `CreatePersonaDraftFromCandidate` first performs a fast pre-check. If it sees `Open`, it attempts `ClaimCandidateForDraft`.
  - On `store.ErrConflict`, the old code re-read the candidate and reused the same non-open evaluator as ordinary later retries.
  - If the winning concurrent goroutine had already linked its draft by the re-read, the losing goroutine saw `Drafted+DraftID` and returned nil as an idempotent success.
  - That made `TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds` intermittently report `winners=2` or `winners=3`.
- Actions taken:
  - Added deterministic coverage: `TestCreatePersonaDraftFromCandidateClaimConflictAfterPeerLinkedReturnsAlreadyDrafted`.
  - Changed the claim-conflict branch so re-reading `Drafted` returns `ErrPersonaCandidateAlreadyDrafted` for the concurrent loser.
  - Kept ordinary later retries idempotent because the fast pre-check still uses `evaluateNonOpenPersonaCandidate`.
  - Updated the `PersonaCandidateStore.ClaimCandidateForDraft` comment to distinguish concurrent claim losers from ordinary terminal-state retries.
  - Committed `908ff35 fix: make persona candidate claim conflicts deterministic`.
- Verification:
  - RED before fix:
    - `/usr/bin/go test ./internal/app -run TestCreatePersonaDraftFromCandidateClaimConflictAfterPeerLinkedReturnsAlreadyDrafted -count=1 -v` failed with `err = <nil>, want ErrPersonaCandidateAlreadyDrafted`.
    - `/usr/bin/go test ./internal/app -run TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds -count=50` reproduced intermittent `winners=2` and `winners=3`.
  - GREEN after fix:
    - `/usr/bin/go test ./internal/app -run 'TestCreatePersonaDraftFromCandidate(ClaimConflictAfterPeerLinkedReturnsAlreadyDrafted|ConcurrentCallsOnlyOneSucceeds|IsIdempotentOnRetry)$' -count=1 -v` passed.
    - `/usr/bin/go test ./internal/app -run TestCreatePersonaDraftFromCandidateConcurrentCallsOnlyOneSucceeds -count=50` passed.
    - `/usr/bin/go test ./internal/app ./internal/store ./cmd/obsidian-harness -count=1` passed.
    - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -V1BetaAcceptance -SkipDiffCheck` passed.
    - `powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(wslpath -w scripts/release-gate.ps1)" -SkipDiffCheck` passed.
- Push:
  - One post-Phase-12 attempt to push over `ssh://git@ssh.github.com:443/Faust7777777/Lore.git` timed out.
  - `git ls-remote https://github.com/Faust7777777/Lore.git HEAD` also hung and was stopped, so the current issue appears to be GitHub network connectivity, not only SSH auth.
  - `timeout 60s git -c http.lowSpeedLimit=1 -c http.lowSpeedTime=30 push https://github.com/Faust7777777/Lore.git HEAD:b-line/audit-followups-2026-06-04` exited with code 124 after no remote response.

### Phase 13: Manual Smoke and Operator Workflow
- **Status:** partially complete; model-dependent items pending
- Workdir:
  - `tmp/v1-rc-smoke-2026-06-14-phase13/workdir`
  - Temporary binary: `tmp/v1-rc-smoke-2026-06-14-phase13/lore-smoke`
  - External transcript fixture: `tmp/v1-rc-smoke-2026-06-14-phase13/external-session.jsonl`
- Actions and results:
  - `/usr/bin/go build -o tmp/v1-rc-smoke-2026-06-14-phase13/lore-smoke ./cmd/lore` passed.
  - `lore-smoke bootstrap tmp/v1-rc-smoke-2026-06-14-phase13/workdir` passed and created 6 managed docs.
  - `lore-smoke status ...` passed; health was blocked only because model config is disabled.
  - `lore-smoke inbox --json ...` passed sequentially with zero action items.
  - `lore-smoke usage ... --json` passed with no usage recorded.
  - `lore-smoke demo-p0a ...` passed and applied a governed `progress_sync` draft.
  - `lore-smoke draft list ...` showed the applied draft.
  - `lore-smoke findings list ...` initially showed no findings.
  - After appending to `vault/03-画像/人物画像.md`, first `daemon run --once` primed the baseline.
  - After a second append, `daemon run --once` created one `governance_review_needed` finding.
  - `lore-smoke inbox --json ...` showed one action item with the open finding.
  - `lore-smoke findings resolve --workdir ... governance_review_needed-03-画像-人物画像.md-1781445764485600234` passed.
  - Final `lore-smoke inbox --json ...` showed zero action items.
- Environment-limited smoke:
  - `lore-smoke import-external-jsonl --workdir ... --input ...` failed because process sink summarizer requires configured `LORE_LLM_BASE_URL` and `LORE_LLM_API_KEY`.
  - `lore-smoke smoke p0 ...` failed in P0-B for the same process sink summarizer requirement.
  - `lore-smoke tui --workdir ... --once "show current status"` failed because the operator agent requires configured `LORE_LLM_BASE_URL` and `LORE_LLM_API_KEY`.
- Notes:
  - Running multiple CLI processes against the same smoke sqlite store in parallel caused transient `database is locked (261)` on read commands. Sequential reruns passed. Manual operator smoke should stay sequential unless testing multi-process locking explicitly.
