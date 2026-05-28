package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"obsidian-harness/internal/model"
)

type Config struct {
	Paths       PathsConfig       `json:"paths"`
	Vault       VaultConfig       `json:"vault"`
	Runtime     RuntimeConfig     `json:"runtime"`
	ProcessSink ProcessSinkConfig `json:"process_sink"`
	Usage       UsageConfig       `json:"usage"`
	LLM         LLMConfig         `json:"llm"`
	Bootstrap   BootstrapConfig   `json:"bootstrap"`
}

type PathsConfig struct {
	WorkDir        string `json:"work_dir"`
	VaultRoot      string `json:"vault_root"`
	StateDir       string `json:"state_dir"`
	AuditLogDir    string `json:"audit_log_dir"`
	ProcessSinkDir string `json:"process_sink_dir"`
}

type VaultConfig struct {
	DebounceWindow time.Duration          `json:"debounce_window"`
	TempSuffix     string                 `json:"temp_suffix"`
	ManagedCore    model.ManagedCorePaths `json:"managed_core"`
	Resolve        VaultResolveConfig     `json:"resolve"`
}

type VaultResolveConfig struct {
	UniqueScoreThreshold float64 `json:"unique_score_threshold"`
	UniqueScoreMargin    float64 `json:"unique_score_margin"`
}

type RuntimeConfig struct {
	ListenAddress string `json:"listen_address"`
	MaxEventQueue int    `json:"max_event_queue"`
	MaxDraftQueue int    `json:"max_draft_queue"`
	MaxMemoryMB   int    `json:"max_memory_mb"`
	ProactiveMode string `json:"proactive_mode"`
	InspectAt     string `json:"inspect_at"`
}

type ProcessSinkConfig struct {
	CheckpointEvery time.Duration `json:"checkpoint_every"`
	DailyRollupAt   string        `json:"daily_rollup_at"`
	RetentionDays   int           `json:"retention_days"`
	WriteEmptySlots bool          `json:"write_empty_slots"`
}

type UsageConfig struct {
	TrackUsage        bool `json:"track_usage"`
	SoftWarningTokens int  `json:"soft_warning_tokens"`
}

type LLMConfig struct {
	ActiveProfile string                      `json:"active_profile,omitempty"`
	Profiles      map[string]LLMProfileConfig `json:"profiles,omitempty"`
}

type LLMProfileConfig struct {
	Provider  string        `json:"provider,omitempty"`
	BaseURL   string        `json:"base_url,omitempty"`
	Model     string        `json:"model,omitempty"`
	Timeout   time.Duration `json:"timeout,omitempty"`
	APIKeyEnv string        `json:"api_key_env,omitempty"`
	APIKeyRef string        `json:"api_key_ref,omitempty"`
}

type BootstrapConfig struct {
	ManagedModeRequired bool `json:"managed_mode_required"`
	ScaffoldIfMissing   bool `json:"scaffold_if_missing"`
}

// hhmmPattern matches "HH:MM" 24-hour clock times. Used by
// Config.Validate to gate Runtime.InspectAt and
// ProcessSink.DailyRollupAt -- both consumed by daemon code that
// assumes the strings can be split on ':'. Catches typos like
// "9am" or "23:65" before runtime open.
var hhmmPattern = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// Validate runs invariant checks on the merged Config and returns
// the first violation. Called by LoadWithOptions before returning
// so a malformed user-global or workspace layer fails at runtime
// open instead of later in a daemon callback or a vault write.
//
// The check set is deliberately small and operator-facing: every
// rule maps to either a path that downstream code dereferences
// blindly, a window/duration that must be non-negative for the
// scheduler to make sense, or a string time that downstream
// parsing assumes is "HH:MM". Rules that would over-constrain
// (e.g. enforcing a specific port range, requiring AuditLogDir
// to be inside WorkDir) are intentionally omitted -- this is
// validation for typos, not policy.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Paths.WorkDir) == "" {
		return fmt.Errorf("config: paths.work_dir must not be empty")
	}
	if strings.TrimSpace(c.Paths.VaultRoot) == "" {
		return fmt.Errorf("config: paths.vault_root must not be empty")
	}
	if strings.TrimSpace(c.Paths.StateDir) == "" {
		return fmt.Errorf("config: paths.state_dir must not be empty")
	}
	if strings.TrimSpace(c.Paths.AuditLogDir) == "" {
		return fmt.Errorf("config: paths.audit_log_dir must not be empty")
	}
	if strings.TrimSpace(c.Paths.ProcessSinkDir) == "" {
		return fmt.Errorf("config: paths.process_sink_dir must not be empty")
	}

	if c.Vault.DebounceWindow < 0 {
		return fmt.Errorf("config: vault.debounce_window must be >= 0, got %s", c.Vault.DebounceWindow)
	}
	if strings.TrimSpace(c.Vault.TempSuffix) == "" {
		return fmt.Errorf("config: vault.temp_suffix must not be empty; required by atomic vault writes")
	}
	if c.Vault.Resolve.UniqueScoreThreshold < 0 || c.Vault.Resolve.UniqueScoreThreshold > 1 {
		return fmt.Errorf("config: vault.resolve.unique_score_threshold must be in [0,1], got %g", c.Vault.Resolve.UniqueScoreThreshold)
	}
	if c.Vault.Resolve.UniqueScoreMargin < 0 || c.Vault.Resolve.UniqueScoreMargin > 1 {
		return fmt.Errorf("config: vault.resolve.unique_score_margin must be in [0,1], got %g", c.Vault.Resolve.UniqueScoreMargin)
	}

	if c.Runtime.MaxEventQueue <= 0 {
		return fmt.Errorf("config: runtime.max_event_queue must be > 0, got %d", c.Runtime.MaxEventQueue)
	}
	if c.Runtime.MaxDraftQueue <= 0 {
		return fmt.Errorf("config: runtime.max_draft_queue must be > 0, got %d", c.Runtime.MaxDraftQueue)
	}
	if c.Runtime.MaxMemoryMB <= 0 {
		return fmt.Errorf("config: runtime.max_memory_mb must be > 0, got %d", c.Runtime.MaxMemoryMB)
	}
	if !hhmmPattern.MatchString(c.Runtime.InspectAt) {
		return fmt.Errorf("config: runtime.inspect_at must be HH:MM (24h clock), got %q", c.Runtime.InspectAt)
	}

	if c.ProcessSink.CheckpointEvery < 0 {
		return fmt.Errorf("config: process_sink.checkpoint_every must be >= 0, got %s", c.ProcessSink.CheckpointEvery)
	}
	if !hhmmPattern.MatchString(c.ProcessSink.DailyRollupAt) {
		return fmt.Errorf("config: process_sink.daily_rollup_at must be HH:MM (24h clock), got %q", c.ProcessSink.DailyRollupAt)
	}
	if c.ProcessSink.RetentionDays < 0 {
		return fmt.Errorf("config: process_sink.retention_days must be >= 0, got %d", c.ProcessSink.RetentionDays)
	}

	if c.Usage.SoftWarningTokens < 0 {
		return fmt.Errorf("config: usage.soft_warning_tokens must be >= 0, got %d", c.Usage.SoftWarningTokens)
	}

	return nil
}

func Default(workDir string) Config {
	return Config{
		Paths: PathsConfig{
			WorkDir:        workDir,
			VaultRoot:      filepath.Join(workDir, "vault"),
			StateDir:       filepath.Join(workDir, "state"),
			AuditLogDir:    filepath.Join(workDir, "state", "audit"),
			ProcessSinkDir: filepath.Join(workDir, "vault", "09-\u8fc7\u7a0b\u6c89\u6dc0"),
		},
		Vault: VaultConfig{
			DebounceWindow: 500 * time.Millisecond,
			TempSuffix:     ".obsidian-harness-tmp",
			ManagedCore: model.ManagedCorePaths{
				SystemDoc:     filepath.Join("00-\u7cfb\u7edf", "\u7cfb\u7edf\u8bf4\u660e.md"),
				ProgressIndex: filepath.Join("0-\u6392\u671f", "00-\u7cfb\u7edf", "\u6587\u6863\u8fdb\u5ea6\u603b\u8868.md"),
				Persona:       filepath.Join("03-\u753b\u50cf", "\u4eba\u7269\u753b\u50cf.md"),
				AgentDoc:      "agent.md",
				IdentityDoc:   "identity.md",
			},
			Resolve: VaultResolveConfig{
				UniqueScoreThreshold: 0.9,
				UniqueScoreMargin:    0.3,
			},
		},
		Runtime: RuntimeConfig{
			ListenAddress: "127.0.0.1:47231",
			MaxEventQueue: 256,
			MaxDraftQueue: 64,
			MaxMemoryMB:   256,
			ProactiveMode: "active",
			InspectAt:     "09:00",
		},
		ProcessSink: ProcessSinkConfig{
			CheckpointEvery: 30 * time.Minute,
			DailyRollupAt:   "23:30",
			RetentionDays:   14,
			WriteEmptySlots: true,
		},
		Usage: UsageConfig{
			TrackUsage:        true,
			SoftWarningTokens: 0,
		},
		Bootstrap: BootstrapConfig{
			ManagedModeRequired: true,
			ScaffoldIfMissing:   true,
		},
	}
}
