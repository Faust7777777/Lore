package codexjsonl

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCursorEncodeDecodeRoundTripAndLegacyFingerprint(t *testing.T) {
	cursor := Cursor{
		Version:            1,
		Fingerprint:        "size=10|mtime=20",
		Offset:             128,
		ReplayOffset:       64,
		ReplayAnchorStart:  32,
		ReplayAnchorEnd:    64,
		ReplayAnchorSHA256: "anchor123",
		LastWindowStart:    time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
		LastEventAt:        time.Date(2026, 4, 22, 9, 15, 0, 0, time.UTC),
		FileSize:           128,
		FileModTimeNS:      20,
		FileHeadSHA256:     "abc123",
		AgentID:            "codex",
		SessionID:          "session-1",
	}

	encoded, err := EncodeCursor(cursor)
	if err != nil {
		t.Fatalf("EncodeCursor() error = %v", err)
	}
	decoded, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if decoded.Offset != cursor.Offset || decoded.ReplayOffset != cursor.ReplayOffset {
		t.Fatalf("decoded offsets = (%d, %d), want (%d, %d)", decoded.Offset, decoded.ReplayOffset, cursor.Offset, cursor.ReplayOffset)
	}
	if decoded.ReplayAnchorSHA256 != cursor.ReplayAnchorSHA256 {
		t.Fatalf("decoded ReplayAnchorSHA256 = %q, want %q", decoded.ReplayAnchorSHA256, cursor.ReplayAnchorSHA256)
	}
	if decoded.FileHeadSHA256 != cursor.FileHeadSHA256 {
		t.Fatalf("decoded FileHeadSHA256 = %q, want %q", decoded.FileHeadSHA256, cursor.FileHeadSHA256)
	}

	legacy, err := DecodeCursor("size=1|mtime=2")
	if err != nil {
		t.Fatalf("DecodeCursor(legacy) error = %v", err)
	}
	if legacy.Version != 0 || legacy.Fingerprint != "size=1|mtime=2" {
		t.Fatalf("legacy cursor = %+v, want legacy fingerprint payload", legacy)
	}
}

func TestReadSourceStateAndCursorMatchesSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.jsonl")
	if err := os.WriteFile(path, []byte("line-1\nline-2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, err := ReadSourceState(path)
	if err != nil {
		t.Fatalf("ReadSourceState() error = %v", err)
	}
	if source.Size == 0 || source.HeadSHA256 == "" {
		t.Fatalf("source = %+v, want non-empty size/head", source)
	}

	cursor := Cursor{
		Version:        1,
		Offset:         5,
		FileSize:       source.Size,
		FileModTimeNS:  source.ModTimeNS,
		FileHeadSHA256: source.HeadSHA256,
	}
	if !cursor.MatchesSource(source) {
		t.Fatal("MatchesSource() = false, want true")
	}

	cursor.FileSize = source.Size + 1
	if cursor.MatchesSource(source) {
		t.Fatal("MatchesSource() = true after truncation, want false")
	}

	cursor.FileSize = source.Size
	cursor.FileHeadSHA256 = "different"
	if cursor.MatchesSource(source) {
		t.Fatal("MatchesSource() = true after head hash mismatch, want false")
	}
}

func TestBuildReplayAnchorAndCursorMatchesReplayAnchor(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.jsonl")
	if err := os.WriteFile(path, []byte("line-1\nline-2\nline-3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, err := ReadSourceState(path)
	if err != nil {
		t.Fatalf("ReadSourceState() error = %v", err)
	}
	start, end, sha, err := BuildReplayAnchor(path, 14)
	if err != nil {
		t.Fatalf("BuildReplayAnchor() error = %v", err)
	}

	cursor := Cursor{
		ReplayOffset:       end,
		ReplayAnchorStart:  start,
		ReplayAnchorEnd:    end,
		ReplayAnchorSHA256: sha,
	}
	if !cursor.MatchesReplayAnchor(source) {
		t.Fatal("MatchesReplayAnchor() = false, want true")
	}

	if err := os.WriteFile(path, []byte("line-1\nline-X\nline-3\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(rewrite) error = %v", err)
	}
	rewrittenSource, err := ReadSourceState(path)
	if err != nil {
		t.Fatalf("ReadSourceState(rewrite) error = %v", err)
	}
	if cursor.MatchesReplayAnchor(rewrittenSource) {
		t.Fatal("MatchesReplayAnchor() = true after boundary rewrite, want false")
	}
}
