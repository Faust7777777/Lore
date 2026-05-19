package console

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config/configtest"
	openai "obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/operatoragent"
)

// TestEndToEndResolveReadFinalFileInspectionTurn drives one real
// console.Session.Handle turn through a real *app.Runtime backed by a
// sqlite state store and a real vault on disk, using a scripted fake
// completion client. It proves the B3 prompt + tool guidance lets a
// single user utterance ("please inspect target and summarize it") complete
// in one turn via vault_resolve -> vault_read -> final, with:
//
//   - Exactly one user input.
//   - Trace length >= 2 (the two read-only tool steps).
//   - Final answer derived from the file's content on disk.
//   - Session WorkingSet records the resolved markdown path.
//   - B2 contract: StopReason = "final", StepCount = 3.
//
// The completion client is fake but the rest of the stack is real:
// the bootstrapped vault, the runtime's vault_resolve / vault_read
// tool implementations, the orchestrator, and the sqlite store are
// all production code paths.
func TestEndToEndResolveReadFinalFileInspectionTurn(t *testing.T) {
	workDir := t.TempDir()
	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("runtime.Close() error = %v", err)
		}
	})
	if _, err := runtime.Bootstrap(time.Now()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	// Place a fixture markdown file inside the bootstrapped vault.
	// vault_resolve must match "target" -> "03-notes/target.md", and
	// vault_read must return its content for the model to quote in
	// the final answer.
	targetRelPath := filepath.ToSlash(filepath.Join("03-notes", "target.md"))
	targetAbsPath := filepath.Join(runtime.Config.Paths.VaultRoot, "03-notes", "target.md")
	if err := os.MkdirAll(filepath.Dir(targetAbsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := "# Target\n\nThis file explains project onboarding.\n"
	if err := os.WriteFile(targetAbsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// The scripted model emits exactly the workflow B3 guides:
	// 1. vault_resolve to map a name to a path,
	// 2. vault_read with the resolved path,
	// 3. final quoting the file content.
	client := &scriptedCompletionClient{
		responses: []openai.ChatCompletionResponse{
			{Content: `{"type":"tool_call","tool":"vault_resolve","arguments":{"query":"target"}}`},
			{Content: `{"type":"tool_call","tool":"vault_read","arguments":{"path":"` + targetRelPath + `"}}`},
			{Content: `{"type":"final","message":"This file explains project onboarding."}`},
		},
	}
	agent := operatoragent.NewModelAgent(client, "openai-compatible", "fake-model")
	session := NewSessionWithAgent("test", agent)
	session.DefaultAgentID = "codex"

	// Single user utterance -- no follow-up prompt expected.
	out, err := session.Handle("please inspect target and summarize it", runtime)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if !strings.Contains(out, "project onboarding") {
		t.Fatalf("final output = %q, want content derived from target.md", out)
	}
	if len(session.LastToolTrace) < 2 {
		t.Fatalf("trace length = %d, want >= 2 tool steps", len(session.LastToolTrace))
	}
	if session.LastToolTrace[0].Name != "vault_resolve" || session.LastToolTrace[1].Name != "vault_read" {
		t.Fatalf("trace order = %v / %v, want vault_resolve then vault_read",
			session.LastToolTrace[0].Name, session.LastToolTrace[1].Name)
	}
	for _, step := range session.LastToolTrace {
		if step.Status != "ok" {
			t.Fatalf("trace step %s status = %q, want ok", step.Name, step.Status)
		}
	}

	// WorkingSet must remember the resolved markdown path so a
	// follow-up turn can build context around the same file without
	// requiring the user to repeat the reference.
	foundTarget := false
	for _, item := range session.WorkingSet {
		if item.Kind == "vault_path" && item.Path == targetRelPath {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		t.Fatalf("WorkingSet missing %s; got %+v", targetRelPath, session.WorkingSet)
	}

	// Only one user-side history turn was produced, matching the
	// "one user input, multi-step completion" goal of B4.
	userTurns := 0
	for _, turn := range session.History {
		if turn.Role == "user" {
			userTurns++
		}
	}
	if userTurns != 1 {
		t.Fatalf("history user turns = %d, want exactly 1", userTurns)
	}

	// The fake client must have served exactly three model calls --
	// no extra step was needed to complete the user's request.
	if client.idx != 3 {
		t.Fatalf("scripted model calls served = %d, want 3", client.idx)
	}
}
