package tools

import (
	"fmt"
	"strings"
	"time"
)

// getString returns the string value at args[key] with whitespace trimmed.
// Returns "" when args is nil, the key is missing, or the value is not a
// string. Mirrors the semantics of internal/mcp/server.go:getString so the
// migration produces byte-equivalent dispatch behavior.
func getString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	text, _ := args[key].(string)
	return strings.TrimSpace(text)
}

// getInt returns the integer value at args[key] with the supplied fallback
// when args is nil, the key is missing, or the value cannot be coerced to
// an int. Accepts both numeric (float64 from JSON) and integer values.
func getInt(args map[string]any, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	value, ok := args[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	}
	return fallback
}

// getDirArg returns the "dir" argument, falling back to the deprecated
// "path" alias when "dir" is empty. Preserves the live MCP behavior for
// vault_list, vault_search_text, and vault_resolve.
func getDirArg(args map[string]any) string {
	if dir := getString(args, "dir"); dir != "" {
		return dir
	}
	return getString(args, "path")
}

// getTargetPathArg returns the "target_path" argument, falling back to the
// deprecated "path" alias when "target_path" is empty. Used by context_pack.
func getTargetPathArg(args map[string]any) string {
	if targetPath := getString(args, "target_path"); targetPath != "" {
		return targetPath
	}
	return getString(args, "path")
}

// parseObservedAt parses the observed_at proposal argument. The value is
// required and must be either RFC3339 (e.g. 2026-04-22T09:00:00Z) or the
// shorter YYYY-MM-DD form. Behavior mirrors internal/mcp/server.go's
// historical parseObservedAt so the proposal-tool migration produces
// identical responses for invalid inputs.
func parseObservedAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("observed_at is required")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("observed_at must be RFC3339 or YYYY-MM-DD")
}
