package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/vault"
)

func clearOperatorEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OBSIDIAN_HARNESS_LLM_BASE_URL",
		"OBSIDIAN_HARNESS_LLM_API_KEY",
		"OBSIDIAN_HARNESS_LLM_MODEL",
		"OBSIDIAN_HARNESS_OPERATOR_BASE_URL",
		"OBSIDIAN_HARNESS_OPERATOR_API_KEY",
		"OBSIDIAN_HARNESS_OPERATOR_MODEL",
		"LORE_LLM_BASE_URL",
		"LORE_LLM_API_KEY",
		"LORE_LLM_MODEL",
		"LORE_OPERATOR_BASE_URL",
		"LORE_OPERATOR_API_KEY",
		"LORE_OPERATOR_MODEL",
	} {
		t.Setenv(key, "")
	}
}

func newOperatorAgentTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	type chatRequest struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			var req chatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			if len(req.Messages) == 0 {
				t.Fatal("chat request missing messages")
			}

			systemPrompt := req.Messages[0].Content
			userPrompt := req.Messages[len(req.Messages)-1].Content
			content := `{"action":"help"}`
			switch {
			case strings.Contains(systemPrompt, "external coding-agent checkpoint window"):
				content = `{"title":"codex checkpoint 09:00-09:30","content":"## Summary\n- checkpoint summary from test provider"}`
			case strings.Contains(systemPrompt, "day of external coding-agent checkpoints"):
				content = `{"title":"codex daily report","content":"## Summary\n- daily summary from test provider"}`
			case strings.Contains(userPrompt, "show current status"):
				content = `{"action":"show_status"}`
			case strings.Contains(userPrompt, "review draft"):
				content = `{"action":"review_draft"}`
			case strings.Contains(userPrompt, "approve current draft"):
				content = `{"action":"approve_draft","use_focused_draft":true}`
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": content}},
				},
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 5,
				},
			})
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "gpt-5.4"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func configureLLMTestEnv(t *testing.T) {
	t.Helper()
	clearOperatorEnv(t)
	server := newOperatorAgentTestServer(t)
	t.Cleanup(server.Close)
	t.Setenv("OBSIDIAN_HARNESS_LLM_BASE_URL", server.URL)
	t.Setenv("OBSIDIAN_HARNESS_LLM_API_KEY", "secret")
	t.Setenv("OBSIDIAN_HARNESS_LLM_MODEL", "gpt-5.4")
}

func TestRunDefaultsToStatus(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(nil, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Obsidian Harness") {
		t.Fatalf("expected status output, got %q", stdout.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"version"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if got := strings.TrimSpace(stdout.String()); got != version {
		t.Fatalf("expected version %q, got %q", version, got)
	}
}

func TestRunStatusUsesProvidedWorkDir(t *testing.T) {
	workDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"status", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), filepath.Join(workDir, "vault")) {
		t.Fatalf("expected status output to include workdir vault path, got %q", stdout.String())
	}
}

func TestRunBootstrap(t *testing.T) {
	workDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"bootstrap", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(workDir, "vault")); err != nil {
		t.Fatalf("expected vault directory to exist: %v", err)
	}
	if !strings.Contains(stdout.String(), "bootstrapped") {
		t.Fatalf("expected bootstrap summary, got %q", stdout.String())
	}
}

func TestRunDemoP0A(t *testing.T) {
	workDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"demo-p0a", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "P0-A completed") {
		t.Fatalf("expected P0-A completion output, got %q", stdout.String())
	}
}

func TestRunDemoP0B(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"demo-p0b", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "P0-B completed") {
		t.Fatalf("expected P0-B completion output, got %q", stdout.String())
	}
}

func TestRunConsoleOnceStatus(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"console", "--workdir", workDir, "--once", "show current status"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Managed Status") {
		t.Fatalf("expected managed status output, got %q", stdout.String())
	}
}

func TestRunModelsList(t *testing.T) {
	clearOperatorEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-5.4"},
				{"id": "gpt-4.1"},
			},
		})
	}))
	defer server.Close()

	t.Setenv("OBSIDIAN_HARNESS_LLM_BASE_URL", server.URL)
	t.Setenv("OBSIDIAN_HARNESS_LLM_API_KEY", "secret")
	t.Setenv("OBSIDIAN_HARNESS_LLM_MODEL", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"models", "list"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Available models (2)") {
		t.Fatalf("expected models output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Recommended model: gpt-5.4") {
		t.Fatalf("expected recommended model output, got %q", stdout.String())
	}
}

func TestRunImportCodexJSONL(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)
	transcriptPath := filepath.Join(workDir, "sample.jsonl")
	content := "" +
		"{\"timestamp\":\"2026-04-22T09:01:00+08:00\",\"type\":\"session_meta\",\"payload\":{\"id\":\"session-1\",\"agent_nickname\":\"Codex\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"build adapter\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:35:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"phase\":\"commentary\",\"message\":\"adapter imported\"}}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"import-codex-jsonl",
		"--workdir", workDir,
		"--input", transcriptPath,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Codex JSONL imported") {
		t.Fatalf("expected import summary, got %q", stdout.String())
	}
}

func TestRunImportCodexJSONLMissingInput(t *testing.T) {
	workDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"import-codex-jsonl", "--workdir", workDir}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "--input is required") {
		t.Fatalf("expected missing input error, got %q", stderr.String())
	}
}

func TestRunSyncCodexJSONL(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)
	transcriptPath := filepath.Join(workDir, "sync.jsonl")
	content := "" +
		"{\"timestamp\":\"2026-04-22T09:01:00+08:00\",\"type\":\"session_meta\",\"payload\":{\"id\":\"session-1\",\"agent_nickname\":\"Codex\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"build adapter\"}}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"sync-codex-jsonl",
		"--workdir", workDir,
		"--input", transcriptPath,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Codex JSONL synced") {
		t.Fatalf("expected sync summary, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = run([]string{
		"sync-codex-jsonl",
		"--workdir", workDir,
		"--input", transcriptPath,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code on unchanged sync, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Codex JSONL unchanged") {
		t.Fatalf("expected unchanged summary, got %q", stdout.String())
	}
}

func TestRunAttachCodexJSONLOnce(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)
	transcriptPath := filepath.Join(workDir, "attach.jsonl")
	content := "" +
		"{\"timestamp\":\"2026-04-22T09:01:00+08:00\",\"type\":\"session_meta\",\"payload\":{\"id\":\"session-attach\",\"agent_nickname\":\"Codex\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"attach mode\"}}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"attach-codex-jsonl",
		"--workdir", workDir,
		"--input", transcriptPath,
		"--once",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Attaching Codex JSONL") {
		t.Fatalf("expected attach banner, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Synced") {
		t.Fatalf("expected attach sync output, got %q", stdout.String())
	}
}

func TestRunDraftListAndReview(t *testing.T) {
	workDir := t.TempDir()
	draftID := seedDraftForCLI(t, workDir, "list review me")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"draft", "list", "--workdir", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Draft Inbox") || !strings.Contains(stdout.String(), draftID[:16]) {
		t.Fatalf("expected draft inbox output, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = run([]string{"draft", "review", "--workdir", workDir, draftID}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Draft Review") || !strings.Contains(stdout.String(), draftID) {
		t.Fatalf("expected draft review output, got %q", stdout.String())
	}
}

func TestRunDraftApproveAndApply(t *testing.T) {
	workDir := t.TempDir()
	draftID := seedDraftForCLI(t, workDir, "apply me")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"draft", "approve", "--workdir", workDir, draftID}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "approved") {
		t.Fatalf("expected approved output, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = run([]string{"draft", "apply", "--workdir", workDir, draftID}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "applied") {
		t.Fatalf("expected applied output, got %q", stdout.String())
	}
}

func TestRunDraftReject(t *testing.T) {
	workDir := t.TempDir()
	draftID := seedDraftForCLI(t, workDir, "reject me")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"draft", "reject", "--workdir", workDir, draftID}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "rejected") {
		t.Fatalf("expected rejected output, got %q", stdout.String())
	}
}

func TestRunProcessSinkDay(t *testing.T) {
	workDir := t.TempDir()
	seedProcessSinkForCLI(t, workDir)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"process-sink", "day", "--workdir", workDir, "--agent", "codex", "--day", "2026-04-22"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Process Sink Day") || !strings.Contains(stdout.String(), "09:00-09:30") {
		t.Fatalf("expected process-sink day output, got %q", stdout.String())
	}
}

func TestRunConsoleREPLDraftFlow(t *testing.T) {
	workDir := t.TempDir()
	seedDraftForCLI(t, workDir, "console flow")
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	input := strings.NewReader("review draft\napprove current draft\nexit\n")
	exitCode := runConsoleCommand([]string{"--workdir", workDir}, input, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Draft Review") {
		t.Fatalf("expected draft review output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "approved") {
		t.Fatalf("expected approve output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Console stopped") {
		t.Fatalf("expected console stop output, got %q", stdout.String())
	}
}

func TestRunDaemonOnceTriggersDraftAfterStablePlanChange(t *testing.T) {
	workDir := t.TempDir()
	clearOperatorEnv(t)

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	if _, err := runtime.Bootstrap(time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	planPath := filepath.Join(runtime.Config.Paths.VaultRoot, "0-排期", "04-执行", "week.md")
	writeMainTestPlan(t, runtime, planPath, "# Week\n\n- [ ] initial", time.Now().Add(-2*time.Second))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"daemon", "run", "--workdir", workDir, "--once"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code on prime scan, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Vault daemon scan") {
		t.Fatalf("expected daemon scan output, got %q", stdout.String())
	}

	writeMainTestPlan(t, runtime, planPath, "# Week\n\n- [x] changed", time.Now().Add(-2*time.Second))
	stdout.Reset()
	stderr.Reset()
	exitCode = run([]string{"daemon", "run", "--workdir", workDir, "--once"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code on changed scan, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "triggered drafts: 1") {
		t.Fatalf("expected triggered draft output, got %q", stdout.String())
	}
}

func TestRunUnknownCommandReturnsUsageError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"nope"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown command: nope") {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "status") {
		t.Fatalf("expected usage text in stderr, got %q", stderr.String())
	}
}

func seedDraftForCLI(t *testing.T, workDir string, content string) string {
	t.Helper()

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	if _, err := runtime.Bootstrap(time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	draft, err := runtime.Harness.ObserveDocumentChange(filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "week.md"), []byte(content), time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ObserveDocumentChange() error = %v", err)
	}
	return draft.ID
}

func writeMainTestPlan(t *testing.T, runtime *app.Runtime, absPath string, content string, modTime time.Time) {
	t.Helper()

	if _, err := vault.WriteFileAtomic(absPath, []byte(content), runtime.Config.Vault.TempSuffix); err != nil {
		t.Fatalf("WriteFileAtomic(plan) error = %v", err)
	}
	if err := os.Chtimes(absPath, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(plan) error = %v", err)
	}
}

func seedProcessSinkForCLI(t *testing.T, workDir string) {
	t.Helper()

	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	windowStart := time.Date(2026, 4, 22, 9, 0, 0, 0, time.Local)
	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "session-1",
		WindowStart: windowStart,
		WindowEnd:   windowStart.Add(30 * time.Minute),
	}
	if _, err := runtime.Harness.IngestSessionWindow(window, "morning checkpoint", "summary", "raw", window.WindowEnd); err != nil {
		t.Fatalf("IngestSessionWindow() error = %v", err)
	}
	if _, err := runtime.Harness.RollupDaily("codex", windowStart, windowStart.Add(12*time.Hour)); err != nil {
		t.Fatalf("RollupDaily() error = %v", err)
	}
}
