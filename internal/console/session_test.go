package console

import (
	"path/filepath"
	osruntime "runtime"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
)

type fakeRuntime struct {
	managed     model.ManagedStatusView
	drafts      []model.Draft
	review      app.DraftReview
	processSink app.ProcessSinkDayView
	writtenNote *model.VaultDocument
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

func (f *fakeRuntime) ApplyDraft(id string) (model.Draft, error) {
	return model.Draft{ID: id, State: model.DraftApplied, Target: model.DocumentRef{Path: "progress.md"}, Title: "applied"}, nil
}

func (f *fakeRuntime) ProcessSinkDay(agentID string, day time.Time) (app.ProcessSinkDayView, error) {
	f.processSink.AgentID = agentID
	f.processSink.Day = day
	return f.processSink, nil
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
