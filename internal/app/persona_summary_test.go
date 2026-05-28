package app

import (
	"os"
	"testing"
	"time"

	"obsidian-harness/internal/persona"
)

func TestPersonaSummaryAggregatesEmptyWorkdirCleanly(t *testing.T) {
	// Fresh workdir, no candidates, no failures. The summary must
	// degrade to zero counts cleanly so a brand new operator sees a
	// clean dashboard instead of a stack trace. OpenRuntime always
	// creates the persona-extract.log file (so ExtractLog.Exists is
	// true), but the file is empty so TotalEntries stays zero.
	rt := openTestRuntime(t)
	if _, err := rt.Bootstrap(time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	got, err := rt.PersonaSummary()
	if err != nil {
		t.Fatalf("PersonaSummary: %v", err)
	}
	if got.LogPath == "" || got.Workdir == "" {
		t.Fatalf("LogPath/Workdir empty: %+v", got)
	}
	if got.Candidates.Open != 0 || got.Candidates.DraftedLinked != 0 ||
		got.Candidates.DraftedOrphan != 0 || got.Candidates.Dismissed != 0 {
		t.Fatalf("empty workdir should yield zero counts; got %+v", got.Candidates)
	}
	if got.ExtractLog.TotalEntries != 0 {
		t.Fatalf("ExtractLog.TotalEntries = %d, want 0 for empty log", got.ExtractLog.TotalEntries)
	}
	if !got.ExtractLog.LastEntry.IsZero() {
		t.Fatalf("ExtractLog.LastEntry should be zero for empty log; got %v", got.ExtractLog.LastEntry)
	}
}

func TestPersonaSummarySplitsDraftedLinkedAndOrphan(t *testing.T) {
	// Seed three drafted candidates: two linked via the normal flow,
	// one forced into the partial-orphan shape by manually setting
	// State=Drafted with empty DraftID. The split must show up as
	// DraftedLinked=2, DraftedOrphan=1.
	rt := openTestRuntime(t)
	linkedA := seedPersonaCandidate(t, rt, "major", "economics", "I study economics")
	linkedB := seedPersonaCandidate(t, rt, "city", "beijing", "I live in beijing")
	if _, _, err := rt.CreatePersonaDraftFromCandidate(linkedA, time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("promote A: %v", err)
	}
	if _, _, err := rt.CreatePersonaDraftFromCandidate(linkedB, time.Date(2026, 5, 28, 10, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("promote B: %v", err)
	}
	orphanID := seedPersonaCandidate(t, rt, "year", "2026", "I'm in 2026")
	// Force partial-orphan: claim Drafted but never link. Reuse the
	// store API directly because the public Runtime methods refuse to
	// leave a candidate in this shape on purpose; we want to simulate
	// the partial-failure scar the recover commands are designed for.
	if _, err := rt.Store.PersonaCandidates().ClaimCandidateForDraft(orphanID, time.Date(2026, 5, 28, 10, 2, 0, 0, time.UTC)); err != nil {
		t.Fatalf("force orphan claim: %v", err)
	}

	got, err := rt.PersonaSummary()
	if err != nil {
		t.Fatalf("PersonaSummary: %v", err)
	}
	if got.Candidates.Open != 0 {
		t.Fatalf("Open = %d, want 0 (all promoted)", got.Candidates.Open)
	}
	if got.Candidates.DraftedLinked != 2 {
		t.Fatalf("DraftedLinked = %d, want 2", got.Candidates.DraftedLinked)
	}
	if got.Candidates.DraftedOrphan != 1 {
		t.Fatalf("DraftedOrphan = %d, want 1", got.Candidates.DraftedOrphan)
	}
	if got.Candidates.Dismissed != 0 {
		t.Fatalf("Dismissed = %d, want 0", got.Candidates.Dismissed)
	}
}

func TestPersonaSummaryBucketsExtractLogByStage(t *testing.T) {
	// Log carries entries across all three legitimate stages plus a
	// malformed line. ByStage must include the three legitimate
	// stages and the machine-facing "malformed" bucket so JSON
	// consumers keep the documented stable key.
	rt := seedPersonaLogFile(t, []string{
		"2026-05-28T09:00:00Z\tstage=extract\tsession=s1\terror=\"e\"",
		"2026-05-28T09:01:00Z\tstage=extract\tsession=s2\terror=\"e\"",
		"2026-05-28T09:02:00Z\tstage=store\tsession=s3\terror=\"e\"",
		"2026-05-28T09:03:00Z\tstage=parse_warning\tsession=s4\terror=\"e\"",
		"garbage tail line",
	})

	got, err := rt.PersonaSummary()
	if err != nil {
		t.Fatalf("PersonaSummary: %v", err)
	}
	if !got.ExtractLog.Exists {
		t.Fatalf("ExtractLog.Exists = false; want true")
	}
	if got.ExtractLog.TotalEntries != 5 {
		t.Fatalf("TotalEntries = %d, want 5", got.ExtractLog.TotalEntries)
	}
	if got.ExtractLog.ByStage["extract"] != 2 {
		t.Fatalf("ByStage[extract] = %d, want 2", got.ExtractLog.ByStage["extract"])
	}
	if got.ExtractLog.ByStage["store"] != 1 {
		t.Fatalf("ByStage[store] = %d, want 1", got.ExtractLog.ByStage["store"])
	}
	if got.ExtractLog.ByStage["parse_warning"] != 1 {
		t.Fatalf("ByStage[parse_warning] = %d, want 1", got.ExtractLog.ByStage["parse_warning"])
	}
	if got.ExtractLog.ByStage["malformed"] != 1 {
		t.Fatalf("ByStage[malformed] = %d, want 1", got.ExtractLog.ByStage["malformed"])
	}
	wantLast, _ := time.Parse(time.RFC3339Nano, "2026-05-28T09:03:00Z")
	if !got.ExtractLog.LastEntry.Equal(wantLast) {
		t.Fatalf("LastEntry = %v, want %v (most recent parsed timestamp)", got.ExtractLog.LastEntry, wantLast)
	}
}

func TestPersonaSummaryReflectsFullStateSeededByFixtureHelpers(t *testing.T) {
	// End-to-end demonstration that the app-level seed helpers
	// (seedPersonaCandidate / seedDraftedPersonaCandidate /
	// seedPartialOrphanPersonaCandidate / seedRejectedLinkedPersonaCandidate /
	// seedDismissedPersonaCandidate) can materialize a complete
	// persona-review dashboard state in a single test body. Acts as
	// both regression coverage for PersonaSummary and a usage example
	// for future TUI tests that need a populated workdir.
	rt := openTestRuntime(t)
	seedPersonaCandidate(t, rt, "major", "economics", "I study economics")
	seedDraftedPersonaCandidate(t, rt, "city", "beijing", "I live in beijing")
	seedPartialOrphanPersonaCandidate(t, rt, "year", "2026", "I'm in 2026")
	seedRejectedLinkedPersonaCandidate(t, rt, "team", "growth", "I'm on the growth team")
	seedDismissedPersonaCandidate(t, rt, "hobby", "chess", "I play chess")

	// Seed one log entry too so the dashboard's ExtractLog section
	// shows non-zero TotalEntries -- matches what a real workdir
	// with at least one mining failure would look like.
	logPath := rt.PersonaExtractLogPath()
	if err := os.WriteFile(logPath, []byte("2026-05-28T09:00:00Z\tstage=extract\tsession=demo\terror=\"timeout\"\n"), 0o644); err != nil {
		t.Fatalf("seed log entry: %v", err)
	}

	got, err := rt.PersonaSummary()
	if err != nil {
		t.Fatalf("PersonaSummary: %v", err)
	}
	if got.Candidates.Open != 1 {
		t.Fatalf("Open = %d, want 1 (single un-promoted seed)", got.Candidates.Open)
	}
	// Drafted bucket aggregates: the pending-review linked, the
	// rejected-linked (still State=Drafted), and the partial-orphan.
	if got.Candidates.DraftedLinked != 2 {
		t.Fatalf("DraftedLinked = %d, want 2 (pending + rejected, both have DraftID)", got.Candidates.DraftedLinked)
	}
	if got.Candidates.DraftedOrphan != 1 {
		t.Fatalf("DraftedOrphan = %d, want 1 (partial scar)", got.Candidates.DraftedOrphan)
	}
	if got.Candidates.Dismissed != 1 {
		t.Fatalf("Dismissed = %d, want 1", got.Candidates.Dismissed)
	}
	if got.ExtractLog.TotalEntries != 1 || got.ExtractLog.ByStage["extract"] != 1 {
		t.Fatalf("ExtractLog should show one extract-stage failure; got %+v", got.ExtractLog)
	}
}

func TestPersonaSummaryDismissedCandidatesCounted(t *testing.T) {
	// A dismissed candidate must contribute to the Dismissed bucket
	// (and only that bucket). Guards against future state-renaming
	// regressions silently merging buckets.
	rt := openTestRuntime(t)
	id := seedPersonaCandidate(t, rt, "major", "economics", "I study economics")
	if _, err := rt.DismissPersonaCandidate(id, time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	got, err := rt.PersonaSummary()
	if err != nil {
		t.Fatalf("PersonaSummary: %v", err)
	}
	if got.Candidates.Dismissed != 1 {
		t.Fatalf("Dismissed = %d, want 1", got.Candidates.Dismissed)
	}
	if got.Candidates.Open != 0 || got.Candidates.DraftedLinked != 0 || got.Candidates.DraftedOrphan != 0 {
		t.Fatalf("dismissed-only workdir leaked into other buckets: %+v", got.Candidates)
	}
	// Sanity: confirm the underlying store path agrees so a future
	// PersonaSummary refactor cannot diverge from ListPersonaCandidates
	// silently.
	dismissed, err := rt.ListPersonaCandidates(persona.PersonaCandidateDismissed, 0)
	if err != nil {
		t.Fatalf("ListPersonaCandidates: %v", err)
	}
	if len(dismissed) != 1 {
		t.Fatalf("ListPersonaCandidates(dismissed) returned %d; PersonaSummary should match", len(dismissed))
	}
}
