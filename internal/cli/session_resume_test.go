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
	assertSessionContextEmpty(t, session)
}

func TestConfigureSessionRecorderResumeIDWorksNonInteractive(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	defer runtime.Close()

	root := sessionLogRoot(runtime)
	recorder, err := sessionlog.Start(root, sessionlog.Meta{SessionID: "lore-test-resume-id", StartedAt: time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("sessionlog.Start() error = %v", err)
	}
	if err := recorder.RecordUser("previous question"); err != nil {
		t.Fatalf("RecordUser() error = %v", err)
	}
	if err := recorder.RecordAssistant("previous answer"); err != nil {
		t.Fatalf("RecordAssistant() error = %v", err)
	}

	session := console.NewSession("test")
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = configureSessionRecorder(session, runtime, false, "lore-test-resume-id", &stdin, &stdout, &stderr, "console")
	if err != nil {
		t.Fatalf("configureSessionRecorder() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no interactive prompt", stdout.String())
	}
	if !strings.Contains(stderr.String(), "console resumed session lore-test-resume-id") {
		t.Fatalf("stderr = %q, want resume notice", stderr.String())
	}
	if session.Recorder == nil {
		t.Fatal("session.Recorder = nil, want resumed recorder")
	}
	if len(session.History) != 2 || session.History[0].Content != "previous question" || session.History[1].Content != "previous answer" {
		t.Fatalf("session.History = %+v, want restored history", session.History)
	}
}

func TestConfigureSessionRecorderResumeWithNoSessionsIsExplicit(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	defer runtime.Close()

	session := console.NewSession("test")
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = configureSessionRecorder(session, runtime, true, "", &stdin, &stdout, &stderr, "console")
	if err == nil {
		t.Fatal("configureSessionRecorder() error = nil, want no previous sessions error")
	}
	if !strings.Contains(err.Error(), "no previous sessions found") {
		t.Fatalf("error = %q, want no previous sessions guidance", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q, want no prompt output", stdout.String(), stderr.String())
	}
	if session.Recorder != nil {
		t.Fatalf("session.Recorder = %#v, want nil on failed resume", session.Recorder)
	}
	assertSessionContextEmpty(t, session)
}

func TestConfigureSessionRecorderResumeIDMissingDoesNotStartFresh(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := app.OpenRuntime(workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	defer runtime.Close()

	session := console.NewSession("test")
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = configureSessionRecorder(session, runtime, false, "missing-session", &stdin, &stdout, &stderr, "console")
	if err == nil {
		t.Fatal("configureSessionRecorder() error = nil, want missing resume-id error")
	}
	if !strings.Contains(err.Error(), "resume missing-session") {
		t.Fatalf("error = %q, want missing resume-id context", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q, want no prompt or resume notice", stdout.String(), stderr.String())
	}
	if session.Recorder != nil {
		t.Fatalf("session.Recorder = %#v, want nil on failed resume-id", session.Recorder)
	}
	assertSessionContextEmpty(t, session)
}

func assertSessionContextEmpty(t *testing.T, session *console.Session) {
	t.Helper()
	if len(session.History) != 0 {
		t.Fatalf("session.History = %+v, want empty on failed resume", session.History)
	}
	if len(session.WorkingSet) != 0 {
		t.Fatalf("session.WorkingSet = %+v, want empty on failed resume", session.WorkingSet)
	}
}
