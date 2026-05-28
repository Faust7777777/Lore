package app

import (
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/persona"
)

// PersonaSummaryView is the dashboard projection of a workdir's
// persona pipeline state. It combines store-side candidate counts
// (Open / Drafted linked vs. orphan / Dismissed) with extract-log
// health (existence, total entries, per-stage breakdown, last
// entry timestamp) so both CLI (`lore persona summary`) and TUI can
// render the same headline numbers from one DTO.
type PersonaSummaryView struct {
	Workdir    string
	LogPath    string
	Candidates PersonaCandidateCounts
	ExtractLog PersonaExtractLogSummary
}

// PersonaCandidateCounts mirrors the four operator-visible buckets
// the dashboard surfaces. DraftedLinked + DraftedOrphan together
// equal the total Drafted count; the split makes "partial-orphan
// health" trivially visible at a glance.
type PersonaCandidateCounts struct {
	Open          int
	DraftedLinked int
	DraftedOrphan int
	Dismissed     int
}

// PersonaExtractLogSummary aggregates the persona-extract.log file
// for dashboard rendering. ByStage carries stage -> count for every
// stage seen in the log; malformed lines are bucketed under the key
// "(malformed)" so the operator notices schema drift. LastEntry is
// the most recent successfully parsed timestamp, or zero when no
// parsed entries exist.
type PersonaExtractLogSummary struct {
	Exists       bool
	TotalEntries int
	ByStage      map[string]int
	LastEntry    time.Time
}

// PersonaSummary returns the dashboard DTO. Read-only: it never
// mutates store or log file. Missing log file degrades to
// Exists=false with empty ByStage rather than an error so a fresh
// workdir that has never seen a failure still renders cleanly.
func (r *Runtime) PersonaSummary() (PersonaSummaryView, error) {
	if r == nil {
		return PersonaSummaryView{}, fmt.Errorf("app: runtime is not initialized")
	}
	view := PersonaSummaryView{
		Workdir:    r.WorkDirPath(),
		LogPath:    r.PersonaExtractLogPath(),
		ExtractLog: PersonaExtractLogSummary{ByStage: map[string]int{}},
	}

	openList, err := r.ListPersonaCandidates(persona.PersonaCandidateOpen, 0)
	if err != nil {
		return view, fmt.Errorf("persona summary: list open: %w", err)
	}
	draftedList, err := r.ListPersonaCandidates(persona.PersonaCandidateDrafted, 0)
	if err != nil {
		return view, fmt.Errorf("persona summary: list drafted: %w", err)
	}
	dismissedList, err := r.ListPersonaCandidates(persona.PersonaCandidateDismissed, 0)
	if err != nil {
		return view, fmt.Errorf("persona summary: list dismissed: %w", err)
	}
	view.Candidates.Open = len(openList)
	view.Candidates.Dismissed = len(dismissedList)
	for _, rec := range draftedList {
		if strings.TrimSpace(rec.DraftID) == "" {
			view.Candidates.DraftedOrphan++
		} else {
			view.Candidates.DraftedLinked++
		}
	}

	entries, exists, err := readPersonaLogFile(view.LogPath)
	if err != nil {
		return view, fmt.Errorf("persona summary: read log: %w", err)
	}
	view.ExtractLog.Exists = exists
	view.ExtractLog.TotalEntries = len(entries)
	var latest time.Time
	for _, e := range entries {
		if e.Parsed {
			view.ExtractLog.ByStage[e.Stage]++
			if e.Timestamp.After(latest) {
				latest = e.Timestamp
			}
		} else {
			view.ExtractLog.ByStage["(malformed)"]++
		}
	}
	view.ExtractLog.LastEntry = latest
	return view, nil
}
