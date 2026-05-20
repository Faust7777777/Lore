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

import "time"

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
