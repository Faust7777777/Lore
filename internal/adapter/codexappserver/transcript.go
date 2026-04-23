package codexappserver

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
)

// TranscriptFromThread converts an app-server thread into the same transcript
// shape used by the JSONL ingestion path so the downstream window/process-sink
// pipeline stays shared.
func TranscriptFromThread(thread Thread, sourcePath string) (codexjsonl.Transcript, error) {
	transcript := codexjsonl.Transcript{
		AgentID:    "codex",
		SessionID:  strings.TrimSpace(thread.ID),
		SourcePath: resolveThreadSourcePath(thread, sourcePath),
	}

	var (
		offset        int64
		lastTimestamp time.Time
	)
	for turnIndex, turn := range thread.Turns {
		baseTimestamp := resolveTurnTimestamp(thread, turn, lastTimestamp)
		events, err := eventsFromTurn(turn, baseTimestamp, turnIndex)
		if err != nil {
			return codexjsonl.Transcript{}, err
		}
		for _, event := range events {
			event.OffsetStart = offset
			rawLen := int64(len(event.RawLine))
			if rawLen > 0 {
				rawLen++ // synthetic newline separator to match JSONL offsets
			}
			offset += rawLen
			event.OffsetEnd = offset
			transcript.Events = append(transcript.Events, event)
			lastTimestamp = event.Timestamp
		}
	}

	codexjsonl.FinalizeTranscript(&transcript)
	return transcript, nil
}

func resolveThreadSourcePath(thread Thread, sourcePath string) string {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath != "" {
		return filepath.Clean(sourcePath)
	}
	threadID := strings.TrimSpace(thread.ID)
	if threadID == "" {
		threadID = "thread"
	}
	return filepath.Join("codex-app-server", sanitizeThreadID(threadID)+".jsonl")
}

func sanitizeThreadID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "thread"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	out := strings.Trim(builder.String(), "-_")
	if out == "" {
		return "thread"
	}
	return out
}

func eventsFromTurn(turn Turn, baseTimestamp time.Time, turnIndex int) ([]codexjsonl.Event, error) {
	if baseTimestamp.IsZero() {
		if turnHasMaterial(turn) {
			turnID := strings.TrimSpace(turn.ID)
			if turnID == "" {
				turnID = "<unknown>"
			}
			return nil, fmt.Errorf("codex app-server turn %s has textual content but no timestamp metadata", turnID)
		}
		return nil, nil
	}
	fallbackRole := inferTurnFallbackRole(turn.Items)
	events := make([]codexjsonl.Event, 0, len(turn.Items)+1)
	for itemIndex, item := range turn.Items {
		role := explicitItemRole(item)
		if role == "" {
			role = fallbackRole
		}
		if role == "" {
			continue
		}
		text := extractItemText(item)
		if text == "" {
			continue
		}
		timestamp := eventTimestamp(baseTimestamp, turnIndex, itemIndex)
		events = append(events, codexjsonl.Event{
			Timestamp: timestamp,
			Role:      role,
			Phase:     strings.TrimSpace(item.Phase),
			Text:      text,
			RawLine:   buildSyntheticRawLine(timestamp, role, strings.TrimSpace(item.Phase), text),
		})
	}
	if strings.TrimSpace(turn.ErrorMessage()) != "" {
		timestamp := eventTimestamp(baseTimestamp, turnIndex, len(turn.Items))
		text := strings.TrimSpace(turn.ErrorMessage())
		events = append(events, codexjsonl.Event{
			Timestamp: timestamp,
			Role:      "assistant",
			Phase:     "error",
			Text:      text,
			RawLine:   buildSyntheticRawLine(timestamp, "assistant", "error", text),
		})
	}
	return events, nil
}

func (t Turn) ErrorMessage() string {
	if t.Error == nil {
		return ""
	}
	return t.Error.Message
}

func inferTurnFallbackRole(items []Item) string {
	var hasUser bool
	var hasAssistant bool
	for _, item := range items {
		switch explicitItemRole(item) {
		case "user":
			hasUser = true
		case "assistant":
			hasAssistant = true
		}
	}
	switch {
	case hasUser && !hasAssistant:
		return "user"
	case hasAssistant && !hasUser:
		return "assistant"
	default:
		return ""
	}
}

func turnHasMaterial(turn Turn) bool {
	if strings.TrimSpace(turn.ErrorMessage()) != "" {
		return true
	}
	for _, item := range turn.Items {
		if strings.TrimSpace(extractItemText(item)) != "" {
			return true
		}
	}
	return false
}

func explicitItemRole(item Item) string {
	normalized := normalizeType(item.Type)
	switch normalized {
	case "user", "user-message", "user_message", "input", "input-message":
		return "user"
	case "assistant", "assistant-message", "assistant_message", "agent", "agent-message", "agent_message", "output", "output-message":
		return "assistant"
	}
	if strings.Contains(normalized, "user") {
		return "user"
	}
	if strings.Contains(normalized, "assistant") || strings.Contains(normalized, "agent") || strings.Contains(normalized, "output") {
		return "assistant"
	}
	return ""
}

func normalizeType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func extractItemText(item Item) string {
	parts := make([]string, 0, len(item.Content)+1)
	seen := make(map[string]struct{}, len(item.Content)+1)

	appendText := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		parts = append(parts, value)
	}

	appendText(item.Text)
	for _, content := range item.Content {
		switch normalizeType(content.Type) {
		case "text", "input-text", "input_text", "output-text", "output_text", "markdown":
			appendText(content.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func resolveTurnTimestamp(thread Thread, turn Turn, lastTimestamp time.Time) time.Time {
	for _, raw := range []int64{turn.CreatedAt, turn.UpdatedAt, thread.CreatedAt, thread.UpdatedAt} {
		if timestamp := parseUnixTimestamp(raw); !timestamp.IsZero() {
			return timestamp.In(time.Local)
		}
	}
	if !lastTimestamp.IsZero() {
		return lastTimestamp.Add(time.Second)
	}
	return time.Time{}
}

func parseUnixTimestamp(raw int64) time.Time {
	if raw <= 0 {
		return time.Time{}
	}
	switch {
	case raw >= 1_000_000_000_000_000_000:
		return time.Unix(0, raw)
	case raw >= 1_000_000_000_000_000:
		return time.Unix(0, raw*int64(time.Microsecond))
	case raw >= 1_000_000_000_000:
		return time.Unix(0, raw*int64(time.Millisecond))
	default:
		return time.Unix(raw, 0)
	}
}

func eventTimestamp(base time.Time, turnIndex int, itemIndex int) time.Time {
	if base.IsZero() {
		return base
	}
	return base.Add(time.Duration(turnIndex)*time.Millisecond + time.Duration(itemIndex)*time.Microsecond)
}

func buildSyntheticRawLine(timestamp time.Time, role string, phase string, text string) string {
	payloadType := "user_message"
	if role == "assistant" {
		payloadType = "agent_message"
	}
	payload := map[string]any{
		"type":    payloadType,
		"message": text,
	}
	if strings.TrimSpace(phase) != "" {
		payload["phase"] = strings.TrimSpace(phase)
	}
	data, err := json.Marshal(map[string]any{
		"timestamp": timestamp.Format(time.RFC3339Nano),
		"type":      "event_msg",
		"payload":   payload,
	})
	if err != nil {
		return fmt.Sprintf(
			`{"timestamp":%q,"type":"event_msg","payload":{"type":%q,"message":%q}}`,
			timestamp.Format(time.RFC3339Nano),
			payloadType,
			text,
		)
	}
	return string(data)
}
