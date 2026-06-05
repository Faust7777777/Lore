package orchestrator

import (
	"errors"
	"sync"
	"testing"
	"time"

	"obsidian-harness/internal/config"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
	"obsidian-harness/internal/store/memory"
)

// TestApplyDraftSerializesConcurrentAppliesToSameTarget pins the apply-path
// serialization (review-v1 P0-2/P2-2). Several approved drafts target the same
// new file; applying them concurrently must yield exactly one `applied` (the
// first writer) and the rest `conflicted` -- never two drafts both applied
// against one file. Without h.applyMu the read-check → write window races and
// more than one apply can slip through. Memory-backed so there is no sqlite
// file lock under goroutine contention.
func TestApplyDraftSerializesConcurrentAppliesToSameTarget(t *testing.T) {
	cfg := config.Default(t.TempDir())
	mem := memory.New()
	h, err := New(cfg, mem)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := h.BootstrapManagedVault(time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BootstrapManagedVault: %v", err)
	}

	const n = 8
	const target = "03-notes/concurrent.md"
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		at := time.Date(2026, 6, 5, 10, 0, 0, i, time.UTC)
		res, err := h.ProposeMarkdownNote(model.MarkdownNoteProposal{
			TargetPath: target,
			Title:      "Concurrent",
			Content:    "# Concurrent\n\nbody",
			SourceKind: "development",
			Evidence:   "concurrency test evidence",
			Reason:     "concurrency test",
			Source:     "test",
			ObservedAt: at,
		}, at)
		if err != nil {
			t.Fatalf("ProposeMarkdownNote[%d]: %v", i, err)
		}
		if _, err := h.ApproveDraft(res.DraftID, at.Add(time.Minute)); err != nil {
			t.Fatalf("ApproveDraft[%d]: %v", i, err)
		}
		ids = append(ids, res.DraftID)
	}

	applyAt := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	results := make([]error, n)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			_, results[i] = h.ApplyDraft(id, applyAt)
		}(i, id)
	}
	wg.Wait()

	applied, conflicted := 0, 0
	for i, err := range results {
		switch {
		case err == nil:
			applied++
		case errors.Is(err, store.ErrConflict):
			conflicted++
		default:
			t.Fatalf("apply[%d] unexpected error: %v", i, err)
		}
	}
	if applied != 1 {
		t.Fatalf("want exactly 1 applied, got %d (conflicted=%d) -- apply path is not serialized", applied, conflicted)
	}
	if conflicted != n-1 {
		t.Fatalf("want %d conflicted, got %d", n-1, conflicted)
	}
}
