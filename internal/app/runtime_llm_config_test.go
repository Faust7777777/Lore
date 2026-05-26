package app

import (
	"context"
	"encoding/json"
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
