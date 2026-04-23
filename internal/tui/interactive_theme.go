package tui

import "github.com/charmbracelet/lipgloss"

var (
	surfaceBorderColor = lipgloss.Color("#4A5568")
	focusBorderColor   = lipgloss.Color("#D97706")
	mutedTextColor     = lipgloss.Color("#6B7280")
	titleTextColor     = lipgloss.Color("#111827")

	basePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(surfaceBorderColor).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(titleTextColor)

	focusedTitleStyle = titleStyle.Copy().
				Foreground(focusBorderColor)

	titleStatusStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor)
)

func paneStyle(focused bool) lipgloss.Style {
	if focused {
		return basePaneStyle.Copy().BorderForeground(focusBorderColor)
	}
	return basePaneStyle
}

func inputPaneStyle(focused bool) lipgloss.Style {
	style := basePaneStyle.Copy().Padding(0, 1)
	if focused {
		return style.BorderForeground(focusBorderColor)
	}
	return style
}
