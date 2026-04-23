package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"obsidian-harness/internal/adapter/codexjsonl"
	"obsidian-harness/internal/model"
	"obsidian-harness/internal/store"
)

const codexJSONLLockStaleAfter = 30 * time.Second

type SyncCodexJSONLResult struct {
	Changed     bool
	CursorKey   string
	Fingerprint string
	Import      ImportCodexJSONLResult
}

type codexJSONLLockPayload struct {
	PID       int    `json:"pid"`
	CreatedAt string `json:"created_at"`
	CursorKey string `json:"cursor_key"`
}

func (r *Runtime) SyncCodexJSONL(params ImportCodexJSONLParams, now time.Time) (SyncCodexJSONLResult, error) {
	inputPath := filepath.Clean(strings.TrimSpace(params.InputPath))
	if inputPath == "" {
		return SyncCodexJSONLResult{}, fmt.Errorf("empty input path")
	}
	absolutePath, err := filepath.Abs(inputPath)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}

	source, err := codexjsonl.ReadSourceState(absolutePath)
	if err != nil {
		r.Harness.UpdateDependencies(true, false)
		return SyncCodexJSONLResult{}, err
	}
	r.Harness.UpdateDependencies(true, true)
	if _, err := r.Bootstrap(now); err != nil {
		return SyncCodexJSONLResult{}, err
	}

	cursorKey := codexJSONLCursorKey(absolutePath)
	releaseLock, err := acquireCodexJSONLLock(r.Config.Paths.StateDir, cursorKey)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}
	defer releaseLock()

	savedCursorRaw, cursor, err := r.loadCodexJSONLCursor(cursorKey)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}

	windowSize := resolveCodexWindowSize(r.Config.ProcessSink.CheckpointEvery, params.Window)
	if savedCursorRaw != "" && codexJSONLSourceUnchanged(cursor, source) {
		return r.syncCodexJSONLPlaceholders(params, now, source, cursorKey, savedCursorRaw, cursor, windowSize)
	}

	baseCursor := codexjsonl.Cursor{}
	readFrom := int64(0)
	allowCursorIdentity := false
	if savedCursorRaw != "" && codexJSONLCanResume(cursor, source) {
		baseCursor = cursor
		readFrom = codexJSONLReplayOffset(cursor)
		allowCursorIdentity = true
	}

	transcript, nextOffset, err := codexjsonl.LoadTail(source.Path, readFrom)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}
	applyCodexJSONLIdentity(&transcript, params, baseCursor, allowCursorIdentity)

	hasNewEvent := codexJSONLHasEventBeyondOffset(transcript.Events, baseCursor.Offset)
	windows := codexjsonl.BuildWindows(transcript, windowSize)
	if allowCursorIdentity && !hasNewEvent {
		windows = nil
	}
	trailing := []codexjsonl.WindowSummary(nil)
	if savedCursorRaw != "" {
		trailing = codexJSONLTrailingPlaceholders(transcript, baseCursor, windows, windowSize, now)
	}
	if allowCursorIdentity && !hasNewEvent && nextOffset == baseCursor.Offset && len(trailing) == 0 {
		return SyncCodexJSONLResult{
			Changed:     false,
			CursorKey:   cursorKey,
			Fingerprint: savedCursorRaw,
		}, nil
	}
	windows = append(windows, trailing...)

	imported, err := r.importCodexWindows(source.Path, transcript, windows, params.SkipRollup, now)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}

	updatedCursor := codexJSONLBuildCursor(baseCursor, transcript, source, windows, nextOffset)
	encodedCursor, err := codexjsonl.EncodeCursor(updatedCursor)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}
	if encodedCursor != savedCursorRaw {
		if err := r.Store.Cursors().SaveCursor(cursorKey, encodedCursor); err != nil {
			return SyncCodexJSONLResult{}, err
		}
	}

	return SyncCodexJSONLResult{
		Changed:     encodedCursor != savedCursorRaw,
		CursorKey:   cursorKey,
		Fingerprint: encodedCursor,
		Import:      imported,
	}, nil
}

func (r *Runtime) syncCodexJSONLPlaceholders(
	params ImportCodexJSONLParams,
	now time.Time,
	source codexjsonl.SourceState,
	cursorKey string,
	savedCursorRaw string,
	cursor codexjsonl.Cursor,
	windowSize time.Duration,
) (SyncCodexJSONLResult, error) {
	transcript := codexjsonl.Transcript{
		AgentID:    cursor.AgentID,
		SessionID:  cursor.SessionID,
		SourcePath: source.Path,
	}
	applyCodexJSONLIdentity(&transcript, params, cursor, true)

	windows := codexJSONLTrailingPlaceholders(transcript, cursor, nil, windowSize, now)
	if len(windows) == 0 {
		return SyncCodexJSONLResult{
			Changed:     false,
			CursorKey:   cursorKey,
			Fingerprint: savedCursorRaw,
		}, nil
	}

	imported, err := r.importCodexWindows(source.Path, transcript, windows, params.SkipRollup, now)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}

	updatedCursor := codexJSONLBuildCursor(cursor, transcript, source, windows, cursor.Offset)
	updatedCursor.Offset = cursor.Offset
	updatedCursor.ReplayOffset = cursor.ReplayOffset
	if updatedCursor.LastEventAt.IsZero() {
		updatedCursor.LastEventAt = cursor.LastEventAt
	}
	encodedCursor, err := codexjsonl.EncodeCursor(updatedCursor)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}
	if encodedCursor != savedCursorRaw {
		if err := r.Store.Cursors().SaveCursor(cursorKey, encodedCursor); err != nil {
			return SyncCodexJSONLResult{}, err
		}
	}

	return SyncCodexJSONLResult{
		Changed:     true,
		CursorKey:   cursorKey,
		Fingerprint: encodedCursor,
		Import:      imported,
	}, nil
}

func (r *Runtime) loadCodexJSONLCursor(cursorKey string) (string, codexjsonl.Cursor, error) {
	savedCursorRaw, err := r.Store.Cursors().GetCursor(cursorKey)
	if err != nil {
		if err == store.ErrNotFound {
			return "", codexjsonl.Cursor{}, nil
		}
		return "", codexjsonl.Cursor{}, err
	}
	cursor, err := codexjsonl.DecodeCursor(savedCursorRaw)
	if err != nil {
		return "", codexjsonl.Cursor{}, err
	}
	return savedCursorRaw, cursor, nil
}

func codexJSONLSourceUnchanged(cursor codexjsonl.Cursor, source codexjsonl.SourceState) bool {
	if cursor.Version == 0 {
		return cursor.Fingerprint != "" && cursor.Fingerprint == source.Fingerprint
	}
	return cursor.FileSize == source.Size &&
		cursor.FileModTimeNS == source.ModTimeNS &&
		cursor.FileHeadSHA256 == source.HeadSHA256
}

func codexJSONLCanResume(cursor codexjsonl.Cursor, source codexjsonl.SourceState) bool {
	if cursor.Version == 0 {
		return false
	}
	if !cursor.MatchesSource(source) {
		return false
	}
	if source.Size <= cursor.FileSize {
		return false
	}
	return true
}

func codexJSONLReplayOffset(cursor codexjsonl.Cursor) int64 {
	if cursor.ReplayOffset > 0 && cursor.ReplayOffset < cursor.Offset {
		return cursor.ReplayOffset
	}
	return 0
}

func codexJSONLHasEventBeyondOffset(events []codexjsonl.Event, offset int64) bool {
	for _, event := range events {
		if event.OffsetEnd > offset {
			return true
		}
	}
	return false
}

func codexJSONLTrailingPlaceholders(
	transcript codexjsonl.Transcript,
	cursor codexjsonl.Cursor,
	windows []codexjsonl.WindowSummary,
	windowSize time.Duration,
	now time.Time,
) []codexjsonl.WindowSummary {
	if windowSize <= 0 {
		return nil
	}
	closedBoundary := now.In(time.Local).Truncate(windowSize)
	if closedBoundary.IsZero() {
		return nil
	}

	anchorStart, sourceOffset := codexJSONLPlaceholderAnchor(cursor, windows)
	if anchorStart.IsZero() {
		return nil
	}

	out := make([]codexjsonl.WindowSummary, 0)
	for start := anchorStart.Add(windowSize); start.Before(closedBoundary); start = start.Add(windowSize) {
		window := codexjsonl.WindowSummary{
			Window: model.SessionWindow{
				AgentID:     transcript.AgentID,
				SessionID:   transcript.SessionID,
				WindowStart: start,
				WindowEnd:   start.Add(windowSize),
			},
			EventCount:   0,
			SourceOffset: sourceOffset,
		}
		out = append(out, window)
	}
	return out
}

func codexJSONLPlaceholderAnchor(cursor codexjsonl.Cursor, windows []codexjsonl.WindowSummary) (time.Time, int64) {
	if len(windows) > 0 {
		last := windows[len(windows)-1]
		return last.Window.WindowStart, last.SourceOffset
	}
	if cursor.LastWindowStart.IsZero() {
		return time.Time{}, 0
	}
	return cursor.LastWindowStart, cursor.ReplayOffset
}

func codexJSONLBuildCursor(
	base codexjsonl.Cursor,
	transcript codexjsonl.Transcript,
	source codexjsonl.SourceState,
	windows []codexjsonl.WindowSummary,
	nextOffset int64,
) codexjsonl.Cursor {
	cursor := base
	cursor.Version = 1
	cursor.Fingerprint = source.Fingerprint
	cursor.Offset = nextOffset
	cursor.FileSize = source.Size
	cursor.FileModTimeNS = source.ModTimeNS
	cursor.FileHeadSHA256 = source.HeadSHA256
	cursor.AgentID = transcript.AgentID
	cursor.SessionID = transcript.SessionID
	if len(windows) > 0 {
		cursor.LastWindowStart = windows[len(windows)-1].Window.WindowStart
		cursor.ReplayOffset = clampCodexJSONLOffset(windows[len(windows)-1].SourceOffset, nextOffset)
	} else {
		cursor.ReplayOffset = clampCodexJSONLOffset(cursor.ReplayOffset, nextOffset)
	}
	if len(transcript.Events) > 0 {
		cursor.LastEventAt = transcript.Events[len(transcript.Events)-1].Timestamp
	}
	return cursor
}

func clampCodexJSONLOffset(offset int64, limit int64) int64 {
	if offset < 0 {
		return 0
	}
	if limit >= 0 && offset > limit {
		return limit
	}
	return offset
}

func codexJSONLCursorKey(path string) string {
	return "codexjsonl:" + filepath.Clean(path)
}

func acquireCodexJSONLLock(stateDir string, cursorKey string) (func(), error) {
	lockPath := filepath.Join(stateDir, "locks", codexJSONLLockName(cursorKey))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}

	payload := codexJSONLLockPayload{
		PID:       os.Getpid(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		CursorKey: cursorKey,
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "  ")
			writeErr := encoder.Encode(payload)
			closeErr := file.Close()
			if writeErr != nil {
				_ = os.Remove(lockPath)
				return nil, writeErr
			}
			if closeErr != nil {
				_ = os.Remove(lockPath)
				return nil, closeErr
			}
			return func() {
				_ = os.Remove(lockPath)
			}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		stale, owner, staleErr := shouldReclaimCodexJSONLLock(lockPath, time.Now())
		if staleErr != nil {
			return nil, staleErr
		}
		if !stale {
			if owner != "" {
				return nil, fmt.Errorf("codex jsonl source already syncing: %s (%s)", cursorKey, owner)
			}
			return nil, fmt.Errorf("codex jsonl source already syncing: %s", cursorKey)
		}
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("codex jsonl source already syncing: %s", cursorKey)
}

func codexJSONLLockName(cursorKey string) string {
	sum := sha256.Sum256([]byte(cursorKey))
	return hex.EncodeToString(sum[:8]) + ".lock"
}

func shouldReclaimCodexJSONLLock(lockPath string, now time.Time) (bool, string, error) {
	payload, parsed := readCodexJSONLLockPayload(lockPath)
	if parsed {
		if createdAt, err := time.Parse(time.RFC3339Nano, payload.CreatedAt); err == nil {
			if now.Sub(createdAt) > codexJSONLLockStaleAfter {
				return true, codexJSONLLockOwner(payload), nil
			}
		}
	}

	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, "", nil
		}
		return false, "", err
	}
	if now.Sub(info.ModTime()) > codexJSONLLockStaleAfter {
		return true, codexJSONLLockOwner(payload), nil
	}
	return false, codexJSONLLockOwner(payload), nil
}

func readCodexJSONLLockPayload(lockPath string) (codexJSONLLockPayload, bool) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return codexJSONLLockPayload{}, false
	}
	var payload codexJSONLLockPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return codexJSONLLockPayload{}, false
	}
	if payload.PID <= 0 || strings.TrimSpace(payload.CreatedAt) == "" {
		return codexJSONLLockPayload{}, false
	}
	return payload, true
}

func codexJSONLLockOwner(payload codexJSONLLockPayload) string {
	if payload.PID <= 0 {
		return ""
	}
	return fmt.Sprintf("pid %d", payload.PID)
}
