package tui

import (
	"fmt"
	"strings"
	"testing"

	"obsidian-harness/internal/operatoragent"
)

func TestUnescapeLiteralNewlines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"basic", `hello\nworld`, "hello\nworld"},
		{"multiple", `a\nb\nc`, "a\nb\nc"},
		{"no escape", "already clean", "already clean"},
		{"empty", "", ""},
		{"only escape", `\n`, "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeLiteralNewlines(tt.input)
			if got != tt.want {
				t.Errorf("unescapeLiteralNewlines(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderInteractiveConversationUnescapesNewlines(t *testing.T) {
	vm := WorkbenchViewModel{
		Conversation: WorkbenchConversation{
			Turns: []operatoragent.ConversationTurn{
				{Role: "assistant", Content: `hello\nworld`},
			},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if strings.Contains(result, `\n`) {
		t.Errorf("literal backslash-n should not appear in rendered output, got: %q", result)
	}
	if !strings.Contains(result, "hello\n") || !strings.Contains(result, "world") {
		t.Errorf("expected real newlines in rendered conversation, got: %q", result)
	}
}

func TestRenderInteractiveConversationEmptyState(t *testing.T) {
	vm := WorkbenchViewModel{}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "Ready.") {
		t.Errorf("empty conversation should show ready prompt, got: %q", result)
	}
	if !strings.Contains(result, "No active output.") {
		t.Errorf("empty conversation should show no active output, got: %q", result)
	}
}

func TestRenderInteractiveConversationRendersAllTurns(t *testing.T) {
	turns := make([]operatoragent.ConversationTurn, 12)
	for i := range turns {
		turns[i] = operatoragent.ConversationTurn{Role: "user", Content: fmt.Sprintf("msg-%d", i)}
	}
	vm := WorkbenchViewModel{
		Conversation: WorkbenchConversation{Turns: turns},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	for i := 0; i < 12; i++ {
		want := fmt.Sprintf("msg-%d", i)
		if !strings.Contains(result, want) {
			t.Errorf("should render all turns, missing %q", want)
		}
	}
}

func TestRenderInteractiveConversationShowsToolTrace(t *testing.T) {
	vm := WorkbenchViewModel{
		ToolTrace: []operatoragent.ToolCallTrace{
			{Name: "managed_status", Status: "ok"},
			{Name: "vault_read", Status: "error", Error: "not found"},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "Tool Calls") {
		t.Errorf("should show Tool Calls section, got: %q", result)
	}
	if !strings.Contains(result, "managed_status") {
		t.Errorf("should show tool name, got: %q", result)
	}
	if !strings.Contains(result, "not found") {
		t.Errorf("should show tool error, got: %q", result)
	}
}

func TestRenderInteractiveConversationRunningState(t *testing.T) {
	vm := WorkbenchViewModel{}
	result := renderInteractiveConversation(vm, "", true, "show status", 80)
	if !strings.Contains(result, "thinking") {
		t.Errorf("running state should show thinking indicator, got: %q", result)
	}
	if !strings.Contains(result, "show status") {
		t.Errorf("running state should show pending input, got: %q", result)
	}
}

func TestRenderInteractiveConversationLastOutputUnescape(t *testing.T) {
	vm := WorkbenchViewModel{}
	result := renderInteractiveConversation(vm, `line1\nline2`, false, "", 80)
	if strings.Contains(result, `\n`) {
		t.Errorf("lastOutput should have literal backslash-n unescaped, got: %q", result)
	}
	if !strings.Contains(result, "line1\n") {
		t.Errorf("lastOutput should contain real newline, got: %q", result)
	}
}

func TestRenderInteractiveStatusBasic(t *testing.T) {
	vm := WorkbenchViewModel{
		Snapshot: WorkbenchSnapshot{
			Profile:      "local",
			Ready:        true,
			HealthStatus: "ok",
			AgentID:      "codex",
		},
	}
	result := renderInteractiveStatus(vm)
	if !strings.Contains(result, "System") {
		t.Errorf("should show System section, got: %q", result)
	}
	if !strings.Contains(result, "yes") {
		t.Errorf("ready should show yes, got: %q", result)
	}
	if !strings.Contains(result, "codex") {
		t.Errorf("should show agent ID, got: %q", result)
	}
}

func TestRenderInputHeaderRunning(t *testing.T) {
	result := renderInputHeader(true, true)
	if !strings.Contains(result, "agent running") {
		t.Errorf("running input header should show agent running, got: %q", result)
	}
}

func TestRenderInputHeaderIdle(t *testing.T) {
	result := renderInputHeader(true, false)
	if !strings.Contains(result, "Enter send") {
		t.Errorf("idle input header should show keybinding hints, got: %q", result)
	}
	if !strings.Contains(result, "Ctrl+Q quit") {
		t.Errorf("idle input header should show quit hint, got: %q", result)
	}
	if !strings.Contains(result, "Ctrl+J newline") {
		t.Errorf("idle input header should show newline hint, got: %q", result)
	}
	if !strings.Contains(result, "Ctrl+C copy") {
		t.Errorf("idle input header should show copy hint, got: %q", result)
	}
}

func TestRenderApprovalPlaceholder(t *testing.T) {
	result := renderApprovalPlaceholder()
	if !strings.Contains(result, "No pending actions.") {
		t.Errorf("approval placeholder should show empty state, got: %q", result)
	}
}

func TestWrapTextASCII(t *testing.T) {
	text := "abcdefghij"
	result := wrapText(text, 5)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), result)
	}
	if lines[0] != "abcde" {
		t.Errorf("first line should be 'abcde', got %q", lines[0])
	}
}

func TestWrapTextCJK(t *testing.T) {
	text := "你好世界测试"
	result := wrapText(text, 8)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines for CJK at width 8 (4 chars = 8 cols, 2 chars = 4 cols), got %d: %q", len(lines), result)
	}
}

func TestWrapTextPreservesNewlines(t *testing.T) {
	text := "line1\nline2"
	result := wrapText(text, 80)
	if result != text {
		t.Errorf("should preserve existing newlines, got %q", result)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	if wrapText("", 80) != "" {
		t.Error("empty input should return empty")
	}
	if wrapText("hello", 0) != "hello" {
		t.Error("zero width should return input unchanged")
	}
}

func TestRenderConversationWrapsLongContent(t *testing.T) {
	long := strings.Repeat("这是一段很长的中文内容", 10)
	vm := WorkbenchViewModel{
		Conversation: WorkbenchConversation{
			Turns: []operatoragent.ConversationTurn{
				{Role: "assistant", Content: long},
			},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 40)
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		w := testVisibleWidth(line)
		if w > 42 {
			t.Errorf("line exceeds wrap width (40+2 indent): width=%d line=%q", w, line)
		}
	}
}

func testVisibleWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

func TestStripANSI(t *testing.T) {
	styled := "\x1b[31mhello\x1b[0m world"
	result := stripANSI(styled)
	if result != "hello world" {
		t.Errorf("stripANSI should remove ANSI codes, got %q", result)
	}
}

func TestStripANSIPlainText(t *testing.T) {
	plain := "no ansi here"
	if stripANSI(plain) != plain {
		t.Error("stripANSI should not modify plain text")
	}
}

func TestNormalizedSelection(t *testing.T) {
	m := &interactiveWorkbenchModel{
		selection: textSelection{
			active:    true,
			startLine: 5,
			endLine:   2,
		},
	}
	sl, el := m.normalizedSelection()
	if sl != 2 || el != 5 {
		t.Errorf("normalizedSelection should swap reversed selection, got %d - %d", sl, el)
	}
}

func TestApplySelectionHighlight(t *testing.T) {
	m := &interactiveWorkbenchModel{
		selection: textSelection{
			active:    true,
			startLine: 1,
			endLine:   1,
		},
	}
	content := "line0\nline1\nline2"
	result := m.applySelectionHighlight(content)
	lines := strings.Split(result, "\n")
	if lines[0] != "line0" {
		t.Errorf("non-selected line should be unchanged, got %q", lines[0])
	}
	if lines[2] != "line2" {
		t.Errorf("non-selected line should be unchanged, got %q", lines[2])
	}
	if !strings.Contains(lines[1], "line1") {
		t.Errorf("selected line should still contain original text, got %q", lines[1])
	}
}

func TestRenderSessionDetail(t *testing.T) {
	snap := WorkbenchSnapshot{
		Profile:         "local",
		AgentID:         "codex",
		SessionID:       "sess-abc-123",
		TranscriptPath:  "/home/user/.lore/logs/2026-04-25.jsonl",
		DailyReportPath: "/home/user/.lore/reports/2026-04-25.md",
		HealthStatus:    "ok",
		HealthMessage:   "all systems go",
	}
	result := renderSessionDetail(snap)
	if !strings.Contains(result, "sess-abc-123") {
		t.Errorf("should show full session ID, got: %q", result)
	}
	if !strings.Contains(result, "2026-04-25.jsonl") {
		t.Errorf("should show full transcript path, got: %q", result)
	}
	if !strings.Contains(result, "2026-04-25.md") {
		t.Errorf("should show full report path, got: %q", result)
	}
	if !strings.Contains(result, "all systems go") {
		t.Errorf("should show health message, got: %q", result)
	}
}

func TestRenderInteractiveStatusShortensLogPath(t *testing.T) {
	vm := WorkbenchViewModel{
		Snapshot: WorkbenchSnapshot{
			Profile:        "local",
			Ready:          true,
			HealthStatus:   "ok",
			AgentID:        "codex",
			TranscriptPath: "/very/long/path/to/logs/session-2026-04-25.jsonl",
		},
	}
	result := renderInteractiveStatus(vm)
	if strings.Contains(result, "/very/long/path") {
		t.Errorf("status pane should show only filename, not full path, got: %q", result)
	}
	if !strings.Contains(result, "session-2026-04-25.jsonl") {
		t.Errorf("status pane should show the filename, got: %q", result)
	}
}
