package tui

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"obsidian-harness/internal/model"
)

type InteractiveWorkbenchDriver interface {
	Load(lastOutput string) (WorkbenchViewModel, error)
	Execute(line string, lastOutput string) (InteractiveWorkbenchUpdate, error)
	ExecuteApprovalAction(action string, draftID string) (InteractiveWorkbenchUpdate, error)
	ExecuteFindingAction(action string, findingID string) (InteractiveWorkbenchUpdate, error)
	// Model management: discover available models and switch the session agent.
	DiscoverModels() ([]ModelInfo, error)
	SwitchModel(name string) ([]ModelInfo, error)
	TestModel(name string) error
}

type InteractiveWorkbenchContextDriver interface {
	InteractiveWorkbenchDriver
	ExecuteContext(ctx context.Context, line string, lastOutput string) (InteractiveWorkbenchUpdate, error)
}

type InteractiveWorkbenchUpdate struct {
	ViewModel  WorkbenchViewModel
	LastOutput string
	Quit       bool
}

// ModelInfo describes a single model entry for the TUI model panel.
// API key information MUST NOT appear in any rendered field.
type ModelInfo struct {
	Name       string // e.g. "gpt-5.4", "deepseek-v4-pro"
	Provider   string // e.g. "openai", "deepseek"
	BaseURL    string // e.g. "https://api.ikuncode.cc/v1" (never contains key)
	Source     string // "env", "discovered", "manual"
	KeyStatus  string // "OK", "missing" (never the key value)
	TestStatus string // "", "OK", "failed: reason"
	Current    bool
}

type interactiveFocus int

const (
	focusInput interactiveFocus = iota
	focusConversation
	focusStatus
	focusFindings
	focusProcessSink
	focusApproval
)

type interactiveResultMsg struct {
	update InteractiveWorkbenchUpdate
	err    error
}

// modelPanelMsg carries the result of a /model list or /model use operation.
type modelPanelMsg struct {
	models []ModelInfo
	err    error
	action string // "list", "switch", "test"
	name   string // model name for switch/test
}

type textSelection struct {
	active    bool
	dragging  bool
	startLine int
	endLine   int
}

type interactiveWorkbenchModel struct {
	driver           InteractiveWorkbenchDriver
	viewModel        WorkbenchViewModel
	lastOutput       string
	width            int
	height           int
	focus            interactiveFocus
	running          bool
	pendingLine      string
	chatViewport     viewport.Model
	statusViewport   viewport.Model
	approvalViewport viewport.Model
	input            textarea.Model
	spin             spinner.Model
	selection        textSelection
	contentLines     []string
	approvalCursor   int
	approvalDetail   bool
	approvalOffset   int
	approvalHeight   int
	findingsCursor   int
	findingsOffset   int
	findingsDetail   bool
	findingsHeight   int
	sinkCursor       int
	sinkOffset       int
	sinkDetail       bool
	sinkHeight       int
	// Model panel state: toggled via /model command.
	modelPanelActive   bool
	modelPanelCursor   int
	modelPanelList     []ModelInfo
	modelPanelEditing  bool
	modelPanelEditText string
}

func RunInteractiveWorkbench(input io.Reader, output io.Writer, driver InteractiveWorkbenchDriver) error {
	initialViewModel, err := driver.Load("")
	if err != nil {
		return err
	}

	model := newInteractiveWorkbenchModel(driver, initialViewModel)
	options := []tea.ProgramOption{
		tea.WithInput(input),
		tea.WithOutput(output),
	}
	if isTerminalWriter(output) {
		options = append(options, tea.WithAltScreen(), tea.WithMouseCellMotion())
	}

	program := tea.NewProgram(model, options...)
	_, err = program.Run()
	return err
}

func newInteractiveWorkbenchModel(driver InteractiveWorkbenchDriver, viewModel WorkbenchViewModel) interactiveWorkbenchModel {
	input := textarea.New()
	input.Prompt = "> "
	input.Placeholder = "Ask Lore about your vault, drafts, or process sink"
	input.ShowLineNumbers = false
	input.SetWidth(80)
	input.SetHeight(3)

	spin := spinner.New(spinner.WithSpinner(spinner.Line))

	model := interactiveWorkbenchModel{
		driver:           driver,
		viewModel:        viewModel,
		focus:            focusInput,
		chatViewport:     viewport.New(80, 20),
		statusViewport:   viewport.New(36, 20),
		approvalViewport: viewport.New(36, 10),
		input:            input,
		spin:             spin,
		width:            120,
		height:           32,
	}

	model.resize()
	model.input.Focus()
	model.refreshContent(true)
	return model
}

func (m interactiveWorkbenchModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m interactiveWorkbenchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.refreshContent(false)
		return m, nil
	case spinner.TickMsg:
		if !m.running {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case interactiveResultMsg:
		if msg.err != nil {
			m.running = false
			m.pendingLine = ""
			m.lastOutput = "Error: " + msg.err.Error()
			if refreshed, err := m.driver.Load(m.lastOutput); err == nil {
				m.viewModel = refreshed
			}
			m.refreshContent(true)
			return m, nil
		}

		m.running = false
		m.pendingLine = ""
		m.viewModel = msg.update.ViewModel
		m.lastOutput = strings.TrimSpace(msg.update.LastOutput)
		m.refreshContent(true)
		if msg.update.Quit {
			return m, tea.Quit
		}
		return m, nil
	case approvalResultMsg:
		return m.handleApprovalResult(msg)
	case findingsResultMsg:
		return m.handleFindingsResult(msg)
	case modelPanelMsg:
		return m.handleModelPanelResult(msg)
	case tea.MouseMsg:
		return m.handleMouse(tea.MouseEvent(msg))
	case tea.KeyMsg:
		// When model panel is active, route all keys to the model handler
		if m.modelPanelActive && !m.running {
			switch msg.String() {
			case "ctrl+c", "ctrl+q":
				m.modelPanelActive = false
				m.refreshContent(false)
				return m, nil
			default:
				return m.handleModelPanelKeys(msg)
			}
		}
		switch msg.String() {
		case "ctrl+c":
			if m.selection.active {
				m.copySelection()
				m.selection = textSelection{}
				m.refreshContent(false)
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+q":
			return m, tea.Quit
		case "esc":
			if m.selection.active {
				m.selection = textSelection{}
				m.refreshContent(false)
				return m, nil
			}
			if m.findingsDetail {
				m.findingsDetail = false
				m.refreshContent(false)
				return m, nil
			}
			if m.sinkDetail {
				m.sinkDetail = false
				m.refreshContent(false)
				return m, nil
			}
			return m, tea.Quit
		case "tab":
			m.focus = nextFocus(m.focus)
			m.resize()
			m.clampCurrentPanelOffset()
			m.refreshContent(false)
			return m, m.applyFocus()
		case "shift+tab":
			m.focus = previousFocus(m.focus)
			m.resize()
			m.clampCurrentPanelOffset()
			m.refreshContent(false)
			return m, m.applyFocus()
		}

		if m.running {
			return m, nil
		}

		switch m.focus {
		case focusConversation:
			var cmd tea.Cmd
			m.chatViewport, cmd = m.chatViewport.Update(msg)
			return m, cmd
		case focusStatus:
			var cmd tea.Cmd
			m.statusViewport, cmd = m.statusViewport.Update(msg)
			return m, cmd
		case focusFindings:
			return m.handleFindingsKeys(msg)
		case focusProcessSink:
			return m.handleSinkKeys(msg)
		case focusApproval:
			return m.handleApprovalKeys(msg)
		default:
			switch msg.String() {
			case "up", "down":
				if m.input.LineCount() > 1 {
					var cmd tea.Cmd
					m.input, cmd = m.input.Update(msg)
					return m, cmd
				}
				var cmd tea.Cmd
				m.chatViewport, cmd = m.chatViewport.Update(msg)
				return m, cmd
			case "pgup", "pgdown":
				var cmd tea.Cmd
				m.chatViewport, cmd = m.chatViewport.Update(msg)
				return m, cmd
			case "ctrl+j":
				m.input.InsertString("\n")
				return m, nil
			case "enter":
				line := strings.TrimSpace(m.input.Value())
				if line == "" {
					return m, nil
				}
				if handled, model, cmd := m.handleLocalCommand(line); handled {
					return model, cmd
				}
				m.running = true
				m.pendingLine = line
				m.input.Reset()
				m.refreshContent(true)
				return m, tea.Batch(m.spin.Tick, m.executeLine(line))
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	if m.focus == focusInput && !m.running {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m interactiveWorkbenchModel) View() string {
	return renderInteractiveWorkbenchLayout(m)
}

func (m *interactiveWorkbenchModel) executeLine(line string) tea.Cmd {
	return func() tea.Msg {
		if contextDriver, ok := m.driver.(InteractiveWorkbenchContextDriver); ok {
			update, err := contextDriver.ExecuteContext(context.Background(), line, m.lastOutput)
			return interactiveResultMsg{update: update, err: err}
		}
		update, err := m.driver.Execute(line, m.lastOutput)
		return interactiveResultMsg{update: update, err: err}
	}
}

// modelPanelCmds dispatch model panel operations via tea.Cmd.
func modelPanelListCmd(driver InteractiveWorkbenchDriver) tea.Cmd {
	return func() tea.Msg {
		models, err := driver.DiscoverModels()
		return modelPanelMsg{models: models, err: err, action: "list"}
	}
}

func modelPanelSwitchCmd(driver InteractiveWorkbenchDriver, name string) tea.Cmd {
	return func() tea.Msg {
		models, err := driver.SwitchModel(name)
		return modelPanelMsg{models: models, err: err, action: "switch", name: name}
	}
}

func modelPanelTestCmd(driver InteractiveWorkbenchDriver, name string) tea.Cmd {
	return func() tea.Msg {
		err := driver.TestModel(name)
		if err != nil {
			return modelPanelMsg{err: err, action: "test", name: name}
		}
		return modelPanelMsg{action: "test", name: name}
	}
}

func (m *interactiveWorkbenchModel) refreshContent(scrollToBottom bool) {
	content := renderInteractiveConversation(m.viewModel, m.lastOutput, m.running, m.pendingLine, m.chatViewport.Width)
	m.contentLines = strings.Split(content, "\n")
	if m.selection.active {
		content = m.applySelectionHighlight(content)
	}
	m.chatViewport.SetContent(content)
	m.statusViewport.SetContent(renderInteractiveStatus(m.viewModel))
	if scrollToBottom {
		m.chatViewport.GotoBottom()
		m.statusViewport.GotoTop()
	}
}

func (m *interactiveWorkbenchModel) resize() {
	if m.width <= 0 {
		m.width = 120
	}
	if m.height <= 0 {
		m.height = 32
	}

	narrow := m.width < 80
	var leftWidth, rightWidth int
	if narrow {
		leftWidth = maxInt(30, (m.width*3)/4)
	} else {
		leftWidth = maxInt(40, (m.width*2)/3)
	}
	rightWidth = maxInt(28, m.width-leftWidth-1)
	contentHeight := maxInt(12, m.height-8)
	rightTopHeight := maxInt(8, (contentHeight*2)/3)
	rightBottomHeight := maxInt(5, contentHeight-rightTopHeight-1)
	inputWidth := maxInt(24, m.width-6)

	m.chatViewport.Width = maxInt(18, leftWidth-4)
	m.chatViewport.Height = maxInt(8, contentHeight-4)
	m.statusViewport.Width = maxInt(16, rightWidth-4)
	m.statusViewport.Height = maxInt(6, rightTopHeight-4)
	m.approvalViewport.Width = maxInt(16, rightWidth-4)
	rightBottomContentHeight := maxInt(3, rightBottomHeight-4)
	m.approvalViewport.Height = rightBottomContentHeight
	m.approvalHeight = rightBottomContentHeight
	m.findingsHeight = rightBottomContentHeight
	m.sinkHeight = rightBottomContentHeight
	m.input.SetWidth(inputWidth)
	m.input.SetHeight(3)
}

func (m *interactiveWorkbenchModel) applyFocus() tea.Cmd {
	switch m.focus {
	case focusConversation, focusStatus, focusFindings, focusProcessSink, focusApproval:
		m.input.Blur()
		return nil
	default:
		return m.input.Focus()
	}
}

const edgeScrollZone = 3
const edgeScrollLines = 3

// paneTopOffset: border top (1) + title line (1) = 2 rows before viewport content starts
const paneTopOffset = 2

func (m interactiveWorkbenchModel) handleMouse(msg tea.MouseEvent) (tea.Model, tea.Cmd) {
	paneBottom := paneTopOffset + m.chatViewport.Height

	if msg.Button == tea.MouseButtonWheelUp {
		m.chatViewport.LineUp(3)
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		m.chatViewport.LineDown(3)
		return m, nil
	}

	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		line := m.chatViewport.YOffset + msg.Y - paneTopOffset
		m.selection = textSelection{
			active:    true,
			dragging:  true,
			startLine: line,
			endLine:   line,
		}
		m.refreshContent(false)
		return m, nil
	}

	if msg.Action == tea.MouseActionMotion && m.selection.dragging {
		line := m.chatViewport.YOffset + msg.Y - paneTopOffset
		m.selection.endLine = line

		if msg.Y <= paneTopOffset+edgeScrollZone {
			m.chatViewport.LineUp(edgeScrollLines)
		} else if msg.Y >= paneBottom-edgeScrollZone {
			m.chatViewport.LineDown(edgeScrollLines)
		}

		m.refreshContent(false)
		return m, nil
	}

	if msg.Action == tea.MouseActionRelease && m.selection.dragging {
		m.selection.dragging = false
		if m.selection.startLine == m.selection.endLine {
			m.selection = textSelection{}
		}
		m.refreshContent(false)
		return m, nil
	}

	return m, nil
}

func (m *interactiveWorkbenchModel) normalizedSelection() (startLine, endLine int) {
	sl, el := m.selection.startLine, m.selection.endLine
	if sl > el {
		sl, el = el, sl
	}
	return sl, el
}

func (m *interactiveWorkbenchModel) copySelection() {
	if !m.selection.active || len(m.contentLines) == 0 {
		return
	}
	sl, el := m.normalizedSelection()
	if sl < 0 {
		sl = 0
	}
	if el >= len(m.contentLines) {
		el = len(m.contentLines) - 1
	}
	var selected []string
	for i := sl; i <= el; i++ {
		selected = append(selected, stripANSI(m.contentLines[i]))
	}
	text := strings.Join(selected, "\n")
	_ = clipboard.WriteAll(strings.TrimSpace(text))
}

func (m *interactiveWorkbenchModel) applySelectionHighlight(content string) string {
	sl, el := m.normalizedSelection()
	lines := strings.Split(content, "\n")
	highlightStyle := lipgloss.NewStyle().Reverse(true)
	for i := range lines {
		if i >= sl && i <= el {
			lines[i] = highlightStyle.Render(lines[i])
		}
	}
	return strings.Join(lines, "\n")
}

func (m *interactiveWorkbenchModel) handleLocalCommand(line string) (bool, tea.Model, tea.Cmd) {
	lower := strings.ToLower(line)
	switch {
	case lower == "/session":
		m.input.Reset()
		m.lastOutput = renderSessionDetail(m.viewModel.Snapshot)
		m.refreshContent(true)
		return true, m, nil
	case lower == "/model", lower == "/model list":
		m.input.Reset()
		m.modelPanelActive = true
		m.modelPanelEditing = false
		m.refreshContent(false)
		return true, m, modelPanelListCmd(m.driver)
	case lower == "/model current":
		m.input.Reset()
		model := m.viewModel.Snapshot.CurrentModel
		if model == "" {
			model = "unknown"
		}
		m.lastOutput = "Current model: " + model
		m.refreshContent(true)
		return true, m, nil
	case strings.HasPrefix(lower, "/model use "):
		name := strings.TrimSpace(line[len("/model use "):])
		if name == "" {
			return true, m, nil
		}
		m.input.Reset()
		m.running = true
		m.pendingLine = "/model use " + name
		m.refreshContent(true)
		m.modelPanelActive = false
		return true, m, modelPanelSwitchCmd(m.driver, name)
	case lower == "/model test":
		m.input.Reset()
		model := m.viewModel.Snapshot.CurrentModel
		if model == "" {
			m.lastOutput = "No current model to test"
			m.refreshContent(true)
			return true, m, nil
		}
		m.running = true
		m.refreshContent(true)
		return true, m, modelPanelTestCmd(m.driver, model)
	default:
		return false, m, nil
	}
}

// --- Model panel handlers ---

func (m interactiveWorkbenchModel) handleModelPanelResult(msg modelPanelMsg) (tea.Model, tea.Cmd) {
	switch msg.action {
	case "list":
		if msg.err != nil {
			m.lastOutput = "Error: model: " + msg.err.Error()
		} else {
			m.modelPanelList = msg.models
			if m.modelPanelCursor >= len(msg.models) {
				m.modelPanelCursor = maxInt(0, len(msg.models)-1)
			}
		}
		m.refreshContent(false)
		return m, nil
	case "switch":
		m.running = false
		m.pendingLine = ""
		if msg.err != nil {
			m.lastOutput = "Error: model: " + msg.err.Error()
		} else {
			m.modelPanelList = msg.models
			m.lastOutput = "Switched to model: " + msg.name
			if refreshed, loadErr := m.driver.Load(m.lastOutput); loadErr == nil {
				m.viewModel = refreshed
			}
		}
		m.refreshContent(true)
		return m, nil
	case "test":
		m.running = false
		m.pendingLine = ""
		if msg.err != nil {
			m.lastOutput = "Error: model test: " + msg.err.Error()
		} else {
			m.lastOutput = "Model " + msg.name + ": OK"
		}
		m.refreshContent(true)
		return m, nil
	}
	return m, nil
}

func (m interactiveWorkbenchModel) handleModelPanelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modelPanelEditing {
		return m.handleModelEditKeys(msg)
	}

	switch msg.String() {
	case "esc":
		m.modelPanelActive = false
		m.modelPanelEditing = false
		m.modelPanelEditText = ""
		m.refreshContent(false)
		return m, nil
	case "up", "k":
		if m.modelPanelCursor > 0 {
			m.modelPanelCursor--
		}
		return m, nil
	case "down", "j":
		if m.modelPanelCursor < len(m.modelPanelList)-1 {
			m.modelPanelCursor++
		}
		return m, nil
	case "r":
		return m, modelPanelListCmd(m.driver)
	case "t":
		if m.modelPanelCursor < len(m.modelPanelList) {
			sel := m.modelPanelList[m.modelPanelCursor]
			m.running = true
			m.refreshContent(true)
			return m, modelPanelTestCmd(m.driver, sel.Name)
		}
		return m, nil
	case "enter":
		if m.modelPanelCursor < len(m.modelPanelList) {
			sel := m.modelPanelList[m.modelPanelCursor]
			if sel.Current {
				return m, nil
			}
			m.running = true
			m.pendingLine = "/model use " + sel.Name
			m.refreshContent(true)
			return m, modelPanelSwitchCmd(m.driver, sel.Name)
		}
		return m, nil
	case "e":
		m.modelPanelEditing = true
		m.modelPanelEditText = ""
		return m, nil
	}
	return m, nil
}

func (m interactiveWorkbenchModel) handleModelEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modelPanelEditing = false
		m.modelPanelEditText = ""
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.modelPanelEditText)
		m.modelPanelEditing = false
		m.modelPanelEditText = ""
		if name == "" {
			return m, nil
		}
		m.running = true
		m.pendingLine = "/model use " + name
		m.refreshContent(true)
		return m, modelPanelSwitchCmd(m.driver, name)
	case "backspace":
		if len(m.modelPanelEditText) > 0 {
			runes := []rune(m.modelPanelEditText)
			m.modelPanelEditText = string(runes[:len(runes)-1])
		}
		return m, nil
	default:
		if len(msg.Runes) == 1 {
			m.modelPanelEditText += string(msg.Runes)
		}
		return m, nil
	}
}

func nextFocus(current interactiveFocus) interactiveFocus {
	switch current {
	case focusInput:
		return focusConversation
	case focusConversation:
		return focusStatus
	case focusStatus:
		return focusFindings
	case focusFindings:
		return focusProcessSink
	case focusProcessSink:
		return focusApproval
	default:
		return focusInput
	}
}

func previousFocus(current interactiveFocus) interactiveFocus {
	switch current {
	case focusInput:
		return focusApproval
	case focusApproval:
		return focusProcessSink
	case focusProcessSink:
		return focusFindings
	case focusFindings:
		return focusStatus
	case focusStatus:
		return focusConversation
	default:
		return focusInput
	}
}

func isTerminalWriter(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Approval pane keyboard handler.
// List view: up/down select, a/r quick-approve/reject, enter opens detail.
// Detail view: a=approve, r=reject, p=apply, esc=back to list.
func (m interactiveWorkbenchModel) handleApprovalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	drafts := m.viewModel.PendingDrafts

	if m.approvalDetail {
		return m.handleApprovalDetailKeys(msg)
	}

	switch msg.String() {
	case "up", "k":
		if m.approvalCursor > 0 {
			m.approvalCursor--
			m.clampApprovalOffset()
		}
		m.refreshContent(false)
		return m, nil
	case "down", "j":
		if m.approvalCursor < len(drafts)-1 {
			m.approvalCursor++
			m.clampApprovalOffset()
		}
		m.refreshContent(false)
		return m, nil
	case "enter":
		if len(drafts) > 0 {
			m.approvalDetail = true
			m.refreshContent(false)
		}
		return m, nil
	case "a":
		if len(drafts) > 0 && m.approvalCursor < len(drafts) && drafts[m.approvalCursor].State == model.DraftPendingReview {
			return m.executeApprovalAction("approve", drafts[m.approvalCursor].ID)
		}
		return m, nil
	case "r":
		if len(drafts) > 0 && m.approvalCursor < len(drafts) && drafts[m.approvalCursor].State == model.DraftPendingReview {
			return m.executeApprovalAction("reject", drafts[m.approvalCursor].ID)
		}
		return m, nil
	}
	return m, nil
}

// clampApprovalOffset keeps the cursor inside the visible window.
func (m *interactiveWorkbenchModel) clampApprovalOffset() {
	m.approvalOffset = clampPanelOffset(m.approvalCursor, m.approvalOffset, m.approvalHeight, 1)
}

func (m interactiveWorkbenchModel) handleApprovalDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	drafts := m.viewModel.PendingDrafts
	if len(drafts) == 0 || m.approvalCursor >= len(drafts) {
		m.approvalDetail = false
		return m, nil
	}

	draft := drafts[m.approvalCursor]

	switch msg.String() {
	case "esc":
		m.approvalDetail = false
		m.refreshContent(false)
		return m, nil
	case "a":
		if draft.State != model.DraftPendingReview {
			return m, nil
		}
		return m.executeApprovalAction("approve", draft.ID)
	case "r":
		if draft.State != model.DraftPendingReview {
			return m, nil
		}
		return m.executeApprovalAction("reject", draft.ID)
	case "p":
		if draft.State != model.DraftApproved {
			return m, nil
		}
		return m.executeApprovalAction("apply", draft.ID)
	}
	return m, nil
}

// --- Findings pane keyboard handler ---

func (m interactiveWorkbenchModel) handleFindingsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	findings := m.viewModel.Findings

	if m.findingsDetail {
		return m.handleFindingsDetailKeys(msg)
	}

	switch msg.String() {
	case "up", "k":
		if m.findingsCursor > 0 {
			m.findingsCursor--
			m.clampFindingsOffset()
		}
		m.refreshContent(false)
		return m, nil
	case "down", "j":
		if m.findingsCursor < len(findings)-1 {
			m.findingsCursor++
			m.clampFindingsOffset()
		}
		m.refreshContent(false)
		return m, nil
	case "enter":
		if len(findings) > 0 {
			m.findingsDetail = true
			m.refreshContent(false)
		}
		return m, nil
	}
	return m, nil
}

func (m *interactiveWorkbenchModel) clampFindingsOffset() {
	m.findingsOffset = clampPanelOffset(m.findingsCursor, m.findingsOffset, m.findingsHeight, 1)
}

func (m interactiveWorkbenchModel) handleFindingsDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	findings := m.viewModel.Findings
	if len(findings) == 0 || m.findingsCursor >= len(findings) {
		m.findingsDetail = false
		return m, nil
	}

	finding := findings[m.findingsCursor]

	switch msg.String() {
	case "esc":
		m.findingsDetail = false
		m.refreshContent(false)
		return m, nil
	case "x":
		if finding.State != model.FindingOpen {
			return m, nil
		}
		return m.executeFindingsAction("resolve", finding.ID)
	case "i":
		if finding.State != model.FindingOpen {
			return m, nil
		}
		return m.executeFindingsAction("ignore", finding.ID)
	}
	return m, nil
}

type findingsResultMsg struct {
	action     string
	findingID  string
	viewModel  WorkbenchViewModel
	lastOutput string
	err        error
}

func (m interactiveWorkbenchModel) executeFindingsAction(action string, findingID string) (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		update, err := m.driver.ExecuteFindingAction(action, findingID)
		return findingsResultMsg{
			action:     action,
			findingID:  findingID,
			viewModel:  update.ViewModel,
			lastOutput: update.LastOutput,
			err:        err,
		}
	}
}

func (m interactiveWorkbenchModel) handleFindingsResult(msg findingsResultMsg) (tea.Model, tea.Cmd) {
	m.findingsDetail = false
	if msg.err != nil {
		m.lastOutput = "Error: findings: " + msg.err.Error()
	} else {
		m.viewModel = msg.viewModel
		m.lastOutput = msg.lastOutput
	}
	if len(m.viewModel.Findings) == 0 {
		m.findingsCursor = 0
		m.findingsOffset = 0
	} else {
		if m.findingsCursor >= len(m.viewModel.Findings) {
			m.findingsCursor = maxInt(0, len(m.viewModel.Findings)-1)
		}
		m.clampFindingsOffset()
	}
	m.refreshContent(true)
	return m, nil
}

// --- Process-sink timeline keyboard handler ---

func (m interactiveWorkbenchModel) handleSinkKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	checkpoints := m.viewModel.ProcessSink.Checkpoints

	if m.sinkDetail {
		return m.handleSinkDetailKeys(msg)
	}

	switch msg.String() {
	case "up", "k":
		if m.sinkCursor > 0 {
			m.sinkCursor--
			m.clampSinkOffset()
		}
		m.refreshContent(false)
		return m, nil
	case "down", "j":
		if m.sinkCursor < len(checkpoints)-1 {
			m.sinkCursor++
			m.clampSinkOffset()
		}
		m.refreshContent(false)
		return m, nil
	case "enter":
		if len(checkpoints) > 0 {
			m.sinkDetail = true
			m.refreshContent(false)
		}
		return m, nil
	}
	return m, nil
}

func (m *interactiveWorkbenchModel) clampSinkOffset() {
	m.sinkOffset = clampPanelOffset(m.sinkCursor, m.sinkOffset, m.sinkHeight, 2)
}

// clampCurrentPanelOffset clamps the offset for whichever panel currently has focus.
// Called on Tab/Shift+Tab so the visible window stays aligned after focus switch.
func (m *interactiveWorkbenchModel) clampCurrentPanelOffset() {
	switch m.focus {
	case focusApproval:
		m.clampApprovalOffset()
	case focusFindings:
		m.clampFindingsOffset()
	case focusProcessSink:
		m.clampSinkOffset()
	}
}

func clampPanelOffset(cursor int, offset int, visibleHeight int, reservedRows int) int {
	if visibleHeight < 1 {
		visibleHeight = 5
	}
	listHeight := visibleHeight - reservedRows
	if listHeight < 1 {
		listHeight = 1
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+listHeight {
		offset = cursor - listHeight + 1
	}
	if offset < 0 {
		return 0
	}
	return offset
}

func (m interactiveWorkbenchModel) handleSinkDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sinkDetail = false
		m.refreshContent(false)
		return m, nil
	}
	return m, nil
}

type approvalResultMsg struct {
	action     string
	draftID    string
	viewModel  WorkbenchViewModel
	lastOutput string
	err        error
}

func (m interactiveWorkbenchModel) executeApprovalAction(action string, draftID string) (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		update, err := m.driver.ExecuteApprovalAction(action, draftID)
		return approvalResultMsg{
			action:     action,
			draftID:    draftID,
			viewModel:  update.ViewModel,
			lastOutput: update.LastOutput,
			err:        err,
		}
	}
}

func (m interactiveWorkbenchModel) handleApprovalResult(msg approvalResultMsg) (tea.Model, tea.Cmd) {
	m.approvalDetail = false
	if msg.err != nil {
		m.lastOutput = "Error: approval: " + msg.err.Error()
	} else {
		m.viewModel = msg.viewModel
		m.lastOutput = msg.lastOutput
	}
	// Clamp cursor to new list length
	if len(m.viewModel.PendingDrafts) == 0 {
		m.approvalCursor = 0
		m.approvalOffset = 0
	} else {
		if m.approvalCursor >= len(m.viewModel.PendingDrafts) {
			m.approvalCursor = maxInt(0, len(m.viewModel.PendingDrafts)-1)
		}
		m.clampApprovalOffset()
	}
	m.refreshContent(true)
	return m, nil
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
