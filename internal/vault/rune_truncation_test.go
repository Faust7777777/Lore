package vault

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTrimPreviewIsRuneSafe(t *testing.T) {
	// Search/backlink previews include CJK; a 180-byte slice could split a
	// multibyte character into an invalid UTF-8 sequence.
	cjk := "\u6587" // U+6587 (文 omitted: escape keeps source ASCII)
	value := strings.Repeat(cjk, 200)
	got := trimPreview(value)
	if !utf8.ValidString(got) {
		t.Fatalf("trimPreview produced invalid UTF-8: %q", got)
	}
	if want := strings.Repeat(cjk, 180) + "..."; got != want {
		t.Fatalf("trimPreview = %q, want %q", got, want)
	}
}
