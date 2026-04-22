package operatoragent

import (
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/model"
)

func TestFallbackAgentDecideSupportsChineseStatus(t *testing.T) {
	agent := NewFallback()

	decision, err := agent.Decide("\u770b\u770b\u5f53\u524d\u72b6\u6001", Context{
		DefaultAgentID: "codex",
		Now:            time.Date(2026, 4, 22, 10, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Action != ActionShowStatus {
		t.Fatalf("decision.Action = %q, want %q", decision.Action, ActionShowStatus)
	}
	if !decision.Day.Equal(model.NormalizeDay(time.Date(2026, 4, 22, 10, 0, 0, 0, time.Local))) {
		t.Fatalf("decision.Day = %v, want normalized today", decision.Day)
	}
}

func TestFallbackAgentDecideRejectsTimedRuntimeRequests(t *testing.T) {
	agent := NewFallback()

	_, err := agent.Decide("\u6bcf30\u5206\u949f\u540c\u6b65 codex session", Context{})
	if err == nil {
		t.Fatal("Decide() error = nil, want background runtime rejection")
	}
	if !strings.Contains(err.Error(), "timed and background") {
		t.Fatalf("error = %q, want timed/background guidance", err)
	}
}

func TestFallbackAgentDecideProcessSinkUsesExplicitDay(t *testing.T) {
	agent := NewFallback()

	decision, err := agent.Decide("show codex daily report 2026-04-20", Context{
		DefaultAgentID: "claude",
		Now:            time.Date(2026, 4, 22, 10, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Action != ActionShowProcessSinkDay {
		t.Fatalf("decision.Action = %q, want %q", decision.Action, ActionShowProcessSinkDay)
	}
	if decision.AgentID != "codex" {
		t.Fatalf("decision.AgentID = %q, want codex", decision.AgentID)
	}
	if got := decision.Day.Format("2006-01-02"); got != "2026-04-20" {
		t.Fatalf("decision.Day = %q, want 2026-04-20", got)
	}
}
