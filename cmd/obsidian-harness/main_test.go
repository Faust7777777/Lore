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
	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/vault"
)

func clearOperatorEnv(t *testing.T) {
	t.Helper()
	configtest.IsolateHome(t)
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

	type responsesRequest struct {
		Instructions string `json:"instructions"`
		Input        []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"input"`
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
			case strings.Contains(userPrompt, "Tool result for vault_write_low"):
				content = `{"type":"final","message":"Diary written to 03-notes/diary.md"}`
			case strings.Contains(userPrompt, "write a diary"):
				content = `{"type":"tool_call","tool":"vault_write_low","arguments":{"path":"03-notes/diary.md","content":"# Diary\n\nToday I reviewed Lore progress.","overwrite":false}}`
			case strings.Contains(userPrompt, "approve current draft"):
				content = `{"action":"approve_draft","use_focused_draft":true}`
			case strings.Contains(userPrompt, "review draft"):
				content = `{"action":"review_draft"}`
			case strings.Contains(userPrompt, "show current status"):
				content = `{"action":"show_status"}`
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
		case "/responses":
			var req responsesRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode responses request: %v", err)
			}
			if len(req.Input) == 0 {
				t.Fatal("responses request missing input")
			}

			systemPrompt := req.Instructions
			userPrompt := req.Input[len(req.Input)-1].Content
			content := `{"action":"help"}`
			switch {
			case strings.Contains(systemPrompt, "external coding-agent checkpoint window"):
				content = `{"title":"codex checkpoint 09:00-09:30","content":"## Summary\n- checkpoint summary from test provider"}`
			case strings.Contains(systemPrompt, "day of external coding-agent checkpoints"):
				content = `{"title":"codex daily report","content":"## Summary\n- daily summary from test provider"}`
			case strings.Contains(userPrompt, "Tool result for vault_write_low"):
				content = `{"type":"final","message":"Diary written to 03-notes/diary.md"}`
			case strings.Contains(userPrompt, "write a diary"):
				content = `{"type":"tool_call","tool":"vault_write_low","arguments":{"path":"03-notes/diary.md","content":"# Diary\n\nToday I reviewed Lore progress.","overwrite":false}}`
			case strings.Contains(userPrompt, "approve current draft"):
				content = `{"action":"approve_draft","use_focused_draft":true}`
			case strings.Contains(userPrompt, "review draft"):
				content = `{"action":"review_draft"}`
			case strings.Contains(userPrompt, "show current status"):
				content = `{"action":"show_status"}`
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]any{
					{
						"type": "message",
						"role": "assistant",
						"content": []map[string]any{
							{"type": "output_text", "text": content},
						},
					},
				},
				"usage": map[string]any{
					"input_tokens":  10,
					"output_tokens": 5,
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
	t.Setenv("LORE_LLM_BASE_URL", server.URL)
	t.Setenv("LORE_LLM_API_KEY", "secret")
	t.Setenv("LORE_LLM_MODEL", "gpt-5.4")
}

func TestRunDefaultsToStatus(t *testing.T) {
	configtest.IsolateHome(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(nil, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Lore") {
		t.Fatalf("expected status output, got %q", stdout.String())
	}
}

func TestRunVersion(t *testing.T) {
	configtest.IsolateHome(t)
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
	configtest.IsolateHome(t)
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
	configtest.IsolateHome(t)
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

	output := stdout.String()
	if !strings.Contains(output, "bootstrapped") {
		t.Fatalf("expected bootstrap summary, got %q", output)
	}
	// Onboarding guidance must be present
	if !strings.Contains(output, "Next steps:") {
		t.Fatalf("bootstrap output should contain 'Next steps:', got %q", output)
	}
	if !strings.Contains(output, "fill in your vault goals") {
		t.Fatalf("bootstrap output should guide user to system doc, got %q", output)
	}
	if !strings.Contains(output, "add your profile") {
		t.Fatalf("bootstrap output should guide user to persona doc, got %q", output)
	}
	if !strings.Contains(output, "lore tui --workdir") {
		t.Fatalf("bootstrap output should suggest lore tui, got %q", output)
	}
}

func TestRunDemoP0A(t *testing.T) {
	configtest.IsolateHome(t)
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

func TestRunSmokeP0(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"smoke", "p0", "--workdir", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "P0 smoke passed") {
		t.Fatalf("expected smoke success output, got %q", stdout.String())
	}
	for _, expected := range []string{
		"managed_core_ready",
		"p0a_draft_applied",
		"p0b_checkpoint_materialized",
		"audit_chain_present",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("expected smoke output to contain %q, got %q", expected, stdout.String())
		}
	}
}

func TestRunSmokeP0FullIncludesGovernedNoteIntake(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"smoke", "p0", "--workdir", workDir, "--full"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	for _, expected := range []string{
		"P0 smoke passed",
		"Governed note intake passed",
		"proposal_created_pending_draft",
		"approved_apply_writes_note",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("expected full smoke output to contain %q, got %q", expected, stdout.String())
		}
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

func TestRunConsoleOnceWritesSessionTranscript(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"console", "--workdir", workDir, "--once", "show current status"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}

	sessionDir := filepath.Join(workDir, "state", "sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir(sessionDir) error = %v", err)
	}
	foundTranscript := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			foundTranscript = true
			data, err := os.ReadFile(filepath.Join(sessionDir, entry.Name()))
			if err != nil {
				t.Fatalf("ReadFile(transcript) error = %v", err)
			}
			text := string(data)
			for _, want := range []string{"session_meta", "user_message", "assistant_message", "session_end"} {
				if !strings.Contains(text, want) {
					t.Fatalf("transcript missing %q: %s", want, text)
				}
			}
		}
	}
	if !foundTranscript {
		t.Fatal("expected at least one jsonl transcript")
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "index.json")); err != nil {
		t.Fatalf("index.json missing: %v", err)
	}
}

func TestRunConsoleResumeIDUsesPreviousHistory(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"console", "--workdir", workDir, "--once", "show current status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("initial console exit = %d, stderr = %q", code, stderr.String())
	}

	sessionDir := filepath.Join(workDir, "state", "sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir(sessionDir) error = %v", err)
	}
	var sessionID string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			sessionID = strings.TrimSuffix(entry.Name(), ".jsonl")
			break
		}
	}
	if sessionID == "" {
		t.Fatal("missing session transcript")
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"console", "--workdir", workDir, "--resume-id", sessionID, "--once", "show current status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("resume console exit = %d, stderr = %q", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(sessionDir, sessionID+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile(resumed transcript) error = %v", err)
	}
	if count := strings.Count(string(data), "user_message"); count < 2 {
		t.Fatalf("resumed transcript user_message count = %d, want at least 2: %s", count, string(data))
	}
}
func TestRunConsoleStartsFreshSessionWithoutResume(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"console", "--workdir", workDir, "--once", "show current status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("first console exit = %d, stderr = %q", code, stderr.String())
	}

	sessionDir := filepath.Join(workDir, "state", "sessions")
	firstEntries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir(sessionDir) error = %v", err)
	}
	firstTranscripts := transcriptFiles(firstEntries)
	if len(firstTranscripts) != 1 {
		t.Fatalf("first transcript count = %d, want 1: %+v", len(firstTranscripts), firstTranscripts)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"console", "--workdir", workDir, "--once", "show current status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("second console exit = %d, stderr = %q", code, stderr.String())
	}

	secondEntries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir(sessionDir) error = %v", err)
	}
	secondTranscripts := transcriptFiles(secondEntries)
	if len(secondTranscripts) != 2 {
		t.Fatalf("second transcript count = %d, want 2: %+v", len(secondTranscripts), secondTranscripts)
	}

	for _, name := range secondTranscripts {
		data, err := os.ReadFile(filepath.Join(sessionDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		if count := strings.Count(string(data), "user_message"); count != 1 {
			t.Fatalf("%s user_message count = %d, want 1: %s", name, count, string(data))
		}
	}
}

func transcriptFiles(entries []os.DirEntry) []string {
	out := make([]string, 0)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			out = append(out, entry.Name())
		}
	}
	return out
}
func TestRunSessionsSearchFindsTranscriptContent(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"console", "--workdir", workDir, "--once", "write a diary"}, &stdout, &stderr); code != 0 {
		t.Fatalf("console exit = %d, stderr = %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"sessions", "search", "--workdir", workDir, "Diary written"}, &stdout, &stderr); code != 0 {
		t.Fatalf("sessions search exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "lore-") || !strings.Contains(stdout.String(), "write a diary") {
		t.Fatalf("sessions search output = %q", stdout.String())
	}
}
func TestRunSessionsShowDisplaysTranscriptSummary(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"console", "--workdir", workDir, "--once", "show current status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("console exit = %d, stderr = %q", code, stderr.String())
	}

	sessionDir := filepath.Join(workDir, "state", "sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir(sessionDir) error = %v", err)
	}
	transcripts := transcriptFiles(entries)
	if len(transcripts) != 1 {
		t.Fatalf("transcripts = %+v, want one", transcripts)
	}
	sessionID := strings.TrimSuffix(transcripts[0], ".jsonl")

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"sessions", "show", "--workdir", workDir, sessionID}, &stdout, &stderr); code != 0 {
		t.Fatalf("sessions show exit = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"Session: " + sessionID, "Conversation:", "user: show current status", "assistant:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("sessions show output missing %q: %s", want, stdout.String())
		}
	}
}
func TestRunConsoleOnceWritesLowRiskDiary(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"console", "--workdir", workDir, "--once", "write a diary from today's report"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Diary written") {
		t.Fatalf("expected diary final output, got %q", stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(workDir, "vault", "03-notes", "diary.md"))
	if err != nil {
		t.Fatalf("ReadFile(diary) error = %v", err)
	}
	if !strings.Contains(string(data), "Today I reviewed Lore progress.") {
		t.Fatalf("diary content = %q", string(data))
	}
}

func TestRunTUIOnceStatus(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"tui", "--workdir", workDir, "--once", "show current status"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	text := stdout.String()
	for _, expected := range []string{
		"Lore",
		"Pending Drafts",
		"Process Sink",
		"Conversation Lane",
		"Latest Output",
		"Managed Status",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected tui output to contain %q, got %q", expected, text)
		}
	}
}

func TestRunTUIInteractiveSurfacesActionErrorsInWorkbench(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	input := strings.NewReader("approve current draft\n/quit\n")
	exitCode := runTUICommand([]string{"--workdir", workDir}, input, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Latest Output") || !strings.Contains(stdout.String(), "Error:") {
		t.Fatalf("expected interactive tui to surface action error in workbench, got %q", stdout.String())
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

	t.Setenv("LORE_LLM_BASE_URL", server.URL)
	t.Setenv("LORE_LLM_API_KEY", "secret")
	t.Setenv("LORE_LLM_MODEL", "")

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

func TestRunImportExternalJSONL(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)
	transcriptPath := filepath.Join(workDir, "external.jsonl")
	content := "" +
		"{\"type\":\"session_meta\",\"agent_id\":\"Claude Code\",\"session_id\":\"class-1\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"role\":\"user\",\"text\":\"summarize the class\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:35:00+08:00\",\"role\":\"assistant\",\"text\":\"class summary ready\"}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"import-external-jsonl",
		"--workdir", workDir,
		"--input", transcriptPath,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "External transcript JSONL imported") {
		t.Fatalf("expected import summary, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "agent: claude-code") {
		t.Fatalf("expected external agent summary, got %q", stdout.String())
	}
}

func TestRunImportExternalJSONLMissingInput(t *testing.T) {
	workDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"import-external-jsonl", "--workdir", workDir}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "--input is required") {
		t.Fatalf("expected missing input error, got %q", stderr.String())
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
	windowStart := time.Now().In(time.Local).Truncate(30 * time.Minute)
	eventAt := windowStart.Add(5 * time.Minute)
	content := "" +
		"{\"timestamp\":\"" + windowStart.Format(time.RFC3339) + "\",\"type\":\"session_meta\",\"payload\":{\"id\":\"session-1\",\"agent_nickname\":\"Codex\"}}\n" +
		"{\"timestamp\":\"" + eventAt.Format(time.RFC3339) + "\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"build adapter\"}}\n"
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

func TestRunFindingsListAndResolve(t *testing.T) {
	workDir := t.TempDir()
	findingID := seedFindingForCLI(t, workDir, "finding-cli-1")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"findings", "list", "--workdir", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	for _, want := range []string{"Findings", findingID, "open", "out_of_band_vault_write", "03-notes/outside.md"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("findings list output missing %q: %s", want, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = run([]string{"findings", "resolve", "--workdir", workDir, findingID}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "resolved") {
		t.Fatalf("expected resolved output, got %q", stdout.String())
	}

	runtime, err := openRuntimeForCLITest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	finding, err := runtime.Store.Findings().GetFinding(findingID)
	if err != nil {
		t.Fatalf("GetFinding() error = %v", err)
	}
	if finding.State != model.FindingResolved {
		t.Fatalf("finding.State = %q, want resolved", finding.State)
	}
	assertFindingStateAudit(t, runtime, findingID, model.FindingResolved)
}

func TestRunFindingsListShowsFullLongIDAndResolveAcceptsIt(t *testing.T) {
	workDir := t.TempDir()
	findingID := "governance_review_needed-03-notes-class-external-agent-generated-very-long-note-title-that-must-remain-copyable-1835347200000000000"
	seedFindingForCLI(t, workDir, findingID)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"findings", "list", "--workdir", workDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), findingID) {
		t.Fatalf("findings list should include full finding ID %q, got %q", findingID, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = run([]string{"findings", "resolve", "--workdir", workDir, findingID}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code resolving long ID, got %d, stderr = %q", exitCode, stderr.String())
	}
	runtime, err := openRuntimeForCLITest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	assertFindingStateAudit(t, runtime, findingID, model.FindingResolved)
}

func TestRunFindingsIgnore(t *testing.T) {
	workDir := t.TempDir()
	findingID := seedFindingForCLI(t, workDir, "finding-cli-ignore")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"findings", "ignore", "--workdir", workDir, findingID}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ignored") {
		t.Fatalf("expected ignored output, got %q", stdout.String())
	}
	runtime, err := openRuntimeForCLITest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	assertFindingStateAudit(t, runtime, findingID, model.FindingIgnored)
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
	if !strings.Contains(stdout.String(), "Lore stopped") {
		t.Fatalf("expected console stop output, got %q", stdout.String())
	}
}

func TestRunDaemonOnceTriggersDraftAfterStablePlanChange(t *testing.T) {
	workDir := t.TempDir()
	clearOperatorEnv(t)

	runtime, err := openRuntimeForCLITest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	if _, err := runtime.Bootstrap(time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	planPath := filepath.Join(runtime.Config.Paths.VaultRoot, "0-\u6392\u671f", "04-\u6267\u884c", "week.md")
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

func TestRunDaemonOnceSyncsCodexJSONLWhenConfigured(t *testing.T) {
	workDir := t.TempDir()
	configureLLMTestEnv(t)

	transcriptPath := filepath.Join(workDir, "daemon-codex.jsonl")
	content := "" +
		"{\"timestamp\":\"2026-04-22T09:01:00+08:00\",\"type\":\"session_meta\",\"payload\":{\"id\":\"session-daemon\",\"agent_nickname\":\"Codex\"}}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":\"build daemon integration\"}}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"daemon", "run",
		"--workdir", workDir,
		"--once",
		"--codex-jsonl", transcriptPath,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Codex JSONL synced") {
		t.Fatalf("expected daemon codex sync output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "- agent: codex") {
		t.Fatalf("expected codex agent output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "- daily reports:") {
		t.Fatalf("expected daily report output, got %q", stdout.String())
	}
}

func TestRunDaemonOnceMissingCodexJSONLRemainsNonFatal(t *testing.T) {
	workDir := t.TempDir()
	clearOperatorEnv(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"daemon", "run",
		"--workdir", workDir,
		"--once",
		"--codex-jsonl", filepath.Join(workDir, "missing.jsonl"),
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Codex JSONL sync failed") {
		t.Fatalf("expected non-fatal daemon sync failure output, got %q", stdout.String())
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

	runtime, err := openRuntimeForCLITest(t, workDir)
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

func seedFindingForCLI(t *testing.T, workDir string, id string) string {
	t.Helper()

	runtime, err := openRuntimeForCLITest(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	now := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	finding := model.Finding{
		ID:       id,
		Kind:     model.FindingOutOfBandVaultWrite,
		State:    model.FindingOpen,
		Severity: model.FindingSeverityInfo,
		Target: model.DocumentRef{
			Path:        "03-notes/outside.md",
			Class:       model.DocClassNote,
			BaseVersion: "hash-1",
		},
		Title:      "Out-of-band vault note change",
		Summary:    "Created by CLI test",
		Source:     "vault_daemon",
		DetectedAt: now,
		UpdatedAt:  now,
	}
	if err := runtime.Store.Findings().SaveFinding(finding); err != nil {
		t.Fatalf("SaveFinding() error = %v", err)
	}
	return id
}

func assertFindingStateAudit(t *testing.T, runtime *app.Runtime, findingID string, state model.FindingState) {
	t.Helper()

	records, err := runtime.Store.Audit().ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	for _, record := range records {
		if record.Kind == model.AuditFindingStateChange && record.CorrelationID == findingID && record.Metadata["state"] == string(state) {
			return
		}
	}
	t.Fatalf("finding state-change audit state=%q not found for %s in %+v", state, findingID, records)
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

	runtime, err := openRuntimeForCLITest(t, workDir)
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

func openRuntimeForCLITest(t *testing.T, workDir string) (*app.Runtime, error) {
	t.Helper()

	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("runtime.Close() error = %v", err)
		}
	})
	return runtime, nil
}
