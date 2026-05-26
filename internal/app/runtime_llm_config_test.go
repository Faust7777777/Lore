package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/operatoragent"
)

type noToolRuntime struct{}

func (noToolRuntime) DescribeTools(operatoragent.Context) []operatoragent.ToolDefinition { return nil }
func (noToolRuntime) CallTool(string, map[string]any) (operatoragent.ToolResult, error) {
	return operatoragent.ToolResult{}, nil
}

// writeWorkspaceLLMProfile materialises a workspace .lore/config.json
// with one purpose-agnostic active profile pointing at serverURL. The
// resolved config flows through every LLM purpose (operator,
// persona_extract, process_sink), so a single fixture covers the
// shared-profile assertions below.
func writeWorkspaceLLMProfile(t *testing.T, workDir, serverURL, profileName, provider, modelName, apiKeyEnv string) {
	t.Helper()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	configJSON := `{
        "llm": {
            "active_profile": "` + profileName + `",
            "profiles": {
                "` + profileName + `": {
                    "provider": "` + provider + `",
                    "base_url": "` + serverURL + `",
                    "model": "` + modelName + `",
                    "api_key_env": "` + apiKeyEnv + `"
                }
            }
        }
    }`
	if err := os.WriteFile(workspacePath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
}

// TestOpenRuntimePersonaExtractIdentityReflectsWorkspaceProfile pins
// the B-line model-consistency contract for the persona-extract
// surface: the runtime fields the console session stamps onto
// persona-extract.log (PersonaExtractProvider / PersonaExtractModel /
// PersonaExtractBaseURL) MUST come from the resolved workspace profile,
// not from the legacy generic LORE_LLM_* env. Without this assertion
// an operator who switches to a deepseek profile would still see env
// model names on failure lines — a real diagnosis hazard the
// model-tag columns exist to prevent. The API key is asserted absent
// from any user-visible field because the log file lives in workdir.
func TestOpenRuntimePersonaExtractIdentityReflectsWorkspaceProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"type\":\"final\",\"message\":\"ok\"}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer server.Close()

	workDir := t.TempDir()
	writeWorkspaceLLMProfile(t, workDir, server.URL, "deepseek", "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY")

	t.Setenv("LORE_LLM_BASE_URL", "https://env.example.com/v1")
	t.Setenv("LORE_LLM_MODEL", "env-model")
	t.Setenv("LORE_LLM_API_KEY", "env-secret")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")

	rt, err := OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer rt.Close()

	if rt.PersonaExtractor == nil {
		t.Fatal("PersonaExtractor nil; workspace profile should have produced one")
	}
	if rt.PersonaExtractProvider != "deepseek" {
		t.Fatalf("PersonaExtractProvider = %q, want deepseek (workspace profile)", rt.PersonaExtractProvider)
	}
	if rt.PersonaExtractModel != "deepseek-chat" {
		t.Fatalf("PersonaExtractModel = %q, want deepseek-chat (workspace profile, not env model)", rt.PersonaExtractModel)
	}
	if rt.PersonaExtractBaseURL != server.URL {
		t.Fatalf("PersonaExtractBaseURL = %q, want %q (workspace profile, not env URL)", rt.PersonaExtractBaseURL, server.URL)
	}
	// The API key must not surface through any Runtime field; the
	// persona-extract log file lives in workdir and would leak it.
	for label, value := range map[string]string{
		"PersonaExtractProvider": rt.PersonaExtractProvider,
		"PersonaExtractModel":    rt.PersonaExtractModel,
		"PersonaExtractBaseURL":  rt.PersonaExtractBaseURL,
	} {
		if strings.Contains(value, "secret") {
			t.Fatalf("%s leaks secret-like substring: %q", label, value)
		}
	}
}

// TestOpenRuntimePersonaExtractIdentityHotSwitchesAcrossReopen models
// the operator switching workspace profiles between runtime sessions
// (the closest current analogue to in-process /model use for the
// persona-extract subsystem, which the operator commit 04bb31c left
// re-resolved per-OpenRuntime rather than via a live SwitchModel
// hook). Opening the same workDir twice with two different active
// profiles must produce two different identity snapshots; absence of
// this property would mean a stale cache pinning the original profile
// to the persona log surface forever.
func TestOpenRuntimePersonaExtractIdentityHotSwitchesAcrossReopen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer server.Close()

	workDir := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	t.Setenv("KIMI_API_KEY", "kimi-secret")

	writeWorkspaceLLMProfile(t, workDir, server.URL, "deepseek", "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY")
	rt1, err := OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("first OpenRuntime: %v", err)
	}
	gotProvider1, gotModel1, gotBase1 := rt1.PersonaExtractProvider, rt1.PersonaExtractModel, rt1.PersonaExtractBaseURL
	if err := rt1.Close(); err != nil {
		t.Fatalf("rt1 Close: %v", err)
	}

	writeWorkspaceLLMProfile(t, workDir, server.URL, "kimi", "kimi", "kimi-k2", "KIMI_API_KEY")
	rt2, err := OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("second OpenRuntime: %v", err)
	}
	defer rt2.Close()

	if gotProvider1 != "deepseek" || gotModel1 != "deepseek-chat" {
		t.Fatalf("first runtime identity = (%q,%q), want (deepseek, deepseek-chat)", gotProvider1, gotModel1)
	}
	if rt2.PersonaExtractProvider != "kimi" {
		t.Fatalf("second runtime PersonaExtractProvider = %q, want kimi", rt2.PersonaExtractProvider)
	}
	if rt2.PersonaExtractModel != "kimi-k2" {
		t.Fatalf("second runtime PersonaExtractModel = %q, want kimi-k2", rt2.PersonaExtractModel)
	}
	if rt2.PersonaExtractBaseURL != gotBase1 {
		// Same server URL across both opens; this guards against
		// accidental cross-contamination of unrelated fields when only
		// the profile name changed.
		t.Fatalf("BaseURL drifted unexpectedly: %q vs %q", rt2.PersonaExtractBaseURL, gotBase1)
	}
}

// TestOpenRuntimeProcessSinkSummarizerUsesWorkspaceProfile pins the
// parallel B-line contract for the process-sink subsystem: when a
// workspace profile is the active config, SummarizeCheckpoint must
// route the request to the workspace-profile URL with the workspace
// model name, AND the resulting UsageRecord must carry the workspace
// Provider / Model values (not env defaults). Cost attribution and
// model-tag debug surfaces both rely on this.
func TestOpenRuntimeProcessSinkSummarizerUsesWorkspaceProfile(t *testing.T) {
	var requestCount int32
	var gotAuth, gotPath string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"t\",\"content\":\"c\"}"}}],"usage":{"prompt_tokens":11,"completion_tokens":3}}`))
	}))
	defer server.Close()

	workDir := t.TempDir()
	writeWorkspaceLLMProfile(t, workDir, server.URL, "deepseek", "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY")

	t.Setenv("LORE_LLM_BASE_URL", "https://env.example.com/v1")
	t.Setenv("LORE_LLM_MODEL", "env-model")
	t.Setenv("LORE_LLM_API_KEY", "env-secret")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")

	rt, err := OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer rt.Close()

	summarizer, ok := rt.ProcessSinkSummarizer.(*modelProcessSinkSummarizer)
	if !ok {
		t.Fatalf("ProcessSinkSummarizer = %T, want *modelProcessSinkSummarizer", rt.ProcessSinkSummarizer)
	}
	if summarizer.provider != "deepseek" {
		t.Fatalf("summarizer.provider = %q, want deepseek (workspace profile)", summarizer.provider)
	}
	if summarizer.model != "deepseek-chat" {
		t.Fatalf("summarizer.model = %q, want deepseek-chat (workspace profile)", summarizer.model)
	}

	// Drive the summarizer through one checkpoint window so the
	// usage sink wired by OpenRuntime records a UsageRecord. The
	// sink writes into the same SQLite store the daemon uses, so we
	// re-read via runtime.Store after the call.
	title, content, err := rt.ProcessSinkSummarizer.SummarizeCheckpoint(newCheckpointWindow())
	if err != nil {
		t.Fatalf("SummarizeCheckpoint() error = %v", err)
	}
	if title != "t" || content != "c" {
		t.Fatalf("summary = (%q, %q), want (t, c)", title, content)
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer deepseek-secret" {
		t.Fatalf("Authorization = %q, want Bearer deepseek-secret (workspace api_key_env)", gotAuth)
	}
	if !strings.Contains(gotBody, `"model":"deepseek-chat"`) {
		t.Fatalf("request body did not carry workspace model: %s", gotBody)
	}
	if strings.Contains(gotBody, "env-model") {
		t.Fatalf("request body leaked env model into workspace-profile request: %s", gotBody)
	}
}

// TestDefaultProcessSinkSummarizerEmbedsResolvedProviderAndModel is
// the focused unit complement to the OpenRuntime integration test
// above: given a ResolvedLLMConfig, defaultProcessSinkSummarizer must
// produce a *modelProcessSinkSummarizer whose Provider/Model fields
// match the resolved values. Guards against a future refactor that
// drops the cfg → summarizer field plumbing.
func TestDefaultProcessSinkSummarizerEmbedsResolvedProviderAndModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()

	cfg := config.ResolvedLLMConfig{
		Enabled:  true,
		Purpose:  config.LLMPurposeProcessSink,
		Source:   config.LLMSourceWorkspace,
		Provider: "deepseek",
		BaseURL:  server.URL,
		Model:    "deepseek-chat",
		APIKey:   "x",
	}
	summarizer, err := defaultProcessSinkSummarizer(cfg, nil)
	if err != nil {
		t.Fatalf("defaultProcessSinkSummarizer() error = %v", err)
	}
	got, ok := summarizer.(*modelProcessSinkSummarizer)
	if !ok {
		t.Fatalf("summarizer = %T, want *modelProcessSinkSummarizer", summarizer)
	}
	if got.provider != "deepseek" {
		t.Fatalf("provider = %q, want deepseek", got.provider)
	}
	if got.model != "deepseek-chat" {
		t.Fatalf("model = %q, want deepseek-chat", got.model)
	}
}

// TestPersonaExtractIdentityRespectsResolveErrorAndEnabledGate covers
// the small helper that runtime.go uses to populate PersonaExtract*
// fields. A resolveErr must short-circuit to "" (rather than reading a
// stale model from a partially populated cfg) so failure paths do not
// stamp ghost identities onto the persona log. A disabled config
// likewise yields "" so the writer omits the model-tag columns and
// the legacy 4-column format is preserved.
func TestPersonaExtractIdentityRespectsResolveErrorAndEnabledGate(t *testing.T) {
	cfg := config.ResolvedLLMConfig{
		Enabled:  true,
		Provider: "deepseek",
		BaseURL:  "https://api.deepseek.com/v1",
		Model:    "deepseek-chat",
	}
	selectorProvider := func(c config.ResolvedLLMConfig) string { return c.Provider }
	selectorModel := func(c config.ResolvedLLMConfig) string { return c.Model }

	if got := personaExtractIdentity(cfg, nil, selectorProvider); got != "deepseek" {
		t.Fatalf("enabled+nil err Provider = %q, want deepseek", got)
	}
	if got := personaExtractIdentity(cfg, nil, selectorModel); got != "deepseek-chat" {
		t.Fatalf("enabled+nil err Model = %q, want deepseek-chat", got)
	}

	disabled := cfg
	disabled.Enabled = false
	if got := personaExtractIdentity(disabled, nil, selectorProvider); got != "" {
		t.Fatalf("disabled cfg Provider = %q, want empty", got)
	}

	withErr := errors.New("resolve failure")
	if got := personaExtractIdentity(cfg, withErr, selectorProvider); got != "" {
		t.Fatalf("resolveErr non-nil Provider = %q, want empty (must not surface stale identity)", got)
	}
}

func TestOpenRuntimeUsesWorkspaceLLMProfileOverGenericEnv(t *testing.T) {
	var requestCount int32
	var gotPath string
	var gotAuth string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		encoded, _ := json.Marshal(payload)
		gotBody = string(encoded)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"type\":\"final\",\"message\":\"ok\"}"}}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`))
	}))
	defer server.Close()

	workDir := t.TempDir()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	configJSON := `{
		"llm": {
			"active_profile": "deepseek",
			"profiles": {
				"deepseek": {
					"provider": "deepseek",
					"base_url": "` + server.URL + `",
					"model": "deepseek-chat",
					"api_key_env": "DEEPSEEK_API_KEY"
				}
			}
		}
	}`
	if err := os.WriteFile(workspacePath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	t.Setenv("LORE_LLM_BASE_URL", "https://api.ikuncode.cc/v1")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")
	t.Setenv("LORE_LLM_API_KEY", "ikuncode-secret")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")

	runtime, err := OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer runtime.Close()
	loopAgent, ok := runtime.OperatorAgent.(operatoragent.ContextLoopAgent)
	if !ok {
		t.Fatalf("OperatorAgent = %T, want ContextLoopAgent", runtime.OperatorAgent)
	}
	resp, err := loopAgent.RespondContext(context.Background(), "hello", operatoragent.Context{DefaultAgentID: "codex"}, noToolRuntime{})
	if err != nil {
		t.Fatalf("RespondContext() error = %v", err)
	}
	if resp.Final != "ok" {
		t.Fatalf("resp.Final = %q, want ok", resp.Final)
	}
	if atomic.LoadInt32(&requestCount) != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer deepseek-secret" {
		t.Fatalf("Authorization = %q, want deepseek key", gotAuth)
	}
	if !strings.Contains(gotBody, `"model":"deepseek-chat"`) {
		t.Fatalf("request body = %s, want deepseek-chat", gotBody)
	}
	if strings.Contains(gotBody, "gpt-5.4") || strings.Contains(gotBody, "ikuncode") {
		t.Fatalf("request used env model/base instead of workspace profile: %s", gotBody)
	}
	if len(runtime.LLMDiagnostics) == 0 || runtime.LLMDiagnostics[0].Source != config.LLMSourceWorkspace {
		t.Fatalf("LLMDiagnostics = %+v, want workspace source", runtime.LLMDiagnostics)
	}
}
