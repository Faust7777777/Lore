package app

import (
	"fmt"
	"path/filepath"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/orchestrator"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/sqlitestore"
	"obsidian-harness/internal/vault"
)

type Runtime struct {
	Config                   config.Config
	ConfigDiagnostics        []config.LoadDiagnostic
	Harness                  *orchestrator.Harness
	Store                    store.StateStore
	ProcessSinkSummarizer    ProcessSinkSummarizer
	processSinkSummarizerErr error
	// PersonaExtractor mines persona update candidates from user
	// turns. nil when no LLM is configured for the runtime so console
	// callers can guard fire-and-forget extraction calls with a nil
	// check instead of branching on env state. Tests can override the
	// field after construction; OpenRuntime populates a default that
	// reads the same LORE_LLM_* env as the operator agent.
	PersonaExtractor    persona.PersonaCandidateExtractor
	personaExtractorErr error
}

type DemoP0BResult struct {
	Checkpoint model.CheckpointDoc
	Report     model.DailyReport
}

// OpenRuntime initializes the Lore runtime using the default layered config
// loader. It reads ~/.lore/config.json and <workDir>/.lore/config.json on top
// of the built-in defaults. Production callers should use this entrypoint.
//
// Tests that must isolate themselves from the developer's real
// ~/.lore/config.json should call OpenRuntimeWithConfigOptions with explicit
// override paths, or use the helpers in internal/config/configtest.
func OpenRuntime(workDir string) (*Runtime, error) {
	return OpenRuntimeWithConfigOptions(workDir, config.LoadOptions{})
}

// OpenRuntimeWithConfigOptions initializes the Lore runtime with explicit
// configuration loader options. This entrypoint exists so tests and
// embedders can disable user-global / workspace layer files by pointing
// the override paths at absent locations, avoiding pollution from the
// developer's real configuration.
func OpenRuntimeWithConfigOptions(workDir string, opts config.LoadOptions) (*Runtime, error) {
	cfg, diagnostics, err := config.LoadWithOptions(workDir, opts)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	statePath := filepath.Join(cfg.Paths.StateDir, "store.db")
	legacyStatePath := filepath.Join(cfg.Paths.StateDir, "store.json")
	st, err := sqlitestore.OpenWithJSONMigration(statePath, legacyStatePath)
	if err != nil {
		return nil, err
	}
	h, err := orchestrator.New(cfg, st)
	if err != nil {
		if closer, ok := any(st).(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		return nil, err
	}
	processSinkSummarizer, processSinkErr := defaultProcessSinkSummarizer()
	personaExtractor, personaErr := defaultPersonaExtractor()
	runtime := &Runtime{
		Config:                   cfg,
		ConfigDiagnostics:        diagnostics,
		Harness:                  h,
		Store:                    st,
		ProcessSinkSummarizer:    processSinkSummarizer,
		processSinkSummarizerErr: processSinkErr,
		PersonaExtractor:         personaExtractor,
		personaExtractorErr:      personaErr,
	}
	// Second-phase wiring: route summarizer cost records into the
	// runtime's usage store. The sink is a no-op for non-model-backed
	// summarizers (e.g. fakes used in tests), so tests that wire their
	// own summarizer via runtime.ProcessSinkSummarizer = ... still work.
	attachUsageSink(processSinkSummarizer, func(rec model.UsageRecord) error {
		return runtime.RecordUsage([]model.UsageRecord{rec})
	})
	// Mirror the same two-phase pattern for the persona extractor so
	// its UsageRecord (Purpose=persona_extract) flows into the same
	// store as chat and process-sink usage.
	attachPersonaExtractorUsageSink(personaExtractor, func(rec model.UsageRecord) error {
		return runtime.RecordUsage([]model.UsageRecord{rec})
	})
	return runtime, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.Store == nil {
		return nil
	}
	if closer, ok := any(r.Store).(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (r *Runtime) Bootstrap(now time.Time) ([]model.DocumentRef, error) {
	return r.Harness.BootstrapManagedVault(now)
}

func (r *Runtime) DemoP0A(now time.Time) (model.Draft, error) {
	r.Harness.UpdateDependencies(true, true)
	if _, err := r.Bootstrap(now); err != nil {
		return model.Draft{}, err
	}

	relPath := filepath.Join("0-\u6392\u671f", "04-\u6267\u884c", "demo-week.md")
	absPath := filepath.Join(r.Config.Paths.VaultRoot, relPath)
	if _, err := vault.WriteFileAtomic(absPath, []byte("# Demo Week\n\n- [x] learn SQL triggers"), r.Config.Vault.TempSuffix); err != nil {
		return model.Draft{}, err
	}

	draft, err := r.Harness.ObserveDocumentChange(relPath, []byte("learn SQL triggers"), now)
	if err != nil {
		return model.Draft{}, err
	}
	if _, err := r.Harness.ApproveDraft(draft.ID, now.Add(time.Minute)); err != nil {
		return model.Draft{}, err
	}
	return r.Harness.ApplyDraft(draft.ID, now.Add(2*time.Minute))
}

func (r *Runtime) DemoP0B(now time.Time) (DemoP0BResult, error) {
	r.Harness.UpdateDependencies(true, true)
	summarizer, err := r.requireProcessSinkSummarizer()
	if err != nil {
		return DemoP0BResult{}, err
	}

	window := model.SessionWindow{
		AgentID:     "codex",
		SessionID:   "demo-thread",
		WindowStart: now,
		WindowEnd:   now.Add(30 * time.Minute),
	}
	windowSummary := codexjsonl.WindowSummary{
		Window:        window,
		Content:       "- source: `demo`\n- transcript events: 2\n\n## User Inputs\n- inspect the latest managed progress\n\n## Agent Outputs\n- [commentary] prepared the next checkpoint",
		RawTranscript: "{\"role\":\"user\",\"message\":\"inspect the latest managed progress\"}\n{\"role\":\"assistant\",\"phase\":\"commentary\",\"message\":\"prepared the next checkpoint\"}",
		EventCount:    2,
	}
	title, content, err := summarizer.SummarizeCheckpoint(windowSummary)
	if err != nil {
		return DemoP0BResult{}, err
	}
	checkpoint, err := r.Harness.IngestSessionWindow(
		window,
		title,
		content,
		windowSummary.RawTranscript,
		window.WindowEnd,
	)
	if err != nil {
		return DemoP0BResult{}, err
	}
	report, err := r.rollupProcessSinkDay(window.AgentID, now, now.Add(12*time.Hour))
	if err != nil {
		return DemoP0BResult{}, err
	}
	return DemoP0BResult{
		Checkpoint: checkpoint,
		Report:     report,
	}, nil
}

func (r *Runtime) requireProcessSinkSummarizer() (ProcessSinkSummarizer, error) {
	if r.ProcessSinkSummarizer != nil {
		return r.ProcessSinkSummarizer, nil
	}
	if r.processSinkSummarizerErr != nil {
		return nil, r.processSinkSummarizerErr
	}
	return nil, fmt.Errorf("process sink summarizer: model-backed summarizer is required; configure LORE_LLM_BASE_URL and LORE_LLM_API_KEY (legacy OBSIDIAN_HARNESS_LLM_* also supported)")
}

func (r *Runtime) rollupProcessSinkDay(agentID string, day time.Time, at time.Time) (model.DailyReport, error) {
	summarizer, err := r.requireProcessSinkSummarizer()
	if err != nil {
		return model.DailyReport{}, err
	}

	checkpoints, err := r.Store.ProcessSink().ListCheckpointsByDay(agentID, day)
	if err != nil {
		return model.DailyReport{}, err
	}
	title, content, err := summarizer.SummarizeDaily(agentID, day, checkpoints)
	if err != nil {
		return model.DailyReport{}, err
	}
	return r.Harness.RollupDailyWithSummary(agentID, day, title, content, at)
}
