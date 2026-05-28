package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/config/configtest"
)

func TestSessionLogMetaSanitizesLLMBaseURL(t *testing.T) {
	configtest.IsolateHome(t)
	workDir := t.TempDir()
	configDir := filepath.Join(workDir, ".lore")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.lore) error = %v", err)
	}
	configJSON := `{
  "llm": {
    "active_profile": "danger",
    "profiles": {
      "danger": {
        "provider": "openai-compatible",
        "base_url": "https://user:pass@example.test/v1?token=sk-test#secret",
        "model": "safe-model",
        "api_key_env": "MODEL_API_KEY"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
	t.Setenv("MODEL_API_KEY", "real-secret-value")

	runtime, err := app.OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json")})
	if err != nil {
		t.Fatalf("OpenRuntimeWithConfigOptions() error = %v", err)
	}
	defer runtime.Close()

	meta := sessionLogMeta(runtime, time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC))
	if meta.BaseURL != "https://example.test/v1" {
		t.Fatalf("BaseURL = %q, want sanitized https://example.test/v1", meta.BaseURL)
	}
	if meta.Provider != "openai-compatible" || meta.Model != "safe-model" || meta.Profile != "danger" || meta.Source != "workspace" {
		t.Fatalf("meta model identity = %+v, want provider/model/profile/source", meta)
	}
	for _, banned := range []string{"real-secret-value", "user", "pass", "token", "sk-test", "secret"} {
		if strings.Contains(meta.BaseURL, banned) {
			t.Fatalf("BaseURL leaked %q: %+v", banned, meta)
		}
	}
}
