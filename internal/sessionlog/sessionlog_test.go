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
