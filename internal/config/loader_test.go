package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func absentPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "absent.json")
}

func TestLoadReturnsDefaultsWhenNoLayerFiles(t *testing.T) {
	workDir := t.TempDir()

	cfg, diagnostics, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: absentPath(t),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}

	expected := Default(workDir)
	if cfg.Vault.TempSuffix != expected.Vault.TempSuffix {
		t.Errorf("temp suffix changed: got %q want %q", cfg.Vault.TempSuffix, expected.Vault.TempSuffix)
	}
	if cfg.Bootstrap.ManagedModeRequired != expected.Bootstrap.ManagedModeRequired {
		t.Errorf("managed mode flag changed: got %v want %v", cfg.Bootstrap.ManagedModeRequired, expected.Bootstrap.ManagedModeRequired)
	}
	if cfg.Runtime.ProactiveMode != expected.Runtime.ProactiveMode {
		t.Errorf("proactive mode changed: got %q want %q", cfg.Runtime.ProactiveMode, expected.Runtime.ProactiveMode)
	}

	if len(diagnostics) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d: %+v", len(diagnostics), diagnostics)
	}
	if diagnostics[0].Source != LayerDefault || diagnostics[0].Status != LayerStatusLoaded {
		t.Errorf("default diag: %+v", diagnostics[0])
	}
	if diagnostics[1].Source != LayerUserGlobal || diagnostics[1].Status != LayerStatusMissing {
		t.Errorf("user-global diag: %+v", diagnostics[1])
	}
	if diagnostics[2].Source != LayerWorkspace || diagnostics[2].Status != LayerStatusMissing {
		t.Errorf("workspace diag: %+v", diagnostics[2])
	}
}

func TestLoadAppliesUserGlobalOverrides(t *testing.T) {
	workDir := t.TempDir()
	userPath := filepath.Join(t.TempDir(), "lore-config.json")
	if err := os.WriteFile(userPath, []byte(`{"vault":{"temp_suffix":".user-tmp"}}`), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	cfg, diagnostics, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: userPath,
		WorkspacePath:  absentPath(t),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}

	if cfg.Vault.TempSuffix != ".user-tmp" {
		t.Errorf("user-global override not applied: got %q", cfg.Vault.TempSuffix)
	}

	defaults := Default(workDir)
	if cfg.Vault.DebounceWindow != defaults.Vault.DebounceWindow {
		t.Errorf("unrelated field changed: got %v want %v", cfg.Vault.DebounceWindow, defaults.Vault.DebounceWindow)
	}
	if cfg.Bootstrap.ManagedModeRequired != defaults.Bootstrap.ManagedModeRequired {
		t.Errorf("unrelated bootstrap field changed: got %v want %v", cfg.Bootstrap.ManagedModeRequired, defaults.Bootstrap.ManagedModeRequired)
	}

	userDiag := diagnostics[1]
	if userDiag.Source != LayerUserGlobal || userDiag.Status != LayerStatusLoaded {
		t.Errorf("user-global diag not loaded: %+v", userDiag)
	}
	if userDiag.Path != userPath {
		t.Errorf("user-global diag path mismatch: got %q want %q", userDiag.Path, userPath)
	}
}

func TestLoadAppliesWorkspaceOverrides(t *testing.T) {
	workDir := t.TempDir()
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(workspacePath, []byte(`{"runtime":{"proactive_mode":"quiet"}}`), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	cfg, diagnostics, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: absentPath(t),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}

	if cfg.Runtime.ProactiveMode != "quiet" {
		t.Errorf("workspace override not applied: got %q", cfg.Runtime.ProactiveMode)
	}

	workspaceDiag := diagnostics[2]
	if workspaceDiag.Source != LayerWorkspace || workspaceDiag.Status != LayerStatusLoaded {
		t.Errorf("workspace diag not loaded: %+v", workspaceDiag)
	}
}

func TestLoadWorkspaceOverridesUserGlobal(t *testing.T) {
	workDir := t.TempDir()
	userPath := filepath.Join(t.TempDir(), "user.json")
	if err := os.WriteFile(userPath, []byte(`{"runtime":{"proactive_mode":"quiet","listen_address":"127.0.0.1:9999"}}`), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(workspacePath, []byte(`{"runtime":{"proactive_mode":"off"}}`), 0o600); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	cfg, _, err := LoadWithOptions(workDir, LoadOptions{UserGlobalPath: userPath})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}

	if cfg.Runtime.ProactiveMode != "off" {
		t.Errorf("workspace did not override user-global proactive_mode: got %q", cfg.Runtime.ProactiveMode)
	}
	if cfg.Runtime.ListenAddress != "127.0.0.1:9999" {
		t.Errorf("workspace clobbered unrelated user-global field listen_address: got %q", cfg.Runtime.ListenAddress)
	}
}

func TestLoadFailsOnMalformedJSON(t *testing.T) {
	workDir := t.TempDir()
	userPath := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(userPath, []byte(`{"vault":{`), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	_, diagnostics, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: userPath,
		WorkspacePath:  absentPath(t),
	})
	if err == nil {
		t.Fatalf("expected error from malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "user_global") {
		t.Errorf("error should mention layer name: %v", err)
	}
	if !strings.Contains(err.Error(), userPath) {
		t.Errorf("error should mention layer path: %v", err)
	}
	if len(diagnostics) < 2 {
		t.Fatalf("expected at least 2 diagnostics: %+v", diagnostics)
	}
	if diagnostics[1].Status != LayerStatusError || diagnostics[1].Err == nil {
		t.Errorf("malformed user-global diag: %+v", diagnostics[1])
	}
}

func TestLoadTreatsEmptyFileAsMissing(t *testing.T) {
	workDir := t.TempDir()
	userPath := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(userPath, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	cfg, diagnostics, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: userPath,
		WorkspacePath:  absentPath(t),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}
	if cfg.Vault.TempSuffix != Default(workDir).Vault.TempSuffix {
		t.Errorf("empty file should not change defaults: got %q", cfg.Vault.TempSuffix)
	}
	if diagnostics[1].Status != LayerStatusMissing {
		t.Errorf("empty file should be reported as missing: %+v", diagnostics[1])
	}
}

func TestLoadHonorsExplicitWorkspacePath(t *testing.T) {
	workDir := t.TempDir()
	customPath := filepath.Join(t.TempDir(), "custom-workspace.json")
	if err := os.WriteFile(customPath, []byte(`{"runtime":{"proactive_mode":"off"}}`), 0o600); err != nil {
		t.Fatalf("write custom config: %v", err)
	}

	cfg, diagnostics, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: absentPath(t),
		WorkspacePath:  customPath,
	})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}
	if cfg.Runtime.ProactiveMode != "off" {
		t.Errorf("custom workspace path not honored: got %q", cfg.Runtime.ProactiveMode)
	}
	if diagnostics[2].Path != customPath {
		t.Errorf("workspace diag path: got %q want %q", diagnostics[2].Path, customPath)
	}
}

func TestLoadDiagnosticOrder(t *testing.T) {
	workDir := t.TempDir()
	userPath := filepath.Join(t.TempDir(), "u.json")
	if err := os.WriteFile(userPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write user: %v", err)
	}
	workspacePath := filepath.Join(workDir, ".lore", "config.json")
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(workspacePath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write workspace: %v", err)
	}

	_, diagnostics, err := LoadWithOptions(workDir, LoadOptions{UserGlobalPath: userPath})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}
	if len(diagnostics) != 3 {
		t.Fatalf("expected 3 diagnostics: %+v", diagnostics)
	}
	expectedSources := []LayerSource{LayerDefault, LayerUserGlobal, LayerWorkspace}
	for i, want := range expectedSources {
		if diagnostics[i].Source != want {
			t.Errorf("diag[%d].Source: got %q want %q", i, diagnostics[i].Source, want)
		}
	}
}

func TestLoadPreservesManagedCorePathsFromDefaults(t *testing.T) {
	workDir := t.TempDir()
	userPath := filepath.Join(t.TempDir(), "u.json")
	if err := os.WriteFile(userPath, []byte(`{"vault":{"temp_suffix":".x"}}`), 0o600); err != nil {
		t.Fatalf("write user: %v", err)
	}

	cfg, _, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: userPath,
		WorkspacePath:  absentPath(t),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}

	defaults := Default(workDir)
	if cfg.Vault.ManagedCore.SystemDoc != defaults.Vault.ManagedCore.SystemDoc {
		t.Errorf("managed core SystemDoc was lost: got %q want %q", cfg.Vault.ManagedCore.SystemDoc, defaults.Vault.ManagedCore.SystemDoc)
	}
	if cfg.Vault.ManagedCore.Persona != defaults.Vault.ManagedCore.Persona {
		t.Errorf("managed core Persona was lost: got %q want %q", cfg.Vault.ManagedCore.Persona, defaults.Vault.ManagedCore.Persona)
	}
}

func TestLoadOverridesManagedCoreWhenRequested(t *testing.T) {
	workDir := t.TempDir()
	userPath := filepath.Join(t.TempDir(), "u.json")
	payload := `{"vault":{"managed_core":{"agent_doc":"runtime/agent.md"}}}`
	if err := os.WriteFile(userPath, []byte(payload), 0o600); err != nil {
		t.Fatalf("write user: %v", err)
	}

	cfg, _, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: userPath,
		WorkspacePath:  absentPath(t),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions returned error: %v", err)
	}

	if cfg.Vault.ManagedCore.AgentDoc != "runtime/agent.md" {
		t.Errorf("managed core AgentDoc not overridden: got %q", cfg.Vault.ManagedCore.AgentDoc)
	}
	defaults := Default(workDir)
	if cfg.Vault.ManagedCore.SystemDoc != defaults.Vault.ManagedCore.SystemDoc {
		t.Errorf("non-overridden managed core fields should remain default: got %q want %q", cfg.Vault.ManagedCore.SystemDoc, defaults.Vault.ManagedCore.SystemDoc)
	}
}
