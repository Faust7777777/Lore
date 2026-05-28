package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/orchestrator"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/sqlitestore"
	"obsidian-harness/internal/vault"
)

// personaExtractLogFile is the workdir-local path (relative to
// StateDir) that OpenRuntime opens for persona-extraction failure
// logging. Operators tail this file while running real-world sessions
// to see why extraction produced zero candidates -- LLM error, parser
// refusal, or store write error. The file is append-only and never
// rotated by lore itself; sessions are bounded enough that growth is
// negligible (one line per failure, not per turn).
const personaExtractLogFile = "logs/persona-extract.log"

// personaExtractTimeoutEnv overrides the default 8 second per-call
// extraction timeout. Accepts any time.ParseDuration syntax (e.g.
// "12s", "1m"). Invalid values are ignored with no error -- the
// console / TUI shells fall back to defaultPersonaExtractTimeout --
// because a malformed env value should never break a chat session.
const personaExtractTimeoutEnv = "LORE_LLM_PERSONA_EXTRACT_TIMEOUT"

type Runtime struct {
	Config                   config.Config
	ConfigDiagnostics        []config.LoadDiagnostic
	configLoadOptions        config.LoadOptions
	LLMDiagnostics           []config.LLMDiagnostic
	OperatorAgent            operatoragent.Agent
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
	// PersonaExtractLogger is the io.Writer the console / TUI shells
	// hand to Session.PersonaExtractLogger so fire-and-forget extraction
	// failures (LLM error, parser refusal, store write error) land in
	// a file the operator can `tail`. OpenRuntime points this at
	// <StateDir>/logs/persona-extract.log when the file opens cleanly;
	// open failures leave it nil so extraction still runs silently
	// instead of breaking the chat. Tests can override the field
	// directly to inject a bytes.Buffer.
	PersonaExtractLogger io.Writer
	// personaExtractLogCloser holds the *os.File when OpenRuntime owns
	// the underlying file handle, so Runtime.Close can release it. Nil
	// when PersonaExtractLogger was injected by a test or when the
	// open failed.
	personaExtractLogCloser io.Closer
	// PersonaExtractTimeout caps a single fire-and-forget extraction.
	// Sourced from LORE_LLM_PERSONA_EXTRACT_TIMEOUT (any time.ParseDuration
	// syntax) when OpenRuntime constructs the runtime. Zero or invalid
	// env values leave this zero so the Session falls through to
	// defaultPersonaExtractTimeout. Console / TUI shells wire this onto
	// Session.PersonaExtractTimeout alongside the extractor itself.
	PersonaExtractTimeout time.Duration
	// PersonaExtractProvider / PersonaExtractModel / PersonaExtractBaseURL
	// snapshot the resolved LLM identity used to build PersonaExtractor
	// so the console / TUI shell can stamp them onto persona-extract.log
	// failure lines. Empty when no LLM is configured. The API key is
	// deliberately not surfaced here -- the log file lives in workdir
	// and must stay key-free.
	PersonaExtractProvider string
	PersonaExtractModel    string
	PersonaExtractBaseURL  string
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
	llmRuntime := resolveRuntimeLLM(cfg, diagnostics)
	processSinkSummarizer, processSinkErr := defaultProcessSinkSummarizer(llmRuntime.byPurpose[config.LLMPurposeProcessSink], llmRuntime.errors[config.LLMPurposeProcessSink])
	personaExtractor, personaErr := defaultPersonaExtractor(llmRuntime.byPurpose[config.LLMPurposePersonaExtract], llmRuntime.errors[config.LLMPurposePersonaExtract])
	personaCfg := llmRuntime.byPurpose[config.LLMPurposePersonaExtract]
	personaLogger, personaLogCloser := openPersonaExtractLog(cfg.Paths.StateDir)
	runtime := &Runtime{
		Config:                   cfg,
		ConfigDiagnostics:        diagnostics,
		configLoadOptions:        opts,
		LLMDiagnostics:           llmRuntime.diagnostics,
		OperatorAgent:            llmRuntime.operatorAgent,
		Harness:                  h,
		Store:                    st,
		ProcessSinkSummarizer:    processSinkSummarizer,
		processSinkSummarizerErr: processSinkErr,
		PersonaExtractor:         personaExtractor,
		personaExtractorErr:      personaErr,
		PersonaExtractLogger:     personaLogger,
		personaExtractLogCloser:  personaLogCloser,
		PersonaExtractTimeout:    parsePersonaExtractTimeoutEnv(),
		PersonaExtractProvider:   personaExtractIdentity(personaCfg, personaErr, llmProvider),
		PersonaExtractModel:      personaExtractIdentity(personaCfg, personaErr, func(c config.ResolvedLLMConfig) string { return c.Model }),
		PersonaExtractBaseURL:    personaExtractIdentity(personaCfg, personaErr, func(c config.ResolvedLLMConfig) string { return config.SanitizeLLMBaseURL(c.BaseURL) }),
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
	if r == nil {
		return nil
	}
	if r.personaExtractLogCloser != nil {
		// Best-effort close. We deliberately swallow the error here:
		// the persona log is observability-only, and a failing close
		// on a fire-and-forget writer should not mask a real store
		// close failure that the caller cares about.
		_ = r.personaExtractLogCloser.Close()
		r.personaExtractLogCloser = nil
	}
	if r.Store == nil {
		return nil
	}
	if closer, ok := any(r.Store).(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// PersonaExtractLogPath returns the workdir-local path of the
// persona extraction failure log file -- the same file OpenRuntime
// opens for write through PersonaExtractLogger. Read-side callers
// (e.g. `lore persona errors`) use this to locate the file without
// duplicating the path layout. Returns "" when the runtime or its
// config is not initialized.
func (r *Runtime) PersonaExtractLogPath() string {
	if r == nil || r.Config.Paths.StateDir == "" {
		return ""
	}
	return filepath.Join(r.Config.Paths.StateDir, personaExtractLogFile)
}

// openPersonaExtractLog opens the workdir-local log file the console
// fire-and-forget extractor writes failure lines to. Returns
// (nil, nil) when the directory cannot be created or the file cannot
// be opened so the runtime stays usable without observability rather
// than refusing to boot. Callers must close the returned io.Closer
// (Runtime.Close does this).
func openPersonaExtractLog(stateDir string) (io.Writer, io.Closer) {
	if stateDir == "" {
		return nil, nil
	}
	dir := filepath.Join(stateDir, filepath.Dir(personaExtractLogFile))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil
	}
	path := filepath.Join(stateDir, personaExtractLogFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil
	}
	return f, f
}

// parsePersonaExtractTimeoutEnv reads LORE_LLM_PERSONA_EXTRACT_TIMEOUT
// and returns the parsed duration, or zero when unset / invalid /
// non-positive. The Session falls back to defaultPersonaExtractTimeout
// when this is zero, so a malformed env value behaves identically to
// "env not set" -- safer than failing the runtime open.
func parsePersonaExtractTimeoutEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(personaExtractTimeoutEnv))
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0
	}
	return d
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
		Content:       "- source: `demo`\n- transcript events: 4\n\n## User Inputs\n- inspect the latest managed progress\n- apply the governed demo-week draft after review\n\n## Agent Outputs\n- [commentary] verified managed core docs are ready\n- [final] approved and applied the demo-week governance draft, then prepared the process-sink checkpoint",
		RawTranscript: "{\"role\":\"user\",\"message\":\"inspect the latest managed progress\"}\n{\"role\":\"assistant\",\"phase\":\"commentary\",\"message\":\"verified managed core docs are ready\"}\n{\"role\":\"user\",\"message\":\"apply the governed demo-week draft after review\"}\n{\"role\":\"assistant\",\"phase\":\"final\",\"message\":\"approved and applied the demo-week governance draft, then prepared the process-sink checkpoint\"}",
		EventCount:    4,
	}
	title, content, err := summarizer.SummarizeCheckpoint(windowSummary)
	if err != nil {
		return DemoP0BResult{}, err
	}
	if strings.TrimSpace(content) == "" && strings.TrimSpace(windowSummary.Content) != "" {
		title = strings.TrimSpace(title)
		if title == "" {
			title = fmt.Sprintf("%s checkpoint %s-%s", window.AgentID, window.WindowStart.Format("15:04"), window.WindowEnd.Format("15:04"))
		}
		content = windowSummary.Content
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
