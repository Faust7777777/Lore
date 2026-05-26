package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/config"
	"obsidian-harness/internal/console"
	"obsidian-harness/internal/llm/openai"
	"obsidian-harness/internal/operatoragent"
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
	return vm, nil
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
		return []tui.ModelInfo{{
			Name:       current,
			Provider:   modelPanelProvider(cfg.Provider, current),
			BaseURL:    cfg.BaseURL,
			Source:     string(cfg.Source),
			KeyStatus:  keyStatus(cfg.APIKey),
			TestStatus: "discovery failed: " + err.Error(),
			Current:    true,
		}}, nil
	}
	current := modelLabel(d.session)
	result := make([]tui.ModelInfo, 0, len(catalog.Models))
	for _, m := range catalog.Models {
		result = append(result, tui.ModelInfo{
			Name:      m,
			Provider:  modelPanelProvider(cfg.Provider, m),
			BaseURL:   cfg.BaseURL,
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
	client, err := openai.NewClient(openai.Config{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   name,
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	personaBinding, err := d.buildPersonaExtractorForSwitch(name)
	if err != nil {
		return nil, err
	}
	agent := operatoragent.NewModelAgent(client, modelPanelProvider(cfg.Provider, name), name)
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

func operatorEnvConfig(cfg config.ResolvedLLMConfig) operatoragent.EnvConfig {
	return operatoragent.EnvConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}
}

func (d interactiveWorkbenchDriver) buildPersonaExtractorForSwitch(modelName string) (*app.PersonaExtractorBinding, error) {
	runtime, ok := d.runtime.(*app.Runtime)
	if !ok || runtime == nil {
		return nil, nil
	}
	binding, err := runtime.BuildPersonaExtractorForOperatorModel(modelName)
	if err != nil {
		return nil, fmt.Errorf("switch persona extractor: %w", err)
	}
	return &binding, nil
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

func modelPanelProvider(configuredProvider string, modelName string) string {
	if provider := strings.TrimSpace(configuredProvider); provider != "" {
		return provider
	}
	return inferProvider(modelName)
}

func inferProvider(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "deepseek") {
		return "deepseek"
	}
	if strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") {
		return "anthropic"
	}
	if strings.Contains(lower, "gpt") || strings.Contains(lower, "o1") || strings.Contains(lower, "o3") || strings.Contains(lower, "o4") {
		return "openai"
	}
	if strings.Contains(lower, "glm") || strings.Contains(lower, "chatglm") {
		return "zhipu"
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

func shortID(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
