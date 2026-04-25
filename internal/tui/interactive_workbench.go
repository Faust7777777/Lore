package tui

import (
	"io"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type InteractiveWorkbenchDriver interface {
	Load(lastOutput string) (WorkbenchViewModel, error)
	Execute(line string, lastOutput string) (InteractiveWorkbenchUpdate, error)
}

type InteractiveWorkbenchUpdate struct {
	ViewModel  WorkbenchViewModel
	LastOutput string
	Quit       bool
}

type interactiveFocus int

const (
	focusInput interactiveFocus = iota
	focusConversation
	focusStatus
)

type interactiveResultMsg struct {
	update InteractiveWorkbenchUpdate
	err    error
}

type textSelection struct {
	active    bool
	dragging  bool
	startLine int
	endLine   int
}

type interactiveWorkbenchModel struct {
	driver         InteractiveWorkbenchDriver
	viewModel      WorkbenchViewModel
	lastOutput     string
	width          int
	height         int
	focus          interactiveFocus
	running        bool
	pendingLine    string
	chatViewport   viewport.Model
	statusViewport viewport.Model
	input          textarea.Model
	spin           spinner.Model
	selection      textSelection
	contentLines   []string
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
		driver:         driver,
		viewModel:      viewModel,
		focus:          focusInput,
		chatViewport:   viewport.New(80, 20),
		statusViewport: viewport.New(36, 20),
		input:          input,
		spin:           spin,
		width:          120,
		height:         32,
	}

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
	case tea.MouseMsg:
		return m.handleMouse(tea.MouseEvent(msg))
	case tea.KeyMsg:
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
			return m, tea.Quit
		case "tab":
			m.focus = nextFocus(m.focus)
			return m, m.applyFocus()
		case "shift+tab":
			m.focus = previousFocus(m.focus)
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
				if handled, model := m.handleLocalCommand(line); handled {
					return model, nil
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
	m.resize()
	m.refreshContent(false)
	return renderInteractiveWorkbenchLayout(m)
}

func (m *interactiveWorkbenchModel) executeLine(line string) tea.Cmd {
	return func() tea.Msg {
		update, err := m.driver.Execute(line, m.lastOutput)
		return interactiveResultMsg{update: update, err: err}
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

	leftWidth := maxInt(40, (m.width*2)/3)
	rightWidth := maxInt(28, m.width-leftWidth-1)
	contentHeight := maxInt(12, m.height-8)
	rightTopHeight := maxInt(8, (contentHeight*2)/3)
	inputWidth := maxInt(24, m.width-6)

	m.chatViewport.Width = maxInt(18, leftWidth-4)
	m.chatViewport.Height = maxInt(8, contentHeight-4)
	m.statusViewport.Width = maxInt(16, rightWidth-4)
	m.statusViewport.Height = maxInt(6, rightTopHeight-4)
	m.input.SetWidth(inputWidth)
	m.input.SetHeight(3)
}

func (m *interactiveWorkbenchModel) applyFocus() tea.Cmd {
	switch m.focus {
	case focusConversation, focusStatus:
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

func (m *interactiveWorkbenchModel) handleLocalCommand(line string) (bool, tea.Model) {
	switch strings.ToLower(line) {
	case "/session":
		m.input.Reset()
		m.lastOutput = renderSessionDetail(m.viewModel.Snapshot)
		m.refreshContent(true)
		return true, m
	default:
		return false, m
	}
}

func nextFocus(current interactiveFocus) interactiveFocus {
	switch current {
	case focusInput:
		return focusConversation
	case focusConversation:
		return focusStatus
	default:
		return focusInput
	}
}

func previousFocus(current interactiveFocus) interactiveFocus {
	switch current {
	case focusInput:
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
