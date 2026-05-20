package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"obsidian-harness/internal/persona"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/jsonstore"
	"obsidian-harness/internal/store/memory"
	"obsidian-harness/internal/store/sqlitestore"
)

// All three backends share the PersonaCandidateStore contract, so a
// single table-driven test exercises identical behavior across them.
// New backends added later only need a constructor here.
type backendFactory struct {
	name        string
	open        func(t *testing.T) store.PersonaCandidateStore
	persistable bool // true when the backend survives a re-open (json, sqlite)
	reopen      func(t *testing.T) store.PersonaCandidateStore
}

func memoryBackend() backendFactory {
	return backendFactory{
		name: "memory",
		open: func(t *testing.T) store.PersonaCandidateStore {
			t.Helper()
			return memory.New().PersonaCandidates()
		},
	}
}

func jsonBackend(t *testing.T) backendFactory {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := jsonstore.New(path, ".tmp")
	if err != nil {
		t.Fatalf("jsonstore.New() error = %v", err)
	}
	return backendFactory{
		name:        "jsonstore",
		open:        func(_ *testing.T) store.PersonaCandidateStore { return first.PersonaCandidates() },
		persistable: true,
		reopen: func(t *testing.T) store.PersonaCandidateStore {
			t.Helper()
			reloaded, err := jsonstore.New(path, ".tmp")
			if err != nil {
				t.Fatalf("jsonstore.New(reload) error = %v", err)
			}
			return reloaded.PersonaCandidates()
		},
	}
}

func sqliteBackend(t *testing.T) backendFactory {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state", "store.db")
	first, err := sqlitestore.New(path)
	if err != nil {
		t.Fatalf("sqlitestore.New() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	return backendFactory{
		name:        "sqlitestore",
		open:        func(_ *testing.T) store.PersonaCandidateStore { return first.PersonaCandidates() },
		persistable: true,
		reopen: func(t *testing.T) store.PersonaCandidateStore {
			t.Helper()
			reloaded, err := sqlitestore.New(path)
			if err != nil {
				t.Fatalf("sqlitestore.New(reload) error = %v", err)
			}
			t.Cleanup(func() { _ = reloaded.Close() })
			return reloaded.PersonaCandidates()
		},
	}
}

func backends(t *testing.T) []backendFactory {
	t.Helper()
	return []backendFactory{
		memoryBackend(),
		jsonBackend(t),
		sqliteBackend(t),
	}
}

func sampleCandidate(id, field, value, evidence string) persona.PersonaCandidateRecord {
	c := persona.PersonaCandidate{
		Field:           field,
		ProposedValue:   value,
		EvidenceQuote:   evidence,
		Confidence:      persona.ConfidenceHigh,
		SourceKind:      persona.SourceConsole,
		SourceSessionID: "lore-session",
		ObservedAt:      time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	return persona.PersonaCandidateRecord{
		ID:        id,
		State:     persona.PersonaCandidateOpen,
		DedupKey:  persona.DedupKey(c),
		Candidate: c,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestPersonaCandidateStoreRoundtrip(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			rec := sampleCandidate("cand-1", "major", "economics", "I study economics")

			stored, isNew, err := s.UpsertCandidate(rec)
			if err != nil {
				t.Fatalf("UpsertCandidate() error = %v", err)
			}
			if !isNew {
				t.Fatalf("expected isNew=true on first insert")
			}
			if stored.ID != rec.ID || stored.Candidate.Field != "major" {
				t.Fatalf("stored = %+v", stored)
			}

			got, err := s.GetCandidate("cand-1")
			if err != nil {
				t.Fatalf("GetCandidate() error = %v", err)
			}
			if got.Candidate.ProposedValue != "economics" {
				t.Fatalf("roundtrip ProposedValue = %q", got.Candidate.ProposedValue)
			}
			if got.DedupKey != rec.DedupKey {
				t.Fatalf("roundtrip DedupKey = %q, want %q", got.DedupKey, rec.DedupKey)
			}
			if !got.Candidate.ObservedAt.Equal(rec.Candidate.ObservedAt) {
				t.Fatalf("roundtrip ObservedAt = %v, want %v", got.Candidate.ObservedAt, rec.Candidate.ObservedAt)
			}
		})
	}
}

func TestPersonaCandidateStoreDedupKeyRejectsDuplicateInsert(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			first := sampleCandidate("cand-1", "major", "economics", "I study economics")
			if _, _, err := s.UpsertCandidate(first); err != nil {
				t.Fatalf("UpsertCandidate(first) error = %v", err)
			}

			// Same DedupKey, different ID. Must return the original
			// row with isNew=false.
			dup := first
			dup.ID = "cand-2"
			dup.CreatedAt = first.CreatedAt.Add(time.Hour)
			dup.UpdatedAt = dup.CreatedAt
			stored, isNew, err := s.UpsertCandidate(dup)
			if err != nil {
				t.Fatalf("UpsertCandidate(dup) error = %v", err)
			}
			if isNew {
				t.Fatalf("duplicate insert reported isNew=true: stored = %+v", stored)
			}
			if stored.ID != "cand-1" {
				t.Fatalf("duplicate insert returned ID = %q, want cand-1 (the original)", stored.ID)
			}

			// And the original record must still be retrievable; the
			// duplicate ID must not exist.
			if _, err := s.GetCandidate("cand-1"); err != nil {
				t.Fatalf("original GetCandidate() error = %v", err)
			}
			if _, err := s.GetCandidate("cand-2"); err == nil {
				t.Fatalf("duplicate ID cand-2 was inserted; should not exist")
			}
		})
	}
}

func TestPersonaCandidateStoreListByStateTransitions(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			open1 := sampleCandidate("o1", "major", "economics", "I study economics")
			open2 := sampleCandidate("o2", "city", "Dalian", "I live in Dalian")
			open2.UpdatedAt = open1.UpdatedAt.Add(time.Minute) // newer
			for _, rec := range []persona.PersonaCandidateRecord{open1, open2} {
				if _, _, err := s.UpsertCandidate(rec); err != nil {
					t.Fatalf("UpsertCandidate(%s) error = %v", rec.ID, err)
				}
			}

			openList, err := s.ListCandidatesByState(persona.PersonaCandidateOpen, 0)
			if err != nil {
				t.Fatalf("ListCandidatesByState(open) error = %v", err)
			}
			if len(openList) != 2 {
				t.Fatalf("open list length = %d, want 2", len(openList))
			}
			// Newest first.
			if openList[0].ID != "o2" || openList[1].ID != "o1" {
				t.Fatalf("open list order = %s,%s want o2,o1 (newest first)", openList[0].ID, openList[1].ID)
			}

			// Transition o1 -> drafted.
			later := open1.UpdatedAt.Add(time.Hour)
			updated, err := s.UpdateCandidateState("o1", persona.PersonaCandidateDrafted, later)
			if err != nil {
				t.Fatalf("UpdateCandidateState(o1) error = %v", err)
			}
			if updated.State != persona.PersonaCandidateDrafted || !updated.UpdatedAt.Equal(later) {
				t.Fatalf("transitioned record = %+v", updated)
			}

			openList, _ = s.ListCandidatesByState(persona.PersonaCandidateOpen, 0)
			draftedList, _ := s.ListCandidatesByState(persona.PersonaCandidateDrafted, 0)
			if len(openList) != 1 || openList[0].ID != "o2" {
				t.Fatalf("post-transition open list = %+v", openList)
			}
			if len(draftedList) != 1 || draftedList[0].ID != "o1" {
				t.Fatalf("post-transition drafted list = %+v", draftedList)
			}

			// Dismissed list must remain empty until a transition
			// puts something there.
			dismissedList, _ := s.ListCandidatesByState(persona.PersonaCandidateDismissed, 0)
			if len(dismissedList) != 0 {
				t.Fatalf("dismissed list = %+v, want empty", dismissedList)
			}

			if _, err := s.UpdateCandidateState("o2", persona.PersonaCandidateDismissed, later.Add(time.Minute)); err != nil {
				t.Fatalf("UpdateCandidateState(o2) error = %v", err)
			}
			dismissedList, _ = s.ListCandidatesByState(persona.PersonaCandidateDismissed, 0)
			if len(dismissedList) != 1 {
				t.Fatalf("dismissed list after transition = %+v, want one entry", dismissedList)
			}
		})
	}
}

func TestPersonaCandidateStorePreservesChineseEvidence(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			rec := sampleCandidate("cn-1", "major", "经济管理", "我在读经济管理,大三在校生")
			if _, _, err := s.UpsertCandidate(rec); err != nil {
				t.Fatalf("UpsertCandidate() error = %v", err)
			}
			got, err := s.GetCandidate("cn-1")
			if err != nil {
				t.Fatalf("GetCandidate() error = %v", err)
			}
			if got.Candidate.ProposedValue != "经济管理" {
				t.Fatalf("ProposedValue = %q (UTF-8 fidelity lost)", got.Candidate.ProposedValue)
			}
			if got.Candidate.EvidenceQuote != "我在读经济管理,大三在校生" {
				t.Fatalf("EvidenceQuote = %q (UTF-8 fidelity lost)", got.Candidate.EvidenceQuote)
			}
			if !strings.Contains(got.DedupKey, "经济管理") {
				t.Fatalf("DedupKey lost Chinese content: %q", got.DedupKey)
			}
		})
	}
}

func TestPersonaCandidateStoreSurvivesReopen(t *testing.T) {
	for _, bf := range backends(t) {
		if !bf.persistable {
			continue
		}
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			rec := sampleCandidate("durable", "city", "Dalian", "我在大连")
			if _, _, err := s.UpsertCandidate(rec); err != nil {
				t.Fatalf("UpsertCandidate() error = %v", err)
			}
			if _, err := s.UpdateCandidateState("durable", persona.PersonaCandidateDrafted, rec.UpdatedAt.Add(time.Hour)); err != nil {
				t.Fatalf("UpdateCandidateState() error = %v", err)
			}

			reopened := bf.reopen(t)
			got, err := reopened.GetCandidate("durable")
			if err != nil {
				t.Fatalf("post-reopen GetCandidate() error = %v", err)
			}
			if got.State != persona.PersonaCandidateDrafted {
				t.Fatalf("post-reopen state = %q, want drafted", got.State)
			}
			if got.Candidate.ProposedValue != "Dalian" {
				t.Fatalf("post-reopen ProposedValue = %q", got.Candidate.ProposedValue)
			}
		})
	}
}

func TestPersonaCandidateStoreLegacyStoresHaveNoCandidates(t *testing.T) {
	// Legacy stores that predate this slice must load cleanly and
	// return empty lists / not-found instead of crashing. Build a
	// jsonstore with no persona_candidates key in the on-disk JSON
	// and a sqlitestore with the table created but empty.

	t.Run("jsonstore-no-key", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")
		// Write a legacy JSON state with everything EXCEPT
		// persona_candidates. This simulates the disk file an older
		// build of Lore would have produced.
		legacy := `{"drafts":{},"checkpoints":{},"reports":{},"audit":[],"findings":{},"usage":[],"cursors":{}}`
		if err := writeFile(t, path, legacy); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		s, err := jsonstore.New(path, ".tmp")
		if err != nil {
			t.Fatalf("jsonstore.New() error = %v", err)
		}
		ps := s.PersonaCandidates()
		list, err := ps.ListCandidatesByState(persona.PersonaCandidateOpen, 0)
		if err != nil {
			t.Fatalf("ListCandidatesByState() error = %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("legacy store yielded candidates: %+v", list)
		}
		if _, err := ps.GetCandidate("nonexistent"); err == nil {
			t.Fatalf("GetCandidate(nonexistent) error = nil, want ErrNotFound")
		}
		// A fresh upsert must still work after legacy load.
		rec := sampleCandidate("post-legacy", "major", "history", "I study history")
		if _, isNew, err := ps.UpsertCandidate(rec); err != nil || !isNew {
			t.Fatalf("post-legacy UpsertCandidate isNew=%v err=%v", isNew, err)
		}
	})

	t.Run("sqlitestore-fresh", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state", "store.db")
		s, err := sqlitestore.New(path)
		if err != nil {
			t.Fatalf("sqlitestore.New() error = %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })
		ps := s.PersonaCandidates()
		list, err := ps.ListCandidatesByState(persona.PersonaCandidateOpen, 0)
		if err != nil {
			t.Fatalf("ListCandidatesByState() error = %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("fresh sqlite yielded candidates: %+v", list)
		}
	})
}

func TestPersonaCandidateStoreNormalizesEmptyStateOnInsert(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			rec := sampleCandidate("ns-1", "major", "history", "I study history")
			// Caller forgot to set State (zero value). The store must
			// promote it to Open so the candidate is visible in the
			// review queue.
			rec.State = ""

			stored, isNew, err := s.UpsertCandidate(rec)
			if err != nil {
				t.Fatalf("UpsertCandidate() error = %v", err)
			}
			if !isNew {
				t.Fatalf("expected isNew=true for fresh empty-state record")
			}
			if stored.State != persona.PersonaCandidateOpen {
				t.Fatalf("stored state after upsert = %q, want open", stored.State)
			}

			got, err := s.GetCandidate("ns-1")
			if err != nil {
				t.Fatalf("GetCandidate() error = %v", err)
			}
			if got.State != persona.PersonaCandidateOpen {
				t.Fatalf("reloaded state = %q, want open", got.State)
			}

			openList, err := s.ListCandidatesByState(persona.PersonaCandidateOpen, 0)
			if err != nil {
				t.Fatalf("ListCandidatesByState(open) error = %v", err)
			}
			found := false
			for _, r := range openList {
				if r.ID == "ns-1" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("empty-state record missing from Open list: %+v", openList)
			}
		})
	}
}

func TestPersonaCandidateStoreRejectsIDCollisionWithDifferentDedupKey(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			first := sampleCandidate("cand-1", "major", "history", "I study history")
			if _, isNew, err := s.UpsertCandidate(first); err != nil || !isNew {
				t.Fatalf("UpsertCandidate(first) isNew=%v err=%v", isNew, err)
			}

			// Same ID, different dedup identity. The contract is:
			// return ErrConflict. Silent overwrite would corrupt the
			// dedup index in memory/json and hit a PK constraint in
			// sqlite; both are wrong.
			collide := sampleCandidate("cand-1", "city", "Dalian", "I live in Dalian")
			stored, isNew, err := s.UpsertCandidate(collide)
			if !errors.Is(err, store.ErrConflict) {
				t.Fatalf("UpsertCandidate(id-collision) err = %v, want ErrConflict; stored=%+v isNew=%v", err, stored, isNew)
			}

			// The original record must still be intact and findable
			// under the original dedup identity.
			original, err := s.GetCandidate("cand-1")
			if err != nil {
				t.Fatalf("GetCandidate(cand-1) after rejected collision: %v", err)
			}
			if original.DedupKey != first.DedupKey {
				t.Fatalf("original record DedupKey corrupted: got %q want %q", original.DedupKey, first.DedupKey)
			}
			if original.Candidate.Field != "major" {
				t.Fatalf("original record overwritten: %+v", original.Candidate)
			}

			// And the colliding dedup key must NOT have been
			// registered as a side effect.
			collideList, err := s.ListCandidatesByState(persona.PersonaCandidateOpen, 0)
			if err != nil {
				t.Fatalf("ListCandidatesByState() error = %v", err)
			}
			for _, r := range collideList {
				if r.DedupKey == collide.DedupKey {
					t.Fatalf("rejected collision still left dedup key in store: %+v", r)
				}
			}
		})
	}
}

func TestPersonaCandidateStoreIdempotentRetryOnSameIDAndKey(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			first := sampleCandidate("retry-1", "major", "history", "I study history")
			if _, isNew, err := s.UpsertCandidate(first); err != nil || !isNew {
				t.Fatalf("UpsertCandidate(first) isNew=%v err=%v", isNew, err)
			}

			// Same ID, same DedupKey -- retry must be idempotent and
			// return the existing record with isNew=false (no error,
			// no overwrite).
			retry := first
			retry.CreatedAt = first.CreatedAt.Add(time.Hour)
			retry.UpdatedAt = retry.CreatedAt
			stored, isNew, err := s.UpsertCandidate(retry)
			if err != nil {
				t.Fatalf("UpsertCandidate(idempotent retry) err = %v", err)
			}
			if isNew {
				t.Fatalf("idempotent retry reported isNew=true")
			}
			if !stored.CreatedAt.Equal(first.CreatedAt) {
				t.Fatalf("idempotent retry replaced CreatedAt: got %v want %v", stored.CreatedAt, first.CreatedAt)
			}
		})
	}
}

func TestPersonaCandidateStoreInvalidInputs(t *testing.T) {
	for _, bf := range backends(t) {
		t.Run(bf.name, func(t *testing.T) {
			s := bf.open(t)
			if _, _, err := s.UpsertCandidate(persona.PersonaCandidateRecord{}); err == nil {
				t.Fatalf("UpsertCandidate(empty) error = nil, want ErrInvalidKey")
			}
			if _, err := s.GetCandidate(""); err == nil {
				t.Fatalf("GetCandidate(empty) error = nil, want ErrInvalidKey")
			}
			if _, err := s.UpdateCandidateState("", persona.PersonaCandidateDrafted, time.Now()); err == nil {
				t.Fatalf("UpdateCandidateState(empty) error = nil, want ErrInvalidKey")
			}
			if _, err := s.UpdateCandidateState("nonexistent", persona.PersonaCandidateDrafted, time.Now()); err == nil {
				t.Fatalf("UpdateCandidateState(nonexistent) error = nil, want ErrNotFound")
			}
		})
	}
}

// writeFile is a tiny helper that writes a legacy fixture to disk.
// Returns the os.WriteFile error so callers can fail the test with a
// clear path/content context.
func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}
