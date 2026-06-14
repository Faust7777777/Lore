package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
)

func TestRuntimeImportExternalTranscriptJSONLWritesCheckpointsAndRollup(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "class-session.jsonl")
	content := "" +
		"{\"type\":\"session_meta\",\"agent_id\":\"Claude Code\",\"session_id\":\"class-1\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:05:00+08:00\",\"role\":\"user\",\"text\":\"summarize today's e-commerce lecture\"}\n" +
		"{\"timestamp\":\"2026-04-22T09:35:00+08:00\",\"role\":\"assistant\",\"phase\":\"final\",\"text\":\"platform strategy and network effects\"}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportExternalTranscriptJSONL(ImportExternalTranscriptJSONLParams{
		InputPath: transcriptPath,
	}, time.Date(2026, 4, 22, 23, 30, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportExternalTranscriptJSONL() error = %v", err)
	}

	if result.AgentID != "claude-code" {
		t.Fatalf("AgentID = %q, want claude-code", result.AgentID)
	}
	if result.SessionID != "class-1" {
		t.Fatalf("SessionID = %q, want class-1", result.SessionID)
	}
	if len(result.Checkpoints) != 2 {
		t.Fatalf("len(Checkpoints) = %d, want 2", len(result.Checkpoints))
	}
	if len(result.Reports) != 1 {
		t.Fatalf("len(Reports) = %d, want 1", len(result.Reports))
	}
	if result.Reports[0].AgentID != "claude-code" {
		t.Fatalf("Report.AgentID = %q, want claude-code", result.Reports[0].AgentID)
	}
	if !result.Reports[0].ReportDay.Equal(model.NormalizeDay(time.Date(2026, 4, 22, 12, 0, 0, 0, loc))) {
		t.Fatalf("ReportDay = %s, want 2026-04-22 local", result.Reports[0].ReportDay.Format(time.RFC3339))
	}

	checkpointMarkdown := readFile(t, result.Checkpoints[0].Path)
	if !strings.Contains(checkpointMarkdown, "summarize today's e-commerce lecture") {
		t.Fatalf("checkpoint markdown missing transcript content: %q", checkpointMarkdown)
	}
	reportMarkdown := readFile(t, result.Reports[0].Path)
	if !strings.Contains(reportMarkdown, "claude-code daily report") {
		t.Fatalf("daily report markdown missing external agent id: %q", reportMarkdown)
	}
}

func TestRuntimeImportExternalTranscriptJSONLStaysProcessSinkOnly(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "external-session.jsonl")
	content := "" +
		"{\"type\":\"session_meta\",\"agent_id\":\"Claude Code\",\"session_id\":\"class-2\"}\n" +
		"{\"timestamp\":\"2026-04-22T10:05:00+08:00\",\"role\":\"user\",\"text\":\"turn this class chat into a durable memory\"}\n" +
		"{\"timestamp\":\"2026-04-22T10:35:00+08:00\",\"role\":\"assistant\",\"text\":\"summary with possible action items\"}\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(transcript) error = %v", err)
	}

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportExternalTranscriptJSONL(ImportExternalTranscriptJSONLParams{
		InputPath: transcriptPath,
	}, time.Date(2026, 4, 22, 23, 30, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportExternalTranscriptJSONL() error = %v", err)
	}

	if len(result.Checkpoints) == 0 {
		t.Fatal("ImportExternalTranscriptJSONL() created no checkpoints; want process-sink checkpoints")
	}
	if len(result.Reports) == 0 {
		t.Fatal("ImportExternalTranscriptJSONL() created no reports; want process-sink rollup")
	}
	processSinkDir, err := filepath.Abs(runtime.Config.Paths.ProcessSinkDir)
	if err != nil {
		t.Fatalf("Abs(ProcessSinkDir) error = %v", err)
	}
	assertUnderProcessSink := func(label string, path string) {
		t.Helper()
		absPath, err := filepath.Abs(path)
		if err != nil {
			t.Fatalf("Abs(%s path %q) error = %v", label, path, err)
		}
		rel, err := filepath.Rel(processSinkDir, absPath)
		if err != nil {
			t.Fatalf("Rel(ProcessSinkDir, %s path %q) error = %v", label, path, err)
		}
		if rel == "." || strings.HasPrefix(filepath.ToSlash(rel), "../") || filepath.IsAbs(rel) {
			t.Fatalf("%s path = %q, want under process sink dir %q", label, path, runtime.Config.Paths.ProcessSinkDir)
		}
	}
	for _, checkpoint := range result.Checkpoints {
		assertUnderProcessSink("checkpoint", checkpoint.Path)
	}
	for _, report := range result.Reports {
		assertUnderProcessSink("report", report.Path)
	}

	drafts, err := runtime.ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("ListDrafts() returned %d draft(s), want none", len(drafts))
	}

	findings, err := runtime.ListFindings(0)
	if err != nil {
		t.Fatalf("ListFindings() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("ListFindings() returned %d finding(s), want none", len(findings))
	}

	for _, state := range []persona.PersonaCandidateState{
		persona.PersonaCandidateOpen,
		persona.PersonaCandidateDrafted,
		persona.PersonaCandidateDismissed,
	} {
		candidates, err := runtime.ListPersonaCandidates(state, 0)
		if err != nil {
			t.Fatalf("ListPersonaCandidates(%s) error = %v", state, err)
		}
		if len(candidates) != 0 {
			t.Fatalf("ListPersonaCandidates(%s) returned %d candidate(s), want none", state, len(candidates))
		}
	}

	notesDir := filepath.Join(runtime.Config.Paths.VaultRoot, "03-notes")
	if _, err := os.Stat(notesDir); err == nil {
		t.Fatalf("external transcript import created ordinary notes directory %s; want process-sink only", notesDir)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat ordinary notes directory %s: %v", notesDir, err)
	}
}

func TestRuntimeImportExternalTranscriptJSONLRequiresInput(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	_, err = runtime.ImportExternalTranscriptJSONL(ImportExternalTranscriptJSONLParams{}, time.Date(2026, 4, 22, 23, 30, 0, 0, time.Local))
	if err == nil || !strings.Contains(err.Error(), "empty input path") {
		t.Fatalf("ImportExternalTranscriptJSONL() error = %v, want empty input path", err)
	}
}

func TestApplyExternalTranscriptIdentityOverridePrecedenceAndNormalization(t *testing.T) {
	// applyExternalTranscriptIdentity overrides the loaded transcript
	// identity with non-blank params, skips blank ones (keeping the
	// loaded value), then runs FinalizeTranscript. The end-to-end import
	// test only exercises the blank-param path; the override-when-set
	// branch was the 60% gap. The observable contract a regression could
	// silently break -- misattributing an imported session -- is
	// asymmetric: AgentID is normalized (lowercased + charset-filtered by
	// sanitizeAgentID) while SessionID is taken verbatim (only trimmed).
	// Pin both branches of both fields.

	// Non-blank params win; AgentID is lowercased, SessionID kept as-is.
	set := codexjsonl.Transcript{AgentID: "loaded", SessionID: "loaded-sess"}
	applyExternalTranscriptIdentity(&set, ImportExternalTranscriptJSONLParams{
		AgentID:   "  Cursor-IDE  ",
		SessionID: "  S-42  ",
	})
	if set.AgentID != "cursor-ide" {
		t.Fatalf("AgentID = %q, want sanitized lowercase %q", set.AgentID, "cursor-ide")
	}
	if set.SessionID != "S-42" {
		t.Fatalf("SessionID = %q, want trimmed-but-verbatim %q", set.SessionID, "S-42")
	}

	// Blank/whitespace params are skipped: the loaded identity is kept
	// (AgentID still normalized by FinalizeTranscript).
	kept := codexjsonl.Transcript{AgentID: "Keep-Agent", SessionID: "keep-sess"}
	applyExternalTranscriptIdentity(&kept, ImportExternalTranscriptJSONLParams{
		AgentID:   "   ",
		SessionID: "",
	})
	if kept.AgentID != "keep-agent" {
		t.Fatalf("blank AgentID param should keep the loaded value, got %q", kept.AgentID)
	}
	if kept.SessionID != "keep-sess" {
		t.Fatalf("blank SessionID param should keep the loaded value, got %q", kept.SessionID)
	}
}
