package app

import (
	"os"
	"path/filepath"
	"testing"

	"obsidian-harness/internal/config/configtest"
)

// TestOpenRuntimeWithConfigOptionsIsolatesUserGlobal reproduces the reviewer's
// failure scenario: a malformed ~/.lore/config.json breaks OpenRuntime but
// must NOT break OpenRuntimeWithConfigOptions when isolated options are passed.
//
// Failure mode before the fix: TestRuntimeSmokeP0 and other tests calling
// OpenRuntime would fail non-deterministically on developer machines with an
// unexpected ~/.lore/config.json.
func TestOpenRuntimeWithConfigOptionsIsolatesUserGlobal(t *testing.T) {
	// Plant a malformed user-global config under a controlled HOME.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	lorePath := filepath.Join(home, ".lore")
	if err := os.MkdirAll(lorePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(.lore): %v", err)
	}
	if err := os.WriteFile(filepath.Join(lorePath, "config.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.json): %v", err)
	}

	workDir := t.TempDir()

	// OpenRuntime (default, no isolation) must fail because of the malformed
	// user-global config — this is the regression scenario.
	_, err := OpenRuntime(workDir)
	if err == nil {
		t.Fatal("OpenRuntime() expected error from malformed user-global config, got nil")
	}

	// OpenRuntimeWithConfigOptions with isolated options must succeed
	// regardless of what is in HOME, because it bypasses the user-global path.
	rt, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions(isolated) error = %v", err)
	}
	if err := rt.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
