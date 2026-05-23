package sessionlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/operatoragent"
)

func TestRecorderWritesIndexAndRestoresSnapshot(t *testing.T) {
	root := t.TempDir()
	started := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	recorder, err := Start(root, Meta{SessionID: "lore-test", AgentID: "lore", Model: "gpt-5.4", StartedAt: started})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordUser("open persona"); err != nil {
		t.Fatalf("RecordUser() error = %v", err)
	}
	if err := recorder.RecordToolTrace([]operatoragent.ToolCallTrace{{Name: "system_doc_get", Arguments: map[string]any{"name": "persona"}, Status: "ok"}}); err != nil {
		t.Fatalf("RecordToolTrace() error = %v", err)
	}
	if err := recorder.RecordAssistant("# Persona"); err != nil {
		t.Fatalf("RecordAssistant() error = %v", err)
	}
	if err := recorder.RecordWorkingSet([]operatoragent.WorkingSetItem{{Kind: "vault_path", Path: "03-profile/persona.md", Source: "system_doc_get"}}); err != nil {
		t.Fatalf("RecordWorkingSet() error = %v", err)
	}

	snapshot, err := Load(root, "lore-test")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if snapshot.Meta.SessionID != "lore-test" || snapshot.Meta.Model != "gpt-5.4" {
		t.Fatalf("snapshot.Meta = %+v", snapshot.Meta)
	}
	if len(snapshot.History) != 2 || snapshot.History[0].Role != "user" || snapshot.History[1].Role != "assistant" {
		t.Fatalf("snapshot.History = %+v", snapshot.History)
	}
	if len(snapshot.WorkingSet) != 1 || snapshot.WorkingSet[0].Path != "03-profile/persona.md" {
		t.Fatalf("snapshot.WorkingSet = %+v", snapshot.WorkingSet)
	}
	if snapshot.Summary.Title != "open persona" || snapshot.Summary.TurnCount != 1 {
		t.Fatalf("snapshot.Summary = %+v", snapshot.Summary)
	}

	recent, err := ListRecent(root, 20)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 1 || recent[0].ID != "lore-test" {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestResumeAppendsSameTranscript(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-resume", StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordUser("first"); err != nil {
		t.Fatalf("RecordUser(first) error = %v", err)
	}

	resumed, snapshot, err := Resume(root, "lore-resume")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if snapshot.Summary.ID != "lore-resume" {
		t.Fatalf("snapshot.Summary.ID = %q", snapshot.Summary.ID)
	}
	if resumed.Path() != recorder.Path() {
		t.Fatalf("resumed path = %q, want %q", resumed.Path(), recorder.Path())
	}
	if err := resumed.RecordAssistant("second"); err != nil {
		t.Fatalf("RecordAssistant(second) error = %v", err)
	}
	loaded, err := Load(root, "lore-resume")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded.History) != 2 {
		t.Fatalf("history = %+v, want two turns", loaded.History)
	}
}

func TestResumeRefreshesIndexTurnCount(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-resume-index", StartedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordUser("first user"); err != nil {
		t.Fatalf("RecordUser(first) error = %v", err)
	}
	if err := recorder.RecordAssistant("first assistant"); err != nil {
		t.Fatalf("RecordAssistant(first) error = %v", err)
	}

	resumed, _, err := Resume(root, "lore-resume-index")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if err := resumed.RecordUser("second user"); err != nil {
		t.Fatalf("RecordUser(second) error = %v", err)
	}
	if err := resumed.RecordAssistant("second assistant"); err != nil {
		t.Fatalf("RecordAssistant(second) error = %v", err)
	}

	recent, err := ListRecent(root, 20)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("recent = %+v, want one session", recent)
	}
	if recent[0].ID != "lore-resume-index" || recent[0].TurnCount != 2 {
		t.Fatalf("recent[0] = %+v, want turn_count 2", recent[0])
	}
}

func TestRecordModelUsageWritesOneEventPerCall(t *testing.T) {
	root := t.TempDir()
	started := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	recorder, err := Start(root, Meta{SessionID: "lore-usage", AgentID: "codex", Model: "gpt-x", StartedAt: started})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	requestStart := time.Date(2026, 4, 25, 10, 0, 5, 0, time.UTC)
	usage := []operatoragent.ModelCallUsage{
		{Provider: "openai-compatible", Model: "gpt-x", PromptTokens: 31, CompletionTokens: 9, StartedAt: requestStart},
		{Provider: "openai-compatible", Model: "gpt-x", PromptTokens: 55, CompletionTokens: 12, StartedAt: requestStart.Add(2 * time.Second)},
	}
	if err := recorder.RecordModelUsage(usage); err != nil {
		t.Fatalf("RecordModelUsage() error = %v", err)
	}

	path := filepath.Join(root, "lore-usage.jsonl")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript error = %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), MaxJSONLLineBytes)
	var usageEvents []Event
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode line: %v", err)
		}
		if event.Type == EventModelUsage {
			usageEvents = append(usageEvents, event)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan error = %v", err)
	}
	if len(usageEvents) != 2 {
		t.Fatalf("usage events = %d, want 2", len(usageEvents))
	}
	first := usageEvents[0]
	if first.Provider != "openai-compatible" || first.Model != "gpt-x" {
		t.Fatalf("event[0] provider/model = %q/%q", first.Provider, first.Model)
	}
	if first.PromptTokens != 31 || first.CompletionTokens != 9 {
		t.Fatalf("event[0] tokens = %d/%d, want 31/9", first.PromptTokens, first.CompletionTokens)
	}
	if !first.StartedAt.Equal(requestStart) {
		t.Fatalf("event[0] StartedAt = %v, want %v", first.StartedAt, requestStart)
	}
	if first.Timestamp.IsZero() {
		t.Fatal("event[0] Timestamp should not be zero")
	}
	if usageEvents[1].PromptTokens != 55 || usageEvents[1].CompletionTokens != 12 {
		t.Fatalf("event[1] tokens = %d/%d, want 55/12", usageEvents[1].PromptTokens, usageEvents[1].CompletionTokens)
	}

	// Reloading a session containing model_usage events must succeed
	// without polluting history or working set.
	snapshot, err := Load(root, "lore-usage")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(snapshot.History) != 0 {
		t.Fatalf("history = %+v, want empty after only usage events", snapshot.History)
	}
	if len(snapshot.WorkingSet) != 0 {
		t.Fatalf("working set = %+v, want empty", snapshot.WorkingSet)
	}
}

func TestRecordTaskTurnEndWritesEvent(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-turn", AgentID: "codex", StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordTaskTurnEnd("final", 3); err != nil {
		t.Fatalf("RecordTaskTurnEnd() error = %v", err)
	}

	path := filepath.Join(root, "lore-turn.jsonl")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript error = %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), MaxJSONLLineBytes)
	var turnEnds []Event
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode line: %v", err)
		}
		if event.Type == EventTaskTurnEnd {
			turnEnds = append(turnEnds, event)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan error = %v", err)
	}
	if len(turnEnds) != 1 {
		t.Fatalf("task_turn_end events = %d, want 1", len(turnEnds))
	}
	got := turnEnds[0]
	if got.StopReason != "final" || got.StepCount != 3 {
		t.Fatalf("event fields = %s / %d, want final / 3", got.StopReason, got.StepCount)
	}

	// Load must not promote a task_turn_end event into history or
	// working set.
	snapshot, err := Load(root, "lore-turn")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(snapshot.History) != 0 || len(snapshot.WorkingSet) != 0 {
		t.Fatalf("snapshot history=%+v workingSet=%+v should both be empty", snapshot.History, snapshot.WorkingSet)
	}
}

func TestRecordTaskTurnEndEmptyReasonIsNoOp(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-turn-empty", StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordTaskTurnEnd("", 1); err != nil {
		t.Fatalf("RecordTaskTurnEnd(empty) error = %v", err)
	}

	path := filepath.Join(root, "lore-turn-empty.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if strings.Contains(string(data), EventTaskTurnEnd) {
		t.Fatalf("transcript should not contain task_turn_end when reason is empty:\n%s", data)
	}
}

func TestRecordModelUsageEmptyIsNoOp(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-usage-empty", StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordModelUsage(nil); err != nil {
		t.Fatalf("RecordModelUsage(nil) error = %v", err)
	}
	if err := recorder.RecordModelUsage([]operatoragent.ModelCallUsage{}); err != nil {
		t.Fatalf("RecordModelUsage([]) error = %v", err)
	}
}

func TestLoadIgnoresUnknownEventTypesForBackwardsCompatibility(t *testing.T) {
	// Simulates an older binary reading a JSONL written by a newer
	// binary that emits an unknown event type. applyEvent silently
	// skips unknown types so the session still loads cleanly.
	root := t.TempDir()
	path := filepath.Join(root, "lore-future.jsonl")
	content := strings.Join([]string{
		`{"type":"session_meta","session_id":"lore-future","timestamp":"2026-04-25T10:00:00Z"}`,
		`{"type":"user_message","timestamp":"2026-04-25T10:00:01Z","text":"hello"}`,
		`{"type":"model_usage","timestamp":"2026-04-25T10:00:02Z","provider":"x","model":"y","prompt_tokens":3,"completion_tokens":1}`,
		`{"type":"future_event_v2","timestamp":"2026-04-25T10:00:03Z","novel_field":"value"}`,
		`{"type":"assistant_message","timestamp":"2026-04-25T10:00:04Z","text":"world"}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot, err := Load(root, "lore-future")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("history = %+v, want 2 turns", snapshot.History)
	}
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none for known/unknown types", snapshot.Warnings)
	}
}

func TestLoadSkipsCorruptedLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lore-corrupt.jsonl")
	content := strings.Join([]string{
		`{"type":"session_meta","session_id":"lore-corrupt","timestamp":"2026-04-25T10:00:00Z"}`,
		`not json`,
		`{"type":"user_message","timestamp":"2026-04-25T10:00:01Z","text":"hello"}`,
		`{"type":"assistant_message","timestamp":"2026-04-25T10:00:02Z","text":"world"}`,
		`{"type":"assistant_message"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot, err := Load(root, "lore-corrupt")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(snapshot.Warnings) != 2 {
		t.Fatalf("warnings = %+v, want 2", snapshot.Warnings)
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("history = %+v", snapshot.History)
	}
}

func TestLoadSupportsLargeJSONLLines(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-large-line", StartedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	largeReply := strings.Repeat("x", 2*1024*1024)
	if err := recorder.RecordAssistant(largeReply); err != nil {
		t.Fatalf("RecordAssistant(large) error = %v", err)
	}
	snapshot, err := Load(root, "lore-large-line")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(snapshot.History) != 1 || snapshot.History[0].Content != largeReply {
		t.Fatalf("history length/content mismatch: len=%d", len(snapshot.History))
	}
}

func TestToolTraceTruncatesArgumentsAndError(t *testing.T) {
	root := t.TempDir()
	recorder, err := StartWithLimits(root, Meta{SessionID: "lore-truncate", StartedAt: time.Now()}, Limits{MaxToolArgumentsBytes: 20, MaxToolErrorBytes: 8})
	if err != nil {
		t.Fatalf("StartWithLimits() error = %v", err)
	}
	if err := recorder.RecordToolTrace([]operatoragent.ToolCallTrace{{
		Name:      "vault_write_low",
		Arguments: map[string]any{"content": strings.Repeat("x", 200)},
		Status:    "error",
		Error:     strings.Repeat("e", 100),
	}}); err != nil {
		t.Fatalf("RecordToolTrace() error = %v", err)
	}
	file, err := os.Open(recorder.Path())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var tool Event
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if event.Type == EventToolCall {
			tool = event
		}
	}
	var truncated map[string]any
	if err := json.Unmarshal(tool.Arguments, &truncated); err != nil {
		t.Fatalf("json.Unmarshal(tool.Arguments) error = %v", err)
	}
	if truncated["truncated"] != true {
		t.Fatalf("tool.Arguments = %s, want truncated envelope", string(tool.Arguments))
	}
	if len(tool.Arguments) > 80 {
		t.Fatalf("arguments too large: %d", len(tool.Arguments))
	}
	if !strings.Contains(tool.Error, "truncated") {
		t.Fatalf("tool.Error = %q, want truncated marker", tool.Error)
	}
}

func TestSearchMatchesTranscriptContent(t *testing.T) {
	root := t.TempDir()
	first, err := Start(root, Meta{SessionID: "lore-search-first", StartedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start(first) error = %v", err)
	}
	if err := first.RecordUser("open weekly review"); err != nil {
		t.Fatalf("RecordUser(first) error = %v", err)
	}
	if err := first.RecordAssistant("Discussed the unique transcript needle."); err != nil {
		t.Fatalf("RecordAssistant(first) error = %v", err)
	}

	second, err := Start(root, Meta{SessionID: "lore-search-second", StartedAt: time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start(second) error = %v", err)
	}
	if err := second.RecordUser("other topic"); err != nil {
		t.Fatalf("RecordUser(second) error = %v", err)
	}
	if err := second.RecordAssistant("No matching content here."); err != nil {
		t.Fatalf("RecordAssistant(second) error = %v", err)
	}

	matches, err := Search(root, "unique transcript needle", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 || matches[0].ID != "lore-search-first" {
		t.Fatalf("matches = %+v, want lore-search-first only", matches)
	}
}

func TestListRecentRebuildsMissingIndex(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-rebuild-index", StartedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordUser("recoverable index"); err != nil {
		t.Fatalf("RecordUser() error = %v", err)
	}
	if err := recorder.RecordAssistant("searchable transcript content"); err != nil {
		t.Fatalf("RecordAssistant() error = %v", err)
	}
	if err := os.Remove(indexPath(root)); err != nil {
		t.Fatalf("Remove(index.json) error = %v", err)
	}

	recent, err := ListRecent(root, 20)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 1 || recent[0].ID != "lore-rebuild-index" || recent[0].TurnCount != 1 {
		t.Fatalf("recent = %+v, want rebuilt session", recent)
	}
	if _, err := os.Stat(indexPath(root)); err != nil {
		t.Fatalf("rebuilt index missing: %v", err)
	}
	matches, err := Search(root, "searchable transcript", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 || matches[0].ID != "lore-rebuild-index" {
		t.Fatalf("matches = %+v, want rebuilt session", matches)
	}
}

func TestListRecentRejectsCorruptIndex(t *testing.T) {
	root := t.TempDir()
	recorder, err := Start(root, Meta{SessionID: "lore-corrupt-index", StartedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := recorder.RecordUser("do not hide corrupt index"); err != nil {
		t.Fatalf("RecordUser() error = %v", err)
	}
	if err := os.WriteFile(indexPath(root), []byte(`{"version":`), 0o644); err != nil {
		t.Fatalf("WriteFile(index.json) error = %v", err)
	}

	if _, err := ListRecent(root, 20); err == nil {
		t.Fatal("ListRecent() error = nil, want corrupt index error")
	}
	data, err := os.ReadFile(indexPath(root))
	if err != nil {
		t.Fatalf("ReadFile(index.json) error = %v", err)
	}
	if string(data) != `{"version":` {
		t.Fatalf("index was overwritten: %q", string(data))
	}
}

func TestSaveIndexReplacesAtomicallyAndCleansTempFile(t *testing.T) {
	root := t.TempDir()
	staleTemp := indexPath(root) + indexTempSuffix
	if err := os.WriteFile(staleTemp, []byte("stale temp"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale temp) error = %v", err)
	}

	first, err := Start(root, Meta{SessionID: "lore-index-atomic-1", StartedAt: time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start(first) error = %v", err)
	}
	if err := first.RecordUser("first"); err != nil {
		t.Fatalf("RecordUser(first) error = %v", err)
	}
	second, err := Start(root, Meta{SessionID: "lore-index-atomic-2", StartedAt: time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Start(second) error = %v", err)
	}
	if err := second.RecordUser("second"); err != nil {
		t.Fatalf("RecordUser(second) error = %v", err)
	}

	if _, err := os.Stat(staleTemp); !os.IsNotExist(err) {
		t.Fatalf("temp index file stat error = %v, want not exist after atomic save", err)
	}
	recent, err := ListRecent(root, 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("recent = %+v, want two indexed sessions", recent)
	}
	if recent[0].ID != "lore-index-atomic-2" || recent[1].ID != "lore-index-atomic-1" {
		t.Fatalf("recent order = %+v, want newest first", recent)
	}
}

func TestListRecentLimitAndOrder(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 25; i++ {
		id := NewSessionID(time.Date(2026, 4, 25, 10, i, 0, 0, time.UTC))
		recorder, err := Start(root, Meta{SessionID: id, StartedAt: time.Date(2026, 4, 25, 10, i, 0, 0, time.UTC)})
		if err != nil {
			t.Fatalf("Start(%d) error = %v", i, err)
		}
		if err := recorder.RecordUser("session"); err != nil {
			t.Fatalf("RecordUser(%d) error = %v", i, err)
		}
	}
	recent, err := ListRecent(root, 20)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 20 {
		t.Fatalf("len(recent) = %d, want 20", len(recent))
	}
	if !recent[0].UpdatedAt.After(recent[len(recent)-1].UpdatedAt) {
		t.Fatalf("recent not sorted: first=%s last=%s", recent[0].UpdatedAt, recent[len(recent)-1].UpdatedAt)
	}
}
