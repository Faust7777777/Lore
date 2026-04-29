package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/adapter/codexappserver"
	"obsidian-harness/internal/config/configtest"
)

type fakeCodexAppServerProcess struct {
	thread   codexappserver.Thread
	lastRead codexappserver.ReadThreadParams
	closed   bool
}

func (f *fakeCodexAppServerProcess) ReadThread(params codexappserver.ReadThreadParams) (codexappserver.Thread, error) {
	f.lastRead = params
	return f.thread, nil
}

func (f *fakeCodexAppServerProcess) Close() error {
	f.closed = true
	return nil
}

func TestRunImportCodexAppServer(t *testing.T) {
	configureLLMTestEnv(t)
	workDir := t.TempDir()

	process := &fakeCodexAppServerProcess{
		thread: codexappserver.Thread{
			ID:        "thread-cli",
			CreatedAt: time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local).Unix(),
			Turns: []codexappserver.Turn{
				{
					ID:        "turn-1",
					CreatedAt: time.Date(2026, 4, 22, 9, 5, 0, 0, time.Local).Unix(),
					Items: []codexappserver.Item{
						{Type: "assistant_message", Text: "imported from cli"},
					},
				},
			},
		},
	}

	var (
		gotCommand string
		gotArgs    []string
		gotVersion string
		gotWorkDir string
	)
	previousStarter := startCodexAppServerProcess
	startCodexAppServerProcess = func(_ context.Context, workDir string, command string, args []string, version string) (codexAppServerReaderCloser, error) {
		gotWorkDir = workDir
		gotCommand = command
		gotArgs = append([]string(nil), args...)
		gotVersion = version
		return process, nil
	}
	t.Cleanup(func() {
		startCodexAppServerProcess = previousStarter
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runImportCodexAppServer([]string{
		"--workdir", workDir,
		"--server-workdir", filepath.Join(workDir, "repo"),
		"--thread", "thread-cli",
		"--agent", "codex",
		"--session", "session-cli",
		"--",
		"codex",
		"app-server",
	}, &stdout, &stderr, "test-version")
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if gotCommand != "codex" || strings.Join(gotArgs, " ") != "app-server" {
		t.Fatalf("starter = (%q, %q), want codex app-server", gotCommand, strings.Join(gotArgs, " "))
	}
	if gotVersion != "test-version" {
		t.Fatalf("version = %q, want test-version", gotVersion)
	}
	if gotWorkDir != filepath.Join(workDir, "repo") {
		t.Fatalf("workDir = %q, want server workdir", gotWorkDir)
	}
	if process.lastRead.ThreadID != "thread-cli" || !process.lastRead.IncludeTurns {
		t.Fatalf("lastRead = %+v, want includeTurns thread read", process.lastRead)
	}
	if !process.closed {
		t.Fatal("expected fake process to be closed")
	}
	if !strings.Contains(stdout.String(), "Codex app-server thread imported") {
		t.Fatalf("stdout = %q, want import summary", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr", stderr.String())
	}
}

func TestParseCodexAppServerFlagsRequiresCommandAfterSeparator(t *testing.T) {
	var stderr bytes.Buffer
	_, _, _, _, _, err := parseCodexAppServerFlags("import-codex-appserver", []string{"--thread", "thread-1"}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "command is required after --") {
		t.Fatalf("parseCodexAppServerFlags() error = %v, want missing command", err)
	}
}

func configureLLMTestEnv(t *testing.T) {
	t.Helper()
	configtest.IsolateHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{
						"content": `{"title":"codex checkpoint","content":"## Summary\n- checkpoint summary from test provider"}`,
					}},
				},
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 5,
				},
			})
		case "/responses":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]any{
					{
						"type": "message",
						"role": "assistant",
						"content": []map[string]any{
							{"type": "output_text", "text": `{"title":"codex checkpoint","content":"## Summary\n- checkpoint summary from test provider"}`},
						},
					},
				},
				"usage": map[string]any{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	for _, key := range []string{
		"OBSIDIAN_HARNESS_LLM_BASE_URL",
		"OBSIDIAN_HARNESS_LLM_API_KEY",
		"OBSIDIAN_HARNESS_LLM_MODEL",
		"OBSIDIAN_HARNESS_OPERATOR_BASE_URL",
		"OBSIDIAN_HARNESS_OPERATOR_API_KEY",
		"OBSIDIAN_HARNESS_OPERATOR_MODEL",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("LORE_LLM_BASE_URL", server.URL)
	t.Setenv("LORE_LLM_API_KEY", "secret")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")
}
