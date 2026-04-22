package model

import (
	"testing"
	"time"
)

func TestSessionWindowKeyStable(t *testing.T) {
	window := SessionWindow{
		AgentID:     "codex",
		SessionID:   "abc",
		WindowStart: time.Date(2026, 4, 22, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		WindowEnd:   time.Date(2026, 4, 22, 10, 30, 0, 0, time.FixedZone("CST", 8*3600)),
	}

	got := window.Key()
	want := "codex|abc|2026-04-22T02:00:00Z|2026-04-22T02:30:00Z"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
