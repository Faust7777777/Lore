package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/console"
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

	return tui.NewWorkbenchViewModel(
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
	), nil
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
		lastOutput = tui.RenderManagedStatus(d.version, managed)
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
