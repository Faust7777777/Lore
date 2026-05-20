package persona

import (
	"fmt"
	"strings"
)

// systemPrompt is the static instruction block sent to the LLM. The
// rules are reinforced both as "Extract ONLY" allowlist and
// "NEVER extract" blocklist so the model has two angles to fall back
// on if one is misinterpreted. The schema is mirrored verbatim in the
// parser; keep them in sync when changing field names.
const systemPrompt = `You extract candidate persona facts from a Lore user's message.
Return exactly one JSON object and nothing else.

Schema:
{
  "candidates": [
    {
      "field": "string identifying the persona field (e.g. major, role, location, goal, weakness)",
      "proposed_value": "the new value the user implies for that field",
      "current_value": "the conflicting current value if any, otherwise empty",
      "evidence_quote": "a VERBATIM substring of the user's message that supports this candidate",
      "reason": "short explanation of why this fact is durable",
      "confidence": "low" | "medium" | "high",
      "conflict": true | false
    }
  ],
  "warnings": ["optional human-readable warnings about ambiguous input"]
}

Extract ONLY:
- stable profile facts (background, education, work, location, role)
- long-term goals and aspirations
- durable preferences (working hours, communication style, tool choices)
- repeated constraints, weaknesses, or blockers
- explicit corrections to existing persona facts ("actually I'm not X anymore, I'm Y")

NEVER extract:
- transient moods or feelings ("I'm tired today")
- one-shot task context ("help me debug this specific function")
- assistant inferences -- only the user's own words count
- inferred facts the user did NOT explicitly state
- candidates without an exact substring from the user's message as evidence_quote

Rules:
- evidence_quote MUST be a verbatim substring of the user message.
  Do not paraphrase, translate, or summarize. Whitespace and case
  may vary; semantic content may not.
- If the user mentions something already matching current persona,
  do NOT emit a candidate.
- If you detect a contradiction with current persona, set conflict=true
  and put the conflicting current fact into current_value.
- If unsure, emit confidence: "low"; low-confidence candidates will be
  discarded downstream.
- If no candidates qualify, return {"candidates": [], "warnings": []}.
- Do not extract from the assistant context; it is only there to help
  you understand short user replies.`

// buildExtractionPrompt returns the (system, user) message pair sent
// to the chat completion endpoint. It deliberately structures the
// user-side message into clearly-labeled sections so the parser can
// later (re)verify that evidence quotes refer to the User message
// section, not to any other context. The PreviousAssistant section
// is included only when AssistantContext is non-empty.
func buildExtractionPrompt(input PersonaExtractionInput) (string, string) {
	var b strings.Builder

	if persona := strings.TrimSpace(input.CurrentPersonaExcerpt); persona != "" {
		b.WriteString("Current persona excerpt:\n")
		b.WriteString(persona)
		b.WriteString("\n\n")
	}
	if rules := strings.TrimSpace(input.SystemRulesExcerpt); rules != "" {
		b.WriteString("System rules excerpt:\n")
		b.WriteString(rules)
		b.WriteString("\n\n")
	}
	if ctx := strings.TrimSpace(input.AssistantContext); ctx != "" {
		b.WriteString("Previous assistant message (context only; do NOT extract from this):\n")
		b.WriteString(ctx)
		b.WriteString("\n\n")
	}
	b.WriteString("User message (the ONLY source for candidates and evidence_quote):\n")
	b.WriteString(strings.TrimSpace(input.UserText))

	if strings.TrimSpace(input.SourceAgentID) != "" || input.SourceKind != "" {
		fmt.Fprintf(&b,
			"\n\n[Metadata for your awareness only; do not place into candidate fields]\nsource_kind: %s\nsource_agent_id: %s",
			input.SourceKind, input.SourceAgentID,
		)
	}

	return systemPrompt, b.String()
}
