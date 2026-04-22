package codexjsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/model"
)

const maxJSONLLine = 4 * 1024 * 1024

type Event struct {
	Timestamp time.Time
	Role      string
	Phase     string
	Text      string
	RawLine   string
}

type Transcript struct {
	AgentID    string
	SessionID  string
	SourcePath string
	Events     []Event
}

type WindowSummary struct {
	Window        model.SessionWindow
	Title         string
	Content       string
	RawTranscript string
	EventCount    int
}

type envelope struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	SessionID string          `json:"session_id"`
	TS        json.RawMessage `json:"ts"`
	Text      string          `json:"text"`
}

func LoadFile(path string) (Transcript, error) {
	inferredSessionID := inferSessionID(path)
	file, err := os.Open(path)
	if err != nil {
		return Transcript{}, err
	}
	defer file.Close()

	transcript := Transcript{
		AgentID:    "codex",
		SessionID:  inferredSessionID,
		SourcePath: filepath.Clean(path),
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLine)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		event, meta, ok, err := parseLine(line)
		if err != nil {
			return Transcript{}, fmt.Errorf("parse %s line %d: %w", path, lineNumber, err)
		}
		if meta.SessionID != "" {
			if transcript.SessionID == "" || transcript.SessionID == "history" {
				transcript.SessionID = meta.SessionID
			}
		}
		if meta.AgentID != "" {
			if transcript.AgentID == "" || transcript.AgentID == "codex" {
				transcript.AgentID = meta.AgentID
			}
		}
		if ok {
			transcript.Events = append(transcript.Events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return Transcript{}, err
	}

	sort.Slice(transcript.Events, func(i, j int) bool {
		if transcript.Events[i].Timestamp.Equal(transcript.Events[j].Timestamp) {
			if transcript.Events[i].Role == transcript.Events[j].Role {
				return transcript.Events[i].Text < transcript.Events[j].Text
			}
			return transcript.Events[i].Role < transcript.Events[j].Role
		}
		return transcript.Events[i].Timestamp.Before(transcript.Events[j].Timestamp)
	})
	transcript.Events = dedupeEvents(transcript.Events)
	transcript.AgentID = sanitizeAgentID(transcript.AgentID)
	if transcript.AgentID == "" {
		transcript.AgentID = "codex"
	}
	if transcript.SessionID == "" {
		transcript.SessionID = inferredSessionID
	}
	return transcript, nil
}

func BuildWindows(transcript Transcript, every time.Duration) []WindowSummary {
	if every <= 0 {
		every = 30 * time.Minute
	}
	if len(transcript.Events) == 0 {
		return nil
	}

	grouped := make(map[string][]Event)
	for _, event := range transcript.Events {
		start := event.Timestamp.Truncate(every)
		window := model.SessionWindow{
			AgentID:     transcript.AgentID,
			SessionID:   transcript.SessionID,
			WindowStart: start,
			WindowEnd:   start.Add(every),
		}
		key := window.Key()
		grouped[key] = append(grouped[key], event)
	}

	firstStart := transcript.Events[0].Timestamp.Truncate(every)
	lastStart := transcript.Events[len(transcript.Events)-1].Timestamp.Truncate(every)
	out := make([]WindowSummary, 0)
	for start := firstStart; !start.After(lastStart); start = start.Add(every) {
		window := model.SessionWindow{
			AgentID:     transcript.AgentID,
			SessionID:   transcript.SessionID,
			WindowStart: start,
			WindowEnd:   start.Add(every),
		}
		events := grouped[window.Key()]
		out = append(out, WindowSummary{
			Window:        window,
			Title:         buildTitle(window, events),
			Content:       summarizeWindow(transcript.SourcePath, events),
			RawTranscript: joinRawLines(events),
			EventCount:    len(events),
		})
	}
	return out
}

type lineMeta struct {
	SessionID string
	AgentID   string
}

func parseLine(line string) (Event, lineMeta, bool, error) {
	var env envelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		return Event{}, lineMeta{}, false, err
	}

	// Lightweight history entries are user-only records.
	if env.SessionID != "" && len(env.TS) != 0 && strings.TrimSpace(env.Text) != "" && env.Type == "" {
		ts, err := parseUnixish(env.TS)
		if err != nil {
			return Event{}, lineMeta{}, false, err
		}
		return Event{
			Timestamp: ts.In(time.Local),
			Role:      "user",
			Text:      strings.TrimSpace(env.Text),
			RawLine:   line,
		}, lineMeta{SessionID: env.SessionID}, true, nil
	}

	switch env.Type {
	case "session_meta":
		var payload struct {
			ID            string `json:"id"`
			AgentNickname string `json:"agent_nickname"`
		}
		if len(env.Payload) != 0 {
			if err := json.Unmarshal(env.Payload, &payload); err != nil {
				return Event{}, lineMeta{}, false, err
			}
		}
		return Event{}, lineMeta{
			SessionID: payload.ID,
			AgentID:   payload.AgentNickname,
		}, false, nil
	case "event_msg":
		var payload struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Phase   string `json:"phase"`
		}
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return Event{}, lineMeta{}, false, err
		}
		role := ""
		switch payload.Type {
		case "user_message":
			role = "user"
		case "agent_message":
			role = "assistant"
		default:
			return Event{}, lineMeta{}, false, nil
		}
		ts, err := parseTimestamp(env.Timestamp)
		if err != nil {
			return Event{}, lineMeta{}, false, err
		}
		text := strings.TrimSpace(payload.Message)
		if text == "" {
			return Event{}, lineMeta{}, false, nil
		}
		return Event{
			Timestamp: ts.In(time.Local),
			Role:      role,
			Phase:     strings.TrimSpace(payload.Phase),
			Text:      text,
			RawLine:   line,
		}, lineMeta{}, true, nil
	case "response_item":
		var payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return Event{}, lineMeta{}, false, err
		}
		if payload.Type != "message" {
			return Event{}, lineMeta{}, false, nil
		}
		role := strings.TrimSpace(payload.Role)
		if role != "user" && role != "assistant" {
			return Event{}, lineMeta{}, false, nil
		}
		parts := make([]string, 0, len(payload.Content))
		for _, item := range payload.Content {
			switch item.Type {
			case "input_text", "text", "output_text":
				if text := strings.TrimSpace(item.Text); text != "" {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) == 0 {
			return Event{}, lineMeta{}, false, nil
		}
		ts, err := parseTimestamp(env.Timestamp)
		if err != nil {
			return Event{}, lineMeta{}, false, err
		}
		return Event{
			Timestamp: ts.In(time.Local),
			Role:      role,
			Text:      strings.Join(parts, "\n\n"),
			RawLine:   line,
		}, lineMeta{}, true, nil
	default:
		return Event{}, lineMeta{}, false, nil
	}
}

func parseTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("missing timestamp")
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return ts, nil
	}
	ts, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return ts, nil
	}
	return time.Time{}, err
}

func parseUnixish(raw json.RawMessage) (time.Time, error) {
	var intValue int64
	if err := json.Unmarshal(raw, &intValue); err == nil {
		return time.Unix(intValue, 0), nil
	}

	var floatValue float64
	if err := json.Unmarshal(raw, &floatValue); err == nil {
		return time.Unix(int64(floatValue), 0), nil
	}

	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		stringValue = strings.TrimSpace(stringValue)
		if stringValue == "" {
			return time.Time{}, fmt.Errorf("empty unix timestamp")
		}
		if len(stringValue) >= 13 {
			if millis, err := time.ParseDuration(stringValue + "ms"); err == nil {
				return time.Unix(0, millis.Nanoseconds()), nil
			}
		}
		if parsed, err := time.Parse(time.RFC3339Nano, stringValue); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported unix timestamp payload")
}

func dedupeEvents(events []Event) []Event {
	out := make([]Event, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		key := fmt.Sprintf("%s|%d|%s", event.Role, event.Timestamp.UnixMilli(), normalizeText(event.Text))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, event)
	}
	return out
}

func inferSessionID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func sanitizeAgentID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		case r == ' ':
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}
