package persona

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// rawExtractionPayload is the on-the-wire shape the LLM produces. It
// mirrors the schema documented in systemPrompt. Fields that the
// parser later overwrites (SourceKind, SourceSessionID, ObservedAt)
// are intentionally absent here so the LLM cannot influence them.
type rawExtractionPayload struct {
	Candidates []rawCandidate `json:"candidates"`
	Warnings   []string       `json:"warnings"`
}

type rawCandidate struct {
	Field         string     `json:"field"`
	ProposedValue string     `json:"proposed_value"`
	CurrentValue  string     `json:"current_value"`
	EvidenceQuote string     `json:"evidence_quote"`
	Reason        string     `json:"reason"`
	Confidence    Confidence `json:"confidence"`
	Conflict      bool       `json:"conflict"`
}

// parseExtractionResponse turns a raw model response into a vetted
// PersonaExtractionResult. The transformation applies, in order:
//
//  1. JSON object extraction (tolerates code fences and surrounding
//     prose, mirroring operatoragent's loop-response parser).
//  2. Per-candidate validation:
//     - non-empty field, proposed_value, evidence_quote;
//     - evidence_quote is a substring of input.UserText after NFKC
//     normalization + whitespace collapse + lowercase;
//     - confidence passes the discard threshold.
//  3. Conflict re-evaluation: the parser sets Conflict=true whenever
//     a non-empty CurrentValue differs from ProposedValue, regardless
//     of what the LLM self-reported (the model's own conflict=true is
//     also preserved, but never silently demoted to false).
//  4. Source metadata population: SourceKind / SourceSessionID /
//     ObservedAt are filled from the input so the LLM cannot forge
//     them.
//
// Warnings are appended for each discarded candidate so reviewers can
// detect prompt or model drift without re-running the extractor.
func parseExtractionResponse(content string, input PersonaExtractionInput) (PersonaExtractionResult, error) {
	jsonPayload, err := extractJSONObject(content)
	if err != nil {
		return PersonaExtractionResult{}, fmt.Errorf("persona extractor: %w", err)
	}

	var raw rawExtractionPayload
	if err := json.Unmarshal([]byte(jsonPayload), &raw); err != nil {
		return PersonaExtractionResult{}, fmt.Errorf("persona extractor: decode response: %w", err)
	}

	normalizedUserText := normalizeForSubstring(input.UserText)

	result := PersonaExtractionResult{
		Warnings: append([]string(nil), raw.Warnings...),
	}
	for i, rc := range raw.Candidates {
		warning, ok := validateRawCandidate(i, rc, normalizedUserText)
		if !ok {
			if warning != "" {
				result.Warnings = append(result.Warnings, warning)
			}
			continue
		}

		conflict := rc.Conflict
		current := strings.TrimSpace(rc.CurrentValue)
		proposed := strings.TrimSpace(rc.ProposedValue)
		if current != "" && current != proposed {
			conflict = true
		}

		result.Candidates = append(result.Candidates, PersonaCandidate{
			Field:           strings.TrimSpace(rc.Field),
			ProposedValue:   proposed,
			CurrentValue:    current,
			EvidenceQuote:   strings.TrimSpace(rc.EvidenceQuote),
			Reason:          strings.TrimSpace(rc.Reason),
			Confidence:      rc.Confidence,
			Conflict:        conflict,
			SourceKind:      input.SourceKind,
			SourceSessionID: input.SourceSessionID,
			ObservedAt:      input.ObservedAt,
		})
	}

	return result, nil
}

// validateRawCandidate decides whether a single LLM-emitted candidate
// passes parser gates. Returns (warning, kept). A discarded candidate
// with no useful diagnostic gets an empty warning so callers can
// suppress it without losing genuine signal.
func validateRawCandidate(index int, rc rawCandidate, normalizedUserText string) (string, bool) {
	field := strings.TrimSpace(rc.Field)
	proposed := strings.TrimSpace(rc.ProposedValue)
	evidence := strings.TrimSpace(rc.EvidenceQuote)

	switch {
	case field == "":
		return fmt.Sprintf("candidate[%d]: empty field; discarded", index), false
	case proposed == "":
		return fmt.Sprintf("candidate[%d]: empty proposed_value; discarded", index), false
	case evidence == "":
		return fmt.Sprintf("candidate[%d] %q: missing evidence_quote; discarded", index, field), false
	case !rc.Confidence.IsKept():
		return fmt.Sprintf("candidate[%d] %q: confidence=%q discarded (need medium or high)", index, field, rc.Confidence), false
	}

	if normalizedUserText == "" {
		return fmt.Sprintf("candidate[%d] %q: user_text empty; cannot validate evidence_quote", index, field), false
	}
	normalizedEvidence := normalizeForSubstring(evidence)
	if normalizedEvidence == "" {
		return fmt.Sprintf("candidate[%d] %q: evidence_quote normalized to empty; discarded", index, field), false
	}
	if !strings.Contains(normalizedUserText, normalizedEvidence) {
		return fmt.Sprintf("candidate[%d] %q: evidence_quote not a substring of user_text; discarded (likely paraphrase)", index, field), false
	}
	return "", true
}

// normalizeForSubstring lower-cases, NFKC-normalizes, and collapses
// whitespace so substring comparison tolerates minor formatting
// variation between the user's message and the LLM's quote. Chinese
// half-/full-width punctuation is unified by NFKC; ASCII case is
// folded so "Beijing" matches "beijing". Semantic content cannot
// vary -- the LLM is expected to keep the same characters.
func normalizeForSubstring(s string) string {
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// extractJSONObject pulls the first balanced JSON object from a model
// response that may include surrounding code fences or prose. Mirrors
// the lenient behavior of operatoragent's loop-response parser so the
// extractor is tolerant of provider-specific output quirks.
func extractJSONObject(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", errors.New("empty response")
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(trimmed[start : end+1])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}
	return "", errors.New("no valid json object found")
}
