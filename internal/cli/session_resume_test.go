package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/console"
	"obsidian-harness/internal/sessionlog"
)

func TestConfigureSessionRecorderResumeRequiresInteractiveStdin(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	defer runtime.Close()

	root := sessionLogRoot(runtime)
	recorder, err := sessionlog.Start(root, sessionlog.Meta{SessionID: "lore-test-resume", StartedAt: time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("sessionlog.Start() error = %v", err)
	}
	if err := recorder.RecordUser("first question"); err != nil {
		t.Fatalf("RecordUser() error = %v", err)
	}
	if err := recorder.RecordAssistant("first answer"); err != nil {
		t.Fatalf("RecordAssistant() error = %v", err)
	}
	if err := recorder.Close("test"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	session := console.NewSession("test")
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = configureSessionRecorder(session, runtime, true, "", &stdin, &stdout, &stderr, "console")
	if err == nil {
		t.Fatal("configureSessionRecorder() error = nil, want non-interactive resume error")
	}
	if !strings.Contains(err.Error(), "--resume-id") {
		t.Fatalf("error = %q, want --resume-id guidance", err)
	}
	if !strings.Contains(stdout.String(), "lore-test-resume") || !strings.Contains(stdout.String(), "Resume session") {
		t.Fatalf("stdout = %q, want recent session list", stdout.String())
	}
	if session.Recorder != nil {
		t.Fatalf("session.Recorder = %#v, want nil on failed resume", session.Recorder)
	}
}
