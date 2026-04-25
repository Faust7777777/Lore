package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	colorBorderDefault = lipgloss.Color("#4A5568")
	colorBorderFocused = lipgloss.Color("#D97706")
	colorMuted         = lipgloss.Color("#6B7280")
	colorTitle         = lipgloss.Color("#C9D1D9")
	colorUser          = lipgloss.Color("#58A6FF")
	colorAssistant     = lipgloss.Color("#D2A8FF")
	colorOK            = lipgloss.Color("#3FB950")
	colorWarn          = lipgloss.Color("#D29922")
	colorErr           = lipgloss.Color("#F85149")
	colorRunning       = lipgloss.Color("#D97706")
	colorToolName      = lipgloss.Color("#79C0FF")
	colorSectionHead   = lipgloss.Color("#8B949E")

	basePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorderDefault).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitle)

	focusedTitleStyle = titleStyle.
				Foreground(colorBorderFocused)

	styleUserLabel = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorUser)

	styleAssistantLabel = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAssistant)

	styleMutedText = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleOK = lipgloss.NewStyle().
		Foreground(colorOK)

	styleWarn = lipgloss.NewStyle().
			Foreground(colorWarn)

	styleErr = lipgloss.NewStyle().
			Foreground(colorErr)

	styleRunning = lipgloss.NewStyle().
			Foreground(colorRunning)

	styleToolName = lipgloss.NewStyle().
			Foreground(colorToolName)

	styleSectionHead = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSectionHead)
)

const (
	glyphFocus    = ">"
	glyphOK       = "*"
	glyphMissing  = "o"
	glyphItem     = "-"
	glyphThinking = "..."
	glyphSep      = "|"
	glyphDash     = "-"
)

func thinRule(width int) string {
	if width <= 0 {
		width = 40
	}
	return styleMutedText.Render(strings.Repeat(glyphDash, minInt(width, 60)))
}

func paneStyle(focused bool) lipgloss.Style {
	if focused {
		return basePaneStyle.BorderForeground(colorBorderFocused)
	}
	return basePaneStyle
}

func inputPaneStyle(focused bool) lipgloss.Style {
	style := basePaneStyle.Padding(0, 1)
	if focused {
		return style.BorderForeground(colorBorderFocused)
	}
	return style
}

func wrapText(text string, width int) string {
	if width <= 0 || text == "" {
		return text
	}
	var builder strings.Builder
	for i, paragraph := range strings.Split(text, "\n") {
		if i > 0 {
			builder.WriteByte('\n')
		}
		if strings.TrimSpace(paragraph) == "" {
			continue
		}
		col := 0
		for _, r := range paragraph {
			w := runeWidth(r)
			if col+w > width && col > 0 {
				builder.WriteByte('\n')
				col = 0
			}
			builder.WriteRune(r)
			col += w
		}
	}
	return builder.String()
}

func runeWidth(r rune) int {
	return runewidth.RuneWidth(r)
}

func unescapeLiteralNewlines(s string) string {
	return strings.ReplaceAll(s, `\n`, "\n")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
