package store_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/jsonstore"
	"obsidian-harness/internal/store/memory"
	"obsidian-harness/internal/store/sqlitestore"
)

// findingBackendFactory mirrors usageBackendFactory but yields a
// FindingStore handle so the state-transition contract can be
// exercised in one table across memory / jsonstore / sqlitestore.
type findingBackendFactory struct {
	name string
	open func(t *testing.T) store.FindingStore
}

func findingBackends(t *testing.T) []findingBackendFactory {
	t.Helper()
	return []findingBackendFactory{
		{
			name: "memory",
			open: func(_ *testing.T) store.FindingStore {
				return memory.New().Findings()
			},
		},
		{
			name: "jsonstore",
			open: func(t *testing.T) store.FindingStore {
				t.Helper()
				path := filepath.Join(t.TempDir(), "state.json")
				s, err := jsonstore.New(path, ".tmp")
				if err != nil {
					t.Fatalf("jsonstore.New: %v", err)
				}
				return s.Findings()
			},
		},
		{
			name: "sqlitestore",
			open: func(t *testing.T) store.FindingStore {
				t.Helper()
				path := filepath.Join(t.TempDir(), "state", "store.db")
				s, err := sqlitestore.New(path)
				if err != nil {
					t.Fatalf("sqlitestore.New: %v", err)
				}
				t.Cleanup(func() { _ = s.Close() })
				return s.Findings()
			},
		},
	}
}

// seedFinding inserts a Finding in the given state and returns its
// id. Used to set up both the happy-path "open -> ..." cases and
// the illegal-source-state assertions.
func seedFinding(t *testing.T, fs store.FindingStore, id string, state model.FindingState) {
	t.Helper()
	if err := fs.SaveFinding(model.Finding{
		ID:         id,
		Kind:       model.FindingOutOfBandVaultWrite,
		State:      state,
		Severity:   model.FindingSeverityInfo,
		Title:      "seed " + id,
		Summary:    "transition test fixture",
		Source:     "test",
		DetectedAt: time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveFinding seed (%s): %v", state, err)
	}
}

func TestUpdateFindingStateAllowsOpenToResolved(t *testing.T) {
	// The first legal transition. Three backends must all agree.
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			seedFinding(t, fs, "f1", model.FindingOpen)
			now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
			got, err := fs.UpdateFindingState("f1", model.FindingResolved, now)
			if err != nil {
				t.Fatalf("UpdateFindingState: %v", err)
			}
			if got.State != model.FindingResolved {
				t.Fatalf("state = %q, want resolved", got.State)
			}
			if !got.UpdatedAt.Equal(now) {
				t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
			}
		})
	}
}

func TestUpdateFindingStateAllowsOpenToIgnored(t *testing.T) {
	// Second legal transition.
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			seedFinding(t, fs, "f2", model.FindingOpen)
			got, err := fs.UpdateFindingState("f2", model.FindingIgnored, time.Now().UTC())
			if err != nil {
				t.Fatalf("UpdateFindingState: %v", err)
			}
			if got.State != model.FindingIgnored {
				t.Fatalf("state = %q, want ignored", got.State)
			}
		})
	}
}

func TestUpdateFindingStateRejectsResolvedToOpen(t *testing.T) {
	// Resolved is terminal -- the architect's audit explicitly
	// called this out as the silent-overwrite path that this
	// fix closes. All three backends must surface a
	// model.ErrFindingTerminalState (or a wrap of it).
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			seedFinding(t, fs, "f3", model.FindingResolved)
			_, err := fs.UpdateFindingState("f3", model.FindingOpen, time.Now().UTC())
			if err == nil {
				t.Fatalf("UpdateFindingState should reject resolved -> open")
			}
			if !errors.Is(err, model.ErrFindingTerminalState) {
				t.Fatalf("err = %v, want wrap of ErrFindingTerminalState", err)
			}
			// And the stored state must NOT have changed.
			got, getErr := fs.GetFinding("f3")
			if getErr != nil {
				t.Fatalf("post-reject GetFinding: %v", getErr)
			}
			if got.State != model.FindingResolved {
				t.Fatalf("state = %q, want resolved (mutation must not happen on rejection)", got.State)
			}
		})
	}
}

func TestUpdateFindingStateRejectsResolvedToIgnored(t *testing.T) {
	// Swapping between terminal states would silently break the
	// audit trail. Also rejected.
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			seedFinding(t, fs, "f4", model.FindingResolved)
			_, err := fs.UpdateFindingState("f4", model.FindingIgnored, time.Now().UTC())
			if err == nil {
				t.Fatalf("UpdateFindingState should reject resolved -> ignored")
			}
			if !errors.Is(err, model.ErrFindingTerminalState) {
				t.Fatalf("err = %v, want wrap of ErrFindingTerminalState", err)
			}
		})
	}
}

func TestUpdateFindingStateRejectsIgnoredToOpen(t *testing.T) {
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			seedFinding(t, fs, "f5", model.FindingIgnored)
			_, err := fs.UpdateFindingState("f5", model.FindingOpen, time.Now().UTC())
			if err == nil {
				t.Fatalf("UpdateFindingState should reject ignored -> open")
			}
			if !errors.Is(err, model.ErrFindingTerminalState) {
				t.Fatalf("err = %v, want wrap of ErrFindingTerminalState", err)
			}
		})
	}
}

func TestUpdateFindingStateRejectsIgnoredToResolved(t *testing.T) {
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			seedFinding(t, fs, "f6", model.FindingIgnored)
			_, err := fs.UpdateFindingState("f6", model.FindingResolved, time.Now().UTC())
			if err == nil {
				t.Fatalf("UpdateFindingState should reject ignored -> resolved")
			}
			if !errors.Is(err, model.ErrFindingTerminalState) {
				t.Fatalf("err = %v, want wrap of ErrFindingTerminalState", err)
			}
		})
	}
}

func TestUpdateFindingStateRejectsUnknownTargetState(t *testing.T) {
	// open is a legal source but "garbage" is not a legal target.
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			seedFinding(t, fs, "f7", model.FindingOpen)
			_, err := fs.UpdateFindingState("f7", model.FindingState("garbage"), time.Now().UTC())
			if err == nil {
				t.Fatalf("UpdateFindingState should reject open -> garbage")
			}
			if !errors.Is(err, model.ErrFindingIllegalTransition) {
				t.Fatalf("err = %v, want wrap of ErrFindingIllegalTransition", err)
			}
		})
	}
}

func TestUpdateFindingStateReturnsNotFoundForMissing(t *testing.T) {
	for _, b := range findingBackends(t) {
		b := b
		t.Run(b.name, func(t *testing.T) {
			fs := b.open(t)
			_, err := fs.UpdateFindingState("missing", model.FindingResolved, time.Now().UTC())
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("err = %v, want store.ErrNotFound", err)
			}
		})
	}
}

func TestValidateFindingTransitionTable(t *testing.T) {
	// Pure model-layer assertion in case a future caller bypasses
	// the store helpers and uses ValidateFindingTransition
	// directly. The matrix here is the canonical state machine.
	cases := []struct {
		name    string
		from    model.FindingState
		to      model.FindingState
		wantErr error
	}{
		{"open->resolved", model.FindingOpen, model.FindingResolved, nil},
		{"open->ignored", model.FindingOpen, model.FindingIgnored, nil},
		{"open->open", model.FindingOpen, model.FindingOpen, model.ErrFindingIllegalTransition},
		{"open->garbage", model.FindingOpen, "garbage", model.ErrFindingIllegalTransition},
		{"resolved->open", model.FindingResolved, model.FindingOpen, model.ErrFindingTerminalState},
		{"resolved->ignored", model.FindingResolved, model.FindingIgnored, model.ErrFindingTerminalState},
		{"resolved->resolved", model.FindingResolved, model.FindingResolved, model.ErrFindingTerminalState},
		{"ignored->open", model.FindingIgnored, model.FindingOpen, model.ErrFindingTerminalState},
		{"ignored->resolved", model.FindingIgnored, model.FindingResolved, model.ErrFindingTerminalState},
		{"empty->resolved", "", model.FindingResolved, nil}, // expected to error on empty source
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			err := model.ValidateFindingTransition(c.from, c.to)
			if c.wantErr == nil && c.name == "empty->resolved" {
				// Empty source state is its own error class --
				// asserting separately because there is no
				// sentinel for "empty" today.
				if err == nil {
					t.Fatalf("empty source should error")
				}
				return
			}
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil for legal %s", err, c.name)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want wrap of %v for %s", err, c.wantErr, c.name)
			}
		})
	}
}
