package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/console"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/tools"
)

type workbenchRuntimeStub struct {
	managed     model.ManagedStatusView
	drafts      []model.Draft
	processSink app.ProcessSinkDayView
	review      app.DraftReview
}

func (s workbenchRuntimeStub) ManagedStatus() (model.ManagedStatusView, error) {
	return s.managed, nil
}

func (s workbenchRuntimeStub) ListDrafts() ([]model.Draft, error) {
	return append([]model.Draft(nil), s.drafts...), nil
}

func (s workbenchRuntimeStub) ReviewDraft(id string) (app.DraftReview, error) {
	return s.review, nil
}

func (s workbenchRuntimeStub) ApproveDraft(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) RejectDraft(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) RequestDraftRevision(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) SupersedeDraft(id string, update model.DraftSupersedeUpdate) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) ApplyDraft(id string) (model.Draft, error) {
	return model.Draft{}, nil
}

func (s workbenchRuntimeStub) ProcessSinkDay(agentID string, day time.Time) (app.ProcessSinkDayView, error) {
	return s.processSink, nil
}

func (s workbenchRuntimeStub) ListFindings(limit int) ([]model.Finding, error) {
	return nil, nil
}

func (s workbenchRuntimeStub) ResolveFinding(id string) (model.Finding, error) {
	return model.Finding{ID: id, State: model.FindingResolved}, nil
}

func (s workbenchRuntimeStub) IgnoreFinding(id string) (model.Finding, error) {
	return model.Finding{ID: id, State: model.FindingIgnored}, nil
}

func (s workbenchRuntimeStub) SystemDocGet(name string) (model.VaultDocument, error) {
	return model.VaultDocument{}, nil
}

func (s workbenchRuntimeStub) VaultRead(relPath string) (model.VaultDocument, error) {
	return model.VaultDocument{}, nil
}

func (s workbenchRuntimeStub) VaultList(relDir string) ([]model.VaultEntry, error) {
	return nil, nil
}

func (s workbenchRuntimeStub) VaultSearchText(query string, relDir string, limit int) ([]model.SearchHit, error) {
	return nil, nil
}

func (s workbenchRuntimeStub) VaultResolve(query string, relDir string, limit int) (model.VaultResolveResult, error) {
	return model.VaultResolveResult{}, nil
}
func (s workbenchRuntimeStub) VaultBacklinks(relPath string, limit int) ([]model.SearchHit, error) {
	return nil, nil
}

func (s workbenchRuntimeStub) DocClassify(relPath string) model.DocClassificationView {
	return model.DocClassificationView{}
}

func (s workbenchRuntimeStub) ContextPack(targetPath string, task string, limit int) (model.ContextPack, error) {
	return model.ContextPack{}, nil
}

func (s workbenchRuntimeStub) WriteLowRiskNote(relPath string, content string, overwrite bool) (model.VaultDocument, error) {
	return model.VaultDocument{}, nil
}

func (s workbenchRuntimeStub) ProposalTools() []tools.Tool {
	return nil
}

func (s workbenchRuntimeStub) BuildCoreContext(limit int) (model.CoreContext, error) {
	return model.CoreContext{}, nil
}

func (s workbenchRuntimeStub) RecordUsage(records []model.UsageRecord) error {
	return nil
}

func (s workbenchRuntimeStub) RecordPersonaCandidate(record persona.PersonaCandidateRecord) (persona.PersonaCandidateRecord, bool, error) {
	return record, true, nil
}

func (s workbenchRuntimeStub) WorkDirPath() string {
	return s.managed.WorkDir
}

func (s workbenchRuntimeStub) VaultRootPath() string {
	return s.managed.VaultRoot
}

func (s workbenchRuntimeStub) StateDirPath() string {
	return "state"
}

func TestLoadWorkbenchViewModelBuildsSnapshotFromRuntimeAndSession(t *testing.T) {
	day := time.Date(2026, 4, 23, 0, 0, 0, 0, time.Local)
	runtime := workbenchRuntimeStub{
		managed: model.ManagedStatusView{
			Ready:     true,
			WorkDir:   "work",
			VaultRoot: "vault",
			Health:    model.HealthSnapshot{Outcome: model.NewOutcome(model.StatusOK), Message: "healthy"},
			CoreDocs: []model.ManagedCoreStatus{
				{Name: "system", Exists: true, Path: "00-system/system.md"},
			},
		},
		drafts: []model.Draft{
			{ID: "draft-1", State: model.DraftPendingReview},
			{ID: "draft-2", State: model.DraftApproved},
		},
		processSink: app.ProcessSinkDayView{
			AgentID: "codex",
			Day:     day,
			Checkpoints: []model.CheckpointDoc{{
				Window: model.SessionWindow{
					AgentID:     "codex",
					SessionID:   "session-1",
					WindowStart: day.Add(9 * time.Hour),
					WindowEnd:   day.Add(9*time.Hour + 30*time.Minute),
				},
				State: model.CheckpointMaterialized,
				Title: "checkpoint",
			}},
		},
		review: app.DraftReview{
			Draft: model.Draft{
				ID:              "draft-1",
				State:           model.DraftPendingReview,
				Summary:         "summary",
				ProposedContent: "- [x] done",
			},
			BaseVersionMatches: true,
		},
	}

	session := console.NewSession("test")
	session.CurrentDraftID = "draft-1"
	session.LastToolTrace = []operatoragent.ToolCallTrace{{
		Name:   "managed_status",
		Status: "ok",
	}}
	session.History = []operatoragent.ConversationTurn{
		{Role: "user", Content: "show status"},
		{Role: "assistant", Content: "ready"},
	}

	viewModel, err := loadWorkbenchViewModel("test", runtime, session, "codex", day, true, false, "ready")
	if err != nil {
		t.Fatalf("loadWorkbenchViewModel returned error: %v", err)
	}

	if got, want := viewModel.Snapshot.Profile, "local-exec"; got != want {
		t.Fatalf("viewModel.Snapshot.Profile = %q, want %q", got, want)
	}
	if got, want := len(viewModel.PendingDrafts), 2; got != want {
		t.Fatalf("len(viewModel.PendingDrafts) = %d, want %d (pending_review + approved)", got, want)
	}
	if viewModel.FocusedReview == nil {
		t.Fatalf("viewModel.FocusedReview = nil, want non-nil")
	}
	if got, want := len(viewModel.ToolTrace), 1; got != want {
		t.Fatalf("len(viewModel.ToolTrace) = %d, want %d", got, want)
	}
	if got, want := len(viewModel.Conversation.Turns), 2; got != want {
		t.Fatalf("len(viewModel.Conversation.Turns) = %d, want %d", got, want)
	}
	if got, want := viewModel.Conversation.LastOutput, "ready"; got != want {
		t.Fatalf("viewModel.Conversation.LastOutput = %q, want %q", got, want)
	}
}

func TestInteractiveWorkbenchModelPanelUsesWorkspaceLLMProfileOverEnv(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	var workspaceHits int32
	workspaceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&workspaceHits, 1)
		if r.URL.Path != "/models" {
			t.Fatalf("workspace server path = %q, want /models", r.URL.Path)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer deepseek-secret"; got != want {
			t.Fatalf("workspace server Authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek-reasoner"},
				{"id": "deepseek-chat"},
			},
		})
	}))
	defer workspaceServer.Close()

	var envHits int32
	envServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&envHits, 1)
		http.Error(w, "env endpoint must not be used", http.StatusTeapot)
	}))
	defer envServer.Close()

	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	t.Setenv("LORE_LLM_BASE_URL", envServer.URL)
	t.Setenv("LORE_LLM_API_KEY", "env-secret")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")

	writeWorkbenchLLMConfig(t, workDir, workspaceServer.URL)

	session := console.NewSessionWithAgent("test", operatoragent.NewUnavailable(nil))
	driver := interactiveWorkbenchDriver{
		version: "test",
		runtime: workbenchRuntimeStub{
			managed: model.ManagedStatusView{WorkDir: workDir},
		},
		session: session,
	}

	models, err := driver.DiscoverModels()
	if err != nil {
		t.Fatalf("DiscoverModels() error = %v", err)
	}
	if got := atomic.LoadInt32(&workspaceHits); got != 1 {
		t.Fatalf("workspace hits = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&envHits); got != 0 {
		t.Fatalf("env hits = %d, want 0", got)
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2: %+v", len(models), models)
	}
	for _, item := range models {
		if item.BaseURL != workspaceServer.URL {
			t.Fatalf("model BaseURL = %q, want workspace %q", item.BaseURL, workspaceServer.URL)
		}
		if item.Source != "workspace" {
			t.Fatalf("model Source = %q, want workspace", item.Source)
		}
		if item.Provider != "deepseek" {
			t.Fatalf("model Provider = %q, want deepseek", item.Provider)
		}
		if item.KeyStatus != "OK" {
			t.Fatalf("model KeyStatus = %q, want OK", item.KeyStatus)
		}
	}
	names := []string{models[0].Name, models[1].Name}
	if got := strings.Join(names, ","); got != "deepseek-chat,deepseek-reasoner" {
		t.Fatalf("model names = %q, want sorted deepseek models", got)
	}
}

func TestInteractiveWorkbenchModelDiscoveryFallbackUsesConfiguredModelAndError(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "models unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	writeWorkbenchLLMConfig(t, workDir, server.URL)

	session := console.NewSessionWithAgent("test", operatoragent.NewUnavailable(nil))
	driver := interactiveWorkbenchDriver{
		version: "test",
		runtime: workbenchRuntimeStub{
			managed: model.ManagedStatusView{WorkDir: workDir},
		},
		session: session,
	}

	models, err := driver.DiscoverModels()
	if err != nil {
		t.Fatalf("DiscoverModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want fallback row: %+v", len(models), models)
	}
	row := models[0]
	if row.Name != "deepseek-chat" {
		t.Fatalf("fallback Name = %q, want configured model", row.Name)
	}
	if row.TestStatus == "" || !strings.Contains(row.TestStatus, "discovery failed") {
		t.Fatalf("fallback TestStatus = %q, want discovery failure", row.TestStatus)
	}
	if row.Source != "workspace" {
		t.Fatalf("fallback Source = %q, want workspace", row.Source)
	}
	if !row.Current {
		t.Fatal("fallback row Current = false, want true")
	}
}

func TestInteractiveWorkbenchSwitchModelUpdatesPersonaExtractor(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "deepseek-chat"},
					{"id": "deepseek-reasoner"},
				},
			})
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"type\":\"final\",\"message\":\"ok\"}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	writeWorkbenchLLMConfig(t, workDir, server.URL)

	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer runtime.Close()

	session := console.NewSessionWithAgent("test", runtime.OperatorAgent)
	session.PersonaExtractor = runtime.PersonaExtractor
	session.PersonaExtractModelInfo = console.PersonaExtractModelInfo{
		Provider: runtime.PersonaExtractProvider,
		Model:    runtime.PersonaExtractModel,
		BaseURL:  runtime.PersonaExtractBaseURL,
	}

	driver := interactiveWorkbenchDriver{
		version: "test",
		runtime: runtime,
		session: session,
	}

	models, err := driver.SwitchModel("deepseek-reasoner")
	if err != nil {
		t.Fatalf("SwitchModel() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want discovered rows", len(models))
	}
	if got := modelLabel(session); got != "deepseek-reasoner" {
		t.Fatalf("session model = %q, want deepseek-reasoner", got)
	}
	if session.PersonaExtractor == nil {
		t.Fatal("PersonaExtractor nil after switch")
	}
	if got := session.PersonaExtractModelInfo.Model; got != "deepseek-reasoner" {
		t.Fatalf("PersonaExtractModelInfo.Model = %q, want switched model", got)
	}
	if got := session.PersonaExtractModelInfo.Provider; got != "deepseek" {
		t.Fatalf("PersonaExtractModelInfo.Provider = %q, want deepseek", got)
	}
	if got := session.PersonaExtractModelInfo.BaseURL; got != server.URL {
		t.Fatalf("PersonaExtractModelInfo.BaseURL = %q, want %q", got, server.URL)
	}

	freshRuntime, err := app.OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("fresh OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer freshRuntime.Close()
	freshCfg, err := freshRuntime.ResolveLLMConfig(config.LLMPurposeOperator)
	if err != nil {
		t.Fatalf("fresh ResolveLLMConfig(operator) error = %v", err)
	}
	if got := freshCfg.Model; got != "deepseek-chat" {
		t.Fatalf("fresh runtime model = %q, want workspace active profile model deepseek-chat; /model use must stay session-scoped", got)
	}
}

func TestInteractiveWorkbenchCreateProfileFromPresetWritesWorkspaceConfigWithoutSecret(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")

	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer runtime.Close()

	driver := interactiveWorkbenchDriver{
		version: "test",
		runtime: runtime,
		session: console.NewSessionWithAgent("test", runtime.OperatorAgent),
	}
	if err := driver.CreateProfileFromPreset("deepseek", "deepseek"); err != nil {
		t.Fatalf("CreateProfileFromPreset(deepseek) error = %v", err)
	}

	configPath := filepath.Join(workDir, ".lore", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.json) error = %v", err)
	}
	body := string(data)
	for _, expected := range []string{"deepseek-v4-pro", "DEEPSEEK_API_KEY", "https://api.deepseek.com/v1"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("workspace config missing %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "deepseek-secret") {
		t.Fatalf("workspace config leaked API key value:\n%s", body)
	}

	profiles, err := driver.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("len(profiles) = %d, want 1: %+v", len(profiles), profiles)
	}
	if profiles[0].ProfileName != "deepseek" || profiles[0].Name != "deepseek-v4-pro" {
		t.Fatalf("profile row = %+v, want deepseek/deepseek-v4-pro", profiles[0])
	}
	if profiles[0].KeyStatus != "present" {
		t.Fatalf("profile KeyStatus = %q, want present", profiles[0].KeyStatus)
	}
}

func TestInteractiveWorkbenchPersistActiveProfileUpdatesWorkspaceAndSession(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"type\":\"final\",\"message\":\"ok\"}"}}]}`))
	}))
	defer server.Close()

	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	t.Setenv("KIMI_API_KEY", "kimi-secret")
	writeWorkbenchMultiProfileLLMConfig(t, workDir, server.URL)

	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer runtime.Close()
	session := console.NewSessionWithAgent("test", runtime.OperatorAgent)
	session.PersonaExtractor = runtime.PersonaExtractor
	session.PersonaExtractModelInfo = console.PersonaExtractModelInfo{
		Provider: runtime.PersonaExtractProvider,
		Model:    runtime.PersonaExtractModel,
		BaseURL:  runtime.PersonaExtractBaseURL,
	}
	driver := interactiveWorkbenchDriver{
		version: "test",
		runtime: runtime,
		session: session,
	}

	if err := driver.PersistActiveProfile("kimi"); err != nil {
		t.Fatalf("PersistActiveProfile(kimi) error = %v", err)
	}
	if got := modelLabel(session); got != "kimi-latest" {
		t.Fatalf("session model = %q, want kimi-latest", got)
	}
	if got := session.PersonaExtractModelInfo.Model; got != "kimi-latest" {
		t.Fatalf("PersonaExtractModelInfo.Model = %q, want kimi-latest", got)
	}
	if got := session.PersonaExtractModelInfo.Provider; got != "kimi" {
		t.Fatalf("PersonaExtractModelInfo.Provider = %q, want kimi", got)
	}

	cfg, err := runtime.ResolveLLMConfig(config.LLMPurposeOperator)
	if err != nil {
		t.Fatalf("ResolveLLMConfig(operator) error = %v", err)
	}
	if cfg.Profile != "kimi" || cfg.Model != "kimi-latest" {
		t.Fatalf("resolved profile/model = %q/%q, want kimi/kimi-latest", cfg.Profile, cfg.Model)
	}
	profiles, err := driver.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	var active string
	for _, profile := range profiles {
		if profile.Active {
			active = profile.ProfileName
		}
	}
	if active != "kimi" {
		t.Fatalf("active profile row = %q, want kimi: %+v", active, profiles)
	}

	freshRuntime, err := app.OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("fresh OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer freshRuntime.Close()
	freshCfg, err := freshRuntime.ResolveLLMConfig(config.LLMPurposeOperator)
	if err != nil {
		t.Fatalf("fresh ResolveLLMConfig(operator) error = %v", err)
	}
	if freshCfg.Profile != "kimi" || freshCfg.Model != "kimi-latest" {
		t.Fatalf("fresh profile/model = %q/%q, want kimi/kimi-latest", freshCfg.Profile, freshCfg.Model)
	}
}

func writeWorkbenchLLMConfig(t *testing.T, workDir string, baseURL string) {
	t.Helper()
	configDir := filepath.Join(workDir, ".lore")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.lore) error = %v", err)
	}
	body := fmt.Sprintf(`{
  "llm": {
    "active_profile": "deepseek",
    "profiles": {
      "deepseek": {
        "provider": "deepseek",
        "base_url": %q,
        "model": "deepseek-chat",
        "api_key_env": "DEEPSEEK_API_KEY"
      }
    }
  }
}`, baseURL)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
}

func writeWorkbenchMultiProfileLLMConfig(t *testing.T, workDir string, baseURL string) {
	t.Helper()
	configDir := filepath.Join(workDir, ".lore")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.lore) error = %v", err)
	}
	body := fmt.Sprintf(`{
  "llm": {
    "active_profile": "deepseek",
    "profiles": {
      "deepseek": {
        "provider": "deepseek",
        "base_url": %q,
        "model": "deepseek-chat",
        "api_key_env": "DEEPSEEK_API_KEY"
      },
      "kimi": {
        "provider": "kimi",
        "base_url": %q,
        "model": "kimi-latest",
        "api_key_env": "KIMI_API_KEY"
      }
    }
  }
}`, baseURL, baseURL)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
}
