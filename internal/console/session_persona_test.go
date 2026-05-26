package console

import (
	"bytes"
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

func TestSessionHandleExtractionFailureWritesLoggerLine(t *testing.T) {
	// B-P9 contract: a failing extractor must leave one structured
	// line on PersonaExtractLogger so operators can `tail` the workdir
	// log to diagnose "why are there zero candidates after a chat
	// session". The user turn itself still succeeds; the swallow
	// behavior of TestSessionHandleExtractionFailureDoesNotFailUserTurn
	// is preserved -- only the silent part changes.
	session, _, runtime := newLoopSession(t)
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	session.PersonaExtractor = &scriptedPersonaExtractor{
		err: errors.New("upstream provider unavailable"),
	}

	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}

	logLine := buf.String()
	if logLine == "" {
		t.Fatalf("PersonaExtractLogger empty; want one failure line")
	}
	for _, want := range []string{"stage=extract", "upstream provider unavailable"} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log line missing %q:\n%s", want, logLine)
		}
	}
	if got := strings.Count(strings.TrimSuffix(logLine, "\n"), "\n"); got != 0 {
		t.Fatalf("log lines count = %d (want exactly one trailing-newline-terminated record), raw=%q", got+1, logLine)
	}
}

func TestSessionHandleStoreFailureWritesLoggerLine(t *testing.T) {
	// B-P9 contract: store-side failures (sqlite error, dedup race,
	// etc.) also surface to the persona log. Distinct stage label so
	// downstream readers / awk filters can separate LLM mining bugs
	// from persistence bugs.
	session, _, runtime := newLoopSession(t)
	runtime.personaErr = errors.New("disk full")
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{
			Candidates: []persona.PersonaCandidate{newPersonaCandidate("major", "history", "I study history")},
		},
	}

	if _, err := session.Handle("I study history", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}

	logLine := buf.String()
	for _, want := range []string{"stage=store", "disk full"} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log line missing %q:\n%s", want, logLine)
		}
	}
}

func TestSessionHandleExtractionSuccessLeavesLoggerSilent(t *testing.T) {
	// Success path must never write to the log. Real-world workdirs
	// run for weeks; if every successful extraction left a line the
	// file would grow without bound and dilute the failures operators
	// are actually looking for when they tail it.
	session, _, runtime := newLoopSession(t)
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{
			Candidates: []persona.PersonaCandidate{newPersonaCandidate("major", "history", "I study history")},
		},
	}

	if _, err := session.Handle("I study history", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}
	if got := buf.String(); got != "" {
		t.Fatalf("logger received output on success path:\n%s", got)
	}
}

func TestSessionHandleNilLoggerSurvivesFailure(t *testing.T) {
	// PersonaExtractLogger nil must NOT panic the goroutine. The
	// legacy P4 wiring path -- tests that do not bother with
	// observability, and embedders that have not wired a log file --
	// must still tolerate extractor failures gracefully.
	session, _, runtime := newLoopSession(t)
	// Intentionally do not set PersonaExtractLogger.
	session.PersonaExtractor = &scriptedPersonaExtractor{
		err: errors.New("nil-logger probe"),
	}

	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v, want extractor failure to be swallowed", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out (panic in goroutine?)")
	}
}

func TestSessionHandleParserWarningsLoggedWhenZeroCandidates(t *testing.T) {
	// Reviewer-flagged blocker: parser discards (paraphrased
	// evidence_quote, low confidence, empty evidence) leave
	// result.Candidates empty with err==nil and warnings in
	// result.Warnings. The fire-and-forget loop must surface these
	// as stage=parse_warning lines so the operator's tail of the
	// log explains "why are there zero candidates after this chat".
	session, _, runtime := newLoopSession(t)
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{
			Warnings: []string{
				"candidate[0]: evidence_quote not a substring of user_text",
				"candidate[1]: confidence=low discarded",
			},
		},
	}

	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}

	logOut := buf.String()
	if got := strings.Count(logOut, "stage=parse_warning"); got != 2 {
		t.Fatalf("stage=parse_warning count = %d, want 2; log:\n%s", got, logOut)
	}
	for _, want := range []string{
		"evidence_quote not a substring of user_text",
		"confidence=low discarded",
	} {
		if !strings.Contains(logOut, want) {
			t.Fatalf("log missing warning %q:\n%s", want, logOut)
		}
	}
	// Confirm no store-side fallout: zero candidates means no
	// RecordPersonaCandidate calls.
	if got := runtime.recordedPersonaCandidates(); len(got) != 0 {
		t.Fatalf("candidates recorded despite parser discards: %+v", got)
	}
}

func TestSessionHandleParserWarningsSuppressedOnSuccess(t *testing.T) {
	// When parser warnings co-exist with at least one kept
	// candidate, the warnings are informational (some siblings were
	// discarded but a useful one landed). Logging them on success
	// would flood the log file in real use because the parser emits
	// a warning for every individually-discarded candidate inside a
	// multi-candidate response. The contract is: log warnings only
	// when zero candidates landed, so the diagnostic value is high
	// and the noise is bounded.
	session, _, runtime := newLoopSession(t)
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{
			Candidates: []persona.PersonaCandidate{newPersonaCandidate("major", "history", "I study history")},
			Warnings:   []string{"candidate[1]: low confidence sibling discarded"},
		},
	}

	if _, err := session.Handle("I study history", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}

	if got := buf.String(); strings.Contains(got, "parse_warning") {
		t.Fatalf("logger should be silent on success path even with sibling warnings:\n%s", got)
	}
	// The kept candidate still landed in the store.
	if got := runtime.recordedPersonaCandidates(); len(got) != 1 {
		t.Fatalf("candidates recorded = %d, want 1 (kept candidate must still land)", len(got))
	}
}

func TestSessionHandleExtractionFailureStampsModelTagColumns(t *testing.T) {
	// B-line model-consistency contract: when Session.PersonaExtractModelInfo
	// is populated (runtime wires it from the resolved persona-extract
	// LLM profile), failure lines must carry the model + base_url
	// columns so operators tailing the workdir log can correlate a
	// failure with the specific upstream model that produced it.
	// Tests on the reader side (cli/persona_errors_test.go) round-trip
	// the same line back into a personaLogEntry; this test pins the
	// writer half of that contract.
	session, _, runtime := newLoopSession(t)
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	session.PersonaExtractModelInfo = PersonaExtractModelInfo{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		BaseURL:  "https://api.deepseek.com/v1",
	}
	session.PersonaExtractor = &scriptedPersonaExtractor{
		err: errors.New("upstream provider unavailable"),
	}

	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}

	line := strings.TrimSuffix(buf.String(), "\n")
	if line == "" {
		t.Fatal("PersonaExtractLogger empty; want one tagged failure line")
	}
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("expected a single log record, got multiple:\n%s", line)
	}
	for _, want := range []string{
		"stage=extract",
		"upstream provider unavailable",
		"\tmodel=deepseek-chat",
		"\tbase_url=\"https://api.deepseek.com/v1\"",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line missing %q:\n%s", want, line)
		}
	}
	// Provider is informational only; the writer deliberately omits it
	// from the TSV columns so the on-disk surface stays minimal.
	if strings.Contains(line, "provider=") {
		t.Fatalf("log line should not carry provider= column (informational only):\n%s", line)
	}
	// Column ordering matters for downstream awk-style parsers: the
	// legacy four columns must precede any model-tag columns so older
	// readers that ignore fields[4:] keep working.
	fields := strings.Split(line, "\t")
	if len(fields) < 6 {
		t.Fatalf("expected >=6 tab fields (ts, stage, session, error, model, base_url); got %d in %q", len(fields), line)
	}
	if !strings.HasPrefix(fields[1], "stage=") || !strings.HasPrefix(fields[2], "session=") || !strings.HasPrefix(fields[3], "error=") {
		t.Fatalf("legacy four-column ordering broken: %v", fields[:4])
	}
}

func TestSessionHandleExtractionFailureOmitsModelTagWhenInfoEmpty(t *testing.T) {
	// Backward-compat: when PersonaExtractModelInfo is the zero value
	// (no LLM resolved, or runtime constructed before the model-tag
	// wiring), the writer must keep emitting the legacy four-column
	// shape so pre-B-line tooling that did not know about the optional
	// columns continues to behave identically.
	session, _, runtime := newLoopSession(t)
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	// PersonaExtractModelInfo intentionally left zero.
	session.PersonaExtractor = &scriptedPersonaExtractor{
		err: errors.New("zero-info probe"),
	}

	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out")
	}

	line := strings.TrimSuffix(buf.String(), "\n")
	if line == "" {
		t.Fatal("PersonaExtractLogger empty")
	}
	for _, banned := range []string{"\tmodel=", "\tbase_url="} {
		if strings.Contains(line, banned) {
			t.Fatalf("zero ModelInfo should not emit %q column:\n%s", banned, line)
		}
	}
	if got := strings.Count(line, "\t"); got != 3 {
		t.Fatalf("legacy line should have exactly 3 tabs (4 columns); got %d in %q", got, line)
	}
}

func TestSessionHandleExtractionTimeoutWritesLoggerLine(t *testing.T) {
	// Slow-LLM path: extractor blocks past PersonaExtractTimeout,
	// ctx.Done fires, the scripted extractor returns ctx.Err(), and
	// the log records a "context deadline exceeded" or "canceled"
	// signal under stage=extract. This is the failure shape an
	// operator hits when LORE_LLM_BASE_URL points at a slow / hung
	// provider and is the most likely real-world reason to grep the
	// log file.
	session, _, runtime := newLoopSession(t)
	var buf bytes.Buffer
	session.PersonaExtractLogger = &buf
	session.PersonaExtractTimeout = 25 * time.Millisecond
	gate := make(chan struct{}) // intentionally never closed
	session.PersonaExtractor = &scriptedPersonaExtractor{
		result: persona.PersonaExtractionResult{},
		start:  gate,
	}

	if _, err := session.Handle("anything", runtime); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	// Drain budget comfortably exceeds the extractor timeout so the
	// goroutine has time to exit via ctx.Done and write the line.
	if !session.DrainPersonaExtractions(time.Second) {
		t.Fatal("DrainPersonaExtractions timed out; goroutine did not exit on ctx deadline")
	}

	logLine := buf.String()
	if logLine == "" {
		t.Fatal("PersonaExtractLogger empty after timeout; want one stage=extract line")
	}
	if !strings.Contains(logLine, "stage=extract") {
		t.Fatalf("log line missing stage=extract:\n%s", logLine)
	}
	if !strings.Contains(logLine, "deadline") && !strings.Contains(logLine, "context") && !strings.Contains(logLine, "canceled") {
		t.Fatalf("log line should mention context/deadline/canceled:\n%s", logLine)
	}
}
