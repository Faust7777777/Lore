package sessionlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"obsidian-harness/internal/vault"
)

const indexTempSuffix = ".tmp"

var indexLocks = struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}{locks: make(map[string]*sync.Mutex)}

func indexPath(rootDir string) string {
	return filepath.Join(rootDir, "index.json")
}

func acquireIndexLock(rootDir string) func() {
	key := indexLockKey(rootDir)
	indexLocks.mu.Lock()
	lock := indexLocks.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		indexLocks.locks[key] = lock
	}
	indexLocks.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func indexLockKey(rootDir string) string {
	rootDir = filepath.Clean(rootDir)
	absolute, err := filepath.Abs(rootDir)
	if err != nil {
		return rootDir
	}
	return filepath.Clean(absolute)
}

func loadIndex(rootDir string) (Index, error) {
	rootDir = filepath.Clean(rootDir)
	unlock := acquireIndexLock(rootDir)
	defer unlock()
	return loadIndexUnlocked(rootDir)
}

func loadIndexUnlocked(rootDir string) (Index, error) {
	data, err := os.ReadFile(indexPath(rootDir))
	if os.IsNotExist(err) {
		return rebuildIndexUnlocked(rootDir)
	}
	if err != nil {
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, err
	}
	if index.Version == 0 {
		index.Version = 1
	}
	return index, nil
}

func rebuildIndex(rootDir string) (Index, error) {
	rootDir = filepath.Clean(rootDir)
	unlock := acquireIndexLock(rootDir)
	defer unlock()
	return rebuildIndexUnlocked(rootDir)
}

func rebuildIndexUnlocked(rootDir string) (Index, error) {
	entries, err := os.ReadDir(rootDir)
	if os.IsNotExist(err) {
		return Index{Version: 1}, nil
	}
	if err != nil {
		return Index{}, err
	}
	index := Index{Version: 1}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if strings.TrimSpace(sessionID) == "" {
			continue
		}
		snapshot, err := Load(rootDir, sessionID)
		if err != nil {
			return Index{}, err
		}
		index.Sessions = append(index.Sessions, snapshot.Summary)
	}
	if err := saveIndexUnlocked(rootDir, index); err != nil {
		return Index{}, err
	}
	return index, nil
}

func saveIndex(rootDir string, index Index) error {
	rootDir = filepath.Clean(rootDir)
	unlock := acquireIndexLock(rootDir)
	defer unlock()
	return saveIndexUnlocked(rootDir, index)
}

func saveIndexUnlocked(rootDir string, index Index) error {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return err
	}
	index.Version = 1
	sort.SliceStable(index.Sessions, func(i, j int) bool {
		return index.Sessions[i].UpdatedAt.After(index.Sessions[j].UpdatedAt)
	})
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	_, err = vault.WriteFileAtomic(indexPath(rootDir), append(data, '\n'), indexTempSuffix)
	return err
}

func upsertIndex(rootDir string, summary Summary) error {
	if strings.TrimSpace(summary.ID) == "" {
		return nil
	}
	if summary.UpdatedAt.IsZero() {
		summary.UpdatedAt = time.Now()
	}
	rootDir = filepath.Clean(rootDir)
	unlock := acquireIndexLock(rootDir)
	defer unlock()
	index, err := loadIndexUnlocked(rootDir)
	if err != nil {
		return err
	}
	updated := false
	for i, existing := range index.Sessions {
		if existing.ID == summary.ID {
			index.Sessions[i] = mergeSummary(existing, summary)
			updated = true
			break
		}
	}
	if !updated {
		index.Sessions = append(index.Sessions, summary)
	}
	return saveIndexUnlocked(rootDir, index)
}

func mergeSummary(existing Summary, next Summary) Summary {
	if next.Path == "" {
		next.Path = existing.Path
	}
	if next.StartedAt.IsZero() {
		next.StartedAt = existing.StartedAt
	}
	if next.UpdatedAt.IsZero() || (!existing.UpdatedAt.IsZero() && existing.UpdatedAt.After(next.UpdatedAt)) {
		next.UpdatedAt = existing.UpdatedAt
	}
	if next.Title == "" {
		next.Title = existing.Title
	}
	if next.TurnCount == 0 {
		next.TurnCount = existing.TurnCount
	}
	if next.Model == "" {
		next.Model = existing.Model
	}
	if next.AgentID == "" {
		next.AgentID = existing.AgentID
	}
	return next
}
