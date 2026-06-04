package app

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/memory"
)

// auditFailingStateStore wraps a real StateStore but hands out an Audit
// store whose AppendAudit always fails, so a test can exercise the
// state-changed-but-audit-failed path.
type auditFailingStateStore struct {
	store.StateStore
}

func (auditFailingStateStore) Audit() store.AuditStore { return failingAuditStore{} }

type failingAuditStore struct{}

func (failingAuditStore) AppendAudit(model.AuditRecord) error {
	return errors.New("audit store unavailable")
}

func (failingAuditStore) ListAudit(int) ([]model.AuditRecord, error) { return nil, nil }

func TestResolveAndIgnoreFindingWriteStateAndAudit(t *testing.T) {
	// ResolveFinding / IgnoreFinding move an open finding to its terminal
	// state AND append a finding_state_change audit record carrying the
	// action / state / finding_id, so the governance trail never loses
	// who changed what. The whole app/findings.go file was untested.
	cases := []struct {
		name   string
		id     string
		apply  func(*Runtime, string) (model.Finding, error)
		want   model.FindingState
		action string
	}{
		{"resolve", "f-resolve", (*Runtime).ResolveFinding, model.FindingResolved, "resolve"},
		{"ignore", "f-ignore", (*Runtime).IgnoreFinding, model.FindingIgnored, "ignore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := memory.New()
			r := &Runtime{Store: st}
			if err := st.Findings().SaveFinding(model.Finding{
				ID:         tc.id,
				Kind:       model.FindingOutOfBandVaultWrite,
				State:      model.FindingOpen,
				Severity:   model.FindingSeverityInfo,
				Title:      "seed",
				Summary:    "fixture",
				Source:     "test",
				Target:     model.DocumentRef{Path: "0-x/y.md"},
				DetectedAt: time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
				UpdatedAt:  time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
			}); err != nil {
				t.Fatalf("SaveFinding: %v", err)
			}

			// Pad the id to also exercise the TrimSpace in updateFindingState.
			got, err := tc.apply(r, "  "+tc.id+"  ")
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got.State != tc.want {
				t.Fatalf("state = %q, want %q", got.State, tc.want)
			}

			audits, err := st.Audit().ListAudit(10)
			if err != nil {
				t.Fatalf("ListAudit: %v", err)
			}
			if len(audits) != 1 {
				t.Fatalf("audit records = %d, want exactly 1: %+v", len(audits), audits)
			}
			a := audits[0]
			if a.Kind != model.AuditFindingStateChange {
				t.Fatalf("audit kind = %q, want %q", a.Kind, model.AuditFindingStateChange)
			}
			if a.CorrelationID != tc.id || a.Target != "0-x/y.md" {
				t.Fatalf("audit correlation/target = %q/%q, want %q/0-x/y.md", a.CorrelationID, a.Target, tc.id)
			}
			if a.Metadata["action"] != tc.action ||
				a.Metadata["state"] != string(tc.want) ||
				a.Metadata["finding_id"] != tc.id {
				t.Fatalf("audit metadata = %+v, want action=%s state=%s finding_id=%s", a.Metadata, tc.action, tc.want, tc.id)
			}
		})
	}
}

func TestUpdateFindingStatePropagatesStoreError(t *testing.T) {
	// A finding that does not exist must surface the store error and must
	// NOT leave a stray audit record behind (audit is appended only after
	// a successful state change).
	st := memory.New()
	r := &Runtime{Store: st}
	if _, err := r.ResolveFinding("does-not-exist"); err == nil {
		t.Fatal("ResolveFinding on a missing id error = nil, want store error")
	}
	audits, err := st.Audit().ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(audits) != 0 {
		t.Fatalf("failed state change must not write an audit record, got %+v", audits)
	}
}

func TestFindingStateAuditIDSanitizesAndBounds(t *testing.T) {
	// The audit ID must be scan-/path-safe and bounded: spaces and
	// slashes in action / findingID collapse to dashes, and an over-long
	// findingID is truncated on a rune boundary so a Chinese finding ID
	// can't split a multibyte sequence.
	at := time.Date(2026, 5, 23, 9, 0, 0, 123, time.UTC)
	id := findingStateAuditID("re solve", "a/b\\c", at)
	if strings.ContainsAny(id, " /\\") {
		t.Fatalf("audit id %q still contains a space or slash", id)
	}
	if !strings.HasPrefix(id, "finding-state-re-solve-a-b-c-") {
		t.Fatalf("audit id = %q, want sanitized prefix finding-state-re-solve-a-b-c-", id)
	}

	// Rune-safe truncation: 100 Chinese runes -> 80 runes, still valid UTF-8.
	long := strings.Repeat("电", 100)
	truncated := truncateFindingIDRunes(long, 80)
	if n := utf8.RuneCountInString(truncated); n != 80 {
		t.Fatalf("truncated rune count = %d, want 80", n)
	}
	if !utf8.ValidString(truncated) {
		t.Fatalf("truncation produced invalid UTF-8: %q", truncated)
	}
	// Non-positive limit -> empty; input under the limit is unchanged.
	if truncateFindingIDRunes("x", 0) != "" {
		t.Fatal("limit 0 should yield an empty string")
	}
	if truncateFindingIDRunes("short", 80) != "short" {
		t.Fatal("input under the limit should be returned unchanged")
	}
}

func TestUpdateFindingStateReturnsFindingWhenAuditFails(t *testing.T) {
	// Write-then-audit consistency: when the state change commits but the
	// audit append fails, the operation must surface the committed change
	// (the updated finding) with a clear error -- not an empty finding
	// implying nothing happened. Aligns with the harness's best-effort
	// recordAudit instead of inverting the result.
	mem := memory.New()
	if err := mem.Findings().SaveFinding(model.Finding{
		ID:         "f1",
		Kind:       model.FindingOutOfBandVaultWrite,
		State:      model.FindingOpen,
		Severity:   model.FindingSeverityInfo,
		Title:      "seed",
		Summary:    "fixture",
		Source:     "test",
		Target:     model.DocumentRef{Path: "0-x/y.md"},
		DetectedAt: time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("SaveFinding: %v", err)
	}
	r := &Runtime{Store: auditFailingStateStore{StateStore: mem}}

	finding, err := r.ResolveFinding("f1")
	if err == nil {
		t.Fatal("ResolveFinding must surface the audit failure")
	}
	if !strings.Contains(err.Error(), "audit") {
		t.Fatalf("error should name the audit failure, got %v", err)
	}
	// The state change committed despite the audit failure: the returned
	// finding reflects it (not a zero value).
	if finding.State != model.FindingResolved {
		t.Fatalf("returned finding state = %q, want resolved (the change committed)", finding.State)
	}
	if finding.ID != "f1" {
		t.Fatalf("returned finding ID = %q, want f1 (not a zero value)", finding.ID)
	}
}
