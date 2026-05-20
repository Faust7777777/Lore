// Package persona implements the LLM-based persona candidate extractor.
//
// The extractor takes a single user message (plus optional persona /
// system context) and asks a model to identify durable profile facts
// worth proposing as updates to the persona document. It returns
// only PersonaCandidate values; it never writes the persona doc, never
// creates a draft, and never auto-approves anything. Downstream
// slices (P3-P6) take candidates -> store -> CLI review -> manual
// draft creation through the existing harness.ProposePersonaUpdate
// governance path.
//
// Design boundaries enforced here (P1+P2):
//   - extracts only from user-origin text; assistant messages may be
//     passed as disambiguation context but must never feed candidates;
//   - every emitted candidate carries a verbatim evidence_quote
//     substring of the user's text (NFKC-normalized + whitespace-
//     collapsed compare) -- LLM paraphrases are rejected by parser;
//   - low-confidence candidates are discarded before they leave the
//     extractor;
//   - conflicts vs current_value are re-evaluated by parser rather
//     than trusted from the model.
//
// This package does NOT depend on store / console / cli / tui / mcp.
// It is a leaf module that the P3+ slices will compose into.
package persona

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Confidence is the candidate's self-reported strength. Only Medium
// and High candidates survive parser filtering; Low is discarded so
// that downstream review queues are not polluted by guesses.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// IsKept reports whether a confidence value passes the discard
// threshold. Unknown / empty values are discarded.
func (c Confidence) IsKept() bool {
	switch c {
	case ConfidenceMedium, ConfidenceHigh:
		return true
	default:
		return false
	}
}

// SourceKind identifies where the user message originated. Today the
// console main loop is the only producer wired in (P4); transcript
// import paths are intentionally not extractor consumers in P4 to
// avoid runaway cost on large back-fill JSONLs.
type SourceKind string

const (
	SourceConsole        SourceKind = "console"
	SourceImportCodex    SourceKind = "import_codex"
	SourceImportExternal SourceKind = "import_external"
)

// PersonaExtractionInput is the request shape handed to an extractor.
// UserText is the only field that contributes to candidate text;
// AssistantContext is optional and is forwarded to the LLM solely as
// disambiguation context (the prompt instructs the model to ignore it
// for extraction).
type PersonaExtractionInput struct {
	UserText              string
	AssistantContext      string
	SourceKind            SourceKind
	SourceSessionID       string
	SourceAgentID         string
	ObservedAt            time.Time
	CurrentPersonaExcerpt string
	SystemRulesExcerpt    string
}

// PersonaCandidate is one structured proposal for a persona update.
// The parser fills SourceKind / SourceSessionID / ObservedAt from the
// input; the LLM never controls those values.
type PersonaCandidate struct {
	Field           string     `json:"field"`
	ProposedValue   string     `json:"proposed_value"`
	CurrentValue    string     `json:"current_value,omitempty"`
	EvidenceQuote   string     `json:"evidence_quote"`
	Reason          string     `json:"reason,omitempty"`
	Confidence      Confidence `json:"confidence"`
	Conflict        bool       `json:"conflict"`
	SourceKind      SourceKind `json:"source_kind"`
	SourceSessionID string     `json:"source_session_id,omitempty"`
	ObservedAt      time.Time  `json:"observed_at"`
}

// PersonaExtractionResult bundles surviving candidates with any
// warnings emitted while parsing. Warnings cover cases like
// "evidence_quote not found in user_text", "missing required field",
// or "discarded low-confidence candidate"; downstream review tools
// surface them so operators can spot prompt or model drift.
type PersonaExtractionResult struct {
	Candidates []PersonaCandidate
	Warnings   []string
}

// PersonaCandidateState names a stored candidate's review lifecycle
// position. Open candidates are visible in the review queue; drafted
// candidates have been promoted into a persona_update draft (P5+) and
// stay in store as historical evidence; dismissed candidates were
// rejected by the operator and remain only to prevent re-emission via
// dedup. Empty state is treated as Open for legacy records that
// predate this field; PersonaCandidateStore implementations MUST run
// new records through NormalizeCandidateState before persisting so
// the on-disk value is always one of the explicit constants.
type PersonaCandidateState string

const (
	PersonaCandidateOpen      PersonaCandidateState = "open"
	PersonaCandidateDrafted   PersonaCandidateState = "drafted"
	PersonaCandidateDismissed PersonaCandidateState = "dismissed"
)

// NormalizeCandidateState collapses the empty PersonaCandidateState
// to PersonaCandidateOpen. Use it on both the write side (so a record
// with zero State lands in the review queue rather than vanishing
// behind exact-match filters) and the read side (so legacy stores
// from before P3 surface their candidates with a meaningful state).
// Other values pass through unchanged; this helper deliberately does
// not validate that the value is one of the known constants because
// future states added in higher slices should not silently downgrade
// to Open here.
func NormalizeCandidateState(s PersonaCandidateState) PersonaCandidateState {
	if s == "" {
		return PersonaCandidateOpen
	}
	return s
}

// PersonaCandidateRecord is the storage envelope around a
// PersonaCandidate. ID is the durable handle CLI / app callers use;
// DedupKey is the normalized identity used by the store to reject
// duplicate insertions of the same field/value/evidence; CreatedAt
// and UpdatedAt track lifecycle.
//
// PersonaCandidate is embedded by value so the on-disk payload stays
// flat and so older readers that ignore the envelope fields can still
// decode the candidate body. New writers populate all envelope
// fields; legacy decoders see zero values and treat them per the
// PersonaCandidateState contract above.
type PersonaCandidateRecord struct {
	ID        string                `json:"id"`
	State     PersonaCandidateState `json:"state"`
	DedupKey  string                `json:"dedup_key"`
	Candidate PersonaCandidate      `json:"candidate"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// NormalizeText returns the canonical comparison form for persona
// text fields. It NFKC-normalizes, lower-cases, and collapses
// whitespace runs to a single space (trimming leading/trailing
// whitespace as a side effect of strings.Fields).
//
// Used by:
//   - the extractor parser, to verify that an LLM-emitted
//     evidence_quote is a verbatim substring of the user's message
//     (tolerating only formatting variation);
//   - DedupKey, to ensure two candidates that differ only in casing
//     or whitespace collapse to the same store identity.
//
// Both call sites need to agree, so they share this single
// definition. Exported so consumers (P3 store, P4 console) outside
// this package can derive the same key.
func NormalizeText(s string) string {
	return normalizeForSubstring(s)
}

// NewCandidateID generates a fresh PersonaCandidateRecord.ID. The
// shape is "pc-<RFC3339Nano UTC>-<8 hex bytes of randomness>": the
// timestamp prefix gives lexicographic ordering for debugging and a
// monotonic-ish tie-breaker, the suffix removes collision risk when
// two candidates land in the same nanosecond. Falls back to a
// deterministic suffix if crypto/rand fails so the caller still
// receives a usable (but less unique) ID.
func NewCandidateID(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	stamp := now.UTC().Format("20060102T150405.000000000Z")
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "pc-" + stamp + "-0000"
	}
	return "pc-" + stamp + "-" + hex.EncodeToString(random[:])
}

// DedupKey returns the canonical identity key for a candidate. Two
// candidates with the same DedupKey are considered the same proposal
// for storage purposes -- the second insertion is rejected as a
// duplicate by PersonaCandidateStore.UpsertCandidate so the review
// queue never accumulates near-identical entries.
//
// The key composes (field, proposed_value, evidence_quote) after
// NormalizeText so trivial casing / whitespace / NFKC variation does
// not bypass dedup. SourceSessionID and ObservedAt are intentionally
// NOT part of the key: the same fact observed twice in different
// sessions should still collapse to one candidate.
func DedupKey(c PersonaCandidate) string {
	field := NormalizeText(c.Field)
	value := NormalizeText(c.ProposedValue)
	evidence := NormalizeText(c.EvidenceQuote)
	return field + "|" + value + "|" + evidence
}
