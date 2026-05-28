package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/config/configtest"
)

// TestRuntimeLLMIdentityReflectsWorkspaceProfileWithoutLeakingKey is
// the load-bearing safety check: TUI / dashboard surfaces must be
// able to call LLMIdentity(purpose) and render every returned field
// verbatim without ever holding the API key value. The test seeds a
// workspace profile whose api_key_env points at a clearly-recognizable
// secret string, then walks every string field on the returned
// LLMIdentity to assert the secret does not appear in any of them.
func TestRuntimeLLMIdentityReflectsWorkspaceProfileWithoutLeakingKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	workDir := t.TempDir()
	writeWorkspaceLLMProfile(t, workDir, server.URL, "deepseek", "deepseek", "deepseek-chat", "DEEPSEEK_API_KEY")
	const apiKeyValue = "leaky-deepseek-secret-DO-NOT-RENDER"
	t.Setenv("DEEPSEEK_API_KEY", apiKeyValue)

	rt, err := OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent-user-global.json"),
	})
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	defer rt.Close()

	identity, err := rt.LLMIdentity(config.LLMPurposePersonaExtract)
	if err != nil {
		t.Fatalf("LLMIdentity: %v", err)
	}
	if !identity.Enabled {
		t.Fatalf("identity.Enabled = false with a workspace profile configured")
	}
	if identity.Provider != "deepseek" || identity.Model != "deepseek-chat" {
		t.Fatalf("identity provider/model = %q/%q, want deepseek/deepseek-chat", identity.Provider, identity.Model)
	}
	if identity.BaseURL != server.URL {
		t.Fatalf("identity.BaseURL = %q, want %q", identity.BaseURL, server.URL)
	}
	if identity.Source != config.LLMSourceWorkspace {
		t.Fatalf("identity.Source = %q, want workspace", identity.Source)
	}
	if identity.Profile != "deepseek" {
		t.Fatalf("identity.Profile = %q, want deepseek", identity.Profile)
	}
	if identity.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Fatalf("identity.APIKeyEnv = %q, want DEEPSEEK_API_KEY (the NAME, not the value)", identity.APIKeyEnv)
	}

	// Walk every string field; the secret value must appear in none.
	// Use reflection so a future field addition is caught automatically.
	v := reflect.ValueOf(identity)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() != reflect.String {
			continue
		}
		if strings.Contains(field.String(), apiKeyValue) {
			t.Fatalf("LLMIdentity.%s leaked the API key value %q", v.Type().Field(i).Name, field.String())
		}
	}

	// And the struct itself must not have an APIKey-typed field.
	// Compile-time enforced by the struct definition; runtime check
	// here pins the contract so a future "just add APIKey for
	// convenience" change is caught.
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if strings.EqualFold(name, "APIKey") {
			t.Fatalf("LLMIdentity must not expose an APIKey field; found %s", name)
		}
	}
}

func TestRuntimeLLMIdentityReturnsDisabledWhenPurposeUnconfigured(t *testing.T) {
	// No LLM env / workspace config: every purpose resolves to
	// Enabled=false rather than erroring out, so TUI can render
	// "process-sink: not configured" cleanly without branching on
	// error types. IsolateHome clears the dev's ~/.lore but env
	// vars must be cleared explicitly; clear all four canonical
	// (operator + llm) bases plus the legacy obsidian-harness
	// aliases so the test is workstation-independent.
	configtest.IsolateHome(t)
	for _, envVar := range []string{
		"LORE_OPERATOR_BASE_URL", "LORE_OPERATOR_API_KEY", "LORE_OPERATOR_MODEL",
		"LORE_LLM_BASE_URL", "LORE_LLM_API_KEY", "LORE_LLM_MODEL",
		"OBSIDIAN_HARNESS_OPERATOR_BASE_URL", "OBSIDIAN_HARNESS_OPERATOR_API_KEY", "OBSIDIAN_HARNESS_OPERATOR_MODEL",
		"OBSIDIAN_HARNESS_LLM_BASE_URL", "OBSIDIAN_HARNESS_LLM_API_KEY", "OBSIDIAN_HARNESS_LLM_MODEL",
	} {
		t.Setenv(envVar, "")
	}
	workDir := t.TempDir()
	rt, err := OpenRuntimeWithConfigOptions(workDir, configtest.IsolatedOptions(t))
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	defer rt.Close()

	for _, purpose := range []string{
		config.LLMPurposeOperator,
		config.LLMPurposePersonaExtract,
		config.LLMPurposeProcessSink,
	} {
		identity, err := rt.LLMIdentity(purpose)
		if err != nil {
			t.Fatalf("LLMIdentity(%s) error = %v, want nil for unconfigured purpose", purpose, err)
		}
		if identity.Enabled {
			t.Fatalf("LLMIdentity(%s).Enabled = true with no config; want false", purpose)
		}
	}
}

func TestRuntimeLLMIdentityRejectsNilReceiver(t *testing.T) {
	// Nil receiver should error rather than panic; matches the
	// ResolveLLMConfig defensive guard the helper delegates to.
	var rt *Runtime
	if _, err := rt.LLMIdentity(config.LLMPurposeOperator); err == nil {
		t.Fatal("LLMIdentity on nil runtime = nil error, want guard failure")
	} else if !strings.Contains(fmt.Sprint(err), "runtime") {
		t.Fatalf("error should mention runtime; got %v", err)
	}
}
