package app

import (
	"fmt"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexappserver"
	"obsidian-harness/internal/adapter/codexjsonl"
)

type codexAppServerThreadReader interface {
	ReadThread(params codexappserver.ReadThreadParams) (codexappserver.Thread, error)
}

type ImportCodexAppServerThreadParams struct {
	Thread     codexappserver.Thread
	SourcePath string
	AgentID    string
	SessionID  string
	Window     time.Duration
	SkipRollup bool
}

type ImportCodexAppServerSourceParams struct {
	ThreadID   string
	SourcePath string
	AgentID    string
	SessionID  string
	Window     time.Duration
	SkipRollup bool
}

func (r *Runtime) ImportCodexAppServerSource(reader codexAppServerThreadReader, params ImportCodexAppServerSourceParams, now time.Time) (ImportCodexJSONLResult, error) {
	if reader == nil {
		return ImportCodexJSONLResult{}, fmt.Errorf("codex app-server reader is required")
	}
	threadID := strings.TrimSpace(params.ThreadID)
	if threadID == "" {
		return ImportCodexJSONLResult{}, fmt.Errorf("codex app-server thread id is required")
	}

	thread, err := reader.ReadThread(codexappserver.ReadThreadParams{
		ThreadID:     threadID,
		IncludeTurns: true,
	})
	if err != nil {
		return ImportCodexJSONLResult{}, err
	}
	return r.ImportCodexAppServerThread(ImportCodexAppServerThreadParams{
		Thread:     thread,
		SourcePath: params.SourcePath,
		AgentID:    params.AgentID,
		SessionID:  params.SessionID,
		Window:     params.Window,
		SkipRollup: params.SkipRollup,
	}, now)
}

func (r *Runtime) ImportCodexAppServerThread(params ImportCodexAppServerThreadParams, now time.Time) (ImportCodexJSONLResult, error) {
	r.Harness.UpdateDependencies(true, true)
	if _, err := r.Bootstrap(now); err != nil {
		return ImportCodexJSONLResult{}, err
	}

	transcript, err := codexappserver.TranscriptFromThread(params.Thread, params.SourcePath)
	if err != nil {
		return ImportCodexJSONLResult{}, err
	}
	applyCodexJSONLIdentity(&transcript, ImportCodexJSONLParams{
		AgentID:   params.AgentID,
		SessionID: params.SessionID,
	}, codexjsonl.Cursor{}, false)
	windows := codexjsonl.BuildWindows(transcript, resolveCodexWindowSize(r.Config.ProcessSink.CheckpointEvery, params.Window))
	return r.importCodexWindows(transcript.SourcePath, transcript, windows, params.SkipRollup, now)
}
