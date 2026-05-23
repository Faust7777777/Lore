package console

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	osruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/persona"
)

type fakeRuntime struct {
	managed           model.ManagedStatusView
	drafts            []model.Draft
	review            app.DraftReview
	processSink       app.ProcessSinkDayView
	vaultResolve      model.VaultResolveResult
	writtenNote       *model.VaultDocument
	supersede         *model.DraftSupersedeUpdate
	coreContext       model.CoreContext
	usageRecords      []model.UsageRecord
	usageErr          error
	personaMu         sync.Mutex
	personaCandidates []persona.PersonaCandidateRecord
	personaErr        error
}

func (f *fakeRuntime) ManagedStatus() (model.ManagedStatusView, error) {
	return f.managed, nil
}

func (f *fakeRuntime) ListDrafts() ([]model.Draft, error) {
	return f.drafts, nil
}

func (f *fakeRuntime) ReviewDraft(id string) (app.DraftReview, error) {
	f.review.Draft.ID = id
	return f.review, nil
}

func (f *fakeRuntime) ApproveDraft(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftApproved, Target: model.DocumentRef{Path: "progress.md"}, Title: "approved"}, nil
}

func (f *fakeRuntime) RejectDraft(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftRejected, Target: model.DocumentRef{Path: "progress.md"}, Title: "rejected"}, nil
}

func (f *fakeRuntime) RequestDraftRevision(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftRevisionRequested, Target: model.DocumentRef{Path: "progress.md"}, Title: "revision"}, nil
}

func (f *fakeRuntime) SupersedeDraft(id string, update model.DraftSupersedeUpdate) (model.Draft, error) {
	f.supersede = &update
	return model.Draft{ID: id + "-revised", State: model.DraftPendingReview, Target: model.DocumentRef{Path: "03-notes/class/ecommerce.md"}, Title: "revised", Supersedes: id}, nil
}

func (f *fakeRuntime) ApplyDraft(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftApplied, Target: model.DocumentRef{Path: "progress.md"}, Title: "applied"}, nil
}

func (f *fakeRuntime) ProcessSinkDay(agentID string, day time.Time) (app.ProcessSinkDayView, error) {
	f.processSink.AgentID = agentID
	f.processSink.Day = day
	return f.processSink, nil
}

func (f *fakeRuntime) ListFindings(limit int) ([]model.Finding, error) {
	return nil, nil
}

func (f *fakeRuntime) ResolveFinding(id string) (model.Finding, error) {
	return model.Finding{ID: id, State: model.FindingResolved}, nil
}

func (f *fakeRuntime) IgnoreFinding(id string) (model.Finding, error) {
	return model.Finding{ID: id, State: model.FindingIgnored}, nil
}

func (f *fakeRuntime) SystemDocGet(name string) (model.VaultDocument, error) {
	return model.VaultDocument{Path: name + ".md", Content: "# " + name}, nil
}

func (f *fakeRuntime) VaultRead(relPath string) (model.VaultDocument, error) {
	return model.VaultDocument{Path: relPath, Content: "content"}, nil
}

func (f *fakeRuntime) VaultList(relDir string) ([]model.VaultEntry, error) {
	return []model.VaultEntry{{Path: relDir, Name: relDir, Kind: "dir"}}, nil
}

func (f *fakeRuntime) VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error) {
	return []model.SearchHit{{Path: "progress.md", Line: 1, Preview: query}}, nil
}

func (f *fakeRuntime) VaultResolve(query string, relDir string, limit int) (model.VaultResolveResult, error) {
	if f.vaultResolve.Status != "" {
		return f.vaultResolve, nil
	}
	return model.VaultResolveResult{Query: query, Status: "unique", SelectedPath: "progress.md", Matches: []model.VaultResolveMatch{{Path: "progress.md", Score: 1, Reason: "test"}}}, nil
}
func (f *fakeRuntime) VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error) {
	return []model.SearchHit{{Path: "ref.md", Line: 3, Preview: relPath}}, nil
}

func (f *fakeRuntime) DocClassify(relPath string) model.DocClassificationView {
	return model.DocClassificationView{Path: relPath, DocClass: model.DocClassNote}
}

func (f *fakeRuntime) ContextPack(targetPath string, task string, limit int) (model.ContextPack, error) {
	return model.ContextPack{
		Task:       task,
		TargetPath: targetPath,
		Managed:    f.managed,
	}, nil
}

func (f *fakeRuntime) WriteLowRiskNote(relPath string, content string, overwrite bool) (model.VaultDocument, error) {
	doc := model.VaultDocument{Path: relPath, DocClass: model.DocClassNote, Content: content, BaseVersion: "hash"}
	f.writtenNote = &doc
	return doc, nil
}

func (f *fakeRuntime) BuildCoreContext(limit int) (model.CoreContext, error) {
	return f.coreContext, nil
}

func (f *fakeRuntime) RecordUsage(records []model.UsageRecord) error {
	if f.usageErr != nil {
		return f.usageErr
	}
	f.usageRecords = append(f.usageRecords, records...)
	return nil
}

func (f *fakeRuntime) RecordPersonaCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	f.personaMu.Lock()
	defer f.personaMu.Unlock()
	if f.personaErr != nil {
		return persona.PersonaCandidateRecord{}, false, f.personaErr
	}
	f.personaCandidates = append(f.personaCandidates, record)
	return record, true, nil
}

// recordedPersonaCandidates returns a snapshot copy so tests can read
// it from the main goroutine while the fire-and-forget extraction
// goroutine may still be appending.
func (f *fakeRuntime) recordedPersonaCandidates() []persona.PersonaCandidateRecord {
	f.personaMu.Lock()
	defer f.personaMu.Unlock()
	return append([]persona.PersonaCandidateRecord(nil), f.personaCandidates...)
}

func (f *fakeRuntime) WorkDirPath() string {
	if f.managed.WorkDir != "" {
		return f.managed.WorkDir
	}
	return "workdir"
}

func (f *fakeRuntime) VaultRootPath() string {
	if f.managed.VaultRoot != "" {
		return f.managed.VaultRoot
	}
	return "workdir/vault"
}

func (f *fakeRuntime) StateDirPath() string {
	if f.managed.WorkDir != "" {
		return f.managed.WorkDir + "/state"
	}
	return "workdir/state"
}

type fakeAgent struct {
	decisions []operatoragent.Decision
	inputs    []string
	contexts  []operatoragent.Context
}

func (f *fakeAgent) Decide(input string, ctx operatoragent.Context) (operatoragent.Decision, error) {
	f.inputs = append(f.inputs, input)
	f.contexts = append(f.contexts, ctx)
	if len(f.decisions) == 0 {
		return operatoragent.Decision{}, nil
	}
	decision := f.decisions[0]
	f.decisions = f.decisions[1:]
	return decision, nil
}

type fakeLoopAgent struct {
	response  operatoragent.Response
	responses []operatoragent.Response
	err       error
	inputs    []string
	contexts  []operatoragent.Context
	tools     [][]operatoragent.ToolDefinition
}

func (f *fakeLoopAgent) Decide(_ string, _ operatoragent.Context) (operatoragent.Decision, error) {
	return operatoragent.Decision{}, nil
}

func (f *fakeLoopAgent) Respond(input string, ctx operatoragent.Context, runtime operatoragent.ToolRuntime) (operatoragent.Response, error) {
	f.inputs = append(f.inputs, input)
	f.contexts = append(f.contexts, ctx)
	f.tools = append(f.tools, runtime.DescribeTools(ctx))
	if len(f.responses) > 0 {
		response := f.responses[0]
		f.responses = f.responses[1:]
		return response, f.err
	}
	return f.response, f.err
}

func (f *fakeLoopAgent) RespondContext(ctx context.Context, input string, agentCtx operatoragent.Context, runtime operatoragent.ToolRuntime) (operatoragent.Response, error) {
	if err := ctx.Err(); err != nil {
		return operatoragent.Response{}, err
	}
	return f.Respond(input, agentCtx, runtime)
}

func TestSessionHandleContextCancelledBeforeLoopAgentDoesNotRecordTurn(t *testing.T) {
	agent := &fakeLoopAgent{response: operatoragent.Response{Final: "ok\n", StopReason: operatoragent.TurnStopFinal, StepCount: 1}}
	session := NewSessionWithAgent("test", agent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	output, err := session.HandleContext(ctx, "show me status", &fakeRuntime{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("HandleContext() error = %v, want context.Canceled", err)
	}
	if output != "" {
		t.Fatalf("output = %q, want empty on cancellation", output)
	}
	if len(agent.inputs) != 0 {
		t.Fatalf("agent inputs = %v, want none after pre-cancel", agent.inputs)
	}
	if len(session.History) != 0 {
		t.Fatalf("history length = %d, want 0", len(session.History))
	}
	if len(session.LastTurnSteps) != 0 {
		t.Fatalf("LastTurnSteps length = %d, want 0", len(session.LastTurnSteps))
	}
}

func TestSessionHandleUsesInjectedAgentForStatusAndHelp(t *testing.T) {
	agent := &fakeAgent{
		decisions: []operatoragent.Decision{
			{Action: operatoragent.ActionShowStatus},
			{Action: operatoragent.ActionHelp},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			Ready:     true,
			WorkDir:   "work",
			VaultRoot: "vault",
			Health:    model.HealthSnapshot{Outcome: model.NewOutcome(model.StatusOK), Message: "healthy"},
		},
	}

	statusView, err := session.Handle("show me the current status", runtime)
	if err != nil {
		t.Fatalf("Handle(status) error = %v", err)
	}
	if !strings.Contains(statusView, "Managed Status") {
		t.Fatalf("status view = %q, want Managed Status", statusView)
	}

	helpView, err := session.Handle("help", runtime)
	if err != nil {
		t.Fatalf("Handle(help) error = %v", err)
	}
	if !strings.Contains(helpView, "Lore Console") {
		t.Fatalf("help view = %q, want Lore Console", helpView)
	}
	if len(agent.inputs) != 2 {
		t.Fatalf("agent inputs = %d, want 2", len(agent.inputs))
	}
}

func TestSessionHandleDraftFlowUsesFocusedDraft(t *testing.T) {
	agent := &fakeAgent{
		decisions: []operatoragent.Decision{
			{Action: operatoragent.ActionReviewDraft},
			{Action: operatoragent.ActionApproveDraft, UseFocusedDraft: true},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{
		drafts: []model.Draft{{
			ID:    "draft-1",
			State: model.DraftPendingReview,
			Kind:  model.DraftKindProgressSync,
			Target: model.DocumentRef{
				Path: "progress.md",
			},
			Title: "pending draft",
		}},
		review: app.DraftReview{
			Draft: model.Draft{
				ID:    "draft-1",
				State: model.DraftPendingReview,
				Kind:  model.DraftKindProgressSync,
				Target: model.DocumentRef{
					Path:  "progress.md",
					Class: model.DocClassProgressIndex,
				},
				Summary:         "review this",
				ProposedContent: "patch",
			},
			BaseVersionMatches: true,
		},
		coreContext: model.CoreContext{
			PersonaSummary:  "Major: E-commerce",
			WeaknessSummary: "Needs structured review",
		},
	}

	reviewView, err := session.Handle("review something", runtime)
	if err != nil {
		t.Fatalf("Handle(review) error = %v", err)
	}
	if !strings.Contains(reviewView, "Draft Review") {
		t.Fatalf("review view = %q, want Draft Review", reviewView)
	}

	approveView, err := session.Handle("approve the focused one", runtime)
	if err != nil {
		t.Fatalf("Handle(approve) error = %v", err)
	}
	if !strings.Contains(approveView, "approved") {
		t.Fatalf("approve view = %q, want approved", approveView)
	}
	if len(agent.contexts) < 2 || agent.contexts[1].CurrentDraftID != "draft-1" {
		t.Fatalf("second agent context = %+v, want CurrentDraftID draft-1", agent.contexts)
	}
	if agent.contexts[0].CoreContext.PersonaSummary != "Major: E-commerce" || agent.contexts[0].CoreContext.WeaknessSummary == "" {
		t.Fatalf("first agent core context = %+v, want runtime-built context", agent.contexts[0].CoreContext)
	}
}

func TestSessionHandleProcessSinkDayUsesAgentDecision(t *testing.T) {
	now := time.Date(2026, 4, 22, 11, 0, 0, 0, time.Local)
	agent := &fakeAgent{
		decisions: []operatoragent.Decision{{
			Action:  operatoragent.ActionShowProcessSinkDay,
			AgentID: "codex",
			Day:     now,
		}},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return now }

	runtime := &fakeRuntime{
		processSink: app.ProcessSinkDayView{
			Checkpoints: []model.CheckpointDoc{{
				Window: model.SessionWindow{
					AgentID:     "codex",
					SessionID:   "s1",
					WindowStart: now.Truncate(24 * time.Hour).Add(9 * time.Hour),
					WindowEnd:   now.Truncate(24 * time.Hour).Add(9*time.Hour + 30*time.Minute),
				},
				State: model.CheckpointMaterialized,
				Title: "checkpoint",
			}},
		},
	}

	view, err := session.Handle("show codex daily report", runtime)
	if err != nil {
		t.Fatalf("Handle(process sink) error = %v", err)
	}
	if !strings.Contains(view, "Process Sink Day") || !strings.Contains(view, "codex") {
		t.Fatalf("view = %q, want Process Sink Day codex", view)
	}
}

type recordingRecorder struct {
	users      []string
	assistants []string
	traces     [][]operatoragent.ToolCallTrace
	worksets   [][]operatoragent.WorkingSetItem
	errors     []string
	commands   []string
	usages     [][]operatoragent.ModelCallUsage
	turnEnds   []turnEndRecord
}

type turnEndRecord struct {
	StopReason string
	StepCount  int
}

func (r *recordingRecorder) RecordUser(text string) error {
	r.users = append(r.users, text)
	return nil
}

func (r *recordingRecorder) RecordAssistant(text string) error {
	r.assistants = append(r.assistants, text)
	return nil
}

func (r *recordingRecorder) RecordToolTrace(trace []operatoragent.ToolCallTrace) error {
	r.traces = append(r.traces, append([]operatoragent.ToolCallTrace(nil), trace...))
	return nil
}

func (r *recordingRecorder) RecordWorkingSet(items []operatoragent.WorkingSetItem) error {
	r.worksets = append(r.worksets, append([]operatoragent.WorkingSetItem(nil), items...))
	return nil
}

func (r *recordingRecorder) RecordLocalCommand(command string) error {
	r.commands = append(r.commands, command)
	return nil
}

func (r *recordingRecorder) SessionID() string {
	return "test-session"
}

func (r *recordingRecorder) Path() string {
	return "state/sessions/test-session.jsonl"
}
func (r *recordingRecorder) RecordError(message string, recoverable bool) error {
	r.errors = append(r.errors, message)
	return nil
}

func (r *recordingRecorder) RecordModelUsage(usage []operatoragent.ModelCallUsage) error {
	r.usages = append(r.usages, append([]operatoragent.ModelCallUsage(nil), usage...))
	return nil
}

func (r *recordingRecorder) RecordTaskTurnEnd(stopReason string, stepCount int) error {
	r.turnEnds = append(r.turnEnds, turnEndRecord{StopReason: stopReason, StepCount: stepCount})
	return nil
}
func TestSessionHandleUsesLoopAgentResponseAndStoresHistory(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final: "Managed Status\n--------------\nready\n",
			Trace: []operatoragent.ToolCallTrace{{
				Name:   "managed_status",
				Status: "ok",
			}},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			Ready:     true,
			WorkDir:   "work",
			VaultRoot: "vault",
			Health:    model.HealthSnapshot{Outcome: model.NewOutcome(model.StatusOK), Message: "healthy"},
		},
	}

	output, err := session.Handle("show me status", runtime)
	if err != nil {
		t.Fatalf("Handle(loop) error = %v", err)
	}
	if !strings.Contains(output, "Managed Status") {
		t.Fatalf("output = %q, want Managed Status", output)
	}
	if len(agent.tools) != 1 || len(agent.tools[0]) == 0 {
		t.Fatalf("loop tools = %+v, want non-empty tool list", agent.tools)
	}
	if len(session.History) != 2 {
		t.Fatalf("history len = %d, want 2", len(session.History))
	}
	if len(session.LastToolTrace) != 1 || session.LastToolTrace[0].Name != "managed_status" {
		t.Fatalf("last tool trace = %+v, want managed_status", session.LastToolTrace)
	}
}

func TestSessionHandlePersistsLoopAgentUsage(t *testing.T) {
	started := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final: "ok\n",
			Usage: []operatoragent.ModelCallUsage{
				{Provider: "openai-compatible", Model: "gpt-x", PromptTokens: 30, CompletionTokens: 4, StartedAt: started},
				{Provider: "openai-compatible", Model: "gpt-x", PromptTokens: 55, CompletionTokens: 12, StartedAt: started.Add(time.Second)},
			},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.DefaultAgentID = "codex"
	recorder := &recordingRecorder{}
	session.Recorder = recorder
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{Ready: true, WorkDir: "work", VaultRoot: "vault"},
	}

	if _, err := session.Handle("show me status", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(runtime.usageRecords) != 2 {
		t.Fatalf("usage records = %d, want 2; got %+v", len(runtime.usageRecords), runtime.usageRecords)
	}
	if len(recorder.usages) != 1 || len(recorder.usages[0]) != 2 {
		t.Fatalf("recorder usages = %+v, want one batch of 2", recorder.usages)
	}
	if recorder.usages[0][0].PromptTokens != 30 || recorder.usages[0][1].PromptTokens != 55 {
		t.Fatalf("recorder usage tokens = %+v", recorder.usages[0])
	}
	first := runtime.usageRecords[0]
	if first.Provider != "openai-compatible" || first.Model != "gpt-x" {
		t.Fatalf("usage[0] provider/model = %q/%q", first.Provider, first.Model)
	}
	if first.AgentID != "codex" || first.SessionID != "test-session" {
		t.Fatalf("usage[0] agent/session = %q/%q, want codex/test-session", first.AgentID, first.SessionID)
	}
	if first.PromptTokens != 30 || first.CompletionTokens != 4 {
		t.Fatalf("usage[0] tokens = %d/%d, want 30/4", first.PromptTokens, first.CompletionTokens)
	}
	if !first.RecordedAt.Equal(started) {
		t.Fatalf("usage[0] RecordedAt = %v, want %v", first.RecordedAt, started)
	}
	second := runtime.usageRecords[1]
	if second.PromptTokens != 55 || second.CompletionTokens != 12 {
		t.Fatalf("usage[1] tokens = %d/%d, want 55/12", second.PromptTokens, second.CompletionTokens)
	}
}

func TestSessionHandlePropagatesUsagePersistenceError(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final: "ok\n",
			Usage: []operatoragent.ModelCallUsage{{PromptTokens: 1, CompletionTokens: 1}},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{usageErr: fmt.Errorf("disk full")}

	_, err := session.Handle("show me status", runtime)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Handle() error = %v, want disk full error", err)
	}
}

func TestSessionHandlePersistsUsageOnRespondFailure(t *testing.T) {
	billed := []operatoragent.ModelCallUsage{
		{Provider: "openai-compatible", Model: "gpt-x", PromptTokens: 42, CompletionTokens: 9, StartedAt: time.Now().UTC()},
	}
	originalErr := fmt.Errorf("operator agent: invalid loop response: parse failure")
	agent := &fakeLoopAgent{
		response: operatoragent.Response{Usage: billed},
		err:      &operatoragent.UsageError{Err: originalErr, Usage: billed},
	}
	session := NewSessionWithAgent("test", agent)
	session.DefaultAgentID = "codex"
	recorder := &recordingRecorder{}
	session.Recorder = recorder
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{managed: model.ManagedStatusView{Ready: true, WorkDir: "work", VaultRoot: "vault"}}

	_, err := session.Handle("show me status", runtime)
	if err == nil || !strings.Contains(err.Error(), "invalid loop response") {
		t.Fatalf("Handle() error = %v, want original error surfaced", err)
	}
	if len(runtime.usageRecords) != 1 {
		t.Fatalf("usage records = %d, want 1 (billed cost must reach store on failed turn)", len(runtime.usageRecords))
	}
	if runtime.usageRecords[0].PromptTokens != 42 || runtime.usageRecords[0].CompletionTokens != 9 {
		t.Fatalf("usage tokens = %d/%d, want 42/9", runtime.usageRecords[0].PromptTokens, runtime.usageRecords[0].CompletionTokens)
	}
	if len(recorder.usages) != 1 || len(recorder.usages[0]) != 1 {
		t.Fatalf("recorder usages = %+v, want one batch of 1", recorder.usages)
	}
}

func TestSessionHandleRecordsErrorWhenUsagePersistenceFailsOnRespondError(t *testing.T) {
	billed := []operatoragent.ModelCallUsage{
		{Provider: "openai-compatible", Model: "gpt-x", PromptTokens: 5, CompletionTokens: 1},
	}
	originalErr := fmt.Errorf("operator agent: final response is empty")
	agent := &fakeLoopAgent{
		response: operatoragent.Response{Usage: billed},
		err:      &operatoragent.UsageError{Err: originalErr, Usage: billed},
	}
	session := NewSessionWithAgent("test", agent)
	recorder := &recordingRecorder{}
	session.Recorder = recorder
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{usageErr: fmt.Errorf("disk full")}
	_, err := session.Handle("show me status", runtime)
	// User must still see the original Respond error, not the
	// store-side disk failure that occurred during best-effort
	// persistence.
	if err == nil || !strings.Contains(err.Error(), "final response is empty") {
		t.Fatalf("Handle() error = %v, want original Respond error", err)
	}
	// The store-side error is captured in the transcript so the
	// failure is not invisible to operators.
	foundDiskFull := false
	for _, msg := range recorder.errors {
		if strings.Contains(msg, "disk full") {
			foundDiskFull = true
			break
		}
	}
	if !foundDiskFull {
		t.Fatalf("recorder.errors = %+v, want disk-full message recorded", recorder.errors)
	}
}

func TestSessionHandleEmitsTaskTurnEndOnSuccess(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final:      "ok\n",
			StopReason: operatoragent.TurnStopFinal,
			StepCount:  3,
		},
	}
	session := NewSessionWithAgent("test", agent)
	recorder := &recordingRecorder{}
	session.Recorder = recorder
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{}
	if _, err := session.Handle("show me status", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(recorder.turnEnds) != 1 {
		t.Fatalf("turnEnds = %+v, want exactly one entry", recorder.turnEnds)
	}
	if recorder.turnEnds[0].StopReason != "final" || recorder.turnEnds[0].StepCount != 3 {
		t.Fatalf("turnEnds[0] = %+v, want final / 3", recorder.turnEnds[0])
	}
}

func TestSessionHandleEmitsTaskTurnEndOnFailure(t *testing.T) {
	billed := []operatoragent.ModelCallUsage{{PromptTokens: 5, CompletionTokens: 1}}
	originalErr := fmt.Errorf("operator agent: invalid loop response: parse failure")
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Usage:      billed,
			StopReason: operatoragent.TurnStopModelError,
			StepCount:  1,
		},
		err: &operatoragent.UsageError{Err: originalErr, Usage: billed},
	}
	session := NewSessionWithAgent("test", agent)
	recorder := &recordingRecorder{}
	session.Recorder = recorder
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{}
	if _, err := session.Handle("show me status", runtime); err == nil {
		t.Fatal("Handle() error = nil, want surfaced parse failure")
	}
	if len(recorder.turnEnds) != 1 {
		t.Fatalf("turnEnds = %+v, want exactly one entry even on failure", recorder.turnEnds)
	}
	if recorder.turnEnds[0].StopReason != "model_error" || recorder.turnEnds[0].StepCount != 1 {
		t.Fatalf("turnEnds[0] = %+v, want model_error / 1", recorder.turnEnds[0])
	}
}

func TestSessionHandleSkipsTaskTurnEndForLegacyDecidePath(t *testing.T) {
	// fakeAgent satisfies Agent but NOT LoopAgent. Handle takes the
	// legacy Decide branch which never populates Response.StopReason,
	// so no task_turn_end event should be emitted.
	agent := &fakeAgent{
		decisions: []operatoragent.Decision{{Action: operatoragent.ActionShowStatus}},
	}
	session := NewSessionWithAgent("test", agent)
	recorder := &recordingRecorder{}
	session.Recorder = recorder
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{managed: model.ManagedStatusView{Ready: true, WorkDir: "w", VaultRoot: "v"}}
	if _, err := session.Handle("show status", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(recorder.turnEnds) != 0 {
		t.Fatalf("legacy decide path emitted turnEnds = %+v, want none", recorder.turnEnds)
	}
}

func TestSessionHandlePopulatesLastTurnStepsFromLoopAgentResponse(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final: "done\n",
			Steps: []operatoragent.TurnStep{
				{Index: 1, Tool: "vault_resolve", Arguments: map[string]any{"query": "target"}, Status: "ok", ObservationExcerpt: "selected=03-notes/target.md"},
				{Index: 2, Tool: "vault_read", Arguments: map[string]any{"path": "03-notes/target.md"}, Status: "ok", ObservationExcerpt: "# Target"},
			},
			StopReason: operatoragent.TurnStopFinal,
			StepCount:  3,
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{}
	if _, err := session.Handle("inspect target", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(session.LastTurnSteps) != 2 {
		t.Fatalf("LastTurnSteps len = %d, want 2", len(session.LastTurnSteps))
	}
	if session.LastTurnSteps[0].Tool != "vault_resolve" || session.LastTurnSteps[1].Tool != "vault_read" {
		t.Fatalf("LastTurnSteps tools = %q / %q, want vault_resolve / vault_read",
			session.LastTurnSteps[0].Tool, session.LastTurnSteps[1].Tool)
	}
}

func TestSessionHandleLastTurnStepsEmptyForFinalOnlyTurn(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final:      "ok\n",
			StopReason: operatoragent.TurnStopFinal,
			StepCount:  1,
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{}
	if _, err := session.Handle("hello", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(session.LastTurnSteps) != 0 {
		t.Fatalf("LastTurnSteps = %+v, want empty for final-only turn", session.LastTurnSteps)
	}
}

func TestSessionHandleLastTurnStepsPreservedOnUsageErrorPath(t *testing.T) {
	executedStep := operatoragent.TurnStep{
		Index: 1, Tool: "vault_resolve",
		Arguments: map[string]any{"query": "target"},
		Status:    "ok", ObservationExcerpt: "selected=03-notes/target.md",
	}
	billed := []operatoragent.ModelCallUsage{{PromptTokens: 4, CompletionTokens: 1}}
	originalErr := fmt.Errorf("operator agent: invalid loop response: parse failure")
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Steps:      []operatoragent.TurnStep{executedStep},
			Usage:      billed,
			StopReason: operatoragent.TurnStopModelError,
			StepCount:  2,
		},
		err: &operatoragent.UsageError{Err: originalErr, Usage: billed},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	if _, err := session.Handle("inspect target", &fakeRuntime{}); err == nil {
		t.Fatal("Handle() error = nil, want surfaced parse failure")
	}
	if len(session.LastTurnSteps) != 1 {
		t.Fatalf("LastTurnSteps len = %d, want 1 (failed turn must keep executed steps)", len(session.LastTurnSteps))
	}
	if session.LastTurnSteps[0].Tool != "vault_resolve" {
		t.Fatalf("LastTurnSteps[0].Tool = %q, want vault_resolve", session.LastTurnSteps[0].Tool)
	}
}

func TestSessionHandleClearsLastTurnStepsBetweenTurns(t *testing.T) {
	agent := &fakeLoopAgent{
		responses: []operatoragent.Response{
			{
				Final: "first\n",
				Steps: []operatoragent.TurnStep{
					{Index: 1, Tool: "vault_resolve", Arguments: map[string]any{"query": "x"}, Status: "ok"},
				},
				StopReason: operatoragent.TurnStopFinal,
				StepCount:  2,
			},
			// Second turn produces no steps; LastTurnSteps from turn 1
			// must not leak into the user-visible state.
			{Final: "second\n", StopReason: operatoragent.TurnStopFinal, StepCount: 1},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }
	runtime := &fakeRuntime{}

	if _, err := session.Handle("turn 1", runtime); err != nil {
		t.Fatalf("Handle(turn 1) error = %v", err)
	}
	if len(session.LastTurnSteps) != 1 {
		t.Fatalf("LastTurnSteps after turn 1 = %+v, want 1 step", session.LastTurnSteps)
	}
	if _, err := session.Handle("turn 2", runtime); err != nil {
		t.Fatalf("Handle(turn 2) error = %v", err)
	}
	if len(session.LastTurnSteps) != 0 {
		t.Fatalf("LastTurnSteps after turn 2 = %+v, want empty (turn 1 steps must clear)", session.LastTurnSteps)
	}
}

func TestSessionHandleLastTurnStepsEmptyForLegacyDecidePath(t *testing.T) {
	// fakeAgent satisfies Agent but NOT LoopAgent; Handle takes the
	// legacy Decide branch. LastTurnSteps must remain empty there
	// because Decide does not produce structured per-step records.
	agent := &fakeAgent{decisions: []operatoragent.Decision{{Action: operatoragent.ActionShowStatus}}}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{managed: model.ManagedStatusView{Ready: true, WorkDir: "w", VaultRoot: "v"}}
	if _, err := session.Handle("show status", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(session.LastTurnSteps) != 0 {
		t.Fatalf("legacy decide path LastTurnSteps = %+v, want empty", session.LastTurnSteps)
	}
}

func TestSessionHandleLastTurnStepsIsolatedFromAgentResponse(t *testing.T) {
	originalArgs := map[string]any{"query": "target"}
	agentResponse := operatoragent.Response{
		Final: "ok\n",
		Steps: []operatoragent.TurnStep{
			{Index: 1, Tool: "vault_resolve", Arguments: originalArgs, Status: "ok"},
		},
		StopReason: operatoragent.TurnStopFinal,
		StepCount:  2,
	}
	agent := &fakeLoopAgent{response: agentResponse}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 5, 19, 9, 0, 0, 0, time.Local) }

	if _, err := session.Handle("inspect target", &fakeRuntime{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(session.LastTurnSteps) != 1 {
		t.Fatalf("LastTurnSteps len = %d, want 1", len(session.LastTurnSteps))
	}
	// Mutating the stored snapshot must not leak into the canned
	// agent response that the fake still holds.
	session.LastTurnSteps[0].Arguments["query"] = "tampered"
	if agent.response.Steps[0].Arguments["query"] != "target" {
		t.Fatalf("mutation leaked into agent response: %+v", agent.response.Steps[0].Arguments)
	}
	if originalArgs["query"] != "target" {
		t.Fatalf("mutation leaked into original Arguments map: %+v", originalArgs)
	}
}

func TestSessionHandleSkipsUsageWhenLoopResponseHasNone(t *testing.T) {
	agent := &fakeLoopAgent{response: operatoragent.Response{Final: "ok\n"}}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }

	runtime := &fakeRuntime{}
	if _, err := session.Handle("show me status", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(runtime.usageRecords) != 0 {
		t.Fatalf("usage records = %+v, want empty", runtime.usageRecords)
	}
}

func TestSessionHandleRecordsTranscriptEvents(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final: "Read complete\n",
			Trace: []operatoragent.ToolCallTrace{{
				Name:      "vault_read",
				Arguments: map[string]any{"path": "notes/example.md"},
				Status:    "ok",
			}},
		},
	}
	recorder := &recordingRecorder{}
	session := NewSessionWithAgent("test", agent)
	session.Recorder = recorder

	if _, err := session.Handle("read example", &fakeRuntime{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(recorder.users) != 1 || recorder.users[0] != "read example" {
		t.Fatalf("recorded users = %+v", recorder.users)
	}
	if len(recorder.assistants) != 1 || recorder.assistants[0] != "Read complete" {
		t.Fatalf("recorded assistants = %+v", recorder.assistants)
	}
	if len(recorder.traces) != 1 || len(recorder.traces[0]) != 1 || recorder.traces[0][0].Name != "vault_read" {
		t.Fatalf("recorded traces = %+v", recorder.traces)
	}
	if len(recorder.worksets) != 1 || len(recorder.worksets[0]) != 1 || recorder.worksets[0][0].Path != "notes/example.md" {
		t.Fatalf("recorded worksets = %+v", recorder.worksets)
	}
}

func TestSessionHandleRemembersVaultResolveSelectedPath(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final: "Resolved the note.",
			Trace: []operatoragent.ToolCallTrace{{
				Name:      "vault_resolve",
				Arguments: map[string]any{"query": "progress", "selected_path": "progress.md"},
				Status:    "ok",
			}},
		},
	}
	session := NewSessionWithAgent("test", agent)

	if _, err := session.Handle("open progress", &fakeRuntime{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(session.WorkingSet) != 1 || session.WorkingSet[0].Path != "progress.md" || session.WorkingSet[0].Source != "vault_resolve" {
		t.Fatalf("working set = %+v, want vault_resolve selected path", session.WorkingSet)
	}
}

func TestSessionHandleDoesNotRememberNonUniqueVaultResolveMatches(t *testing.T) {
	for _, tt := range []struct {
		name  string
		final string
	}{
		{
			name:  "ambiguous",
			final: `{"query":"progress","status":"ambiguous","matches":[{"path":"progress.md","score":0.6},{"path":"project-progress.md","score":0.55}],"reason":"score_below_unique_threshold_or_margin"}`,
		},
		{
			name:  "not_found",
			final: `{"query":"missing","status":"not_found","reason":"no_path_match"}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent := &fakeLoopAgent{
				response: operatoragent.Response{
					Final: tt.final,
					Trace: []operatoragent.ToolCallTrace{{
						Name:      "vault_resolve",
						Arguments: map[string]any{"query": "progress"},
						Status:    "ok",
					}},
				},
			}
			session := NewSessionWithAgent("test", agent)

			if _, err := session.Handle("resolve progress", &fakeRuntime{}); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(session.WorkingSet) != 0 {
				t.Fatalf("working set = %+v, want empty for non-unique vault_resolve", session.WorkingSet)
			}
		})
	}
}

func TestSessionHandleRemembersUniqueVaultResolveJSONSelectedPath(t *testing.T) {
	agent := &fakeLoopAgent{
		response: operatoragent.Response{
			Final: `{"query":"progress","status":"unique","selected_path":"progress.md","matches":[{"path":"other.md","score":0.2}]}`,
			Trace: []operatoragent.ToolCallTrace{{
				Name:      "vault_resolve",
				Arguments: map[string]any{"query": "progress"},
				Status:    "ok",
			}},
		},
	}
	session := NewSessionWithAgent("test", agent)

	if _, err := session.Handle("resolve progress", &fakeRuntime{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(session.WorkingSet) != 1 || session.WorkingSet[0].Path != "progress.md" {
		t.Fatalf("working set = %+v, want unique selected_path only", session.WorkingSet)
	}
}

func TestSessionHandleCarriesVaultPathWorkingSetAcrossFollowUp(t *testing.T) {
	agent := &fakeLoopAgent{
		responses: []operatoragent.Response{
			{
				Final: "03-画像 目录下有人物画像文件：\n- 03-画像/人物画像.md\n",
				Trace: []operatoragent.ToolCallTrace{{
					Name:      "vault_list",
					Arguments: map[string]any{"dir": "03-画像"},
					Status:    "ok",
				}},
			},
			{Final: "读取完成。"},
		},
	}
	session := NewSessionWithAgent("test", agent)
	session.Now = func() time.Time { return time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local) }
	runtime := &fakeRuntime{}

	if _, err := session.Handle("看看人物画像", runtime); err != nil {
		t.Fatalf("Handle(first) error = %v", err)
	}
	if len(session.WorkingSet) != 1 || session.WorkingSet[0].Path != "03-画像/人物画像.md" {
		t.Fatalf("working set = %+v, want 人物画像 path", session.WorkingSet)
	}

	if _, err := session.Handle("读取", runtime); err != nil {
		t.Fatalf("Handle(follow-up) error = %v", err)
	}
	if len(agent.contexts) != 2 {
		t.Fatalf("contexts = %d, want 2", len(agent.contexts))
	}
	if len(agent.contexts[1].WorkingSet) != 1 || agent.contexts[1].WorkingSet[0].Path != "03-画像/人物画像.md" {
		t.Fatalf("follow-up context working set = %+v, want 人物画像 path", agent.contexts[1].WorkingSet)
	}
}

func TestSessionHandlePendingShellConfirmationRunsCommand(t *testing.T) {
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}
	session := NewSessionWithAgent("test", &fakeAgent{})
	command := "printf lore-shell-confirmed"
	if osruntime.GOOS == "windows" {
		command = "Write-Output lore-shell-confirmed"
	}
	session.PendingShellCommand = &pendingShellCommand{
		Command:        command,
		TimeoutSeconds: 5,
	}

	output, err := session.Handle("yes", runtime)
	if err != nil {
		t.Fatalf("Handle(confirm) error = %v", err)
	}
	if session.PendingShellCommand != nil {
		t.Fatal("pending shell command not cleared after confirmation")
	}
	if !strings.Contains(output, "lore-shell-confirmed") || !strings.Contains(output, "exit_code: 0") {
		t.Fatalf("output = %q, want shell output and exit code", output)
	}
	if len(session.LastToolTrace) != 1 || session.LastToolTrace[0].Name != "shell_exec" || session.LastToolTrace[0].Status != "ok" {
		t.Fatalf("last tool trace = %+v, want one ok shell_exec trace", session.LastToolTrace)
	}
	if len(session.History) != 2 || session.History[0].Role != "user" || session.History[1].Role != "assistant" {
		t.Fatalf("history = %+v, want one confirmed shell turn", session.History)
	}
}

func TestSessionHandlePendingShellConfirmationCancelsCommand(t *testing.T) {
	session := NewSessionWithAgent("test", &fakeAgent{})
	session.PendingShellCommand = &pendingShellCommand{
		Command:        "echo lore",
		TimeoutSeconds: 5,
	}
	workDir := t.TempDir()
	runtime := &fakeRuntime{
		managed: model.ManagedStatusView{
			WorkDir:   workDir,
			VaultRoot: filepath.Join(workDir, "vault"),
		},
	}

	output, err := session.Handle("cancel", runtime)
	if err != nil {
		t.Fatalf("Handle(cancel) error = %v", err)
	}
	if session.PendingShellCommand != nil {
		t.Fatal("pending shell command not cleared after cancellation")
	}
	if !strings.Contains(output, "Shell command cancelled.") {
		t.Fatalf("output = %q, want cancellation text", output)
	}
	if len(session.LastToolTrace) != 1 || session.LastToolTrace[0].Status != "cancelled" {
		t.Fatalf("last tool trace = %+v, want cancelled trace", session.LastToolTrace)
	}
}
