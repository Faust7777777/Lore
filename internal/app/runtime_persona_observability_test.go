package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/config/configtest"
)

func TestOpenRuntimePersonaExtractTimeoutFromEnv(t *testing.T) {
	// Operator wants the fire-and-forget extractor to wait longer for
	// a slow provider. LORE_LLM_PERSONA_EXTRACT_TIMEOUT accepts any
	// time.ParseDuration syntax; the value lands on
	// Runtime.PersonaExtractTimeout for the CLI to wire onto the
	// console Session.
	configtest.IsolateHome(t)
	t.Setenv("LORE_LLM_PERSONA_EXTRACT_TIMEOUT", "12s")

	rt, err := OpenRuntimeWithConfigOptions(t.TempDir(), configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	defer rt.Close()

	if rt.PersonaExtractTimeout != 12*time.Second {
		t.Fatalf("PersonaExtractTimeout = %v, want 12s", rt.PersonaExtractTimeout)
	}
}

func TestOpenRuntimePersonaExtractTimeoutEnvAbsentIsZero(t *testing.T) {
	// Without the env knob, Runtime.PersonaExtractTimeout stays zero
	// so the Session falls through to defaultPersonaExtractTimeout.
	// Existing P4 behavior preserved.
	configtest.IsolateHome(t)
	t.Setenv("LORE_LLM_PERSONA_EXTRACT_TIMEOUT", "")

	rt, err := OpenRuntimeWithConfigOptions(t.TempDir(), configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	defer rt.Close()

	if rt.PersonaExtractTimeout != 0 {
		t.Fatalf("PersonaExtractTimeout = %v, want 0 (no env)", rt.PersonaExtractTimeout)
	}
}

func TestOpenRuntimePersonaExtractTimeoutIgnoresInvalidEnv(t *testing.T) {
	// A garbled env value must NOT fail the runtime open: extraction
	// is observability-only, and refusing to boot on a typo would
	// break the chat. Invalid values silently fall back to the
	// default (Session sees Timeout=0, uses defaultPersonaExtractTimeout).
	configtest.IsolateHome(t)
	t.Setenv("LORE_LLM_PERSONA_EXTRACT_TIMEOUT", "not-a-duration")

	rt, err := OpenRuntimeWithConfigOptions(t.TempDir(), configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v (must tolerate invalid env)", err)
	}
	defer rt.Close()

	if rt.PersonaExtractTimeout != 0 {
		t.Fatalf("PersonaExtractTimeout = %v, want 0 (invalid env)", rt.PersonaExtractTimeout)
	}
}

func TestOpenRuntimePersonaExtractTimeoutIgnoresNegativeEnv(t *testing.T) {
	// A negative or zero duration is meaningless for a wait budget;
	// treated identically to "env unset".
	configtest.IsolateHome(t)
	t.Setenv("LORE_LLM_PERSONA_EXTRACT_TIMEOUT", "-5s")

	rt, err := OpenRuntimeWithConfigOptions(t.TempDir(), configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	defer rt.Close()

	if rt.PersonaExtractTimeout != 0 {
		t.Fatalf("PersonaExtractTimeout = %v, want 0 (negative env)", rt.PersonaExtractTimeout)
	}
}

func TestOpenRuntimeOpensPersonaExtractLogFile(t *testing.T) {
	// The log file lives at <StateDir>/logs/persona-extract.log so
	// operators can `tail` it. OpenRuntime creates the parent directory
	// and the file (O_APPEND|O_CREATE), then exposes the writer via
	// Runtime.PersonaExtractLogger. Subsequent writes via the Session
	// reach disk; we exercise that round-trip here directly through
	// the runtime field so the test does not depend on a real LLM.
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	rt, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	defer rt.Close()

	if rt.PersonaExtractLogger == nil {
		t.Fatal("PersonaExtractLogger is nil; expected an open log file")
	}
	if _, err := rt.PersonaExtractLogger.Write([]byte("probe-line\n")); err != nil {
		t.Fatalf("write probe to logger: %v", err)
	}

	logPath := filepath.Join(rt.Config.Paths.StateDir, "logs", "persona-extract.log")
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read persona-extract.log: %v", err)
	}
	if !strings.Contains(string(got), "probe-line") {
		t.Fatalf("log file missing probe content:\n%s", got)
	}
}

func TestRuntimeCloseReleasesPersonaExtractLogFile(t *testing.T) {
	// Close must release the OS file handle so a subsequent OpenRuntime
	// in the same workdir can re-open it for append without sharing
	// violations on Windows. We exercise that by closing once and
	// re-opening the workdir; both opens succeed without error.
	configtest.IsolateHome(t)
	workDir := t.TempDir()

	rt1, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("first OpenRuntime: %v", err)
	}
	if _, err := rt1.PersonaExtractLogger.Write([]byte("from-rt1\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := rt1.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	rt2, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("second OpenRuntime after Close: %v", err)
	}
	defer rt2.Close()
	if _, err := rt2.PersonaExtractLogger.Write([]byte("from-rt2\n")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	logPath := filepath.Join(rt2.Config.Paths.StateDir, "logs", "persona-extract.log")
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read persona-extract.log: %v", err)
	}
	for _, want := range []string{"from-rt1", "from-rt2"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("log file missing %q across runtime lifetimes:\n%s", want, got)
		}
	}
}
