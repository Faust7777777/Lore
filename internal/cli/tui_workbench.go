package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/console"
	"obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"
	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/tui"
)

type interactiveWorkbenchDriver struct {
	version      string
	runtime      console.Runtime
	session      *console.Session
	agentID      string
	day          time.Time
	localExec    bool
	shellEnabled bool
}

type llmConfigRuntime interface {
	ResolveLLMConfig(purpose string) (config.ResolvedLLMConfig, error)
}

type llmProfileRuntime interface {
	ListLLMProfiles() ([]app.LLMProfileView, error)
	UpsertLLMProfile(profileName string, profile config.LLMProfileConfig) error
	SetActiveLLMProfile(profileName string) error
}

type operatorAgentModelBuilder interface {
	BuildOperatorAgentForModel(profileName string, modelName string) (operatoragent.Agent, error)
}

type personaExtractorModelBuilder interface {
	BuildPersonaExtractorForModel(profileName string, modelName string) (app.PersonaExtractorBinding, error)
}

func loadWorkbenchViewModel(version string, runtime console.Runtime, session *console.Session, agentID string, day time.Time, localExec bool, shellEnabled bool, lastOutput string) (tui.WorkbenchViewModel, error) {
	managed, err := runtime.ManagedStatus()
	if err != nil {
		return tui.WorkbenchViewModel{}, err
	}

	drafts, err := runtime.ListDrafts()
	if err != nil {
		return tui.WorkbenchViewModel{}, err
	}

	processSink, err := runtime.ProcessSinkDay(agentID, day)
	if err != nil {
		return tui.WorkbenchViewModel{}, err
	}

	findings, err := runtime.ListFindings(64)
	if err != nil {
		return tui.WorkbenchViewModel{}, err
	}

	var focusedReview *app.DraftReview
	if strings.TrimSpace(session.CurrentDraftID) != "" {
		review, err := runtime.ReviewDraft(session.CurrentDraftID)
		if err == nil {
			focusedReview = &review
		}
	}
	candidates, err := loadWorkbenchPersonaCandidates(runtime)
	if err != nil {
		return tui.WorkbenchViewModel{}, err
	}

	vm := tui.NewWorkbenchViewModel(
		version,
		managed,
		drafts,
		processSink,
		focusedReview,
		findings,
		session.LastTurnSteps,
		session.LastToolTrace,
		session.History,
		localExec,
		shellEnabled,
		lastOutput,
		session.TranscriptInfo().SessionID,
		session.TranscriptInfo().Path,
	)
	vm.Snapshot.CurrentModel = modelLabel(session)
	vm.CandidateList = candidates
	vm.PendingActions = session.PendingActions()
	vm.Snapshot.PendingActions = len(vm.PendingActions)
	// Load LLM identity for status panel (key-safe projections)
	vm.ChatModelIdentity = loadLLMIdentity(runtime, config.LLMPurposeOperator)
	vm.PersonaModelIdentity = loadLLMIdentity(runtime, config.LLMPurposePersonaExtract)
	vm.SinkModelIdentity = loadLLMIdentity(runtime, config.LLMPurposeProcessSink)
	return vm, nil
}

func loadLLMIdentity(runtime console.Runtime, purpose string) *app.LLMIdentity {
	rt, ok := runtime.(*app.Runtime)
	if !ok || rt == nil {
		return nil
	}
	identity, err := rt.LLMIdentity(purpose)
	if err != nil {
		return nil
	}
	return &identity
}

// modelLabel returns the active model name from the session's agent.
func modelLabel(session *console.Session) string {
	if session == nil || session.Agent == nil {
		return ""
	}
	if ma, ok := session.Agent.(interface{ CurrentModel() string }); ok {
		return ma.CurrentModel()
	}
	return ""
}

func (d interactiveWorkbenchDriver) Load(lastOutput string) (tui.WorkbenchViewModel, error) {
	return loadWorkbenchViewModel(d.version, d.runtime, d.session, d.agentID, d.day, d.localExec, d.shellEnabled, lastOutput)
}

func (d interactiveWorkbenchDriver) Execute(line string, lastOutput string) (tui.InteractiveWorkbenchUpdate, error) {
	return d.ExecuteContext(context.Background(), line, lastOutput)
}

func (d interactiveWorkbenchDriver) ExecuteContext(ctx context.Context, line string, lastOutput string) (tui.InteractiveWorkbenchUpdate, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	line = strings.TrimSpace(line)
	switch strings.ToLower(line) {
	case "", "/refresh":
		viewModel, err := d.Load(lastOutput)
		return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput}, err
	case "/quit", "/exit":
		viewModel, err := d.Load(lastOutput)
		return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput, Quit: true}, err
	case "/status":
		managed, err := d.runtime.ManagedStatus()
		if err != nil {
			return tui.InteractiveWorkbenchUpdate{}, err
		}
		lastOutput = tui.RenderManagedStatus(d.version, managed) + runtimeDiagnostics(d.runtime)
		viewModel, err := d.Load(lastOutput)
		return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput}, err
	case "/drafts":
		drafts, err := d.runtime.ListDrafts()
		if err != nil {
			return tui.InteractiveWorkbenchUpdate{}, err
		}
		lastOutput = tui.RenderDraftList(drafts)
		viewModel, err := d.Load(lastOutput)
		return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput}, err
	default:
		output, err := d.session.HandleContext(ctx, line, d.runtime)
		if err != nil {
			lastOutput = "Error: " + err.Error()
			viewModel, loadErr := d.Load(lastOutput)
			return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput}, loadErr
		}
		lastOutput = output
		viewModel, err := d.Load(lastOutput)
		return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput}, err
	}
}

// --- Model management ---

func (d interactiveWorkbenchDriver) DiscoverModels() ([]tui.ModelInfo, error) {
	cfg, err := d.resolveOperatorLLMConfig()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	catalog, err := operatoragent.DiscoverModels(ctx, operatorEnvConfig(cfg))
	if err != nil {
		current := modelLabel(d.session)
		if current == "" {
			current = strings.TrimSpace(cfg.Model)
		}
		displayBaseURL := config.SanitizeLLMBaseURL(cfg.BaseURL)
		return []tui.ModelInfo{{
			Name:       current,
			Provider:   modelPanelProvider(cfg.Provider, current),
			BaseURL:    displayBaseURL,
			Source:     string(cfg.Source),
			KeyStatus:  keyStatus(cfg.APIKey),
			TestStatus: "discovery failed: " + err.Error(),
			Current:    true,
		}}, nil
	}
	current := modelLabel(d.session)
	result := make([]tui.ModelInfo, 0, len(catalog.Models))
	displayBaseURL := config.SanitizeLLMBaseURL(cfg.BaseURL)
	for _, m := range catalog.Models {
		result = append(result, tui.ModelInfo{
			Name:      m,
			Provider:  modelPanelProvider(cfg.Provider, m),
			BaseURL:   displayBaseURL,
			Source:    string(cfg.Source),
			KeyStatus: keyStatus(cfg.APIKey),
			Current:   m == current,
		})
	}
	return result, nil
}

func (d interactiveWorkbenchDriver) SwitchModel(name string) ([]tui.ModelInfo, error) {
	cfg, err := d.resolveOperatorLLMConfig()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if d.session == nil {
		return nil, fmt.Errorf("session is required")
	}
	agent, personaBinding, err := d.buildModelSwitchBindings(cfg.Profile, name, cfg)
	if err != nil {
		return nil, err
	}
	d.session.DrainPersonaExtractions(consolePersonaDrainTimeout)
	d.session.Agent = agent
	if personaBinding != nil {
		d.session.PersonaExtractor = personaBinding.Extractor
		d.session.PersonaExtractModelInfo = console.PersonaExtractModelInfo{
			Provider: personaBinding.Provider,
			Model:    personaBinding.Model,
			BaseURL:  personaBinding.BaseURL,
		}
	}
	return d.DiscoverModels()
}

func (d interactiveWorkbenchDriver) TestModel(name string) error {
	cfg, err := d.resolveOperatorLLMConfig()
	if err != nil {
		return err
	}
	client, err := openai.NewClient(openai.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   name,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	// Ping the model with a minimal completion
	_, err = client.ChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Messages:    []openai.Message{{Role: "user", Content: "ping"}},
		MaxTokens:   1,
		Temperature: 0,
	})
	return err
}

func (d interactiveWorkbenchDriver) resolveOperatorLLMConfig() (config.ResolvedLLMConfig, error) {
	if resolver, ok := d.runtime.(llmConfigRuntime); ok && resolver != nil {
		cfg, err := resolver.ResolveLLMConfig(config.LLMPurposeOperator)
		if err != nil {
			return cfg, fmt.Errorf("load config: %w", err)
		}
		if !cfg.Enabled {
			return cfg, fmt.Errorf("LLM config not configured (set llm.active_profile in .lore/config.json or LORE_LLM_BASE_URL + LORE_LLM_API_KEY + LORE_LLM_MODEL)")
		}
		return cfg, nil
	}
	workDir := ""
	if d.runtime != nil {
		workDir = d.runtime.WorkDirPath()
	}
	cfg, err := config.ResolveLLMConfig(workDir, config.LLMPurposeOperator)
	if err != nil {
		return cfg, fmt.Errorf("load config: %w", err)
	}
	if !cfg.Enabled {
		return cfg, fmt.Errorf("LLM config not configured (set llm.active_profile in .lore/config.json or LORE_LLM_BASE_URL + LORE_LLM_API_KEY + LORE_LLM_MODEL)")
	}
	return cfg, nil
}

func (d interactiveWorkbenchDriver) buildModelSwitchBindings(profileName string, modelName string, cfg config.ResolvedLLMConfig) (operatoragent.Agent, *app.PersonaExtractorBinding, error) {
	if builder, ok := d.runtime.(operatorAgentModelBuilder); ok && builder != nil {
		agent, err := builder.BuildOperatorAgentForModel(profileName, modelName)
		if err != nil {
			return nil, nil, fmt.Errorf("switch operator agent: %w", err)
		}
		personaBinding, err := d.buildPersonaExtractorForSwitch(profileName, modelName)
		if err != nil {
			return nil, nil, err
		}
		return agent, personaBinding, nil
	}
	client, err := openai.NewClient(openai.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   modelName,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create client: %w", err)
	}
	personaBinding, err := d.buildPersonaExtractorForSwitch(profileName, modelName)
	if err != nil {
		return nil, nil, err
	}
	agent := operatoragent.NewModelAgent(client, modelPanelProvider(cfg.Provider, modelName), modelName)
	return agent, personaBinding, nil
}

func operatorEnvConfig(cfg config.ResolvedLLMConfig) operatoragent.EnvConfig {
	return operatoragent.EnvConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}
}

func (d interactiveWorkbenchDriver) buildPersonaExtractorForSwitch(profileName string, modelName string) (*app.PersonaExtractorBinding, error) {
	if builder, ok := d.runtime.(personaExtractorModelBuilder); ok && builder != nil {
		binding, err := builder.BuildPersonaExtractorForModel(profileName, modelName)
		if err != nil {
			return nil, err
		}
		return &binding, nil
	}
	return nil, nil
}

func runtimeDiagnostics(runtime console.Runtime) string {
	rt, ok := runtime.(*app.Runtime)
	if !ok || rt == nil {
		return ""
	}
	return tui.RenderConfigLayers(rt.ConfigDiagnostics) + renderLLMConfigDiagnostics(rt.LLMDiagnostics)
}

func keyStatus(apiKey string) string {
	if strings.TrimSpace(apiKey) == "" {
		return "missing"
	}
	return "OK"
}

func keyStatusFromEnvName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.TrimSpace(os.Getenv(name)) == "" {
		return "missing"
	}
	return "present"
}

func modelPanelProvider(configuredProvider string, modelName string) string {
	if strings.TrimSpace(configuredProvider) != "" {
		return strings.TrimSpace(configuredProvider)
	}
	return inferProvider(modelName)
}

func inferProvider(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "deepseek") {
		return "deepseek"
	}
	if strings.Contains(lower, "gpt") || strings.Contains(lower, "o1") || strings.Contains(lower, "o3") || strings.Contains(lower, "o4") {
		return "openai"
	}
	return "openai"
}

func (d interactiveWorkbenchDriver) ExecuteApprovalAction(action string, draftID string) (tui.InteractiveWorkbenchUpdate, error) {
	var lastOutput string
	switch action {
	case "approve":
		draft, err := d.runtime.ApproveDraft(draftID)
		if err != nil {
			return tui.InteractiveWorkbenchUpdate{}, err
		}
		lastOutput = fmt.Sprintf("Draft %s approved (%s). Use /drafts to review or ask Lore to apply it.", shortID(draft.ID, 8), draft.Kind)
	case "reject":
		draft, err := d.runtime.RejectDraft(draftID)
		if err != nil {
			return tui.InteractiveWorkbenchUpdate{}, err
		}
		lastOutput = fmt.Sprintf("Draft %s rejected (%s).", shortID(draft.ID, 8), draft.Kind)
	case "apply":
		draft, err := d.runtime.ApplyDraft(draftID)
		if err != nil {
			return tui.InteractiveWorkbenchUpdate{}, err
		}
		lastOutput = fmt.Sprintf("Draft %s applied (%s -> %s).", shortID(draft.ID, 8), draft.Kind, draft.Target.Path)
	default:
		return tui.InteractiveWorkbenchUpdate{}, fmt.Errorf("unknown approval action: %s", action)
	}
	viewModel, err := d.Load(lastOutput)
	return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput}, err
}

func (d interactiveWorkbenchDriver) ExecuteFindingAction(action string, findingID string) (tui.InteractiveWorkbenchUpdate, error) {
	var lastOutput string
	switch action {
	case "resolve":
		finding, err := d.runtime.ResolveFinding(findingID)
		if err != nil {
			return tui.InteractiveWorkbenchUpdate{}, err
		}
		lastOutput = fmt.Sprintf("Finding %s resolved.", shortID(finding.ID, 8))
	case "ignore":
		finding, err := d.runtime.IgnoreFinding(findingID)
		if err != nil {
			return tui.InteractiveWorkbenchUpdate{}, err
		}
		lastOutput = fmt.Sprintf("Finding %s ignored.", shortID(finding.ID, 8))
	default:
		return tui.InteractiveWorkbenchUpdate{}, fmt.Errorf("unknown finding action: %s", action)
	}
	viewModel, err := d.Load(lastOutput)
	return tui.InteractiveWorkbenchUpdate{ViewModel: viewModel, LastOutput: lastOutput}, err
}

func (d interactiveWorkbenchDriver) ListProfiles() ([]tui.ModelInfo, error) {
	profileRuntime, ok := d.runtime.(llmProfileRuntime)
	if !ok || profileRuntime == nil {
		return nil, fmt.Errorf("llm profile runtime is not available")
	}
	profiles, err := profileRuntime.ListLLMProfiles()
	if err != nil {
		return nil, err
	}
	rows := make([]tui.ModelInfo, 0, len(profiles))
	for _, profile := range profiles {
		rows = append(rows, tui.ModelInfo{
			Name:        profile.Model,
			Provider:    modelPanelProvider(profile.Provider, profile.Model),
			BaseURL:     config.SanitizeLLMBaseURL(profile.BaseURL),
			Source:      "workspace",
			KeyStatus:   keyStatusFromEnvName(profile.APIKeyEnv),
			Current:     profile.Active && profile.Model == modelLabel(d.session),
			ProfileName: profile.Name,
			IsProfile:   true,
			Active:      profile.Active,
			APIKeyEnv:   strings.TrimSpace(profile.APIKeyEnv),
		})
	}
	return rows, nil
}

func (d interactiveWorkbenchDriver) CreateProfileFromPreset(presetName string, profileName string) error {
	profileRuntime, ok := d.runtime.(llmProfileRuntime)
	if !ok || profileRuntime == nil {
		return fmt.Errorf("llm profile runtime is not available")
	}
	profile, err := config.LLMProfileFromPreset(presetName, config.LLMProfileConfig{})
	if err != nil {
		return err
	}
	if strings.TrimSpace(profileName) == "" {
		profileName = strings.TrimSpace(presetName)
	}
	return profileRuntime.UpsertLLMProfile(profileName, profile)
}

func (d interactiveWorkbenchDriver) PersistActiveProfile(profileName string) error {
	profileRuntime, ok := d.runtime.(llmProfileRuntime)
	if !ok || profileRuntime == nil {
		return fmt.Errorf("llm profile runtime is not available")
	}
	if d.session == nil {
		return fmt.Errorf("session is required")
	}
	// Pre-flight: resolve the profile and build bindings BEFORE writing to disk.
	// If resolution or build fails, workspace config stays on the old profile.
	cfg, err := config.ResolveLLMProfileConfig(d.runtime.WorkDirPath(), config.LLMPurposeOperator, profileName)
	if err != nil {
		return fmt.Errorf("persist: cannot resolve profile %q: %w", profileName, err)
	}
	agent, personaBinding, err := d.buildModelSwitchBindings(profileName, cfg.Model, cfg)
	if err != nil {
		return fmt.Errorf("persist: cannot build agent for profile %q: %w", profileName, err)
	}
	// Build succeeded — now it is safe to persist.
	if err := profileRuntime.SetActiveLLMProfile(profileName); err != nil {
		return err
	}
	// Hot-switch session to the new profile.
	d.session.DrainPersonaExtractions(consolePersonaDrainTimeout)
	d.session.Agent = agent
	if personaBinding != nil {
		d.session.PersonaExtractor = personaBinding.Extractor
		d.session.PersonaExtractModelInfo = console.PersonaExtractModelInfo{
			Provider: personaBinding.Provider,
			Model:    personaBinding.Model,
			BaseURL:  personaBinding.BaseURL,
		}
	}
	return nil
}

func (d interactiveWorkbenchDriver) AvailablePresets() []tui.ModelProfilePreset {
	presets := config.BuiltinLLMProfilePresets()
	result := make([]tui.ModelProfilePreset, 0, len(presets))
	for _, p := range presets {
		result = append(result, tui.ModelProfilePreset{
			Name:        p.Name,
			Provider:    p.Provider,
			BaseURL:     p.BaseURL,
			Model:       p.Model,
			APIKeyEnv:   p.APIKeyEnv,
			Description: p.Description,
		})
	}
	return result
}

// --- Persona candidate management ---

type personaCandidateRuntime interface {
	ListPersonaCandidates(state persona.PersonaCandidateState, limit int) ([]persona.PersonaCandidateRecord, error)
	GetPersonaCandidateView(id string) (app.PersonaCandidateView, error)
	CreatePersonaDraftFromCandidate(id string, now time.Time) (persona.PersonaCandidateRecord, model.PersonaUpdateProposalResult, error)
	DismissPersonaCandidate(id string, now time.Time) (persona.PersonaCandidateRecord, error)
	ForceDismissPartialPersonaCandidate(id string, now time.Time) (persona.PersonaCandidateRecord, error)
	RetryRejectedPersonaDraft(id string, now time.Time) (persona.PersonaCandidateRecord, model.PersonaUpdateProposalResult, error)
	PersonaCandidateActions(rec persona.PersonaCandidateRecord) app.PersonaCandidateActions
}

func loadWorkbenchPersonaCandidates(runtime console.Runtime) ([]tui.PersonaCandidateInfo, error) {
	rt, ok := runtime.(personaCandidateRuntime)
	if !ok || rt == nil {
		return nil, nil
	}
	records, err := rt.ListPersonaCandidates(persona.PersonaCandidateOpen, 64)
	if err != nil {
		return nil, err
	}
	return convertCandidateRecords(rt, records), nil
}

func (d interactiveWorkbenchDriver) ListPersonaCandidates() ([]tui.PersonaCandidateInfo, error) {
	rt, ok := d.runtime.(personaCandidateRuntime)
	if !ok || rt == nil {
		return nil, fmt.Errorf("persona candidate management requires app runtime")
	}
	records, err := rt.ListPersonaCandidates(persona.PersonaCandidateOpen, 64)
	if err != nil {
		return nil, err
	}
	// Also include drafted/dismissed for visibility — but surface errors
	// instead of silently degrading to partial lists (user-facing surface).
	drafted, draftErr := rt.ListPersonaCandidates(persona.PersonaCandidateDrafted, 64)
	dismissed, dismissErr := rt.ListPersonaCandidates(persona.PersonaCandidateDismissed, 64)
	if draftErr != nil {
		return nil, fmt.Errorf("list drafted candidates: %w", draftErr)
	}
	if dismissErr != nil {
		return nil, fmt.Errorf("list dismissed candidates: %w", dismissErr)
	}
	records = append(records, drafted...)
	records = append(records, dismissed...)
	return convertCandidateRecords(rt, records), nil
}

func (d interactiveWorkbenchDriver) DraftPersonaCandidate(id string) ([]tui.PersonaCandidateInfo, error) {
	rt, ok := d.runtime.(personaCandidateRuntime)
	if !ok || rt == nil {
		return nil, fmt.Errorf("persona candidate management requires app runtime")
	}
	_, _, err := rt.CreatePersonaDraftFromCandidate(id, time.Now())
	if err != nil {
		return nil, err
	}
	return d.ListPersonaCandidates()
}

func (d interactiveWorkbenchDriver) DismissPersonaCandidate(id string) ([]tui.PersonaCandidateInfo, error) {
	rt, ok := d.runtime.(personaCandidateRuntime)
	if !ok || rt == nil {
		return nil, fmt.Errorf("persona candidate management requires app runtime")
	}
	// The Dismiss action means "abandon this candidate". For partial-orphan
	// (drafted+empty DraftID), the B-line says Dismiss should route through
	// ForceDismissPartialPersonaCandidate, not the normal dismiss path.
	// Use GetPersonaCandidateView to get the authoritative Actions.
	view, err := rt.GetPersonaCandidateView(id)
	if err != nil {
		return nil, err
	}
	if view.State == persona.PersonaCandidateDrafted && view.DraftID == "" {
		_, err = rt.ForceDismissPartialPersonaCandidate(id, time.Now())
	} else {
		_, err = rt.DismissPersonaCandidate(id, time.Now())
	}
	if err != nil {
		return nil, err
	}
	return d.ListPersonaCandidates()
}

func (d interactiveWorkbenchDriver) RecoverPersonaCandidate(id string) ([]tui.PersonaCandidateInfo, error) {
	rt, ok := d.runtime.(personaCandidateRuntime)
	if !ok || rt == nil {
		return nil, fmt.Errorf("persona candidate management requires app runtime")
	}
	// Recover is only valid for non-orphan paths where the user provides
	// a draft ID via CLI (RecoverPersonaCandidateLink). For partial-orphan,
	// the TUI offers force-dismiss via the f key which routes through
	// DismissPersonaCandidate → ForceDismissPartialPersonaCandidate.
	view, err := rt.GetPersonaCandidateView(id)
	if err != nil {
		return nil, err
	}
	if view.State == persona.PersonaCandidateDrafted && view.DraftID == "" {
		return nil, fmt.Errorf("partial-orphan: use CLI to link (lore persona candidates recover %s --link <draft_id>) or press f to force-dismiss", id)
	}
	return nil, fmt.Errorf("recover unavailable: %s", view.Actions.RecoverReason)
}

func (d interactiveWorkbenchDriver) RetryPersonaCandidate(id string) ([]tui.PersonaCandidateInfo, error) {
	rt, ok := d.runtime.(personaCandidateRuntime)
	if !ok || rt == nil {
		return nil, fmt.Errorf("persona candidate management requires app runtime")
	}
	// B-line contract: CanRetry = true means the linked draft is in a
	// terminal rejected/expired/superseded state. Call RetryRejectedPersonaDraft.
	_, _, err := rt.RetryRejectedPersonaDraft(id, time.Now())
	if err != nil {
		return nil, err
	}
	return d.ListPersonaCandidates()
}

func convertCandidateRecords(rt personaCandidateRuntime, records []persona.PersonaCandidateRecord) []tui.PersonaCandidateInfo {
	result := make([]tui.PersonaCandidateInfo, 0, len(records))
	for _, r := range records {
		actions := rt.PersonaCandidateActions(r)
		result = append(result, tui.PersonaCandidateInfo{
			ID:            r.ID,
			State:         string(r.State),
			Field:         r.Candidate.Field,
			ProposedValue: r.Candidate.ProposedValue,
			CurrentValue:  r.Candidate.CurrentValue,
			EvidenceQuote: r.Candidate.EvidenceQuote,
			Reason:        r.Candidate.Reason,
			Confidence:    string(r.Candidate.Confidence),
			Conflict:      r.Candidate.Conflict,
			SourceKind:    string(r.Candidate.SourceKind),
			SourceSession: r.Candidate.SourceSessionID,
			ObservedAt:    r.Candidate.ObservedAt.Format("2006-01-02 15:04"),
			DraftID:       r.DraftID,
			DedupKey:      r.DedupKey,
			Actions: tui.PersonaCandidateActions{
				CanDraft:      actions.CanDraft,
				DraftReason:   actions.DraftReason,
				CanDismiss:    actions.CanDismiss,
				DismissReason: actions.DismissReason,
				CanRecover:    actions.CanRecover,
				RecoverReason: actions.RecoverReason,
				CanRetry:      actions.CanRetry,
				RetryReason:   actions.RetryReason,
			},
		})
	}
	return result
}

func (d interactiveWorkbenchDriver) ListErrors() ([]tui.ErrorEntry, error) {
	var errors []tui.ErrorEntry
	// Load persona extraction errors
	type personaErrorsLoader interface {
		ListPersonaExtractErrors(filter app.PersonaErrorsFilter) (app.PersonaExtractErrorsResult, error)
		PersonaExtractLogPath() string
	}
	if loader, ok := d.runtime.(personaErrorsLoader); ok && loader != nil {
		result, err := loader.ListPersonaExtractErrors(app.PersonaErrorsFilter{Tail: 10})
		if err == nil {
			for _, e := range result.Entries {
				errors = append(errors, tui.ErrorEntry{
					Time:    e.Timestamp.Format("2006-01-02 15:04"),
					Stage:   "persona_extract",
					Model:   e.Model,
					BaseURL: config.SanitizeLLMBaseURL(e.BaseURL),
					Error:   safeDiagnosticError(fmt.Errorf("%s", e.Error)),
				})
			}
		}
	}
	// Load LLM diagnostics for model errors
	type llmDiagnosticLoader interface {
		LLMIdentity(purpose string) (app.LLMIdentity, error)
	}
	if loader, ok := d.runtime.(llmDiagnosticLoader); ok && loader != nil {
		for _, purpose := range []string{config.LLMPurposeOperator, config.LLMPurposePersonaExtract, config.LLMPurposeProcessSink} {
			_, err := loader.LLMIdentity(purpose)
			if err != nil {
				errors = append(errors, tui.ErrorEntry{
					Time:  time.Now().Format("2006-01-02 15:04"),
					Stage: purpose,
					Error: safeDiagnosticError(err),
				})
			}
		}
	}
	return errors, nil
}

func shortID(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
