package app

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateForSummaryIsRuneSafe(t *testing.T) {
	// CJK transcript content; a byte slice at limit=4 would split a 3-byte
	// character before the text reaches the summarizing LLM.
	cjk := "\u6587" // U+6587 (文 omitted: escape keeps source ASCII)
	value := strings.Repeat(cjk, 10)
	got := truncateForSummary(value, 4)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateForSummary produced invalid UTF-8: %q", got)
	}
	if want := strings.Repeat(cjk, 4) + "..."; got != want {
		t.Fatalf("truncateForSummary = %q, want %q", got, want)
	}
}
