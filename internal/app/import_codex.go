package app

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/model"
)

type ImportCodexJSONLParams struct {
	InputPath  string
	AgentID    string
	SessionID  string
	Window     time.Duration
	SkipRollup bool
}

type ImportCodexJSONLResult struct {
	InputPath   string
	AgentID     string
	SessionID   string
	Checkpoints []model.CheckpointDoc
	Reports     []model.DailyReport
}

func (r *Runtime) ImportCodexJSONL(params ImportCodexJSONLParams, now time.Time) (ImportCodexJSONLResult, error) {
	return r.ImportCodexJSONLContext(context.Background(), params, now)
}

// ImportCodexJSONLContext is ImportCodexJSONL with a caller-supplied
// context so the CLI can cancel a long import (e.g. on Ctrl-C) mid-flight;
// the context is threaded to each per-window model summarization.
func (r *Runtime) ImportCodexJSONLContext(ctx context.Context, params ImportCodexJSONLParams, now time.Time) (ImportCodexJSONLResult, error) {
	r.Harness.UpdateDependencies(true, true)
	if _, err := r.Bootstrap(now); err != nil {
		return ImportCodexJSONLResult{}, err
	}

	inputPath := filepath.Clean(strings.TrimSpace(params.InputPath))
	transcript, err := codexjsonl.LoadFile(inputPath)
	if err != nil {
		return ImportCodexJSONLResult{}, err
	}
	applyCodexJSONLIdentity(&transcript, params, codexjsonl.Cursor{}, false)
	windows := codexjsonl.BuildWindows(transcript, resolveCodexWindowSize(r.Config.ProcessSink.CheckpointEvery, params.Window))
	return r.importCodexWindowsContext(ctx, inputPath, transcript, windows, params.SkipRollup, now)
}

func resolveCodexWindowSize(defaultWindow time.Duration, requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return defaultWindow
}

func applyCodexJSONLIdentity(transcript *codexjsonl.Transcript, params ImportCodexJSONLParams, cursor codexjsonl.Cursor, allowCursor bool) {
	if allowCursor {
		if strings.TrimSpace(cursor.AgentID) != "" {
			transcript.AgentID = strings.TrimSpace(cursor.AgentID)
		}
		if strings.TrimSpace(cursor.SessionID) != "" {
			transcript.SessionID = strings.TrimSpace(cursor.SessionID)
		}
	}
	if agentID := strings.TrimSpace(params.AgentID); agentID != "" {
		transcript.AgentID = strings.ToLower(agentID)
	}
	if sessionID := strings.TrimSpace(params.SessionID); sessionID != "" {
		transcript.SessionID = sessionID
	}
}

func (r *Runtime) importCodexWindows(inputPath string, transcript codexjsonl.Transcript, windows []codexjsonl.WindowSummary, skipRollup bool, now time.Time) (ImportCodexJSONLResult, error) {
	return r.importCodexWindowsContext(context.Background(), inputPath, transcript, windows, skipRollup, now)
}

func (r *Runtime) importCodexWindowsContext(ctx context.Context, inputPath string, transcript codexjsonl.Transcript, windows []codexjsonl.WindowSummary, skipRollup bool, now time.Time) (ImportCodexJSONLResult, error) {
	summarizer, err := r.requireProcessSinkSummarizer()
	if err != nil {
		return ImportCodexJSONLResult{}, err
	}

	result := ImportCodexJSONLResult{
		InputPath: filepath.Clean(inputPath),
		AgentID:   transcript.AgentID,
		SessionID: transcript.SessionID,
	}

	reportDays := make(map[string]time.Time)
	for _, payload := range windows {
		title, content, err := summarizer.SummarizeCheckpointContext(ctx, payload)
		if err != nil {
			return ImportCodexJSONLResult{}, err
		}
		checkpoint, err := r.Harness.IngestSessionWindow(
			payload.Window,
			title,
			content,
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

	if skipRollup {
		return result, nil
	}

	keys := make([]string, 0, len(reportDays))
	for key := range reportDays {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		report, err := r.rollupProcessSinkDayContext(ctx, transcript.AgentID, reportDays[key], now)
		if err != nil {
			return ImportCodexJSONLResult{}, err
		}
		result.Reports = append(result.Reports, report)
	}
	return result, nil
}
