package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SyncCodexJSONLResult struct {
	Changed     bool
	CursorKey   string
	Fingerprint string
	Import      ImportCodexJSONLResult
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

	info, err := os.Stat(absolutePath)
	if err != nil {
		r.Harness.UpdateDependencies(true, false)
		return SyncCodexJSONLResult{}, err
	}
	r.Harness.UpdateDependencies(true, true)

	cursorKey := codexJSONLCursorKey(absolutePath)
	fingerprint := codexJSONLFingerprint(info)
	releaseLock, err := acquireCodexJSONLLock(r.Config.Paths.StateDir, cursorKey)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}
	defer releaseLock()

	if saved, err := r.Store.Cursors().GetCursor(cursorKey); err == nil && saved == fingerprint {
		return SyncCodexJSONLResult{
			Changed:     false,
			CursorKey:   cursorKey,
			Fingerprint: fingerprint,
		}, nil
	}

	params.InputPath = absolutePath
	imported, err := r.ImportCodexJSONL(params, now)
	if err != nil {
		return SyncCodexJSONLResult{}, err
	}
	if err := r.Store.Cursors().SaveCursor(cursorKey, fingerprint); err != nil {
		return SyncCodexJSONLResult{}, err
	}

	return SyncCodexJSONLResult{
		Changed:     true,
		CursorKey:   cursorKey,
		Fingerprint: fingerprint,
		Import:      imported,
	}, nil
}

func codexJSONLCursorKey(path string) string {
	return "codexjsonl:" + filepath.Clean(path)
}

func codexJSONLFingerprint(info os.FileInfo) string {
	return fmt.Sprintf("size=%d|mtime=%d", info.Size(), info.ModTime().UTC().UnixNano())
}

func acquireCodexJSONLLock(stateDir string, cursorKey string) (func(), error) {
	lockPath := filepath.Join(stateDir, "locks", codexJSONLLockName(cursorKey))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("codex jsonl source already syncing: %s", cursorKey)
		}
		return nil, err
	}
	_, _ = file.WriteString(cursorKey + "\n")
	_ = file.Close()

	return func() {
		_ = os.Remove(lockPath)
	}, nil
}

func codexJSONLLockName(cursorKey string) string {
	sum := sha256.Sum256([]byte(cursorKey))
	return hex.EncodeToString(sum[:8]) + ".lock"
}
