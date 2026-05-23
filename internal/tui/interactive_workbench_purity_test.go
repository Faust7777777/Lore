package tui

import (
	"strings"
	"testing"

	"obsidian-harness/internal/operatoragent"
)

func TestInteractiveWorkbenchViewDoesNotRefreshContent(t *testing.T) {
	vm := WorkbenchViewModel{
		Conversation: WorkbenchConversation{
			Turns: []operatoragent.ConversationTurn{
				{Role: "assistant", Content: "original-rendered-turn"},
			},
		},
	}
	m := newInteractiveWorkbenchModel(&panelDriverStub{}, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	m.viewModel.Conversation.Turns = append(m.viewModel.Conversation.Turns, operatoragent.ConversationTurn{
		Role:    "assistant",
		Content: "mutated-without-update",
	})

	rendered := m.View()
	if !strings.Contains(rendered, "original-rendered-turn") {
		t.Fatalf("View() lost pre-rendered viewport content:\n%s", rendered)
	}
	if strings.Contains(rendered, "mutated-without-update") {
		t.Fatalf("View() refreshed model-derived content; refresh must happen in Update/init paths only:\n%s", rendered)
	}
}
