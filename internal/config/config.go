package config

import (
	"path/filepath"
	"time"

	"obsidian-harness/internal/model"
)

type Config struct {
	Paths       PathsConfig       `json:"paths"`
	Vault       VaultConfig       `json:"vault"`
	Runtime     RuntimeConfig     `json:"runtime"`
	ProcessSink ProcessSinkConfig `json:"process_sink"`
	Usage       UsageConfig       `json:"usage"`
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

type BootstrapConfig struct {
	ManagedModeRequired bool `json:"managed_mode_required"`
	ScaffoldIfMissing   bool `json:"scaffold_if_missing"`
}

func Default(workDir string) Config {
	return Config{
		Paths: PathsConfig{
			WorkDir:        workDir,
			VaultRoot:      filepath.Join(workDir, "vault"),
			StateDir:       filepath.Join(workDir, "state"),
			AuditLogDir:    filepath.Join(workDir, "state", "audit"),
			ProcessSinkDir: filepath.Join(workDir, "vault", "09-过程沉淀"),
		},
		Vault: VaultConfig{
			DebounceWindow: 500 * time.Millisecond,
			TempSuffix:     ".obsidian-harness-tmp",
			ManagedCore: model.ManagedCorePaths{
				SystemDoc:     filepath.Join("00-系统", "系统说明.md"),
				ProgressIndex: filepath.Join("0-排期", "00-系统", "文档进度总表.md"),
				Persona:       filepath.Join("03-画像", "人物画像.md"),
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
