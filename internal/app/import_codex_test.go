package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/model"
)

func TestRuntimeImportCodexJSONLWritesPlaceholderCheckpointAndSingleDayRollup(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "single-day.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-1","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"review the weekly drift"}}`,
		`{"timestamp":"2026-04-22T10:05:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"drafted the next checkpoint"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-1",
	}, time.Date(2026, 4, 22, 23, 45, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportCodexJSONL() error = %v", err)
	}

	if result.AgentID != "codex" {
		t.Fatalf("AgentID = %q, want codex", result.AgentID)
	}
	if result.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", result.SessionID)
	}
	if len(result.Checkpoints) != 3 {
		t.Fatalf("len(Checkpoints) = %d, want 3", len(result.Checkpoints))
	}

	placeholder := result.Checkpoints[1]
	if placeholder.State != model.CheckpointPlaceholder {
		t.Fatalf("placeholder State = %q, want %q", placeholder.State, model.CheckpointPlaceholder)
	}
	if !strings.Contains(placeholder.Content, "no incremental transcript was ingested") {
		t.Fatalf("placeholder Content = %q, want empty-window marker", placeholder.Content)
	}
	if placeholder.Window.WindowStart.Hour() != 9 || placeholder.Window.WindowStart.Minute() != 30 {
		t.Fatalf("placeholder window start = %s, want 09:30 local window", placeholder.Window.WindowStart.Format(time.RFC3339))
	}

	if len(result.Reports) != 1 {
		t.Fatalf("len(Reports) = %d, want 1", len(result.Reports))
	}
	report := result.Reports[0]
	wantDay := model.NormalizeDay(time.Date(2026, 4, 22, 12, 0, 0, 0, loc))
	if !report.ReportDay.Equal(wantDay) {
		t.Fatalf("ReportDay = %s, want %s", report.ReportDay.Format(time.RFC3339), wantDay.Format(time.RFC3339))
	}
	if len(report.WindowKeys) != 3 {
		t.Fatalf("len(report.WindowKeys) = %d, want 3", len(report.WindowKeys))
	}

	checkpointMarkdown := readFile(t, placeholder.Path)
	if !strings.Contains(checkpointMarkdown, "state: `placeholder`") {
		t.Fatalf("checkpoint markdown missing placeholder state: %q", checkpointMarkdown)
	}
	if !strings.Contains(checkpointMarkdown, "09:30-10:00") {
		t.Fatalf("checkpoint markdown missing empty window range: %q", checkpointMarkdown)
	}

	reportMarkdown := readFile(t, report.Path)
	if !strings.Contains(reportMarkdown, "checkpoints: 3") {
		t.Fatalf("daily report markdown missing checkpoint count: %q", reportMarkdown)
	}
	if !strings.Contains(reportMarkdown, "09:30") {
		t.Fatalf("daily report markdown missing placeholder window entry: %q", reportMarkdown)
	}
}

func TestRuntimeImportCodexJSONLRollsUpAcrossTouchedDays(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "cross-day.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T23:40:00+08:00","type":"session_meta","payload":{"id":"session-cross","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T23:50:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"wrap the evening session"}}`,
		`{"timestamp":"2026-04-23T00:10:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"started the next day handoff"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.ImportCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-cross",
	}, time.Date(2026, 4, 23, 8, 30, 0, 0, loc))
	if err != nil {
		t.Fatalf("ImportCodexJSONL() error = %v", err)
	}

	if len(result.Checkpoints) != 2 {
		t.Fatalf("len(Checkpoints) = %d, want 2", len(result.Checkpoints))
	}
	if len(result.Reports) != 2 {
		t.Fatalf("len(Reports) = %d, want 2", len(result.Reports))
	}

	firstDay := model.NormalizeDay(time.Date(2026, 4, 22, 23, 0, 0, 0, loc))
	secondDay := model.NormalizeDay(time.Date(2026, 4, 23, 1, 0, 0, 0, loc))

	if !result.Reports[0].ReportDay.Equal(firstDay) {
		t.Fatalf("Reports[0].ReportDay = %s, want %s", result.Reports[0].ReportDay.Format(time.RFC3339), firstDay.Format(time.RFC3339))
	}
	if !result.Reports[1].ReportDay.Equal(secondDay) {
		t.Fatalf("Reports[1].ReportDay = %s, want %s", result.Reports[1].ReportDay.Format(time.RFC3339), secondDay.Format(time.RFC3339))
	}
	if len(result.Reports[0].WindowKeys) != 1 {
		t.Fatalf("len(Reports[0].WindowKeys) = %d, want 1", len(result.Reports[0].WindowKeys))
	}
	if len(result.Reports[1].WindowKeys) != 1 {
		t.Fatalf("len(Reports[1].WindowKeys) = %d, want 1", len(result.Reports[1].WindowKeys))
	}

	firstMarkdown := readFile(t, result.Reports[0].Path)
	if !strings.Contains(firstMarkdown, "23:30") {
		t.Fatalf("first daily report missing 23:30 entry: %q", firstMarkdown)
	}

	secondMarkdown := readFile(t, result.Reports[1].Path)
	if !strings.Contains(secondMarkdown, "00:00") {
		t.Fatalf("second daily report missing 00:00 entry: %q", secondMarkdown)
	}
}

func TestRuntimeSyncCodexJSONLStoresStructuredCursorAndSkipsUnchangedTail(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "sync.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-sync","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"review the weekly drift"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	first, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-sync",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err != nil {
		t.Fatalf("first SyncCodexJSONL() error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first sync Changed = false, want true")
	}

	second, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-sync",
	}, time.Date(2026, 4, 22, 9, 11, 0, 0, loc))
	if err != nil {
		t.Fatalf("second SyncCodexJSONL() error = %v", err)
	}
	if second.Changed {
		t.Fatal("second sync Changed = true, want false")
	}
	if second.Fingerprint != first.Fingerprint {
		t.Fatalf("second fingerprint = %q, want %q", second.Fingerprint, first.Fingerprint)
	}

	cursor, err := runtime.Store.Cursors().GetCursor(first.CursorKey)
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor != first.Fingerprint {
		t.Fatalf("cursor = %q, want %q", cursor, first.Fingerprint)
	}
	decoded, err := codexjsonl.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if decoded.Version != 1 {
		t.Fatalf("decoded.Version = %d, want 1", decoded.Version)
	}
	if decoded.Offset == 0 {
		t.Fatal("decoded.Offset = 0, want non-zero safe offset")
	}
	if decoded.LastWindowStart.Hour() != 9 || decoded.LastWindowStart.Minute() != 0 {
		t.Fatalf("decoded.LastWindowStart = %s, want 09:00 local window", decoded.LastWindowStart.Format(time.RFC3339))
	}
}

func TestRuntimeSyncCodexJSONLReimportsWhenFileChanges(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "sync-change.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-sync-change","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"first event"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	first, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-sync-change",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err != nil {
		t.Fatalf("first SyncCodexJSONL() error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first sync Changed = false, want true")
	}

	time.Sleep(10 * time.Millisecond)
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-sync-change","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"first event"}}`,
		`{"timestamp":"2026-04-22T09:35:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"second event"}}`,
	)

	second, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-sync-change",
	}, time.Date(2026, 4, 22, 9, 40, 0, 0, loc))
	if err != nil {
		t.Fatalf("second SyncCodexJSONL() error = %v", err)
	}
	if !second.Changed {
		t.Fatal("second sync Changed = false, want true")
	}
	if second.Fingerprint == first.Fingerprint {
		t.Fatalf("second fingerprint = %q, want changed fingerprint", second.Fingerprint)
	}
	if len(second.Import.Checkpoints) != 2 {
		t.Fatalf("len(second.Import.Checkpoints) = %d, want 2", len(second.Import.Checkpoints))
	}
}

func TestRuntimeSyncCodexJSONLRejectsConcurrentSourceLock(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "locked.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-locked","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"lock me"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	absolutePath, err := filepath.Abs(transcriptPath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	cursorKey := codexJSONLCursorKey(absolutePath)
	lockPath := filepath.Join(runtime.Config.Paths.StateDir, "locks", codexJSONLLockName(cursorKey))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(lock dir) error = %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("busy\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(lockPath) error = %v", err)
	}

	_, err = runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-locked",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err == nil || !strings.Contains(err.Error(), "already syncing") {
		t.Fatalf("SyncCodexJSONL() error = %v, want source lock rejection", err)
	}
}

func TestRuntimeSyncCodexJSONLRejectsLiveOwnerLockPayload(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "live-lock.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-live-lock","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"live lock"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	absolutePath, err := filepath.Abs(transcriptPath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	cursorKey := codexJSONLCursorKey(absolutePath)
	lockPath := filepath.Join(runtime.Config.Paths.StateDir, "locks", codexJSONLLockName(cursorKey))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(lock dir) error = %v", err)
	}
	lockPID := 424242
	lockPayload := fmt.Sprintf(
		"{\"pid\":%d,\"created_at\":%q,\"cursor_key\":%q}\n",
		lockPID,
		time.Now().UTC().Format(time.RFC3339Nano),
		cursorKey,
	)
	if err := os.WriteFile(lockPath, []byte(lockPayload), 0o644); err != nil {
		t.Fatalf("WriteFile(lockPath) error = %v", err)
	}

	previousProcessAlive := codexJSONLProcessAliveFunc
	codexJSONLProcessAliveFunc = func(pid int) (bool, error) {
		if pid != lockPID {
			t.Fatalf("processAlive pid = %d, want %d", pid, lockPID)
		}
		return true, nil
	}
	t.Cleanup(func() {
		codexJSONLProcessAliveFunc = previousProcessAlive
	})

	_, err = runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-live-lock",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err == nil || !strings.Contains(err.Error(), "already syncing") {
		t.Fatalf("SyncCodexJSONL() error = %v, want live-owner source lock rejection", err)
	}
}

func TestRuntimeSyncCodexJSONLReclaimsStaleSourceLock(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "stale-lock.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-stale-lock","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"recover stale lock"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	absolutePath, err := filepath.Abs(transcriptPath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	cursorKey := codexJSONLCursorKey(absolutePath)
	lockPath := filepath.Join(runtime.Config.Paths.StateDir, "locks", codexJSONLLockName(cursorKey))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(lock dir) error = %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(lockPath) error = %v", err)
	}
	staleAt := time.Now().Add(-codexJSONLLockStaleAfter - time.Second)
	if err := os.Chtimes(lockPath, staleAt, staleAt); err != nil {
		t.Fatalf("Chtimes(lockPath) error = %v", err)
	}

	result, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-stale-lock",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err != nil {
		t.Fatalf("SyncCodexJSONL() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want stale-lock recovery import")
	}
	if _, err := os.Stat(lockPath); err == nil {
		t.Fatalf("expected lock file %s to be removed after sync", lockPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(lockPath) error = %v", err)
	}
}

func TestRuntimeSyncCodexJSONLReclaimsDeadOwnerLockImmediately(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "dead-owner-lock.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-dead-owner-lock","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"recover dead owner lock"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	absolutePath, err := filepath.Abs(transcriptPath)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	cursorKey := codexJSONLCursorKey(absolutePath)
	lockPath := filepath.Join(runtime.Config.Paths.StateDir, "locks", codexJSONLLockName(cursorKey))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(lock dir) error = %v", err)
	}
	lockPID := 434343
	lockPayload := fmt.Sprintf(
		"{\"pid\":%d,\"created_at\":%q,\"cursor_key\":%q}\n",
		lockPID,
		time.Now().UTC().Format(time.RFC3339Nano),
		cursorKey,
	)
	if err := os.WriteFile(lockPath, []byte(lockPayload), 0o644); err != nil {
		t.Fatalf("WriteFile(lockPath) error = %v", err)
	}

	previousProcessAlive := codexJSONLProcessAliveFunc
	codexJSONLProcessAliveFunc = func(pid int) (bool, error) {
		if pid != lockPID {
			t.Fatalf("processAlive pid = %d, want %d", pid, lockPID)
		}
		return false, nil
	}
	t.Cleanup(func() {
		codexJSONLProcessAliveFunc = previousProcessAlive
	})

	result, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-dead-owner-lock",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err != nil {
		t.Fatalf("SyncCodexJSONL() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want dead-owner lock recovery import")
	}
	if _, err := os.Stat(lockPath); err == nil {
		t.Fatalf("expected lock file %s to be removed after dead-owner recovery", lockPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(lockPath) error = %v", err)
	}
}

func TestAcquireCodexJSONLLockOldReleaseDoesNotDeleteReacquiredLock(t *testing.T) {
	stateDir := t.TempDir()
	cursorKey := "codexjsonl:test-cursor"

	releaseOld, err := acquireCodexJSONLLock(stateDir, cursorKey)
	if err != nil {
		t.Fatalf("acquireCodexJSONLLock(old) error = %v", err)
	}

	lockPath := filepath.Join(stateDir, "locks", codexJSONLLockName(cursorKey))
	firstPayload, parsed := readCodexJSONLLockPayload(lockPath)
	if !parsed || strings.TrimSpace(firstPayload.Token) == "" {
		t.Fatalf("first lock payload = %+v, want parsed tokenized payload", firstPayload)
	}

	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("Remove(old lock) error = %v", err)
	}

	releaseNew, err := acquireCodexJSONLLock(stateDir, cursorKey)
	if err != nil {
		t.Fatalf("acquireCodexJSONLLock(new) error = %v", err)
	}
	secondPayload, parsed := readCodexJSONLLockPayload(lockPath)
	if !parsed || strings.TrimSpace(secondPayload.Token) == "" {
		t.Fatalf("second lock payload = %+v, want parsed tokenized payload", secondPayload)
	}
	if secondPayload.Token == firstPayload.Token {
		t.Fatalf("second token = %q, want different token from first", secondPayload.Token)
	}

	releaseOld()

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("Stat(new lock after old release) error = %v, want new lock to survive", err)
	}
	payloadAfterOldRelease, parsed := readCodexJSONLLockPayload(lockPath)
	if !parsed || payloadAfterOldRelease.Token != secondPayload.Token {
		t.Fatalf("payload after old release = %+v, want new token %q", payloadAfterOldRelease, secondPayload.Token)
	}

	releaseNew()

	if _, err := os.Stat(lockPath); err == nil {
		t.Fatalf("expected lock file %s to be removed by current owner release", lockPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(lockPath) error = %v", err)
	}
}

func TestRuntimeSyncCodexJSONLReplaysLastWindowWithoutDuplicatingCheckpoint(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "same-window.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-same-window","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"first event"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	first, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-same-window",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err != nil {
		t.Fatalf("first SyncCodexJSONL() error = %v", err)
	}
	if len(first.Import.Checkpoints) != 1 {
		t.Fatalf("len(first.Import.Checkpoints) = %d, want 1", len(first.Import.Checkpoints))
	}

	time.Sleep(10 * time.Millisecond)
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-same-window","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"first event"}}`,
		`{"timestamp":"2026-04-22T09:20:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"second event in same window"}}`,
	)

	second, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-same-window",
	}, time.Date(2026, 4, 22, 9, 25, 0, 0, loc))
	if err != nil {
		t.Fatalf("second SyncCodexJSONL() error = %v", err)
	}
	if len(second.Import.Checkpoints) != 1 {
		t.Fatalf("len(second.Import.Checkpoints) = %d, want 1", len(second.Import.Checkpoints))
	}

	windowKey := first.Import.Checkpoints[0].WindowKey
	stored, err := runtime.Store.ProcessSink().GetCheckpointByWindowKey(windowKey)
	if err != nil {
		t.Fatalf("GetCheckpointByWindowKey() error = %v", err)
	}
	if stored.CreatedAt != first.Import.Checkpoints[0].CreatedAt {
		t.Fatalf("CreatedAt changed: %s != %s", stored.CreatedAt.Format(time.RFC3339Nano), first.Import.Checkpoints[0].CreatedAt.Format(time.RFC3339Nano))
	}
	if !strings.Contains(stored.Content, "second event in same window") {
		t.Fatalf("stored.Content = %q, want updated second event", stored.Content)
	}

	report, err := runtime.Store.ProcessSink().GetDailyReport("codex", time.Date(2026, 4, 22, 12, 0, 0, 0, loc))
	if err != nil {
		t.Fatalf("GetDailyReport() error = %v", err)
	}
	if len(report.WindowKeys) != 1 {
		t.Fatalf("len(report.WindowKeys) = %d, want 1", len(report.WindowKeys))
	}
}

func TestRuntimeSyncCodexJSONLDefersIncompleteTailUntilRecordCompletes(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "partial-tail.jsonl")
	metaLine := `{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-partial","agent_nickname":"Codex"}}`
	firstLine := `{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"first event"}}`
	partialLine := `{"timestamp":"2026-04-22T09:20:00+08:00","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"completed later"}}`
	writeCodexJSONL(t, transcriptPath, metaLine, firstLine)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	first, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-partial",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc))
	if err != nil {
		t.Fatalf("first SyncCodexJSONL() error = %v", err)
	}

	if err := os.WriteFile(transcriptPath, []byte(metaLine+"\n"+firstLine+"\n"+partialLine), 0o644); err != nil {
		t.Fatalf("WriteFile(partial) error = %v", err)
	}
	second, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-partial",
	}, time.Date(2026, 4, 22, 9, 21, 0, 0, loc))
	if err != nil {
		t.Fatalf("second SyncCodexJSONL() error = %v", err)
	}
	if second.Changed {
		t.Fatal("second sync Changed = true, want false for incomplete tail")
	}
	if second.Fingerprint != first.Fingerprint {
		t.Fatalf("second.Fingerprint = %q, want %q", second.Fingerprint, first.Fingerprint)
	}

	if err := os.WriteFile(transcriptPath, []byte(metaLine+"\n"+firstLine+"\n"+partialLine+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(completed) error = %v", err)
	}
	third, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-partial",
	}, time.Date(2026, 4, 22, 9, 22, 0, 0, loc))
	if err != nil {
		t.Fatalf("third SyncCodexJSONL() error = %v", err)
	}
	if !third.Changed {
		t.Fatal("third sync Changed = false, want true after line completion")
	}
	if len(third.Import.Checkpoints) != 1 {
		t.Fatalf("len(third.Import.Checkpoints) = %d, want 1", len(third.Import.Checkpoints))
	}
	if !strings.Contains(third.Import.Checkpoints[0].Content, "completed later") {
		t.Fatalf("checkpoint content = %q, want completed later snippet", third.Import.Checkpoints[0].Content)
	}
}

func TestRuntimeSyncCodexJSONLFallsBackWhenReplayBoundaryChanges(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "boundary-rewrite.jsonl")

	metaLine := `{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-boundary","agent_nickname":"Codex"}}`
	makeUserLine := func(timestamp string, message string) string {
		return fmt.Sprintf(
			`{"timestamp":"%s","type":"event_msg","payload":{"type":"user_message","message":"%s"}}`,
			timestamp,
			message,
		)
	}
	makeAgentLine := func(timestamp string, message string) string {
		return fmt.Sprintf(
			`{"timestamp":"%s","type":"event_msg","payload":{"type":"agent_message","phase":"commentary","message":"%s"}}`,
			timestamp,
			message,
		)
	}

	filler := make([]string, 0, 9)
	for i := 0; i < 8; i++ {
		filler = append(filler, makeUserLine(
			fmt.Sprintf("2026-04-22T09:%02d:00+08:00", i+1),
			fmt.Sprintf("filler-%d %s", i, strings.Repeat("x", 700)),
		))
	}
	boundaryOriginal := "boundary-old " + strings.Repeat("b", 700)
	boundaryRewritten := "boundary-rewritten " + strings.Repeat("c", 700)

	initialLines := []string{metaLine}
	initialLines = append(initialLines, filler...)
	initialLines = append(initialLines,
		makeUserLine("2026-04-22T09:29:00+08:00", boundaryOriginal),
		makeAgentLine("2026-04-22T09:35:00+08:00", "later window"),
	)
	writeCodexJSONL(t, transcriptPath, initialLines...)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	first, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-boundary",
	}, time.Date(2026, 4, 22, 9, 40, 0, 0, loc))
	if err != nil {
		t.Fatalf("first SyncCodexJSONL() error = %v", err)
	}
	if len(first.Import.Checkpoints) != 2 {
		t.Fatalf("len(first.Import.Checkpoints) = %d, want 2", len(first.Import.Checkpoints))
	}

	absolutePath, err := filepath.Abs(transcriptPath)
	if err != nil {
		t.Fatalf("Abs(transcriptPath) error = %v", err)
	}
	cursorRaw, err := runtime.Store.Cursors().GetCursor(codexJSONLCursorKey(absolutePath))
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	cursor, err := codexjsonl.DecodeCursor(cursorRaw)
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if cursor.ReplayOffset <= 4096 {
		t.Fatalf("cursor.ReplayOffset = %d, want > 4096 so boundary rewrite escapes head hash checks", cursor.ReplayOffset)
	}

	time.Sleep(10 * time.Millisecond)
	rewrittenLines := []string{metaLine}
	rewrittenLines = append(rewrittenLines, filler...)
	rewrittenLines = append(rewrittenLines,
		makeUserLine("2026-04-22T09:29:00+08:00", boundaryRewritten),
		makeAgentLine("2026-04-22T09:35:00+08:00", "later window"),
		makeAgentLine("2026-04-22T09:40:00+08:00", "appended after rewrite"),
	)
	writeCodexJSONL(t, transcriptPath, rewrittenLines...)

	second, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-boundary",
	}, time.Date(2026, 4, 22, 9, 45, 0, 0, loc))
	if err != nil {
		t.Fatalf("second SyncCodexJSONL() error = %v", err)
	}
	if len(second.Import.Checkpoints) != 2 {
		t.Fatalf("len(second.Import.Checkpoints) = %d, want 2 after full fallback replay", len(second.Import.Checkpoints))
	}

	firstWindowKey := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "session-boundary",
		WindowStart: time.Date(2026, 4, 22, 9, 0, 0, 0, loc),
		WindowEnd:   time.Date(2026, 4, 22, 9, 30, 0, 0, loc),
	}.Key()
	firstWindow, err := runtime.Store.ProcessSink().GetCheckpointByWindowKey(firstWindowKey)
	if err != nil {
		t.Fatalf("GetCheckpointByWindowKey() error = %v", err)
	}
	if !strings.Contains(firstWindow.RawTranscript, boundaryRewritten) {
		t.Fatalf("firstWindow.RawTranscript = %q, want rewritten boundary content", firstWindow.RawTranscript)
	}
	if strings.Contains(firstWindow.RawTranscript, boundaryOriginal) {
		t.Fatalf("firstWindow.RawTranscript = %q, want old boundary content removed", firstWindow.RawTranscript)
	}
}

func TestRuntimeSyncCodexJSONLGeneratesPlaceholderForClosedIdleWindow(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "idle-slot.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-idle","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"only one active slot"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	if _, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-idle",
	}, time.Date(2026, 4, 22, 9, 10, 0, 0, loc)); err != nil {
		t.Fatalf("first SyncCodexJSONL() error = %v", err)
	}

	second, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-idle",
	}, time.Date(2026, 4, 22, 10, 1, 0, 0, loc))
	if err != nil {
		t.Fatalf("second SyncCodexJSONL() error = %v", err)
	}
	if !second.Changed {
		t.Fatal("second sync Changed = false, want placeholder sync")
	}
	if len(second.Import.Checkpoints) != 1 {
		t.Fatalf("len(second.Import.Checkpoints) = %d, want 1 placeholder", len(second.Import.Checkpoints))
	}
	if second.Import.Checkpoints[0].State != model.CheckpointPlaceholder {
		t.Fatalf("checkpoint state = %q, want %q", second.Import.Checkpoints[0].State, model.CheckpointPlaceholder)
	}
	if second.Import.Checkpoints[0].Window.WindowStart.Hour() != 9 || second.Import.Checkpoints[0].Window.WindowStart.Minute() != 30 {
		t.Fatalf("placeholder window = %s, want 09:30", second.Import.Checkpoints[0].Window.WindowStart.Format(time.RFC3339))
	}
	if len(second.Import.Reports) != 1 {
		t.Fatalf("len(second.Import.Reports) = %d, want 1", len(second.Import.Reports))
	}
	if len(second.Import.Reports[0].WindowKeys) != 2 {
		t.Fatalf("len(second.Import.Reports[0].WindowKeys) = %d, want 2", len(second.Import.Reports[0].WindowKeys))
	}

	firstWindowKey := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "session-idle",
		WindowStart: time.Date(2026, 4, 22, 9, 0, 0, 0, loc),
		WindowEnd:   time.Date(2026, 4, 22, 9, 30, 0, 0, loc),
	}.Key()
	firstCheckpoint, err := runtime.Store.ProcessSink().GetCheckpointByWindowKey(firstWindowKey)
	if err != nil {
		t.Fatalf("GetCheckpointByWindowKey(first) error = %v", err)
	}
	if firstCheckpoint.State != model.CheckpointMaterialized {
		t.Fatalf("first checkpoint state = %q, want %q", firstCheckpoint.State, model.CheckpointMaterialized)
	}
}

func TestRuntimeSyncCodexJSONLInitialSyncDoesNotBackfillHistoricalIdleWindows(t *testing.T) {
	loc := useFixedLocalZone(t)
	workDir := t.TempDir()
	transcriptPath := filepath.Join(workDir, "historical-initial.jsonl")
	writeCodexJSONL(t, transcriptPath,
		`{"timestamp":"2026-04-22T09:00:00+08:00","type":"session_meta","payload":{"id":"session-historical","agent_nickname":"Codex"}}`,
		`{"timestamp":"2026-04-22T09:05:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"historical event"}}`,
	)

	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, workDir)
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.SyncCodexJSONL(ImportCodexJSONLParams{
		InputPath: transcriptPath,
		AgentID:   "codex",
		SessionID: "session-historical",
	}, time.Date(2026, 4, 23, 10, 0, 0, 0, loc))
	if err != nil {
		t.Fatalf("SyncCodexJSONL() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want initial import change")
	}
	if len(result.Import.Checkpoints) != 1 {
		t.Fatalf("len(result.Import.Checkpoints) = %d, want 1 material checkpoint", len(result.Import.Checkpoints))
	}
	if result.Import.Checkpoints[0].State != model.CheckpointMaterialized {
		t.Fatalf("checkpoint state = %q, want %q", result.Import.Checkpoints[0].State, model.CheckpointMaterialized)
	}
	if len(result.Import.Reports) != 1 {
		t.Fatalf("len(result.Import.Reports) = %d, want 1", len(result.Import.Reports))
	}
	if len(result.Import.Reports[0].WindowKeys) != 1 {
		t.Fatalf("len(result.Import.Reports[0].WindowKeys) = %d, want 1", len(result.Import.Reports[0].WindowKeys))
	}
}

func useFixedLocalZone(t *testing.T) *time.Location {
	t.Helper()

	previous := time.Local
	loc := time.FixedZone("UTC+8", 8*60*60)
	time.Local = loc
	t.Cleanup(func() {
		time.Local = previous
	})
	return loc
}

func writeCodexJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func openRuntimeWithFakeProcessSinkSummarizer(t *testing.T, workDir string) (*Runtime, error) {
	t.Helper()
	runtime, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("runtime.Close() error = %v", err)
		}
	})
	runtime.ProcessSinkSummarizer = fakeProcessSinkSummarizer{}
	runtime.processSinkSummarizerErr = nil
	return runtime, nil
}

type fakeProcessSinkSummarizer struct{}

func (fakeProcessSinkSummarizer) SummarizeCheckpoint(window codexjsonl.WindowSummary) (string, string, error) {
	if strings.TrimSpace(window.RawTranscript) == "" {
		return "", "", nil
	}
	title := strings.TrimSpace(window.Title)
	if title == "" {
		title = fmt.Sprintf(
			"%s checkpoint %s-%s",
			window.Window.AgentID,
			window.Window.WindowStart.Format("15:04"),
			window.Window.WindowEnd.Format("15:04"),
		)
	}
	return title, strings.TrimSpace(window.Content), nil
}

func (fakeProcessSinkSummarizer) SummarizeDaily(agentID string, _ time.Time, checkpoints []model.CheckpointDoc) (string, string, error) {
	lines := make([]string, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		lines = append(lines, fmt.Sprintf("- %s -> %s", checkpoint.Window.WindowStart.Format("15:04"), checkpoint.Title))
	}
	if len(lines) == 0 {
		lines = append(lines, "- no checkpoints recorded")
	}
	return fmt.Sprintf("%s daily report", agentID), strings.Join(lines, "\n"), nil
}
