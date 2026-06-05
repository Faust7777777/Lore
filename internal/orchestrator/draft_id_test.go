package orchestrator

import (
	"strings"
	"testing"
	"time"
)

func TestNewDraftIDIsUniquePerCall(t *testing.T) {
	// Two drafts minted in the same nanosecond must not collide -- the random
	// suffix breaks the tie (review-v1 P1-1).
	at := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	a, b := newDraftID(at), newDraftID(at)
	if a == b {
		t.Fatalf("two draft IDs minted at the same instant collided: %q", a)
	}
	for _, id := range []string{a, b} {
		// The "draft-<nano>" prefix is preserved so consumers that match on
		// it (and lexical ordering by timestamp) keep working.
		if !strings.HasPrefix(id, "draft-") {
			t.Fatalf("draft ID %q lost the draft- prefix", id)
		}
	}
}
