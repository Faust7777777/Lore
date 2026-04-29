package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/sessionlog"
)

func TestSessionsListEmptyState(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runSessionsCommand([]string{"list", "--workdir", workDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("sessions list exit = %d, stderr = %q", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "No sessions found." {
		t.Fatalf("stdout = %q, want empty state", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestSessionsSearchEmptyState(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	createCLISession(t, workDir, "lore-search-empty", "alpha topic", "beta answer")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runSessionsCommand([]string{"search", "--workdir", workDir, "missing query"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("sessions search exit = %d, stderr = %q", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "No sessions found." {
		t.Fatalf("stdout = %q, want empty search state", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestSessionsShowMissingTranscriptFails(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runSessionsCommand([]string{"show", "--workdir", workDir, "missing-session"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("sessions show exit = 0, want failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "sessions show:") || !strings.Contains(stderr.String(), "missing-session.jsonl") {
		t.Fatalf("stderr = %q, want missing transcript error", stderr.String())
	}
}

func TestSessionsCommandsRejectCorruptIndex(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	root := createCLISession(t, workDir, "lore-corrupt-index-cli", "topic", "answer")
	if err := os.WriteFile(filepath.Join(root, "index.json"), []byte(`{"version":`), 0o644); err != nil {
		t.Fatalf("WriteFile(index.json) error = %v", err)
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "list", args: []string{"list", "--workdir", workDir}, want: "sessions list:"},
		{name: "search", args: []string{"search", "--workdir", workDir, "topic"}, want: "sessions search:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := runSessionsCommand(tt.args, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("%s exit = 0, want failure", tt.name)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestSessionsShowIsReadOnly(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	root := createCLISession(t, workDir, "lore-show-readonly", "show me", "shown")
	indexPath := filepath.Join(root, "index.json")
	transcriptPath := filepath.Join(root, "lore-show-readonly.jsonl")
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("ReadFile(index before) error = %v", err)
	}
	transcriptBefore, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile(transcript before) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runSessionsCommand([]string{"show", "--workdir", workDir, "lore-show-readonly"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("sessions show exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Session: lore-show-readonly") || !strings.Contains(stdout.String(), "Conversation:") {
		t.Fatalf("stdout = %q, want session summary", stdout.String())
	}
	assertFileBytesEqual(t, indexPath, indexBefore)
	assertFileBytesEqual(t, transcriptPath, transcriptBefore)
}

func createCLISession(t *testing.T, workDir string, sessionID string, user string, assistant string) string {
	t.Helper()
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	defer runtime.Close()
	root := sessionLogRoot(runtime)
	recorder, err := sessionlog.Start(root, sessionlog.Meta{SessionID: sessionID, StartedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("sessionlog.Start() error = %v", err)
	}
	if err := recorder.RecordUser(user); err != nil {
		t.Fatalf("RecordUser() error = %v", err)
	}
	if err := recorder.RecordAssistant(assistant); err != nil {
		t.Fatalf("RecordAssistant() error = %v", err)
	}
	return root
}

func assertFileBytesEqual(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s changed during read-only command", path)
	}
}
