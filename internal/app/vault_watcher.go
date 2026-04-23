package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"

	"obsidian-harness/internal/vault"
)

type vaultWatcher struct {
	root    string
	watcher *fsnotify.Watcher
	watched map[string]struct{}
}

func newVaultWatcher(root string) (*vaultWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	tracker := &vaultWatcher{
		root:    filepath.Clean(root),
		watcher: watcher,
		watched: make(map[string]struct{}),
	}
	if err := tracker.addRecursive(tracker.root); err != nil {
		watcher.Close()
		return nil, err
	}
	return tracker, nil
}

func (v *vaultWatcher) Events() <-chan fsnotify.Event {
	return v.watcher.Events
}

func (v *vaultWatcher) Errors() <-chan error {
	return v.watcher.Errors
}

func (v *vaultWatcher) Close() error {
	return v.watcher.Close()
}

func (v *vaultWatcher) CollectPaths(event fsnotify.Event) ([]string, error) {
	absPath := filepath.Clean(event.Name)
	relPath, ok := v.relativePath(absPath)
	if !ok || relPath == "" || vault.ShouldIgnoreRelativePath(relPath) {
		return nil, nil
	}

	info, err := os.Stat(absPath)
	if err == nil && info.IsDir() {
		if event.Op&(fsnotify.Create|fsnotify.Rename) == 0 {
			return nil, nil
		}
		if err := v.addRecursive(absPath); err != nil {
			return nil, err
		}
		paths, walkErr := vault.WalkMarkdownPaths(v.root, relPath)
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil, nil
			}
			return nil, walkErr
		}
		return paths, nil
	}

	if err != nil {
		if os.IsNotExist(err) {
			v.forgetRecursive(absPath)
			if strings.ToLower(filepath.Ext(relPath)) != ".md" {
				return nil, nil
			}
			return []string{relPath}, nil
		}
		return nil, err
	}

	if strings.ToLower(filepath.Ext(relPath)) != ".md" {
		return nil, nil
	}
	return []string{relPath}, nil
}

func (v *vaultWatcher) addRecursive(absDir string) error {
	absDir = filepath.Clean(absDir)
	return filepath.WalkDir(absDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}

		relPath, ok := v.relativePath(path)
		if !ok {
			return filepath.SkipDir
		}
		if relPath != "" && vault.ShouldIgnoreRelativePath(relPath) {
			return filepath.SkipDir
		}
		if _, exists := v.watched[path]; exists {
			return nil
		}
		if err := v.watcher.Add(path); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		v.watched[path] = struct{}{}
		return nil
	})
}

func (v *vaultWatcher) relativePath(absPath string) (string, bool) {
	relPath, err := filepath.Rel(v.root, absPath)
	if err != nil {
		return "", false
	}
	relPath = vault.NormalizeRelativePath(relPath)
	if relPath == "." {
		relPath = ""
	}
	if relPath == "" {
		return "", true
	}
	if relPath == ".." || strings.HasPrefix(relPath, "../") {
		return "", false
	}
	return relPath, true
}

func (v *vaultWatcher) forgetRecursive(absPath string) {
	absPath = filepath.Clean(absPath)
	prefix := absPath + string(filepath.Separator)
	for watchedPath := range v.watched {
		if watchedPath == absPath || strings.HasPrefix(watchedPath, prefix) {
			delete(v.watched, watchedPath)
		}
	}
}
