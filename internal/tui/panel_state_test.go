package tui

import (
	"fmt"
	"strings"
	"testing"

	"obsidian-harness/internal/app"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/operatoragent"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Findings state machine tests ---

func TestFindingsListCursorMovement(t *testing.T) {
	findings := []model.Finding{
		{ID: "f1", Title: "Finding A", State: model.FindingOpen, Severity: model.FindingSeverityWarning},
		{ID: "f2", Title: "Finding B", State: model.FindingOpen, Severity: model.FindingSeverityCritical},
		{ID: "f3", Title: "Finding C", State: model.FindingResolved, Severity: model.FindingSeverityInfo},
	}
	driver := &panelDriverStub{findings: findings}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusFindings

	// Initial cursor should be 0
	if m.findingsCursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.findingsCursor)
	}

	// Move down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(interactiveWorkbenchModel)
	if m.findingsCursor != 1 {
		t.Fatalf("after j: cursor = %d, want 1", m.findingsCursor)
	}

	// Move down again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(interactiveWorkbenchModel)
	if m.findingsCursor != 2 {
		t.Fatalf("after second j: cursor = %d, want 2", m.findingsCursor)
	}

	// Move down past end — should stay
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(interactiveWorkbenchModel)
	if m.findingsCursor != 2 {
		t.Fatalf("after third j: cursor = %d, want 2 (clamped)", m.findingsCursor)
	}

	// Move up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(interactiveWorkbenchModel)
	if m.findingsCursor != 1 {
		t.Fatalf("after k: cursor = %d, want 1", m.findingsCursor)
	}
}

func TestFindingsOffsetUsesFindingsPanelHeight(t *testing.T) {
	findings := make([]model.Finding, 8)
	for i := range findings {
		findings[i] = model.Finding{
			ID:       fmt.Sprintf("f%d", i),
			Title:    fmt.Sprintf("Finding %d", i),
			State:    model.FindingOpen,
			Severity: model.FindingSeverityWarning,
		}
	}
	driver := &panelDriverStub{findings: findings}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.focus = focusFindings
	m.approvalViewport.Height = 1
	m.findingsHeight = 4

	for i := 0; i < 3; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(interactiveWorkbenchModel)
	}
	if m.findingsCursor != 3 {
		t.Fatalf("cursor = %d, want 3", m.findingsCursor)
	}
	if m.findingsOffset != 1 {
		t.Fatalf("findings offset = %d, want 1 based on findingsHeight=4", m.findingsOffset)
	}
}

func TestFindingsDetailToggle(t *testing.T) {
	findings := []model.Finding{
		{ID: "f1", Title: "Finding A", State: model.FindingOpen, Severity: model.FindingSeverityWarning},
	}
	driver := &panelDriverStub{findings: findings}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusFindings

	if m.findingsDetail {
		t.Fatal("should start in list view")
	}

	// Enter detail
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(interactiveWorkbenchModel)
	if !m.findingsDetail {
		t.Fatal("after enter: should be in detail view")
	}

	// Esc back to list
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(interactiveWorkbenchModel)
	if m.findingsDetail {
		t.Fatal("after esc: should be back in list view")
	}
}

func TestFindingsResolveAction(t *testing.T) {
	findings := []model.Finding{
		{ID: "f1", Title: "Finding A", State: model.FindingOpen, Severity: model.FindingSeverityWarning},
	}
	driver := &panelDriverStub{findings: findings}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusFindings
	m.findingsDetail = true

	// Press x to resolve
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(interactiveWorkbenchModel)

	if cmd == nil {
		t.Fatal("expected a command after pressing 'x'")
	}

	// Simulate result
	result := cmd()
	msg, ok := result.(findingsResultMsg)
	if !ok {
		t.Fatalf("expected findingsResultMsg, got %T", result)
	}
	if msg.action != "resolve" {
		t.Fatalf("action = %q, want resolve", msg.action)
	}
	if msg.findingID != "f1" {
		t.Fatalf("findingID = %q, want f1", msg.findingID)
	}

	// Process result — verify finding state changed
	updated, _ = m.Update(msg)
	m = updated.(interactiveWorkbenchModel)
	if len(m.viewModel.Findings) != 1 {
		t.Fatalf("findings count = %d, want 1", len(m.viewModel.Findings))
	}
	if m.viewModel.Findings[0].State != model.FindingResolved {
		t.Fatalf("finding state = %s, want resolved", m.viewModel.Findings[0].State)
	}
	if m.findingsDetail {
		t.Fatal("detail should close after action")
	}
}

func TestFindingsIgnoreAction(t *testing.T) {
	findings := []model.Finding{
		{ID: "f1", Title: "Finding A", State: model.FindingOpen, Severity: model.FindingSeverityWarning},
	}
	driver := &panelDriverStub{findings: findings}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusFindings
	m.findingsDetail = true

	// Press i to ignore
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = updated.(interactiveWorkbenchModel)

	if cmd == nil {
		t.Fatal("expected a command after pressing 'i'")
	}

	result := cmd()
	msg, ok := result.(findingsResultMsg)
	if !ok {
		t.Fatalf("expected findingsResultMsg, got %T", result)
	}
	if msg.action != "ignore" {
		t.Fatalf("action = %q, want ignore", msg.action)
	}

	// Process result — verify finding state changed
	updated, _ = m.Update(msg)
	m = updated.(interactiveWorkbenchModel)
	if m.viewModel.Findings[0].State != model.FindingIgnored {
		t.Fatalf("finding state = %s, want ignored", m.viewModel.Findings[0].State)
	}
}

func TestFindingsNoOpOnNonOpenState(t *testing.T) {
	findings := []model.Finding{
		{ID: "f1", Title: "Finding A", State: model.FindingResolved, Severity: model.FindingSeverityWarning},
	}
	driver := &panelDriverStub{findings: findings}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusFindings
	m.findingsDetail = true

	// Press x on already-resolved — should be no-op
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(interactiveWorkbenchModel)
	if cmd != nil {
		t.Fatal("x on resolved finding should be no-op (nil cmd)")
	}
	// Press i on already-resolved — should be no-op
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = updated.(interactiveWorkbenchModel)
	if cmd != nil {
		t.Fatal("i on resolved finding should be no-op (nil cmd)")
	}
}

// --- Process-sink timeline state machine tests ---

func TestSinkTimelineCursorMovement(t *testing.T) {
	sink := app.ProcessSinkDayView{
		AgentID: "codex",
		Checkpoints: []model.CheckpointDoc{
			{Title: "CP1", State: model.CheckpointMaterialized},
			{Title: "CP2", State: model.CheckpointMaterialized},
			{Title: "CP3", State: model.CheckpointMaterialized},
		},
	}
	driver := &panelDriverStub{sink: sink}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusProcessSink

	if m.sinkCursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.sinkCursor)
	}

	// Move down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(interactiveWorkbenchModel)
	if m.sinkCursor != 1 {
		t.Fatalf("after j: cursor = %d, want 1", m.sinkCursor)
	}

	// Move up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(interactiveWorkbenchModel)
	if m.sinkCursor != 0 {
		t.Fatalf("after k: cursor = %d, want 0", m.sinkCursor)
	}
}

func TestSinkOffsetUsesSinkPanelHeight(t *testing.T) {
	checkpoints := make([]model.CheckpointDoc, 8)
	for i := range checkpoints {
		checkpoints[i] = model.CheckpointDoc{
			Title: fmt.Sprintf("CP%d", i),
			State: model.CheckpointMaterialized,
		}
	}
	sink := app.ProcessSinkDayView{
		AgentID:     "codex",
		Checkpoints: checkpoints,
	}
	driver := &panelDriverStub{sink: sink}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.focus = focusProcessSink
	m.approvalViewport.Height = 1
	m.sinkHeight = 5

	for i := 0; i < 4; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(interactiveWorkbenchModel)
	}
	if m.sinkCursor != 4 {
		t.Fatalf("cursor = %d, want 4", m.sinkCursor)
	}
	if m.sinkOffset != 2 {
		t.Fatalf("sink offset = %d, want 2 based on sinkHeight=5 and two reserved rows", m.sinkOffset)
	}
}

func TestApprovalOffsetUsesApprovalPanelHeight(t *testing.T) {
	drafts := make([]model.Draft, 8)
	for i := range drafts {
		drafts[i] = model.Draft{
			ID:    fmt.Sprintf("d%d", i),
			Title: fmt.Sprintf("Draft %d", i),
			State: model.DraftPendingReview,
		}
	}
	driver := &panelDriverStub{drafts: drafts}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.focus = focusApproval
	m.approvalViewport.Height = 99
	m.approvalHeight = 3

	for i := 0; i < 4; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(interactiveWorkbenchModel)
	}
	if m.approvalCursor != 4 {
		t.Fatalf("cursor = %d, want 4", m.approvalCursor)
	}
	if m.approvalOffset != 3 {
		t.Fatalf("approval offset = %d, want 3 based on approvalHeight=3", m.approvalOffset)
	}
}

func TestSinkDetailToggle(t *testing.T) {
	sink := app.ProcessSinkDayView{
		AgentID: "codex",
		Checkpoints: []model.CheckpointDoc{
			{Title: "CP1", State: model.CheckpointMaterialized},
		},
	}
	driver := &panelDriverStub{sink: sink}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusProcessSink

	if m.sinkDetail {
		t.Fatal("should start in list view")
	}

	// Enter detail
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(interactiveWorkbenchModel)
	if !m.sinkDetail {
		t.Fatal("after enter: should be in detail view")
	}

	// Esc back
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(interactiveWorkbenchModel)
	if m.sinkDetail {
		t.Fatal("after esc: should be back in list view")
	}
}

// --- Tab cycle tests ---

func TestTabCycleIncludesNewPanels(t *testing.T) {
	_ = &panelDriverStub{}

	// Tab through all states: input → conversation → status → findings → sink → approval → input
	expectedCycle := []interactiveFocus{
		focusConversation,
		focusStatus,
		focusFindings,
		focusProcessSink,
		focusApproval,
		focusCandidates,
		focusInput,
	}

	current := interactiveFocus(focusInput)
	for i, expected := range expectedCycle {
		current = nextFocus(current)
		if current != expected {
			t.Fatalf("step %d: nextFocus = %d, want %d", i, current, expected)
		}
	}

	// Reverse cycle
	reverseCycle := []interactiveFocus{
		focusCandidates,
		focusApproval,
		focusProcessSink,
		focusFindings,
		focusStatus,
		focusConversation,
		focusInput,
	}

	current = interactiveFocus(focusInput)
	for i, expected := range reverseCycle {
		current = previousFocus(current)
		if current != expected {
			t.Fatalf("reverse step %d: previousFocus = %d, want %d", i, current, expected)
		}
	}
}

// --- Error prefix regression tests ---

func TestFindingsActionErrorSurfacedWithCorrectPrefix(t *testing.T) {
	findings := []model.Finding{
		{ID: "f1", Title: "Finding A", State: model.FindingOpen, Severity: model.FindingSeverityWarning},
	}
	driver := &errorDriverStub{findings: findings}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusFindings
	m.findingsDetail = true

	// Press x — the error driver will return an error
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(interactiveWorkbenchModel)
	if cmd == nil {
		t.Fatal("expected a command")
	}

	result := cmd()
	msg := result.(findingsResultMsg)
	updated, _ = m.Update(msg)
	m = updated.(interactiveWorkbenchModel)

	// lastOutput must start with "Error:" so the renderer surfaces it
	if !strings.HasPrefix(m.lastOutput, "Error:") {
		t.Fatalf("findings error lastOutput = %q, want Error: prefix", m.lastOutput)
	}
}

func TestApprovalActionErrorSurfacedWithCorrectPrefix(t *testing.T) {
	drafts := []model.Draft{
		{ID: "d1", Title: "Draft A", State: model.DraftPendingReview},
	}
	driver := &errorDriverStub{drafts: drafts}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	m.focus = focusApproval
	m.approvalDetail = true

	// Press a — the error driver will return an error
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(interactiveWorkbenchModel)
	if cmd == nil {
		t.Fatal("expected a command")
	}

	result := cmd()
	msg := result.(approvalResultMsg)
	updated, _ = m.Update(msg)
	m = updated.(interactiveWorkbenchModel)

	if !strings.HasPrefix(m.lastOutput, "Error:") {
		t.Fatalf("approval error lastOutput = %q, want Error: prefix", m.lastOutput)
	}
}

// --- Shared stub for panel tests ---

type panelDriverStub struct {
	findings     []model.Finding
	sink         app.ProcessSinkDayView
	drafts       []model.Draft
	conversation []operatoragent.ConversationTurn
	models       []ModelInfo
	profiles     []ModelInfo
	presets      []ModelProfilePreset
	candidates   []PersonaCandidateInfo
	errors       []ErrorEntry
	persisted    []string
	created      []string
}

func (d *panelDriverStub) Load(lastOutput string) (WorkbenchViewModel, error) {
	return WorkbenchViewModel{
		Findings:      d.findings,
		ProcessSink:   d.sink,
		PendingDrafts: d.drafts,
		Snapshot:      WorkbenchSnapshot{},
		Conversation: WorkbenchConversation{
			Turns: d.conversation,
		},
	}, nil
}

func (d *panelDriverStub) Execute(line string, lastOutput string) (InteractiveWorkbenchUpdate, error) {
	vm, _ := d.Load(lastOutput)
	return InteractiveWorkbenchUpdate{ViewModel: vm, LastOutput: lastOutput}, nil
}

func (d *panelDriverStub) ExecuteApprovalAction(action string, draftID string) (InteractiveWorkbenchUpdate, error) {
	vm, _ := d.Load("")
	return InteractiveWorkbenchUpdate{ViewModel: vm, LastOutput: action + " " + draftID}, nil
}

func (d *panelDriverStub) ExecuteFindingAction(action string, findingID string) (InteractiveWorkbenchUpdate, error) {
	// Simulate state transition on findings
	for i, f := range d.findings {
		if f.ID != findingID {
			continue
		}
		switch action {
		case "resolve":
			f.State = model.FindingResolved
			d.findings[i] = f
		case "ignore":
			f.State = model.FindingIgnored
			d.findings[i] = f
		}
		break
	}
	vm, _ := d.Load("")
	return InteractiveWorkbenchUpdate{ViewModel: vm, LastOutput: action + " " + findingID}, nil
}

func (d *panelDriverStub) DiscoverModels() ([]ModelInfo, error) {
	return d.models, nil
}

func (d *panelDriverStub) SwitchModel(name string) ([]ModelInfo, error) {
	for i := range d.models {
		d.models[i].Current = (d.models[i].Name == name)
	}
	return d.models, nil
}

func (d *panelDriverStub) TestModel(name string) error {
	return nil
}

func (d *panelDriverStub) ListProfiles() ([]ModelInfo, error) {
	return d.profiles, nil
}

func (d *panelDriverStub) CreateProfileFromPreset(presetName string, profileName string) error {
	d.created = append(d.created, presetName+":"+profileName)
	d.profiles = append(d.profiles, ModelInfo{
		Name:        presetName + "-model",
		Provider:    "preset",
		Source:      "workspace",
		KeyStatus:   "present",
		ProfileName: profileName,
		IsProfile:   true,
	})
	return nil
}

func (d *panelDriverStub) PersistActiveProfile(profileName string) error {
	d.persisted = append(d.persisted, profileName)
	for i := range d.profiles {
		d.profiles[i].Active = d.profiles[i].ProfileName == profileName
	}
	return nil
}

func (d *panelDriverStub) AvailablePresets() []ModelProfilePreset {
	if d.presets != nil {
		return d.presets
	}
	return []ModelProfilePreset{{
		Name:      "deepseek",
		Provider:  "deepseek",
		BaseURL:   "https://api.deepseek.com/v1",
		Model:     "deepseek-v4-pro",
		APIKeyEnv: "DEEPSEEK_API_KEY",
	}}
}

func (d *panelDriverStub) ListPersonaCandidates() ([]PersonaCandidateInfo, error) {
	return d.candidates, nil
}

func (d *panelDriverStub) DraftPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return d.candidates, nil
}

func (d *panelDriverStub) DismissPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return d.candidates, nil
}

func (d *panelDriverStub) RecoverPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return d.candidates, nil
}

func (d *panelDriverStub) ListErrors() ([]ErrorEntry, error) {
	return d.errors, nil
}

func (d *panelDriverStub) RetryPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return d.candidates, nil
}

// errorDriverStub always returns errors from ExecuteFindingAction and ExecuteApprovalAction.
type errorDriverStub struct {
	findings []model.Finding
	drafts   []model.Draft
}

func (d *errorDriverStub) Load(lastOutput string) (WorkbenchViewModel, error) {
	return WorkbenchViewModel{
		Findings:      d.findings,
		PendingDrafts: d.drafts,
		Snapshot:      WorkbenchSnapshot{},
	}, nil
}

func (d *errorDriverStub) Execute(line string, lastOutput string) (InteractiveWorkbenchUpdate, error) {
	vm, _ := d.Load(lastOutput)
	return InteractiveWorkbenchUpdate{ViewModel: vm, LastOutput: lastOutput}, nil
}

func (d *errorDriverStub) ExecuteApprovalAction(action string, draftID string) (InteractiveWorkbenchUpdate, error) {
	return InteractiveWorkbenchUpdate{}, fmt.Errorf("approval %s %s failed: stub error", action, draftID)
}

func (d *errorDriverStub) ExecuteFindingAction(action string, findingID string) (InteractiveWorkbenchUpdate, error) {
	return InteractiveWorkbenchUpdate{}, fmt.Errorf("finding %s %s failed: stub error", action, findingID)
}

func (d *errorDriverStub) DiscoverModels() ([]ModelInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *errorDriverStub) SwitchModel(name string) ([]ModelInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *errorDriverStub) TestModel(name string) error {
	return fmt.Errorf("not implemented")
}

func (d *errorDriverStub) ListProfiles() ([]ModelInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *errorDriverStub) CreateProfileFromPreset(presetName string, profileName string) error {
	return fmt.Errorf("not implemented")
}

func (d *errorDriverStub) PersistActiveProfile(profileName string) error {
	return fmt.Errorf("not implemented")
}

func (d *errorDriverStub) AvailablePresets() []ModelProfilePreset {
	return nil
}

func (d *errorDriverStub) ListPersonaCandidates() ([]PersonaCandidateInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *errorDriverStub) DraftPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *errorDriverStub) DismissPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *errorDriverStub) RecoverPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *errorDriverStub) ListErrors() ([]ErrorEntry, error) {
	return nil, nil
}

func (d *errorDriverStub) RetryPersonaCandidate(id string) ([]PersonaCandidateInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

// TestP0PaneOverlapPrevention verifies that the layout never exceeds
// model.height lines and that pane content doesn't bleed across panes
// when Tab-cycling through all focus states with long conversation content.
func TestP0PaneOverlapPrevention(t *testing.T) {
	// Build 50 long conversation turns that fill the chat viewport
	turns := make([]operatoragent.ConversationTurn, 50)
	for i := range turns {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		turns[i] = operatoragent.ConversationTurn{
			Role:    role,
			Content: fmt.Sprintf("Turn %d: This is a very long conversation message that wraps across multiple lines to simulate real usage patterns with extensive dialogue and detailed responses from the LLM agent.", i),
		}
	}

	drafts := []model.Draft{
		{ID: "d1", Title: "Persona update proposal: target_role -> 学生", State: model.DraftPendingReview},
	}
	findings := []model.Finding{
		{ID: "f1", Title: "Test finding", State: model.FindingOpen, Severity: model.FindingSeverityWarning},
	}
	checkpoints := []model.CheckpointDoc{
		{Title: "CP 1 09:00-09:30", State: model.CheckpointMaterialized},
		{Title: "CP 2 10:00-10:30", State: model.CheckpointMaterialized},
	}

	driver := &panelDriverStub{
		drafts:       drafts,
		findings:     findings,
		conversation: turns,
		sink: app.ProcessSinkDayView{
			Checkpoints: checkpoints,
		},
	}

	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	// Terminal dimensions: 160 columns x 32 rows
	m.width = 160
	m.height = 32
	m.resize()
	m.refreshContent(true)

	// Cycle through all 6 focus states
	focusOrder := []interactiveFocus{
		focusInput, focusConversation, focusStatus,
		focusFindings, focusProcessSink, focusApproval, focusCandidates,
	}

	for _, f := range focusOrder {
		m.focus = f
		m.resize()
		m.clampCurrentPanelOffset()
		m.refreshContent(false)

		rendered := m.View()
		lines := strings.Split(rendered, "\n")
		if len(lines) > m.height {
			t.Fatalf("focus %d: rendered %d lines, want ≤ %d (height). First 40 lines:\n%s", f, len(lines), m.height, strings.Join(lines[:minInt(40, len(lines))], "\n"))
		}

		// Verify Reviewable Drafts appears at most once (no overlap repeat)
		draftCount := 0
		for _, line := range lines {
			if strings.Contains(line, "Reviewable Drafts") {
				draftCount++
			}
		}
		if draftCount > 1 {
			t.Fatalf("focus %d: 'Reviewable Drafts' appears %d times, want ≤ 1 (pane overlap)", f, draftCount)
		}
	}

	// Verify full Tab cycle: start at focusInput, tab through every focus state, should return to input
	m.focus = focusInput
	for i := 0; i < len(focusOrder); i++ {
		m.focus = nextFocus(m.focus)
		m.resize()
		m.clampCurrentPanelOffset()
		m.refreshContent(false)
		rendered := m.View()
		lines := strings.Split(rendered, "\n")
		if len(lines) > m.height {
			t.Fatalf("Tab cycle step %d: rendered %d lines > height %d", i, len(lines), m.height)
		}
	}
	if m.focus != focusInput {
		t.Fatalf("after 6 Tabs, focus = %d, want %d (focusInput)", m.focus, focusInput)
	}
}

// --- Model panel tests ---

func TestModelPanelEnterExit(t *testing.T) {
	models := []ModelInfo{
		{Name: "gpt-5.4", Provider: "openai", Source: "discovered", KeyStatus: "OK"},
		{Name: "deepseek-v4-pro", Provider: "deepseek", Source: "discovered", KeyStatus: "OK", Current: true},
	}
	driver := &panelDriverStub{models: models}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	// Enter model panel — handleLocalCommand returns a Cmd that populates the list
	handled, updated, cmd := m.handleLocalCommand("/model")
	if !handled {
		t.Fatal("/model should be handled as local command")
	}
	m = *updated.(*interactiveWorkbenchModel)
	if !m.modelPanelActive {
		t.Fatal("model panel should be active after /model")
	}
	// Execute the Cmd to populate model list
	if cmd == nil {
		t.Fatal("/model should return a Cmd to load models")
	}
	result := cmd()
	if msg, ok := result.(modelPanelMsg); ok && msg.err != nil {
		t.Fatalf("loading model list: %v", msg.err)
	}
	// Process the msg in Update
	updated, _ = m.Update(result)
	m = updated.(interactiveWorkbenchModel)
	if len(m.modelPanelList) == 0 {
		t.Fatal("model list should be populated after /model")
	}

	// Exit with esc
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(interactiveWorkbenchModel)
	if m.modelPanelActive {
		t.Fatal("model panel should not be active after esc")
	}
}

func TestModelPanelNavigation(t *testing.T) {
	models := []ModelInfo{
		{Name: "gpt-5.4", Provider: "openai", Source: "discovered", KeyStatus: "OK"},
		{Name: "deepseek-v4-pro", Provider: "deepseek", Source: "discovered", KeyStatus: "OK", Current: true},
		{Name: "claude-4", Provider: "anthropic", Source: "discovered", KeyStatus: "OK"},
	}
	driver := &panelDriverStub{models: models}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	handled, updated, cmd := m.handleLocalCommand("/model")
	if !handled {
		t.Fatal("/model should be handled")
	}
	m = *updated.(*interactiveWorkbenchModel)
	// Execute Cmd to populate list
	result := cmd()
	updated, _ = m.Update(result)
	m = updated.(interactiveWorkbenchModel)
	if m.modelPanelCursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.modelPanelCursor)
	}

	// j moves down
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(interactiveWorkbenchModel)
	if m.modelPanelCursor != 1 {
		t.Fatalf("after j cursor = %d, want 1", m.modelPanelCursor)
	}

	// k moves up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(interactiveWorkbenchModel)
	if m.modelPanelCursor != 0 {
		t.Fatalf("after k cursor = %d, want 0", m.modelPanelCursor)
	}

	// k at top should stay
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(interactiveWorkbenchModel)
	if m.modelPanelCursor != 0 {
		t.Fatalf("after k at top cursor = %d, want 0", m.modelPanelCursor)
	}

	// j at bottom should stay
	for i := 0; i < 5; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(interactiveWorkbenchModel)
	}
	if m.modelPanelCursor != 2 {
		t.Fatalf("after j at bottom cursor = %d, want 2", m.modelPanelCursor)
	}
}

func TestModelLocalCommandCurrent(t *testing.T) {
	vm := WorkbenchViewModel{
		Snapshot: WorkbenchSnapshot{CurrentModel: "gpt-5.4 @ api.ikuncode.cc"},
	}
	m := newInteractiveWorkbenchModel(&panelDriverStub{}, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	handled, _, _ := m.handleLocalCommand("/model current")
	if !handled {
		t.Fatal("/model current should be handled")
	}
	if !strings.Contains(m.lastOutput, "gpt-5.4") {
		t.Fatalf("current model output should contain model name, got: %q", m.lastOutput)
	}
}

func TestModelPanelRenderShowsFooterActions(t *testing.T) {
	models := []ModelInfo{
		{Name: "gpt-5.4", Provider: "openai", BaseURL: "https://api.test.com/v1", Source: "discovered", KeyStatus: "OK"},
		{Name: "deepseek-v4-pro", Provider: "deepseek", BaseURL: "https://api.test.com/v1", Source: "discovered", KeyStatus: "OK"},
	}
	result := renderModelPanel(models, 0, false, "", 50, 10)
	for _, expected := range []string{"r=refresh", "t=test", "enter=use", "e=edit", "esc=back"} {
		if !strings.Contains(result, expected) {
			t.Errorf("model panel footer missing %q in output:\n%s", expected, result)
		}
	}
	if !strings.Contains(result, "api.test.com") {
		t.Errorf("model panel should show base URL (not secret), got:\n%s", result)
	}
	// API key must not appear
	if strings.Contains(result, "sk-") || strings.Contains(result, "API_KEY") {
		t.Errorf("model panel must NOT contain API key, got:\n%s", result)
	}
}

func TestModelPanelRenderEmpty(t *testing.T) {
	result := renderModelPanel(nil, 0, false, "", 50, 10)
	if !strings.Contains(result, "No models discovered") {
		t.Errorf("empty state should show 'No models discovered', got: %q", result)
	}
}

func TestModelPanelProfilesPersistSelectedProfile(t *testing.T) {
	profiles := []ModelInfo{
		{Name: "gpt-5.4", Provider: "openai", Source: "workspace", KeyStatus: "present", ProfileName: "openai", IsProfile: true, Active: true},
		{Name: "deepseek-v4-pro", Provider: "deepseek", Source: "workspace", KeyStatus: "present", ProfileName: "deepseek", IsProfile: true},
	}
	driver := &panelDriverStub{profiles: profiles}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	handled, updated, cmd := m.handleLocalCommand("/model profiles")
	if !handled {
		t.Fatal("/model profiles should be handled")
	}
	m = *updated.(*interactiveWorkbenchModel)
	if cmd == nil {
		t.Fatal("/model profiles should return a Cmd")
	}
	updated, _ = m.Update(cmd())
	m = updated.(interactiveWorkbenchModel)
	if m.modelPanelMode != modelPanelProfiles {
		t.Fatalf("modelPanelMode = %d, want profiles", m.modelPanelMode)
	}
	if len(m.modelPanelList) != 2 {
		t.Fatalf("len(modelPanelList) = %d, want 2", len(m.modelPanelList))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(interactiveWorkbenchModel)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(interactiveWorkbenchModel)
	if cmd == nil {
		t.Fatal("enter on profile should return persist Cmd")
	}
	updated, _ = m.Update(cmd())
	m = updated.(interactiveWorkbenchModel)

	if got := strings.Join(driver.persisted, ","); got != "deepseek" {
		t.Fatalf("persisted profiles = %q, want deepseek", got)
	}
	if !strings.Contains(m.lastOutput, "Workspace active profile set to: deepseek") {
		t.Fatalf("lastOutput = %q, want profile persisted message", m.lastOutput)
	}
	if !driver.profiles[1].Active {
		t.Fatal("selected profile Active = false, want true")
	}
}

func TestModelPanelCreateProfileFromPreset(t *testing.T) {
	driver := &panelDriverStub{
		profiles: nil,
		presets: []ModelProfilePreset{{
			Name:      "deepseek",
			Provider:  "deepseek",
			BaseURL:   "https://api.deepseek.com/v1",
			Model:     "deepseek-v4-pro",
			APIKeyEnv: "DEEPSEEK_API_KEY",
		}},
	}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	handled, updated, cmd := m.handleLocalCommand("/model profiles")
	if !handled {
		t.Fatal("/model profiles should be handled")
	}
	m = *updated.(*interactiveWorkbenchModel)
	updated, _ = m.Update(cmd())
	m = updated.(interactiveWorkbenchModel)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(interactiveWorkbenchModel)
	if m.modelPanelMode != modelPanelPresets {
		t.Fatalf("modelPanelMode = %d, want presets", m.modelPanelMode)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(interactiveWorkbenchModel)
	if cmd == nil {
		t.Fatal("enter on preset should create profile")
	}
	updated, _ = m.Update(cmd())
	m = updated.(interactiveWorkbenchModel)

	if got := strings.Join(driver.created, ","); got != "deepseek:deepseek" {
		t.Fatalf("created profiles = %q, want deepseek:deepseek", got)
	}
	if !strings.Contains(m.lastOutput, "Profile created: deepseek") {
		t.Fatalf("lastOutput = %q, want profile created message", m.lastOutput)
	}
	if m.modelPanelMode != modelPanelProfiles {
		t.Fatalf("modelPanelMode after create = %d, want profiles", m.modelPanelMode)
	}
}

func TestModelProfileAndPresetRenderDoNotLeakSecrets(t *testing.T) {
	profiles := []ModelInfo{{
		Name:        "deepseek-v4-pro",
		Provider:    "deepseek",
		BaseURL:     "https://api.deepseek.com/v1",
		Source:      "workspace",
		KeyStatus:   "present",
		ProfileName: "deepseek",
		IsProfile:   true,
		Active:      true,
		APIKeyEnv:   "DEEPSEEK_API_KEY",
	}}
	profilePanel := renderProfilePanel(profiles, 0, false, "", 96, 8)
	presetPanel := renderPresetPicker([]ModelProfilePreset{{
		Name:        "deepseek",
		Provider:    "deepseek",
		BaseURL:     "https://api.deepseek.com/v1",
		Model:       "deepseek-v4-pro",
		APIKeyEnv:   "DEEPSEEK_API_KEY",
		Description: "DeepSeek official endpoint",
	}}, 0, 96, 8)
	combined := profilePanel + "\n" + presetPanel
	for _, expected := range []string{"deepseek", "deepseek-v4-pro", "DEEPSEEK_API_KEY"} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("rendered panel missing %q:\n%s", expected, combined)
		}
	}
	for _, forbidden := range []string{"sk-", "deepseek-secret", "Bearer "} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("rendered panel leaked %q:\n%s", forbidden, combined)
		}
	}
}

func TestErrorsCommandRendersDiagnosticsWithHintsAndNoSecrets(t *testing.T) {
	driver := &panelDriverStub{errors: []ErrorEntry{{
		Time:    "2026-05-28 19:20",
		Stage:   "persona_extract",
		Model:   "deepseek-v4-pro",
		BaseURL: "https://api.deepseek.com/v1",
		Error:   "400 model not found: use deepseek-v4-pro, not v4-pro",
	}}}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	handled, updated, cmd := m.handleLocalCommand("/errors")
	if !handled {
		t.Fatal("/errors should be handled")
	}
	m = *updated.(*interactiveWorkbenchModel)
	if cmd == nil {
		t.Fatal("/errors should return diagnostics Cmd")
	}
	updated, _ = m.Update(cmd())
	m = updated.(interactiveWorkbenchModel)

	for _, want := range []string{
		"Recent Errors",
		"persona_extract",
		"deepseek-v4-pro",
		"https://api.deepseek.com/v1",
		"hint:",
		"Model name may be incorrect",
	} {
		if !strings.Contains(m.lastOutput, want) {
			t.Fatalf("error diagnostics output missing %q:\n%s", want, m.lastOutput)
		}
	}
	for _, forbidden := range []string{"sk-", "Bearer ", "deepseek-secret"} {
		if strings.Contains(m.lastOutput, forbidden) {
			t.Fatalf("error diagnostics leaked %q:\n%s", forbidden, m.lastOutput)
		}
	}
}

func TestErrorsCommandEmptyState(t *testing.T) {
	driver := &panelDriverStub{}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)

	handled, updated, cmd := m.handleLocalCommand("/errors")
	if !handled {
		t.Fatal("/errors should be handled")
	}
	m = *updated.(*interactiveWorkbenchModel)
	if cmd == nil {
		t.Fatal("/errors should return diagnostics Cmd")
	}
	updated, _ = m.Update(cmd())
	m = updated.(interactiveWorkbenchModel)
	if m.lastOutput != "No recent errors." {
		t.Fatalf("lastOutput = %q, want empty diagnostics message", m.lastOutput)
	}
}
