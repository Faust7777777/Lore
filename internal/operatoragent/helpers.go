package operatoragent

import (
	"regexp"
	"strings"
	"time"

	"obsidian-harness/internal/model"
)

var datePattern = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

func resolveDay(value string, now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	if match := datePattern.FindString(value); match != "" {
		if day, err := time.ParseInLocation("2006-01-02", match, time.Local); err == nil {
			return model.NormalizeDay(day)
		}
	}
	if hasAny(value, "yesterday") {
		return model.NormalizeDay(now.AddDate(0, 0, -1))
	}
	return model.NormalizeDay(now)
}

func asksForBackgroundRuntime(value string) bool {
	return hasAny(value,
		"attach-codex-jsonl",
		"sync-codex-jsonl",
		"import-codex-jsonl",
		"scheduler",
		"schedule",
		"poll",
		"every 30 minutes",
		"every 30 mins",
		"every half hour",
		"half hour")
}

func hasAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func withFallback(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
