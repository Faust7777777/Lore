package externaljsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
)

const maxExternalJSONLLine = 4 * 1024 * 1024

type envelope struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Role      string `json:"role"`
	Phase     string `json:"phase"`
	Text      string `json:"text"`
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
}

func LoadFile(path string) (codexjsonl.Transcript, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexjsonl.Transcript{}, err
	}
	defer file.Close()

	transcript := codexjsonl.Transcript{
		AgentID:    "external",
		SessionID:  inferSessionID(path),
		SourcePath: filepath.Clean(path),
	}

	var offset int64
	var agentMetaBound bool
	var sessionMetaBound bool
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxExternalJSONLLine)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		lineStart := offset
		offset += int64(len(raw)) + 1
		if line == "" {
			continue
		}
		if len(raw) > maxExternalJSONLLine {
			return codexjsonl.Transcript{}, fmt.Errorf("parse %s line %d: line exceeds %d bytes", transcript.SourcePath, lineNumber, maxExternalJSONLLine)
		}

		event, meta, ok, err := parseLine(line)
		if err != nil {
			return codexjsonl.Transcript{}, fmt.Errorf("parse %s line %d offset %d: %w", transcript.SourcePath, lineNumber, lineStart, err)
		}
		if meta.AgentID != "" && !agentMetaBound {
			transcript.AgentID = meta.AgentID
			agentMetaBound = true
		}
		if meta.SessionID != "" && !sessionMetaBound {
			transcript.SessionID = meta.SessionID
			sessionMetaBound = true
		}
		if ok {
			event.RawLine = line
			event.OffsetStart = lineStart
			event.OffsetEnd = offset
			transcript.Events = append(transcript.Events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return codexjsonl.Transcript{}, err
	}

	codexjsonl.FinalizeTranscript(&transcript)
	return transcript, nil
}

type lineMeta struct {
	AgentID   string
	SessionID string
}

func parseLine(line string) (codexjsonl.Event, lineMeta, bool, error) {
	var env envelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		return codexjsonl.Event{}, lineMeta{}, false, err
	}

	if env.Type == "session_meta" {
		return codexjsonl.Event{}, lineMeta{
			AgentID:   strings.TrimSpace(env.AgentID),
			SessionID: strings.TrimSpace(env.SessionID),
		}, false, nil
	}

	role := normalizeRole(env.Role)
	if role == "" {
		return codexjsonl.Event{}, lineMeta{}, false, fmt.Errorf("unsupported role %q", env.Role)
	}
	text := strings.TrimSpace(env.Text)
	if text == "" {
		return codexjsonl.Event{}, lineMeta{}, false, nil
	}
	ts, err := parseTimestamp(env.Timestamp)
	if err != nil {
		return codexjsonl.Event{}, lineMeta{}, false, err
	}
	return codexjsonl.Event{
			Timestamp: ts.In(time.Local),
			Role:      role,
			Phase:     strings.TrimSpace(env.Phase),
			Text:      text,
		}, lineMeta{
			AgentID:   strings.TrimSpace(env.AgentID),
			SessionID: strings.TrimSpace(env.SessionID),
		}, true, nil
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "user"
	case "assistant", "agent", "model":
		return "assistant"
	default:
		return ""
	}
}

func parseTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("missing timestamp")
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, nil
	}
	return time.Parse(time.RFC3339, value)
}

func inferSessionID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
