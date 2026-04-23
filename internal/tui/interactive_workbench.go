package tui

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
		options = append(options, tea.WithAltScreen())
	}

	program := tea.NewProgram(model, options...)
	_, err = program.Run()
	return err
}

func newInteractiveWorkbenchModel(driver InteractiveWorkbenchDriver, viewModel WorkbenchViewModel) interactiveWorkbenchModel {
	input := textarea.New()
	input.Prompt = "lore> "
	input.Placeholder = "Ask Lore about your vault, drafts, or process sink"
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
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
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
			if msg.String() == "enter" {
				line := strings.TrimSpace(m.input.Value())
				if line == "" {
					return m, nil
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
	m.chatViewport.SetContent(renderInteractiveConversation(m.viewModel, m.lastOutput, m.running, m.pendingLine))
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
