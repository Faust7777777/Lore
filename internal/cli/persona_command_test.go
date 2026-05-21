package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
)

func openRuntimeForPersonaCLITest(t *testing.T, workDir string) *app.Runtime {
	t.Helper()
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

// seedPersonaCandidateCLI creates a persona candidate against the
// given workdir and returns its ID. The runtime is closed before
// return so the CLI under test can open its own fresh handle to the
// sqlite store.
func seedPersonaCandidateCLI(t *testing.T, workDir, field, value, evidence string) string {
	t.Helper()
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("seed OpenRuntime() error = %v", err)
	}
	if _, err := runtime.Bootstrap(time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)); err != nil {
		runtime.Close()
		t.Fatalf("Bootstrap() error = %v", err)
	}
	c := persona.PersonaCandidate{
		Field:           field,
		ProposedValue:   value,
		EvidenceQuote:   evidence,
		Reason:          "test seed",
		Confidence:      persona.ConfidenceHigh,
		SourceKind:      persona.SourceConsole,
		SourceSessionID: "lore-cli-test",
		ObservedAt:      time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)
	rec := persona.PersonaCandidateRecord{
		ID:        persona.NewCandidateID(now),
		State:     persona.PersonaCandidateOpen,
		DedupKey:  persona.DedupKey(c),
		Candidate: c,
		CreatedAt: now,
		UpdatedAt: now,
	}
	stored, isNew, err := runtime.RecordPersonaCandidate(rec)
	if err != nil || !isNew {
		runtime.Close()
		t.Fatalf("RecordPersonaCandidate isNew=%v err=%v", isNew, err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime after seed: %v", err)
	}
	return stored.ID
}

func TestRunPersonaCandidatesList(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "list", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Persona Candidates", "State: open", id, "major", "economics", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunPersonaCandidatesListEmpty(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir) // pre-create state dir

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "list", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No candidates.") {
		t.Fatalf("output missing friendly empty message:\n%s", stdout.String())
	}
}

func TestRunPersonaCandidatesListStateFilter(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")
	// Dismiss the candidate via the CLI itself, then list by
	// dismissed and assert it shows up there but not in open.
	var stdout, stderr bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "dismiss", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test"); exit != 0 {
		t.Fatalf("dismiss exit = %d, stderr=%q", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--state", "dismissed"}, &bytes.Buffer{}, &stdout, &stderr, "test"); exit != 0 {
		t.Fatalf("list dismissed exit = %d, stderr=%q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), id) || !strings.Contains(stdout.String(), "State: dismissed") {
		t.Fatalf("dismissed list missing seeded id; got:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--state", "open"}, &bytes.Buffer{}, &stdout, &stderr, "test"); exit != 0 {
		t.Fatalf("list open exit = %d, stderr=%q", exit, stderr.String())
	}
	if strings.Contains(stdout.String(), id) {
		t.Fatalf("open list should NOT contain dismissed id; got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "No candidates.") {
		t.Fatalf("open list missing empty marker; got:\n%s", stdout.String())
	}
}

func TestRunPersonaCandidatesListRejectsInvalidState(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "list", "--workdir", workDir, "--state", "bogus"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for invalid state; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "must be one of") {
		t.Fatalf("stderr missing validation message: %q", stderr.String())
	}
}

func TestRunPersonaCandidatesShow(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "show", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, want := range []string{
		id,
		"Field:          major",
		"Proposed value: economics",
		"Evidence:       I major in economics",
		"Confidence:     high",
		"Source:         console",
		"Dedup key:",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunPersonaCandidatesShowMissingIDFails(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "show", "--workdir", workDir}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for missing id; stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "persona candidate id is required") {
		t.Fatalf("stderr missing required-id message: %q", stderr.String())
	}
}

func TestRunPersonaCandidatesDismissTransitionsState(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "dismiss", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Action: dismiss") || !strings.Contains(stdout.String(), "State:  dismissed") {
		t.Fatalf("output missing dismissed transition:\n%s", stdout.String())
	}

	runtime := openRuntimeForPersonaCLITest(t, workDir)
	got, err := runtime.GetPersonaCandidate(id)
	if err != nil {
		t.Fatalf("post-dismiss GetPersonaCandidate() error = %v", err)
	}
	if got.State != persona.PersonaCandidateDismissed {
		t.Fatalf("state = %q, want dismissed", got.State)
	}
}

func TestRunPersonaCandidatesDraftCreatesPersonaUpdateDraft(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, id}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Action: draft", "State:  drafted", "Draft:", "pending_review"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	// Reopen the runtime and confirm:
	//   - candidate state == drafted;
	//   - a persona_update draft exists in pending_review with the
	//     candidate evidence preserved in the summary.
	runtime := openRuntimeForPersonaCLITest(t, workDir)
	candidate, err := runtime.GetPersonaCandidate(id)
	if err != nil {
		t.Fatalf("GetPersonaCandidate() error = %v", err)
	}
	if candidate.State != persona.PersonaCandidateDrafted {
		t.Fatalf("candidate state = %q, want drafted", candidate.State)
	}

	drafts, err := runtime.Store.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	var personaDraft *model.Draft
	for i := range drafts {
		if drafts[i].Kind == model.DraftKindPersonaUpdate {
			personaDraft = &drafts[i]
			break
		}
	}
	if personaDraft == nil {
		t.Fatalf("no persona_update draft found; drafts = %+v", drafts)
	}
	if personaDraft.State != model.DraftPendingReview {
		t.Fatalf("draft state = %q, want pending_review (CLI must not approve/apply)", personaDraft.State)
	}
	if !strings.Contains(personaDraft.Summary, "I major in economics") {
		t.Fatalf("draft summary missing evidence:\n%s", personaDraft.Summary)
	}
	if !strings.Contains(personaDraft.Summary, "confidence: high") {
		t.Fatalf("draft summary missing confidence:\n%s", personaDraft.Summary)
	}
}

func TestRunPersonaCandidatesDraftIsIdempotentOnRetry(t *testing.T) {
	// CLI re-invocation must NOT create a duplicate persona_update
	// draft. The second call returns the same DraftID and the store
	// still has exactly one persona_update draft.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	id := seedPersonaCandidateCLI(t, workDir, "major", "economics", "I major in economics")

	var firstStdout bytes.Buffer
	if exit := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, id}, &bytes.Buffer{}, &firstStdout, &bytes.Buffer{}, "test"); exit != 0 {
		t.Fatalf("first draft exit = %d", exit)
	}
	firstDraftID := extractDraftIDFromOutput(t, firstStdout.String())

	var secondStdout, secondStderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "draft", "--workdir", workDir, id}, &bytes.Buffer{}, &secondStdout, &secondStderr, "test")
	if exitCode != 0 {
		t.Fatalf("second draft exit = %d (want idempotent success); stderr=%q", exitCode, secondStderr.String())
	}
	secondDraftID := extractDraftIDFromOutput(t, secondStdout.String())
	if secondDraftID != firstDraftID {
		t.Fatalf("second DraftID = %q, want same as first %q (idempotent retry must reuse draft)", secondDraftID, firstDraftID)
	}

	// Underlying store has exactly one persona_update draft.
	runtime := openRuntimeForPersonaCLITest(t, workDir)
	drafts, err := runtime.Store.Drafts().ListDrafts()
	if err != nil {
		t.Fatalf("ListDrafts() error = %v", err)
	}
	count := 0
	for _, d := range drafts {
		if d.Kind == model.DraftKindPersonaUpdate {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("persona_update drafts in store = %d, want 1", count)
	}
}

// extractDraftIDFromOutput pulls the "Draft:  <id> ..." line out of
// the renderPersonaCandidateActionResult output so tests can compare
// DraftIDs across CLI invocations without parsing the full record.
func extractDraftIDFromOutput(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Draft:") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "Draft:"))
		if rest == "" {
			continue
		}
		// "Draft:  draft-123 (pending_review; use ...)" -> "draft-123"
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		return fields[0]
	}
	t.Fatalf("output missing Draft: line:\n%s", output)
	return ""
}

func TestRunPersonaCandidatesShowReturnsNotFoundForMissingID(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	openRuntimeForPersonaCLITest(t, workDir)

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona", "candidates", "show", "--workdir", workDir, "nonexistent"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for missing id; stdout=%q", stdout.String())
	}
	// The store-level error propagates via the CLI error message.
	if !strings.Contains(stderr.String(), "not found") && !errors.Is(errors.New(stderr.String()), store.ErrNotFound) {
		t.Fatalf("stderr missing not-found context: %q", stderr.String())
	}
}

func TestRunPersonaWithoutSubcommandShowsUsage(t *testing.T) {
	configtest.IsolateHome(t)
	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"persona"}, &bytes.Buffer{}, &stdout, &stderr, "test")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for no subcommand")
	}
	if !strings.Contains(stderr.String(), "lore persona candidates") {
		t.Fatalf("stderr missing usage hint: %q", stderr.String())
	}
}
