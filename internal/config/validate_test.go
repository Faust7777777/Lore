package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateDefaultPasses(t *testing.T) {
	// The shipped Default(workDir) baseline must always pass
	// Validate. A regression here means the package's own defaults
	// would refuse to boot, which is a code smell even though
	// LoadWithOptions runs Validate against the merged result.
	cfg := Default(filepath.Join(t.TempDir(), "work"))
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default config failed Validate: %v", err)
	}
}

func TestValidateRejectsEmptyPaths(t *testing.T) {
	cases := []struct {
		field string
		set   func(*Config)
	}{
		{"work_dir", func(c *Config) { c.Paths.WorkDir = "" }},
		{"vault_root", func(c *Config) { c.Paths.VaultRoot = "" }},
		{"state_dir", func(c *Config) { c.Paths.StateDir = "" }},
		{"audit_log_dir", func(c *Config) { c.Paths.AuditLogDir = "" }},
		{"process_sink_dir", func(c *Config) { c.Paths.ProcessSinkDir = "" }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.field, func(t *testing.T) {
			cfg := Default(filepath.Join(t.TempDir(), "work"))
			tc.set(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("empty %s should fail Validate", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("err = %v, must name the offending field %q", err, tc.field)
			}
		})
	}
}

func TestValidateRejectsBadVaultConfig(t *testing.T) {
	t.Run("negative_debounce", func(t *testing.T) {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.Vault.DebounceWindow = -time.Second
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "debounce_window") {
			t.Fatalf("err = %v, want debounce_window violation", err)
		}
	})
	t.Run("empty_temp_suffix", func(t *testing.T) {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.Vault.TempSuffix = ""
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "temp_suffix") {
			t.Fatalf("err = %v, want temp_suffix violation", err)
		}
	})
	t.Run("threshold_out_of_range", func(t *testing.T) {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.Vault.Resolve.UniqueScoreThreshold = 1.5
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "unique_score_threshold") {
			t.Fatalf("err = %v, want threshold violation", err)
		}
	})
	t.Run("margin_negative", func(t *testing.T) {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.Vault.Resolve.UniqueScoreMargin = -0.1
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "unique_score_margin") {
			t.Fatalf("err = %v, want margin violation", err)
		}
	})
}

func TestValidateRejectsBadRuntimeConfig(t *testing.T) {
	cases := []struct {
		field string
		set   func(*Config)
	}{
		{"max_event_queue", func(c *Config) { c.Runtime.MaxEventQueue = 0 }},
		{"max_draft_queue", func(c *Config) { c.Runtime.MaxDraftQueue = -1 }},
		{"max_memory_mb", func(c *Config) { c.Runtime.MaxMemoryMB = 0 }},
		{"inspect_at", func(c *Config) { c.Runtime.InspectAt = "9am" }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.field, func(t *testing.T) {
			cfg := Default(filepath.Join(t.TempDir(), "work"))
			tc.set(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("invalid %s should fail Validate", tc.field)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("err = %v, must name the offending field %q", err, tc.field)
			}
		})
	}
}

func TestValidateAcceptsBoundaryHHMM(t *testing.T) {
	// 00:00 and 23:59 are the only legal extremes for the
	// HH:MM pattern; reject 24:00 and -01:00.
	cfg := Default(filepath.Join(t.TempDir(), "work"))
	for _, ok := range []string{"00:00", "09:00", "23:59"} {
		cfg.Runtime.InspectAt = ok
		cfg.ProcessSink.DailyRollupAt = ok
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%q should be a legal HH:MM, got err = %v", ok, err)
		}
	}
	for _, bad := range []string{"24:00", "9:00", "12:60", "1:23pm", "23.59", ""} {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.Runtime.InspectAt = bad
		if err := cfg.Validate(); err == nil {
			t.Fatalf("%q should be rejected by inspect_at", bad)
		}
	}
}

func TestValidateRejectsBadProcessSinkConfig(t *testing.T) {
	t.Run("negative_checkpoint", func(t *testing.T) {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.ProcessSink.CheckpointEvery = -time.Minute
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "checkpoint_every") {
			t.Fatalf("err = %v, want checkpoint_every violation", err)
		}
	})
	t.Run("daily_rollup_bad_format", func(t *testing.T) {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.ProcessSink.DailyRollupAt = "midnight"
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "daily_rollup_at") {
			t.Fatalf("err = %v, want daily_rollup_at violation", err)
		}
	})
	t.Run("retention_negative", func(t *testing.T) {
		cfg := Default(filepath.Join(t.TempDir(), "work"))
		cfg.ProcessSink.RetentionDays = -1
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "retention_days") {
			t.Fatalf("err = %v, want retention_days violation", err)
		}
	})
}

func TestValidateRejectsBadUsageConfig(t *testing.T) {
	cfg := Default(filepath.Join(t.TempDir(), "work"))
	cfg.Usage.SoftWarningTokens = -100
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "soft_warning_tokens") {
		t.Fatalf("err = %v, want soft_warning_tokens violation", err)
	}
}

func TestLoadWithOptionsRunsValidate(t *testing.T) {
	// A workspace layer file that overrides a required field
	// to an invalid value must cause LoadWithOptions to error,
	// proving Validate runs on the merged result instead of
	// silently accepting it.
	workDir := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "workspace.json")
	bad := []byte(`{"vault":{"temp_suffix":""}}`)
	if err := os.WriteFile(workspacePath, bad, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, _, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent.json"),
		WorkspacePath:  workspacePath,
	})
	if err == nil {
		t.Fatalf("LoadWithOptions should reject a layer that violates Validate")
	}
	if !strings.Contains(err.Error(), "temp_suffix") {
		t.Fatalf("err = %v, must name the offending field", err)
	}
}

func TestLoadWithOptionsKeepsValidLayerWorking(t *testing.T) {
	// Regression guard: a workspace layer that overrides legal
	// values must still load without errors after the new
	// Validate gate.
	workDir := t.TempDir()
	workspacePath := filepath.Join(t.TempDir(), "workspace.json")
	good := []byte(`{"runtime":{"inspect_at":"08:30","max_event_queue":512,"max_draft_queue":128,"max_memory_mb":512}}`)
	if err := os.WriteFile(workspacePath, good, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg, _, err := LoadWithOptions(workDir, LoadOptions{
		UserGlobalPath: filepath.Join(t.TempDir(), "absent.json"),
		WorkspacePath:  workspacePath,
	})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if cfg.Runtime.InspectAt != "08:30" {
		t.Fatalf("InspectAt = %q, want 08:30", cfg.Runtime.InspectAt)
	}
	if cfg.Runtime.MaxEventQueue != 512 {
		t.Fatalf("MaxEventQueue = %d, want 512", cfg.Runtime.MaxEventQueue)
	}
}
