package orchestrator

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// cjkRune is 文 (U+6587), a 3-byte UTF-8 character. Written as a \u escape so
// the source stays ASCII and cannot be corrupted to mojibake by tooling.
const cjkRune = "\u6587"

func TestExcerptIsRuneSafe(t *testing.T) {
	// 10 CJK runes = 30 bytes. A byte slice at limit=5 would land inside the
	// second character and emit an invalid UTF-8 sequence.
	value := strings.Repeat(cjkRune, 10)
	got := excerpt(value, 5)
	if !utf8.ValidString(got) {
		t.Fatalf("excerpt produced invalid UTF-8: %q", got)
	}
	if want := strings.Repeat(cjkRune, 5) + "..."; got != want {
		t.Fatalf("excerpt = %q, want %q", got, want)
	}
}

func TestSummarizeContentIsRuneSafe(t *testing.T) {
	// >80 CJK runes; byte truncation at 80 would split the 27th character.
	got := summarizeContent([]byte(strings.Repeat(cjkRune, 100)))
	if !utf8.ValidString(got) {
		t.Fatalf("summarizeContent produced invalid UTF-8: %q", got)
	}
	if want := strings.Repeat(cjkRune, 80) + "..."; got != want {
		t.Fatalf("summarizeContent = %q, want %q", got, want)
	}
}
