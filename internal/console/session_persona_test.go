package console

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/persona"
)

// scriptedPersonaExtractor is a deterministic in-memory extractor for
// P4 tests. It records every Extract input it sees so test assertions
// can check what the console session actually fed in (e.g.
// AssistantContext, SourceSessionID), and returns the pre-staged
// result / error. start gates the goroutine so tests can verify
// fire-and-forget semantics: Handle returns before Extract resolves.
type scriptedPersonaExtractor struct {
	result persona.PersonaExtractionResult
	err    error
	start  chan struct{}

	mu     sync.Mutex
	inputs []persona.PersonaExtractionInput
}

func (s *scriptedPersonaExtractor) Extract(ctx context.Context, input persona.PersonaExtractionInput) (persona.PersonaExtractionResult, error) {
	s.mu.Lock()
	s.inputs = append(s.inputs, input)
	s.mu.Unlock()
	if s.start != nil {
		select {
		case <-s.start:
		case <-ctx.Done():
			return persona.PersonaExtractionResult{}, ctx.Err()
		}
	}
	return s.result, s.err
}

func (s *scriptedPersonaExtractor) capturedInputs() []persona.PersonaExtractionInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]persona.PersonaExtractionInput(nil), s.inputs...)
}

func newPersonaCandidate(field, value, evidence string) persona.PersonaCandidate {
	return persona.PersonaCandidate{
		Field:         field,
		ProposedValue: value,
		EvidenceQuote: evidence,
		Confidence:    persona.ConfidenceHigh,
	}
}

func newLoopSession(t *testing.T) (*Session, *fakeLoopAgent, *fakeRuntime) {
	t.Helper()
	agent := &fakeLoopAgent{response: operatoragent.Response{Final: "ok\n", StopReason: operatoragent.TurnStopFinal, StepCount: 1}}
	session := NewSessionWithAgent("test", agent)
	session.DefaultAgentID = "codex"
	session.Now = func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) }
	runtime := &fakeRuntime{managed: model.ManagedStatusView{Ready: true, WorkDir: "w", VaultRoot: "v"}}
	return session, agent, runtime
}

func TestSessionHandleFiresPersonaExtractionAndDoesNotBlockUserTurn(t *testing.T) {
	session, _, runtime := newLoopSession(t)
	gate := make(chan struct{})
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{
			Candidates: []persona.PersonaCandidate{
				newPersonaCandidate("major", "economics", "I study economics"),
			},
		},
		start: gate,
	}

	startedAt := time.Now()
	out, err := session.Handle("I study economics", runtime)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("output = %q, want ok", out)
	}
	// Handle must NOT have waited on the extractor; if it had, this
	// assertion would fail because the gate is still closed-and-blocked.
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("Handle blocked on persona extractor for %v; fire-and-forget contract broken", elapsed)
	}
	// Store has not received the candidate yet -- the goroutine is
	// parked at the gate.
	if got := runtime.recordedPersonaCandidates(); len(got) != 0 {
		t.Fatalf("candidate landed before extractor unblocked: %+v", got)
	}

	// Release the gate so the goroutine finishes, then drain and
	// verify the candidate did land.
	close(gate)
	if !session.DrainPersonaExtractions(2 * time.Second) {
		t.Fatal("DrainPersonaExtractions timed out; goroutine never finished")
	}
	got := runtime.recordedPersonaCandidates()
	if len(got) != 1 {
		t.Fatalf("persona candidates after drain = %d, want 1", len(got))
	}
	if got[0].Candidate.ProposedValue != "economics" {
		t.Fatalf("candidate stored = %+v", got[0])
	}
	if got[0].State != persona.PersonaCandidateOpen {
		t.Fatalf("candidate state = %q, want open", got[0].State)
	}
	if got[0].DedupKey == "" {
		t.Fatalf("candidate DedupKey empty")
	}
}

func TestSessionHandleSkipsPersonaExtractionWhenExtractorNil(t *testing.T) {
	session, _, runtime := newLoopSession(t)
	// PersonaExtractor stays nil. No goroutine should spawn; Drain
	// returns immediately because the WaitGroup counter is zero.
	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(50 * time.Millisecond) {
		t.Fatal("DrainPersonaExtractions timed out for nil extractor")
	}
	if got := runtime.recordedPersonaCandidates(); len(got) != 0 {
		t.Fatalf("candidates recorded with nil extractor: %+v", got)
	}
}

func TestSessionHandleSkipsPersonaExtractionForLegacyDecidePath(t *testing.T) {
	// fakeAgent satisfies Agent but NOT LoopAgent. Handle takes the
	// legacy Decide branch which must NOT trigger extraction (the P4
	// trigger lives only inside the LoopAgent block).
	agent := &fakeAgent{decisions: []operatoragent.Decision{{Action: operatoragent.ActionShowStatus}}}
	session := NewSessionWithAgent("test", agent)
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{Candidates: []persona.PersonaCandidate{newPersonaCandidate("x", "y", "y")}},
	}
	session.Now = func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) }

	runtime := &fakeRuntime{managed: model.ManagedStatusView{Ready: true, WorkDir: "w", VaultRoot: "v"}}
	if _, err := session.Handle("show status", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out unexpectedly")
	}
	if got := runtime.recordedPersonaCandidates(); len(got) != 0 {
		t.Fatalf("legacy decide path recorded persona candidates: %+v", got)
	}
}

func TestSessionHandleExtractionFailureDoesNotFailUserTurn(t *testing.T) {
	session, _, runtime := newLoopSession(t)
	session.PersonaExtractor = &scriptedPersonaExtractor{
		err: errors.New("upstream provider unavailable"),
	}

	out, err := session.Handle("anything", runtime)
	if err != nil {
		t.Fatalf("Handle() error = %v, want extractor failure to be swallowed", err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("output = %q, want ok", out)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}
	if got := runtime.recordedPersonaCandidates(); len(got) != 0 {
		t.Fatalf("candidates recorded despite extractor failure: %+v", got)
	}
}

func TestSessionHandleExtractionStoreFailureIsSilentlySwallowed(t *testing.T) {
	session, _, runtime := newLoopSession(t)
	runtime.personaErr = errors.New("disk full")
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{
			Candidates: []persona.PersonaCandidate{newPersonaCandidate("major", "history", "I study history")},
		},
	}

	out, err := session.Handle("I study history", runtime)
	if err != nil {
		t.Fatalf("Handle() error = %v, want store failure to be swallowed", err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("output = %q, want ok", out)
	}
	// Drain succeeds; the goroutine ran to completion despite the
	// store failure.
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out after store failure")
	}
}

func TestSessionHandlePassesPriorAssistantContextToExtractor(t *testing.T) {
	session, agent, runtime := newLoopSession(t)
	// Two scripted responses so Handle can run two turns.
	agent.responses = []operatoragent.Response{
		{Final: "your major?\n", StopReason: operatoragent.TurnStopFinal, StepCount: 1},
		{Final: "noted\n", StopReason: operatoragent.TurnStopFinal, StepCount: 1},
	}
	extractor := &scriptedPersonaExtractor{}
	session.PersonaExtractor = extractor

	if _, err := session.Handle("hi", runtime); err != nil {
		t.Fatalf("Handle(turn 1) error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("drain turn 1 timed out")
	}
	if _, err := session.Handle("economics", runtime); err != nil {
		t.Fatalf("Handle(turn 2) error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("drain turn 2 timed out")
	}

	inputs := extractor.capturedInputs()
	if len(inputs) != 2 {
		t.Fatalf("extractor inputs = %d, want 2", len(inputs))
	}
	if inputs[0].UserText != "hi" || inputs[0].AssistantContext != "" {
		t.Fatalf("turn 1 input = %+v, want UserText=hi AssistantContext=empty", inputs[0])
	}
	if inputs[1].UserText != "economics" {
		t.Fatalf("turn 2 input UserText = %q, want economics", inputs[1].UserText)
	}
	if !strings.Contains(inputs[1].AssistantContext, "your major") {
		t.Fatalf("turn 2 AssistantContext = %q, want prior assistant (your major?...)", inputs[1].AssistantContext)
	}
	if inputs[1].SourceKind != persona.SourceConsole {
		t.Fatalf("source kind = %q, want console", inputs[1].SourceKind)
	}
	if inputs[1].SourceAgentID != "codex" {
		t.Fatalf("source agent id = %q, want codex", inputs[1].SourceAgentID)
	}
}

func TestSessionHandleSkipsExtractionOnEmptyFinal(t *testing.T) {
	// Empty final triggers the "operator agent returned an empty
	// response" error path. Even though rememberTurn has already run,
	// no extraction should be launched.
	agent := &fakeLoopAgent{
		response: operatoragent.Response{Final: "", StopReason: operatoragent.TurnStopFinal, StepCount: 1},
	}
	session := NewSessionWithAgent("test", agent)
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{Candidates: []persona.PersonaCandidate{newPersonaCandidate("x", "y", "y")}},
	}
	session.Now = func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) }

	runtime := &fakeRuntime{}
	if _, err := session.Handle("hi", runtime); err == nil {
		t.Fatal("Handle() error = nil, want empty-response error")
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("drain timed out")
	}
	if got := runtime.recordedPersonaCandidates(); len(got) != 0 {
		t.Fatalf("empty final path recorded persona candidates: %+v", got)
	}
}

func TestSessionHandlePassesCoreContextExcerptsToExtractor(t *testing.T) {
	session, _, runtime := newLoopSession(t)
	runtime.coreContext = model.CoreContext{
		PersonaSummary:     "Persona: PM, e-commerce major",
		WeaknessSummary:    "Weakness: timeline planning is shaky",
		SystemRulesSummary: "Rules: drafts must be reviewed before apply",
	}
	extractor := &scriptedPersonaExtractor{}
	session.PersonaExtractor = extractor

	if _, err := session.Handle("any input", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("drain timed out")
	}

	inputs := extractor.capturedInputs()
	if len(inputs) != 1 {
		t.Fatalf("extractor inputs = %d, want 1", len(inputs))
	}
	got := inputs[0]
	if !strings.Contains(got.CurrentPersonaExcerpt, "Persona: PM") {
		t.Fatalf("CurrentPersonaExcerpt missing PersonaSummary: %q", got.CurrentPersonaExcerpt)
	}
	if !strings.Contains(got.CurrentPersonaExcerpt, "Weakness: timeline planning is shaky") {
		t.Fatalf("CurrentPersonaExcerpt missing WeaknessSummary: %q", got.CurrentPersonaExcerpt)
	}
	if got.SystemRulesExcerpt != "Rules: drafts must be reviewed before apply" {
		t.Fatalf("SystemRulesExcerpt = %q, want trimmed SystemRulesSummary", got.SystemRulesExcerpt)
	}
}

func TestSessionHandlePassesEmptyExcerptWhenCoreContextEmpty(t *testing.T) {
	// runtime.coreContext zero-value: PersonaSummary, WeaknessSummary,
	// SystemRulesSummary are all empty. The extractor must receive
	// empty strings (not garbage / formatting artifacts) so the
	// prompt's persona/system sections are simply omitted.
	session, _, runtime := newLoopSession(t)
	extractor := &scriptedPersonaExtractor{}
	session.PersonaExtractor = extractor

	if _, err := session.Handle("any input", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("drain timed out")
	}
	inputs := extractor.capturedInputs()
	if len(inputs) != 1 {
		t.Fatalf("extractor inputs = %d, want 1", len(inputs))
	}
	if inputs[0].CurrentPersonaExcerpt != "" {
		t.Fatalf("CurrentPersonaExcerpt = %q, want empty for zero-value CoreContext", inputs[0].CurrentPersonaExcerpt)
	}
	if inputs[0].SystemRulesExcerpt != "" {
		t.Fatalf("SystemRulesExcerpt = %q, want empty for zero-value CoreContext", inputs[0].SystemRulesExcerpt)
	}
}

func TestSessionDrainPersonaExtractionsTimesOutOnSlowExtractor(t *testing.T) {
	session, _, runtime := newLoopSession(t)
	gate := make(chan struct{})
	defer close(gate)
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{Candidates: []persona.PersonaCandidate{newPersonaCandidate("x", "y", "y")}},
		start:  gate,
	}

	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	// Goroutine is parked on the gate. A 50ms drain must time out.
	if drained := session.DrainPersonaExtractions(50 * time.Millisecond); drained {
		t.Fatal("DrainPersonaExtractions returned true while goroutine still parked at gate")
	}
}
