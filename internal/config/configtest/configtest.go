// Package configtest provides test helpers that isolate configuration loading
// from the developer's real machine. It exists so unit tests can avoid being
// polluted by ~/.lore/config.json or workspace override files left over from
// manual experimentation.
package configtest

import (
	"path/filepath"
	"testing"

	"obsidian-harness/internal/config"
)

// IsolatedOptions returns config.LoadOptions safe for unit tests: both
// override paths point to absent files inside t.TempDir(), so neither the
// user-global nor the workspace layer files affect the loaded config.
//
// Pass the result to config.LoadWithOptions or
// app.OpenRuntimeWithConfigOptions when a test directly invokes the
// configuration loader or runtime.
func IsolatedOptions(t *testing.T) config.LoadOptions {
	t.Helper()
	base := t.TempDir()
	return config.LoadOptions{
		UserGlobalPath: filepath.Join(base, "absent-user-global.json"),
		WorkspacePath:  filepath.Join(base, "absent-workspace.json"),
	}
}

// IsolateHome sets HOME and USERPROFILE to t.TempDir() so that
// os.UserHomeDir() and any other code path resolving the user home
// directory inside the test sees an empty directory. Use this in tests
// that exercise app.OpenRuntime indirectly, for example through the CLI
// run() entrypoint where injecting LoadOptions is impractical.
//
// The returned path is the temporary HOME directory, useful for tests
// that need to plant fixture files under ~/.lore/ or similar.
func IsolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}
