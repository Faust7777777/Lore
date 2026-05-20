package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
	"obsidian-harness/internal/persona"
)

// llmTestServer stands in for an OpenAI-compatible chat endpoint
// during CLI plumbing tests. It dispatches between the operator-agent
// path (returns a benign final response) and the persona extractor
// path (returns one candidate). We discriminate by the system prompt
// because they're served by the same /chat/completions endpoint.
func llmTestServer(t *testing.T, userQuote string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "gpt-4o"}},
			})
		case "/chat/completions":
			var req struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			if len(req.Messages) == 0 {
				t.Fatal("chat request missing messages")
			}
			systemPrompt := req.Messages[0].Content
			content := `{"type":"final","message":"noted"}`
			if strings.Contains(systemPrompt, "extract candidate persona facts") {
				// Persona extractor path: return one candidate
				// whose evidence_quote is a substring of the user's
				// turn so parser substring validation passes.
				candidate := map[string]any{
					"field":          "major",
					"proposed_value": "economics",
					"evidence_quote": userQuote,
					"confidence":     "high",
					"conflict":       false,
				}
				payload, err := json.Marshal(map[string]any{
					"candidates": []any{candidate},
					"warnings":   []any{},
				})
				if err != nil {
					t.Fatalf("marshal persona payload: %v", err)
				}
				content = string(payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": content}},
				},
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 5,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// TestRunConsoleOnceWiresPersonaExtractionEndToEnd locks the P4
// production-path wiring blocker reviewer raised: OpenRuntime
// constructs a PersonaExtractor, but unless RunConsoleCommand
// assigns it onto Session and drains before closeRuntime, the
// fire-and-forget extraction never reaches the store in a real
// console invocation. This test drives the actual RunConsoleCommand
// against a fake LLM endpoint and asserts a persona candidate
// landed in the sqlite store after the command returns.
func TestRunConsoleOnceWiresPersonaExtractionEndToEnd(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	utterance := "Hi, I major in economics."
	server := llmTestServer(t, utterance)
	defer server.Close()
	t.Setenv("LORE_LLM_BASE_URL", server.URL)
	t.Setenv("LORE_LLM_API_KEY", "secret")
	t.Setenv("LORE_LLM_MODEL", "gpt-4o")

	var stdin, stdout, stderr bytes.Buffer
	exitCode := RunConsoleCommand([]string{
		"--workdir", workDir,
		"--once", utterance,
	}, &stdin, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("RunConsoleCommand() exit = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "noted") {
		t.Fatalf("stdout missing operator agent reply: %q", stdout.String())
	}

	// Re-open runtime to inspect the persisted store. The CLI's own
	// runtime closed when RunConsoleCommand returned, so sqlite is
	// unlocked.
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime(reopen) error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	candidates, err := runtime.Store.PersonaCandidates().ListCandidatesByState(persona.PersonaCandidateOpen, 0)
	if err != nil {
		t.Fatalf("ListCandidatesByState() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("persona candidates after CLI run = %d, want 1; got %+v", len(candidates), candidates)
	}
	got := candidates[0]
	if got.Candidate.ProposedValue != "economics" {
		t.Fatalf("candidate proposed_value = %q, want economics", got.Candidate.ProposedValue)
	}
	if got.Candidate.SourceKind != persona.SourceConsole {
		t.Fatalf("candidate source kind = %q, want console", got.Candidate.SourceKind)
	}
	if got.State != persona.PersonaCandidateOpen {
		t.Fatalf("candidate state = %q, want open", got.State)
	}
}

// TestRunTUIOnceWiresPersonaExtractionEndToEnd is the mirror of the
// console test for the TUI one-shot path. Both shells share the
// extractor-wiring + drain-defer fix; one test per shell guards
// against regressions in either path.
func TestRunTUIOnceWiresPersonaExtractionEndToEnd(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	utterance := "Hi, I major in economics."
	server := llmTestServer(t, utterance)
	defer server.Close()
	t.Setenv("LORE_LLM_BASE_URL", server.URL)
	t.Setenv("LORE_LLM_API_KEY", "secret")
	t.Setenv("LORE_LLM_MODEL", "gpt-4o")

	var stdin, stdout, stderr bytes.Buffer
	exitCode := RunTUICommand([]string{
		"--workdir", workDir,
		"--once", utterance,
	}, &stdin, &stdout, &stderr, "test")
	if exitCode != 0 {
		t.Fatalf("RunTUICommand() exit = %d, stderr=%q", exitCode, stderr.String())
	}

	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime(reopen) error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	candidates, err := runtime.Store.PersonaCandidates().ListCandidatesByState(persona.PersonaCandidateOpen, 0)
	if err != nil {
		t.Fatalf("ListCandidatesByState() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("persona candidates after TUI run = %d, want 1; got %+v", len(candidates), candidates)
	}
	if candidates[0].Candidate.ProposedValue != "economics" {
		t.Fatalf("candidate proposed_value = %q, want economics", candidates[0].Candidate.ProposedValue)
	}
}
