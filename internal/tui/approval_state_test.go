package tui

import (
	"fmt"
	"testing"

	"obsidian-harness/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

// approvalDriverStub tracks actions and returns deterministic results.
type approvalDriverStub struct {
	drafts    []model.Draft
	actions   []string
	lastID    string
}

func (d *approvalDriverStub) Load(lastOutput string) (WorkbenchViewModel, error) {
	vm := WorkbenchViewModel{
		PendingDrafts: d.drafts,
		Snapshot:      WorkbenchSnapshot{PendingDrafts: len(d.drafts)},
	}
	return vm, nil
}

func (d *approvalDriverStub) Execute(line string, lastOutput string) (InteractiveWorkbenchUpdate, error) {
	vm, _ := d.Load(lastOutput)
	return InteractiveWorkbenchUpdate{ViewModel: vm, LastOutput: lastOutput}, nil
}

func (d *approvalDriverStub) ExecuteApprovalAction(action string, draftID string) (InteractiveWorkbenchUpdate, error) {
	d.actions = append(d.actions, action)
	d.lastID = draftID

	// Simulate state transition by mutating the stub's draft list.
	for i, draft := range d.drafts {
		if draft.ID != draftID {
			continue
		}
		switch action {
		case "approve":
			draft.State = model.DraftApproved
			d.drafts[i] = draft
		case "reject":
			// Remove from list (pending_review -> rejected, no longer reviewable)
			d.drafts = append(d.drafts[:i], d.drafts[i+1:]...)
		case "apply":
			// Remove from list (applied, no longer reviewable)
			d.drafts = append(d.drafts[:i], d.drafts[i+1:]...)
		}
		break
	}

	vm, _ := d.Load("")
	return InteractiveWorkbenchUpdate{ViewModel: vm, LastOutput: action + " " + draftID}, nil
}

func newApprovalModel(drafts []model.Draft) interactiveWorkbenchModel {
	driver := &approvalDriverStub{drafts: drafts}
	vm, _ := driver.Load("")
	m := newInteractiveWorkbenchModel(driver, vm)
	m.width = 100
	m.height = 30
	m.resize()
	m.refreshContent(true)
	return m
}

// --- State machine tests ---

func TestApprovalFlow_PendingApproveStillVisibleThenApply(t *testing.T) {
	drafts := []model.Draft{
		{ID: "d1", Kind: model.DraftKindMarkdownNoteWrite, Title: "Note A", State: model.DraftPendingReview},
	}
	m := newApprovalModel(drafts)

	// Focus the approval pane
	m.focus = focusApproval
	m.approvalDetail = true

	// Step 1: approve the pending draft
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(interactiveWorkbenchModel)

	// The approve command was dispatched
	if cmd == nil {
		t.Fatal("expected a command after pressing 'a'")
	}
	// Simulate the async result coming back
	result := cmd()
	msg, ok := result.(approvalResultMsg)
	if !ok {
		t.Fatal("expected approvalResultMsg from command")
	}
	updated, _ = m.Update(msg)
	m = updated.(interactiveWorkbenchModel)

	// Draft should still be in the list (now approved, not removed)
	if len(m.viewModel.PendingDrafts) != 1 {
		t.Fatalf("after approve: len(PendingDrafts) = %d, want 1 (still visible as approved)", len(m.viewModel.PendingDrafts))
	}
	if m.viewModel.PendingDrafts[0].State != model.DraftApproved {
		t.Fatalf("after approve: state = %s, want approved", m.viewModel.PendingDrafts[0].State)
	}

	// Detail view should have closed after action
	if m.approvalDetail {
		t.Fatal("approvalDetail should be false after action result")
	}

	// Step 2: open detail again and apply
	m.approvalDetail = true
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = updated.(interactiveWorkbenchModel)
	result = cmd()
	msg = result.(approvalResultMsg)
	updated, _ = m.Update(msg)
	m = updated.(interactiveWorkbenchModel)

	// Draft should now be removed from the list
	if len(m.viewModel.PendingDrafts) != 0 {
		t.Fatalf("after apply: len(PendingDrafts) = %d, want 0", len(m.viewModel.PendingDrafts))
	}
	if m.approvalCursor != 0 || m.approvalOffset != 0 {
		t.Fatalf("after apply emptied list: cursor=%d offset=%d, want 0,0", m.approvalCursor, m.approvalOffset)
	}
}

func TestApprovalFlow_PendingPressPIsNoOp(t *testing.T) {
	drafts := []model.Draft{
		{ID: "d1", Kind: model.DraftKindMarkdownNoteWrite, Title: "Note A", State: model.DraftPendingReview},
	}
	m := newApprovalModel(drafts)
	m.focus = focusApproval
	m.approvalDetail = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = updated.(interactiveWorkbenchModel)

	// No command dispatched — apply is not allowed on pending_review
	if cmd != nil {
		t.Fatal("pressing 'p' on pending_review should not dispatch a command")
	}
	if len(m.viewModel.PendingDrafts) != 1 {
		t.Fatal("pending draft should still be there after pressing 'p'")
	}
}

func TestApprovalFlow_ApprovedPressAAndRAreNoOp(t *testing.T) {
	drafts := []model.Draft{
		{ID: "d1", Kind: model.DraftKindMarkdownNoteWrite, Title: "Note A", State: model.DraftApproved},
	}
	m := newApprovalModel(drafts)
	m.focus = focusApproval
	m.approvalDetail = true

	for _, key := range []rune{'a', 'r'} {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m2 := updated.(interactiveWorkbenchModel)
		if cmd != nil {
			t.Fatalf("pressing '%c' on approved draft should not dispatch a command", key)
		}
		if len(m2.viewModel.PendingDrafts) != 1 {
			t.Fatalf("pressing '%c': approved draft should still be there", key)
		}
	}
}

func TestApprovalFlow_ScrollThenActionShrinksList(t *testing.T) {
	drafts := make([]model.Draft, 8)
	for i := range drafts {
		drafts[i] = model.Draft{
			ID:    fmt.Sprintf("d%d", i),
			Kind:  model.DraftKindMarkdownNoteWrite,
			Title: fmt.Sprintf("Draft %d", i),
			State: model.DraftPendingReview,
		}
	}
	m := newApprovalModel(drafts)
	m.focus = focusApproval

	// Scroll down to item 5
	for i := 0; i < 5; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(interactiveWorkbenchModel)
	}
	if m.approvalCursor != 5 {
		t.Fatalf("cursor = %d, want 5", m.approvalCursor)
	}

	// Open detail and reject it
	m.approvalDetail = true
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(interactiveWorkbenchModel)
	result := cmd()
	msg := result.(approvalResultMsg)
	updated, _ = m.Update(msg)
	m = updated.(interactiveWorkbenchModel)

	// List now has 7 items, cursor should be clamped, offset should be valid
	if len(m.viewModel.PendingDrafts) != 7 {
		t.Fatalf("after reject: len = %d, want 7", len(m.viewModel.PendingDrafts))
	}
	if m.approvalCursor > 6 {
		t.Fatalf("cursor = %d, should be <= 6", m.approvalCursor)
	}
	if m.approvalOffset < 0 || m.approvalOffset > m.approvalCursor {
		t.Fatalf("offset = %d, cursor = %d — offset should be >= 0 and <= cursor", m.approvalOffset, m.approvalCursor)
	}
}
