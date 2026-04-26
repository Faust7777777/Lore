package sessionlog

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/operatoragent"
)

type Recorder struct {
	rootDir string
	path    string
	meta    Meta
	limits  Limits
}

type Limits struct {
	MaxToolArgumentsBytes int
	MaxToolErrorBytes     int
}

func defaultLimits(limits Limits) Limits {
	if limits.MaxToolArgumentsBytes <= 0 {
		limits.MaxToolArgumentsBytes = DefaultMaxToolArgumentsBytes
	}
	if limits.MaxToolErrorBytes <= 0 {
		limits.MaxToolErrorBytes = DefaultMaxToolErrorBytes
	}
	return limits
}

func Start(rootDir string, meta Meta) (*Recorder, error) {
	return StartWithLimits(rootDir, meta, Limits{})
}

func StartWithLimits(rootDir string, meta Meta, limits Limits) (*Recorder, error) {
	rootDir = filepath.Clean(rootDir)
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}
	if meta.StartedAt.IsZero() {
		meta.StartedAt = time.Now()
	}
	if strings.TrimSpace(meta.SessionID) == "" {
		meta.SessionID = NewSessionID(meta.StartedAt)
	}
	path := filepath.Join(rootDir, meta.SessionID+".jsonl")
	recorder := &Recorder{rootDir: rootDir, path: path, meta: meta, limits: defaultLimits(limits)}
	if err := recorder.append(Event{
		Version:   1,
		Type:      EventSessionMeta,
		Timestamp: meta.StartedAt,
		SessionID: meta.SessionID,
		AgentID:   meta.AgentID,
		Model:     meta.Model,
		WorkDir:   meta.WorkDir,
		VaultRoot: meta.VaultRoot,
	}); err != nil {
		return nil, err
	}
	if err := upsertIndex(rootDir, summaryFromMeta(meta, recorder.relativePath(), meta.StartedAt)); err != nil {
		return nil, err
	}
	return recorder, nil
}

func Resume(rootDir string, sessionID string) (*Recorder, Snapshot, error) {
	return ResumeWithLimits(rootDir, sessionID, Limits{})
}

func ResumeWithLimits(rootDir string, sessionID string, limits Limits) (*Recorder, Snapshot, error) {
	rootDir = filepath.Clean(rootDir)
	snapshot, err := Load(rootDir, sessionID)
	if err != nil {
		return nil, Snapshot{}, err
	}
	meta := snapshot.Meta
	if strings.TrimSpace(meta.SessionID) == "" {
		meta.SessionID = strings.TrimSpace(sessionID)
	}
	path := filepath.Join(rootDir, meta.SessionID+".jsonl")
	return &Recorder{rootDir: rootDir, path: path, meta: meta, limits: defaultLimits(limits)}, snapshot, nil
}

func (r *Recorder) SessionID() string {
	if r == nil {
		return ""
	}
	return r.meta.SessionID
}

func (r *Recorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

func (r *Recorder) RecordUser(text string) error {
	return r.appendAndIndex(Event{Type: EventUserMessage, Timestamp: time.Now(), Text: text})
}

func (r *Recorder) RecordAssistant(text string) error {
	return r.appendAndIndex(Event{Type: EventAssistantMessage, Timestamp: time.Now(), Text: text})
}

func (r *Recorder) RecordToolTrace(trace []operatoragent.ToolCallTrace) error {
	for _, item := range trace {
		arguments, err := json.Marshal(item.Arguments)
		if err != nil {
			arguments = []byte(`{"error":"unmarshalable arguments"}`)
		}
		event := Event{
			Type:      EventToolCall,
			Timestamp: time.Now(),
			Name:      strings.TrimSpace(item.Name),
			Arguments: json.RawMessage(truncateBytes(arguments, r.limits.MaxToolArgumentsBytes)),
			Status:    strings.TrimSpace(item.Status),
			Error:     truncateString(item.Error, r.limits.MaxToolErrorBytes),
		}
		if err := r.appendAndIndex(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recorder) RecordWorkingSet(items []operatoragent.WorkingSetItem) error {
	cloned := append([]operatoragent.WorkingSetItem(nil), items...)
	return r.appendAndIndex(Event{Type: EventWorkingSet, Timestamp: time.Now(), Items: cloned})
}

func (r *Recorder) RecordLocalCommand(command string) error {
	return r.appendAndIndex(Event{Type: EventLocalCommand, Timestamp: time.Now(), Command: strings.TrimSpace(command)})
}

func (r *Recorder) RecordError(message string, recoverable bool) error {
	return r.appendAndIndex(Event{Type: EventError, Timestamp: time.Now(), Error: truncateString(message, r.limits.MaxToolErrorBytes), Recoverable: recoverable})
}

func (r *Recorder) Close(reason string) error {
	return r.appendAndIndex(Event{Type: EventSessionEnd, Timestamp: time.Now(), Reason: strings.TrimSpace(reason)})
}

func (r *Recorder) appendAndIndex(event Event) error {
	if err := r.append(event); err != nil {
		return err
	}
	snapshot, err := Load(r.rootDir, r.meta.SessionID)
	if err != nil {
		return err
	}
	return upsertIndex(r.rootDir, snapshot.Summary)
}

func (r *Recorder) append(event Event) error {
	if r == nil {
		return nil
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.SessionID = r.meta.SessionID
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func Load(rootDir string, sessionID string) (Snapshot, error) {
	rootDir = filepath.Clean(rootDir)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Snapshot{}, fmt.Errorf("session id is required")
	}
	path := filepath.Join(rootDir, sessionID+".jsonl")
	file, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer file.Close()

	snapshot := Snapshot{}
	summary := Summary{ID: sessionID, Path: filepath.ToSlash(sessionID + ".jsonl")}
	var pendingUser string
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, MaxJSONLLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("skip invalid jsonl line %d", lineNumber))
			continue
		}
		applyEvent(&snapshot, &summary, &pendingUser, event)
	}
	if err := scanner.Err(); err != nil {
		return Snapshot{}, err
	}
	if summary.Title == "" {
		summary.Title = sessionID
	}
	if summary.StartedAt.IsZero() {
		summary.StartedAt = snapshot.Meta.StartedAt
	}
	if summary.UpdatedAt.IsZero() {
		summary.UpdatedAt = summary.StartedAt
	}
	snapshot.Summary = summary
	return snapshot, nil
}

func applyEvent(snapshot *Snapshot, summary *Summary, pendingUser *string, event Event) {
	if event.Timestamp.After(summary.UpdatedAt) {
		summary.UpdatedAt = event.Timestamp
	}
	switch event.Type {
	case EventSessionMeta:
		snapshot.Meta = Meta{SessionID: event.SessionID, AgentID: event.AgentID, Model: event.Model, WorkDir: event.WorkDir, VaultRoot: event.VaultRoot, StartedAt: event.Timestamp}
		summary.ID = event.SessionID
		summary.AgentID = event.AgentID
		summary.Model = event.Model
		summary.StartedAt = event.Timestamp
		if summary.Path == "" {
			summary.Path = filepath.ToSlash(event.SessionID + ".jsonl")
		}
	case EventUserMessage:
		text := strings.TrimSpace(event.Text)
		if text != "" {
			snapshot.History = append(snapshot.History, operatoragent.ConversationTurn{Role: "user", Content: text})
			*pendingUser = text
			if summary.Title == "" {
				summary.Title = titleFromText(text, summary.ID)
			}
		}
	case EventAssistantMessage:
		text := strings.TrimSpace(event.Text)
		if text != "" {
			snapshot.History = append(snapshot.History, operatoragent.ConversationTurn{Role: "assistant", Content: text})
			if strings.TrimSpace(*pendingUser) != "" {
				summary.TurnCount++
				*pendingUser = ""
			}
		}
	case EventWorkingSet:
		snapshot.WorkingSet = append([]operatoragent.WorkingSetItem(nil), event.Items...)
	}
	if len(snapshot.History) > 20 {
		snapshot.History = append([]operatoragent.ConversationTurn(nil), snapshot.History[len(snapshot.History)-20:]...)
	}
}

func ListRecent(rootDir string, limit int) ([]Summary, error) {
	index, err := loadIndex(rootDir)
	if err != nil {
		return nil, err
	}
	sessions := append([]Summary(nil), index.Sessions...)
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt) })
	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}
	return sessions, nil
}

func Search(rootDir string, query string, limit int) ([]Summary, error) {
	rootDir = filepath.Clean(rootDir)
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	recent, err := ListRecent(rootDir, 0)
	if err != nil {
		return nil, err
	}
	matches := make([]Summary, 0)
	for _, summary := range recent {
		matched := strings.Contains(strings.ToLower(summary.Title), query) || strings.Contains(strings.ToLower(summary.ID), query)
		if !matched {
			matched = transcriptContains(rootDir, summary, query)
		}
		if matched {
			matches = append(matches, summary)
		}
		if limit > 0 && len(matches) >= limit {
			break
		}
	}
	return matches, nil
}

func transcriptContains(rootDir string, summary Summary, query string) bool {
	path := filepath.Join(rootDir, summary.ID+".jsonl")
	if strings.TrimSpace(summary.Path) != "" {
		path = filepath.Join(rootDir, filepath.FromSlash(summary.Path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), query)
}

func NewSessionID(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	var random [2]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "lore-" + now.Format("20060102-150405") + "-0000"
	}
	return "lore-" + now.Format("20060102-150405") + "-" + hex.EncodeToString(random[:])
}

func summaryFromMeta(meta Meta, relPath string, updatedAt time.Time) Summary {
	return Summary{ID: meta.SessionID, Path: relPath, StartedAt: meta.StartedAt, UpdatedAt: updatedAt, Title: meta.SessionID, Model: meta.Model, AgentID: meta.AgentID}
}

func (r *Recorder) relativePath() string {
	rel, err := filepath.Rel(r.rootDir, r.path)
	if err != nil {
		return filepath.ToSlash(filepath.Base(r.path))
	}
	return filepath.ToSlash(rel)
}

func titleFromText(text string, fallback string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return fallback
	}
	runes := []rune(text)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	return text
}

func truncateString(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + "...[truncated]"
}

func truncateBytes(value []byte, maxBytes int) []byte {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	preview := string(value[:maxBytes])
	truncated, err := json.Marshal(map[string]any{
		"truncated": true,
		"preview":   preview,
	})
	if err != nil {
		return []byte(`{"truncated":true}`)
	}
	return truncated
}
