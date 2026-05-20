package persona

import (
	"strings"
	"testing"
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
