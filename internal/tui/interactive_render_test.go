package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"obsidian-harness/internal/model"
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

func TestRenderInteractiveConversationShowsTaskSteps(t *testing.T) {
	vm := WorkbenchViewModel{
		TurnSteps: []operatoragent.TurnStep{
			{Index: 1, Tool: "managed_status", Status: "ok"},
			{Index: 2, Tool: "vault_read", Status: "error", Error: "not found"},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "Task Steps") {
		t.Errorf("should show Task Steps section, got: %q", result)
	}
	if !strings.Contains(result, "managed_status") {
		t.Errorf("should show tool name, got: %q", result)
	}
	if !strings.Contains(result, "not found") {
		t.Errorf("should show tool error, got: %q", result)
	}
	if !strings.Contains(result, "1.") {
		t.Errorf("should show step number, got: %q", result)
	}
}

func TestRenderTaskStepsArgSummary(t *testing.T) {
	vm := WorkbenchViewModel{
		TurnSteps: []operatoragent.TurnStep{
			{Index: 1, Tool: "vault_resolve", Status: "ok", Arguments: map[string]any{"query": "人物背景"}},
			{Index: 2, Tool: "vault_read", Status: "ok", Arguments: map[string]any{"path": "03-画像/人物画像.md"}},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "query=人物背景") {
		t.Errorf("should show vault_resolve argument, got: %q", result)
	}
	if !strings.Contains(result, "path=03-画像/人物画像.md") {
		t.Errorf("should show vault_read argument, got: %q", result)
	}
}

func TestRenderTaskStepsTruncatesObservation(t *testing.T) {
	longObs := strings.Repeat("abcdefghij", 20) // 200 chars
	vm := WorkbenchViewModel{
		TurnSteps: []operatoragent.TurnStep{
			{Index: 1, Tool: "vault_read", Status: "ok", ObservationExcerpt: longObs, Arguments: map[string]any{"path": "x.md"}},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "obs:") {
		t.Errorf("should show observation label, got: %q", result)
	}
	// Observation should be truncated (oneLine caps at 120)
	lines := strings.Split(result, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "obs:") {
			found = true
			if len(line) > 250 {
				t.Errorf("observation line too long (%d chars): %q", len(line), line)
			}
		}
	}
	if !found {
		t.Errorf("no obs line found in output")
	}
}

func TestRenderTaskStepsErrorStep(t *testing.T) {
	vm := WorkbenchViewModel{
		TurnSteps: []operatoragent.TurnStep{
			{Index: 1, Tool: "vault_read", Status: "error", Error: "file not found", Arguments: map[string]any{"path": "missing.md"}},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "error") {
		t.Errorf("should show error status, got: %q", result)
	}
	if !strings.Contains(result, "file not found") {
		t.Errorf("should show error message, got: %q", result)
	}
}

func TestRenderTaskStepsNonErrorLastOutputNotShown(t *testing.T) {
	vm := WorkbenchViewModel{
		TurnSteps: []operatoragent.TurnStep{
			{Index: 1, Tool: "managed_status", Status: "ok"},
		},
	}
	// Non-error lastOutput should NOT appear in the conversation pane
	result := renderInteractiveConversation(vm, "some normal output text", false, "", 80)
	if strings.Contains(result, "some normal output text") {
		t.Errorf("non-error lastOutput should not be rendered, got: %q", result)
	}
	if !strings.Contains(result, "Task Steps") {
		t.Errorf("task steps should still render, got: %q", result)
	}
}

func TestRenderTaskStepsWithObservation(t *testing.T) {
	vm := WorkbenchViewModel{
		TurnSteps: []operatoragent.TurnStep{
			{Index: 1, Tool: "vault_read", Status: "ok", ObservationExcerpt: "# 人物背景\n\n- 海边自习\n- 电子商务", Arguments: map[string]any{"path": "03-画像/人物背景.md"}},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "obs:") {
		t.Errorf("should show observation label, got: %q", result)
	}
	if !strings.Contains(result, "人物背景") {
		t.Errorf("observation should contain content, got: %q", result)
	}
}

func TestRenderTaskStepsTruncatedMarker(t *testing.T) {
	longObs := "# Long Doc\n\n" + strings.Repeat("paragraph text here. ", 50) + "\n[truncated 2048 bytes]"
	vm := WorkbenchViewModel{
		TurnSteps: []operatoragent.TurnStep{
			{Index: 1, Tool: "vault_read", Status: "ok", ObservationExcerpt: longObs, Arguments: map[string]any{"path": "long.md"}},
		},
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if !strings.Contains(result, "obs:") {
		t.Errorf("should show observation, got: %q", result)
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("truncated marker should be preserved, got: %q", result)
	}
}

func TestRenderTaskStepsEmptyNoRender(t *testing.T) {
	vm := WorkbenchViewModel{
		TurnSteps: nil,
	}
	result := renderInteractiveConversation(vm, "", false, "", 80)
	if strings.Contains(result, "Task Steps") {
		t.Errorf("empty TurnSteps should not render Task Steps block, got: %q", result)
	}
}

func TestRenderObservationExcerptRuneSafe(t *testing.T) {
	// CJK characters: each rune is 3 bytes, must not cut mid-rune
	cjk := strings.Repeat("人物背景画像", 10) // 40 runes, 120 bytes
	result := renderObservationExcerpt(cjk, 20)
	runes := []rune(result)
	if len(runes) > 23 { // 20 + "..." = 23 max
		t.Fatalf("result has %d runes, expected <= 23: %q", len(runes), result)
	}
	// Should end with "..."
	if !strings.HasSuffix(result, "...") {
		t.Fatalf("should end with ..., got: %q", result)
	}
}

func TestRenderObservationExcerptPreservesTruncatedMarker(t *testing.T) {
	text := strings.Repeat("abcdefghij", 30) + " [truncated 2048 bytes]"
	result := renderObservationExcerpt(text, 50)
	if !strings.Contains(result, "[truncated 2048 bytes]") {
		t.Fatalf("marker should be preserved, got: %q", result)
	}
	if !strings.Contains(result, "...") {
		t.Fatalf("should have ellipsis, got: %q", result)
	}
}

func TestRenderObservationExcerptCollapsesWhitespace(t *testing.T) {
	result := renderObservationExcerpt("line1\r\nline2\ttagged  extra", 80)
	if result != "line1 line2 tagged extra" {
		t.Fatalf("should collapse \\r\\n and \\t into single spaces, got: %q", result)
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

func TestRenderInteractiveConversationLastOutputNotRendered(t *testing.T) {
	vm := WorkbenchViewModel{}
	result := renderInteractiveConversation(vm, `line1\nline2`, false, "", 80)
	// lastOutput is no longer rendered in the conversation pane (it stays as internal state only)
	if strings.Contains(result, "Latest Output") {
		t.Errorf("conversation pane should not render Latest Output section, got: %q", result)
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

func TestRenderApprovalPaneEmpty(t *testing.T) {
	result := renderApprovalPane(nil, 0, 0, false, nil, 30, 10)
	if !strings.Contains(result, "No reviewable drafts") {
		t.Errorf("approval pane empty state should show 'No reviewable drafts', got: %q", result)
	}
}

func TestRenderApprovalListWithDrafts(t *testing.T) {
	drafts := []model.Draft{
		{ID: "d1", Kind: model.DraftKindMarkdownNoteWrite, Title: "Test note", State: model.DraftPendingReview},
		{ID: "d2", Kind: model.DraftKindPersonaUpdate, Title: "Update persona", State: model.DraftApproved},
	}
	result := renderApprovalPane(drafts, 0, 0, false, nil, 36, 10)
	if !strings.Contains(result, "Test note") {
		t.Errorf("approval list should contain first draft title, got: %q", result)
	}
	if !strings.Contains(result, "Update persona") {
		t.Errorf("approval list should contain second draft title, got: %q", result)
	}
	if !strings.Contains(result, "enter=detail") {
		t.Errorf("approval list should show enter hint, got: %q", result)
	}
	// Cursor is on first item
	if !strings.Contains(result, "1/2") {
		t.Errorf("approval list should show position indicator, got: %q", result)
	}
}

func TestRenderApprovalDetailPendingReview(t *testing.T) {
	draft := model.Draft{
		ID: "d1", Kind: model.DraftKindMarkdownNoteWrite, Title: "Test note",
		State: model.DraftPendingReview, Summary: "A summary here", ProposedContent: "Note content",
	}
	result := renderApprovalPane([]model.Draft{draft}, 0, 0, true, nil, 36, 20)
	if !strings.Contains(result, "Draft Review") {
		t.Errorf("approval detail should show 'Draft Review', got: %q", result)
	}
	if !strings.Contains(result, "a=approve") {
		t.Errorf("pending review detail should show approve action, got: %q", result)
	}
	if strings.Contains(result, "p=apply") {
		t.Errorf("pending review detail should NOT show apply action, got: %q", result)
	}
}

func TestRenderApprovalDetailApproved(t *testing.T) {
	draft := model.Draft{
		ID: "d1", Kind: model.DraftKindMarkdownNoteWrite, Title: "Test note",
		State: model.DraftApproved, Summary: "A summary here", ProposedContent: "Note content",
	}
	result := renderApprovalPane([]model.Draft{draft}, 0, 0, true, nil, 36, 20)
	if !strings.Contains(result, "p=apply") {
		t.Errorf("approved detail should show apply action, got: %q", result)
	}
	if strings.Contains(result, "a=approve") {
		t.Errorf("approved detail should NOT show approve action, got: %q", result)
	}
}

func TestRenderApprovalListScrolling(t *testing.T) {
	drafts := make([]model.Draft, 10)
	for i := range drafts {
		drafts[i] = model.Draft{ID: fmt.Sprintf("d%d", i), Kind: model.DraftKindMarkdownNoteWrite, Title: fmt.Sprintf("Draft %d", i), State: model.DraftPendingReview}
	}
	// Only 3 visible lines, cursor at index 7, offset should make items 5-7 visible
	result := renderApprovalPane(drafts, 7, 5, false, nil, 36, 4)
	if !strings.Contains(result, "Draft 7") {
		t.Errorf("scrolled list should show cursor item, got: %q", result)
	}
	if !strings.Contains(result, "8/10") {
		t.Errorf("scrolled list should show correct position, got: %q", result)
	}
	if strings.Contains(result, "Draft 0") {
		t.Errorf("scrolled list should NOT show off-screen item, got: %q", result)
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

func TestRenderApprovalDetailPersonaUpdateStructured(t *testing.T) {
	prop := model.PersonaUpdateProposal{
		Field:         "目标角色",
		CurrentValue:  "讲师",
		ProposedValue: "学生",
		Evidence:      "用户说'我是个学生'",
		Reason:        "用户自我描述与当前画像不一致",
		Confidence:    "high",
	}
	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatal(err)
	}

	draft := model.Draft{
		ID:              "d1",
		Kind:            model.DraftKindPersonaUpdate,
		State:           model.DraftPendingReview,
		Title:           "Persona update: 目标角色",
		ProposedContent: string(data),
	}
	result := renderApprovalPane([]model.Draft{draft}, 0, 0, true, nil, 60, 30)

	for _, expected := range []string{
		"目标角色",
		"学生",
		"讲师",
		"Evidence",
		"我是个学生",
		"Confidence",
		"high",
		"a=approve",
	} {
		if !strings.Contains(result, expected) {
			t.Errorf("persona_update detail missing %q in output:\n%s", expected, result)
		}
	}
}

func TestRenderApprovalDetailMarkdownNoteStructured(t *testing.T) {
	prop := model.MarkdownNoteProposal{
		Title:    "学习计划",
		Reason:   "每天下午有固定学习时间",
		Content:  "# 学习计划\n\n- 下午 2-5 点复习",
		Evidence: "用户说'我下午2-5点学习'",
	}
	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatal(err)
	}

	draft := model.Draft{
		ID:              "d2",
		Kind:            model.DraftKindMarkdownNoteWrite,
		State:           model.DraftPendingReview,
		Title:           "Markdown note: 学习计划",
		ProposedContent: string(data),
	}
	result := renderApprovalPane([]model.Draft{draft}, 0, 0, true, nil, 60, 30)

	for _, expected := range []string{
		"学习计划",
		"每天下午有固定学习时间",
		"下午 2-5 点复习",
		"Evidence",
		"Proposed Content",
	} {
		if !strings.Contains(result, expected) {
			t.Errorf("markdown_note detail missing %q in output:\n%s", expected, result)
		}
	}

	// Verify raw JSON is NOT leaked
	if strings.Contains(result, `"field"`) || strings.Contains(result, `"title"`) {
		t.Errorf("markdown_note detail should not show raw JSON keys, got:\n%s", result)
	}
}

func TestRenderApprovalDetailFallbackRawJSON(t *testing.T) {
	// Invalid JSON should fall back to generic Raw display
	draft := model.Draft{
		ID:              "d3",
		Kind:            model.DraftKindPersonaUpdate,
		State:           model.DraftPendingReview,
		Title:           "Broken proposal",
		ProposedContent: `{not valid json}`,
	}
	result := renderApprovalPane([]model.Draft{draft}, 0, 0, true, nil, 60, 30)

	// Should show the raw content as fallback
	if !strings.Contains(result, "{not valid json}") {
		t.Errorf("fallback should show raw ProposedContent, got:\n%s", result)
	}
	// Should NOT crash or show structured section headers
	if strings.Contains(result, "Plan") {
		t.Errorf("fallback should NOT show structured sections, got:\n%s", result)
	}
}
