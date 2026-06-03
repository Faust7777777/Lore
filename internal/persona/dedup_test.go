package persona

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTextNFKCAndWhitespace(t *testing.T) {
	// Full-width digit/letter NFKC-normalize to half-width; trailing
	// and inner whitespace collapses; case folds.
	got := NormalizeText("  HELLO  ＷＯＲＬＤ \t 123  ")
	want := "hello world 123"
	if got != want {
		t.Fatalf("NormalizeText = %q, want %q", got, want)
	}
}

func TestDedupKeyIgnoresCasingAndWhitespace(t *testing.T) {
	a := PersonaCandidate{
		Field:         "major",
		ProposedValue: "Economics and Management",
		EvidenceQuote: "I'm majoring in economics and management",
	}
	b := PersonaCandidate{
		Field:         "major",
		ProposedValue: "economics  and management",
		EvidenceQuote: "I'M MAJORING IN ECONOMICS AND MANAGEMENT",
	}
	if DedupKey(a) != DedupKey(b) {
		t.Fatalf("DedupKey differs across formatting variants:\n a = %q\n b = %q", DedupKey(a), DedupKey(b))
	}
}

func TestDedupKeyDistinguishesFieldOrValue(t *testing.T) {
	base := PersonaCandidate{Field: "major", ProposedValue: "economics", EvidenceQuote: "I study economics"}
	otherField := base
	otherField.Field = "minor"
	if DedupKey(base) == DedupKey(otherField) {
		t.Fatalf("DedupKey collides across different field names")
	}
	otherValue := base
	otherValue.ProposedValue = "history"
	if DedupKey(base) == DedupKey(otherValue) {
		t.Fatalf("DedupKey collides across different proposed values")
	}
	otherEvidence := base
	otherEvidence.EvidenceQuote = "I study history of economics"
	if DedupKey(base) == DedupKey(otherEvidence) {
		t.Fatalf("DedupKey collides across different evidence quotes")
	}
}

func TestDedupKeyPreservesChineseFidelity(t *testing.T) {
	// Two Chinese candidates with the same field+value+evidence (modulo
	// half/full-width punctuation) must collide; different content must
	// not.
	a := PersonaCandidate{
		Field:         "major",
		ProposedValue: "经济管理",
		EvidenceQuote: "我在读经济管理",
	}
	b := PersonaCandidate{
		Field:         "major",
		ProposedValue: "经济管理",
		EvidenceQuote: "我在读经济管理",
	}
	if DedupKey(a) != DedupKey(b) {
		t.Fatalf("DedupKey differs across identical Chinese candidates:\n a = %q\n b = %q", DedupKey(a), DedupKey(b))
	}
	c := PersonaCandidate{
		Field:         "major",
		ProposedValue: "计算机",
		EvidenceQuote: "我在读经济管理",
	}
	if DedupKey(a) == DedupKey(c) {
		t.Fatalf("Chinese DedupKey collided across different proposed values")
	}
	// The dedup_key should contain the normalized Chinese text so the
	// store row can be inspected without re-running normalize.
	if !strings.Contains(DedupKey(a), "经济管理") {
		t.Fatalf("DedupKey lost Chinese content: %q", DedupKey(a))
	}
}

func TestNormalizeCandidateStateDefaultsEmptyToOpen(t *testing.T) {
	// Empty state -> Open so legacy / zero-state records land in the
	// review queue instead of vanishing behind exact-match filters. Any
	// explicit value -- including a future state this package doesn't
	// know yet -- must pass through unchanged so it is never silently
	// downgraded to Open.
	if got := NormalizeCandidateState(""); got != PersonaCandidateOpen {
		t.Fatalf("NormalizeCandidateState(\"\") = %q, want %q", got, PersonaCandidateOpen)
	}
	for _, s := range []PersonaCandidateState{
		PersonaCandidateOpen,
		PersonaCandidateDrafted,
		PersonaCandidateDismissed,
		PersonaCandidateState("some-future-state"),
	} {
		if got := NormalizeCandidateState(s); got != s {
			t.Fatalf("NormalizeCandidateState(%q) = %q, want passthrough", s, got)
		}
	}
}

func TestNewCandidateIDFormatAndUniqueness(t *testing.T) {
	// ID shape is "pc-<RFC3339Nano UTC>-<8 hex>": the timestamp prefix
	// gives debuggable lexical ordering, the random suffix removes
	// same-nanosecond collision risk. Two IDs minted at the same instant
	// must differ; a zero time falls back to now rather than emitting the
	// year-0001 zero stamp.
	at := time.Date(2026, 5, 21, 8, 30, 15, 123456789, time.UTC)
	const wantPrefix = "pc-20260521T083015.123456789Z-"
	id := NewCandidateID(at)
	if !strings.HasPrefix(id, wantPrefix) {
		t.Fatalf("NewCandidateID = %q, want prefix %q", id, wantPrefix)
	}
	suffix := strings.TrimPrefix(id, wantPrefix)
	if len(suffix) != 8 {
		t.Fatalf("random suffix = %q (len %d), want 8 hex chars", suffix, len(suffix))
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Fatalf("random suffix %q is not valid hex: %v", suffix, err)
	}

	// Same instant, different IDs (random suffix breaks the tie).
	if a, b := NewCandidateID(at), NewCandidateID(at); a == b {
		t.Fatalf("two IDs minted at the same instant collided: %q", a)
	}

	// Zero time falls back to now: well-formed and not the zero stamp.
	zeroID := NewCandidateID(time.Time{})
	if !strings.HasPrefix(zeroID, "pc-") {
		t.Fatalf("zero-time NewCandidateID = %q, want pc- prefix", zeroID)
	}
	if strings.HasPrefix(zeroID, "pc-00010101T") {
		t.Fatalf("zero time should fall back to now, got zero-stamp ID %q", zeroID)
	}
}
