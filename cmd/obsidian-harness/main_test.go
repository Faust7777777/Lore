package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRunImportCodexJSONL(t *testing.T) {
	workDir := t.TempDir()
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
