package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/adapter/externaljsonl"
	"obsidian-harness/internal/model"
)

type ImportExternalTranscriptJSONLParams struct {
	InputPath  string
	AgentID    string
	SessionID  string
	Window     time.Duration
	SkipRollup bool
}

type ImportExternalTranscriptJSONLResult struct {
	InputPath   string
	AgentID     string
	SessionID   string
	Checkpoints []model.CheckpointDoc
	Reports     []model.DailyReport
}

func (r *Runtime) ImportExternalTranscriptJSONL(params ImportExternalTranscriptJSONLParams, now time.Time) (ImportExternalTranscriptJSONLResult, error) {
	r.Harness.UpdateDependencies(true, true)
	if _, err := r.Bootstrap(now); err != nil {
		return ImportExternalTranscriptJSONLResult{}, err
	}

	inputPath := filepath.Clean(strings.TrimSpace(params.InputPath))
	if inputPath == "." || inputPath == "" {
		return ImportExternalTranscriptJSONLResult{}, fmt.Errorf("empty input path")
	}
	transcript, err := externaljsonl.LoadFile(inputPath)
	if err != nil {
		return ImportExternalTranscriptJSONLResult{}, err
	}
	applyExternalTranscriptIdentity(&transcript, params)
	windows := codexjsonl.BuildWindows(transcript, resolveCodexWindowSize(r.Config.ProcessSink.CheckpointEvery, params.Window))
	imported, err := r.importCodexWindows(inputPath, transcript, windows, params.SkipRollup, now)
	if err != nil {
		return ImportExternalTranscriptJSONLResult{}, err
	}
	return ImportExternalTranscriptJSONLResult{
		InputPath:   imported.InputPath,
		AgentID:     imported.AgentID,
		SessionID:   imported.SessionID,
		Checkpoints: imported.Checkpoints,
		Reports:     imported.Reports,
	}, nil
}

func applyExternalTranscriptIdentity(transcript *codexjsonl.Transcript, params ImportExternalTranscriptJSONLParams) {
	if agentID := strings.TrimSpace(params.AgentID); agentID != "" {
		transcript.AgentID = agentID
	}
	if sessionID := strings.TrimSpace(params.SessionID); sessionID != "" {
		transcript.SessionID = sessionID
	}
	codexjsonl.FinalizeTranscript(transcript)
}
