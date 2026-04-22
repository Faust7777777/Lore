package app

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/model"
)

type ImportCodexJSONLParams struct {
	InputPath   string
	AgentID     string
	SessionID   string
	Window      time.Duration
	SkipRollup  bool
}

type ImportCodexJSONLResult struct {
	InputPath   string
	AgentID     string
	SessionID   string
	Checkpoints []model.CheckpointDoc
	Reports     []model.DailyReport
}

func (r *Runtime) ImportCodexJSONL(params ImportCodexJSONLParams, now time.Time) (ImportCodexJSONLResult, error) {
	r.Harness.UpdateDependencies(true, true)
	if _, err := r.Bootstrap(now); err != nil {
		return ImportCodexJSONLResult{}, err
	}

	inputPath := filepath.Clean(strings.TrimSpace(params.InputPath))
	transcript, err := codexjsonl.LoadFile(inputPath)
	if err != nil {
		return ImportCodexJSONLResult{}, err
	}

	if agentID := strings.TrimSpace(params.AgentID); agentID != "" {
		transcript.AgentID = strings.ToLower(agentID)
	}
	if sessionID := strings.TrimSpace(params.SessionID); sessionID != "" {
		transcript.SessionID = sessionID
	}

	windowSize := params.Window
	if windowSize <= 0 {
		windowSize = r.Config.ProcessSink.CheckpointEvery
	}
	windows := codexjsonl.BuildWindows(transcript, windowSize)

	result := ImportCodexJSONLResult{
		InputPath: filepath.Clean(inputPath),
		AgentID:   transcript.AgentID,
		SessionID: transcript.SessionID,
	}

	reportDays := make(map[string]time.Time)
	for _, payload := range windows {
		checkpoint, err := r.Harness.IngestSessionWindow(
			payload.Window,
			payload.Title,
			payload.Content,
			payload.RawTranscript,
			payload.Window.WindowEnd,
		)
		if err != nil {
			return ImportCodexJSONLResult{}, err
		}
		result.Checkpoints = append(result.Checkpoints, checkpoint)
		day := model.NormalizeDay(payload.Window.WindowStart)
		reportDays[day.Format("2006-01-02")] = day
	}

	if params.SkipRollup {
		return result, nil
	}

	keys := make([]string, 0, len(reportDays))
	for key := range reportDays {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		report, err := r.Harness.RollupDaily(transcript.AgentID, reportDays[key], now)
		if err != nil {
			return ImportCodexJSONLResult{}, err
		}
		result.Reports = append(result.Reports, report)
	}
	return result, nil
}

