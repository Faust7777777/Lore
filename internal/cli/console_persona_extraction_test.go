package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

// llmTestServerFailingExtractor mirrors llmTestServer but returns
// HTTP 500 when the request's system prompt matches the persona
// extractor signature. The operator-agent path stays healthy so
// the chat turn itself still succeeds; only extraction fails. Used
// to verify the B-P9 production-path log wiring: RunConsoleCommand
// must thread Session.PersonaExtractLogger from runtime so a real
// failure leaves a `stage=extract` line on disk.
func llmTestServerFailingExtractor(t *testing.T) *httptest.Server {
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
			if strings.Contains(systemPrompt, "extract candidate persona facts") {
				http.Error(w, "simulated upstream provider outage", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": `{"type":"final","message":"noted"}`}},
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

func TestRunConsoleOnceWritesPersonaErrorLogOnExtractFailure(t *testing.T) {
	// B-P9 production-path: when the LLM call inside the extractor
	// fails (transport 5xx, timeout, etc.) the fire-and-forget
	// goroutine must emit one stage=extract line to the workdir
	// log so `lore persona errors` can surface it. This test drives
	// the actual RunConsoleCommand against a fake LLM that 500s the
	// extractor request, then reads the log file directly to verify
	// the line landed.
	//
	// Companion of the happy-path TestRunConsoleOnceWiresPersonaExtractionEndToEnd
	// above: between them they pin the wiring on both branches of
	// the goroutine (success -> store, failure -> log).
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	utterance := "Hi, I major in economics."
	server := llmTestServerFailingExtractor(t)
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
		t.Fatalf("operator agent reply missing despite extractor failure -- the chat path should be independent:\n%s", stdout.String())
	}

	// Open the runtime in read-only mode to resolve the log path,
	// then inspect the file content directly. We do NOT use
	// `lore persona errors` here because that path is exercised
	// elsewhere; this test pins the writer side.
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime(reopen): %v", err)
	}
	logPath := runtime.PersonaExtractLogPath()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read persona-extract.log at %s: %v", logPath, err)
	}
	body := string(data)
	for _, want := range []string{
		"stage=extract",
		"model request failed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("log missing %q after simulated extractor failure:\n%s", want, body)
		}
	}

	// Store side must remain empty: a failed extractor never lands
	// a candidate, regardless of how the chat path responded.
	runtime2, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime(post-log inspect): %v", err)
	}
	t.Cleanup(func() { _ = runtime2.Close() })
	candidates, err := runtime2.Store.PersonaCandidates().ListCandidatesByState(persona.PersonaCandidateOpen, 0)
	if err != nil {
		t.Fatalf("ListCandidatesByState: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected zero stored candidates after extractor failure, got %d: %+v", len(candidates), candidates)
	}
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
