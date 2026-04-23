package cli

import (
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
		session.LastToolTrace,
		session.History,
		localExec,
		shellEnabled,
		lastOutput,
	), nil
}

func (d interactiveWorkbenchDriver) Load(lastOutput string) (tui.WorkbenchViewModel, error) {
	return loadWorkbenchViewModel(d.version, d.runtime, d.session, d.agentID, d.day, d.localExec, d.shellEnabled, lastOutput)
}

func (d interactiveWorkbenchDriver) Execute(line string, lastOutput string) (tui.InteractiveWorkbenchUpdate, error) {
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
		output, err := d.session.Handle(line, d.runtime)
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
