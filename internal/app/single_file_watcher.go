package app

import (
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type singleFileWatcher struct {
	path    string
	watcher *fsnotify.Watcher
}

func newSingleFileWatcher(path string) (*singleFileWatcher, error) {
	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(filepath.Dir(absolutePath)); err != nil {
		watcher.Close()
		return nil, err
	}
	return &singleFileWatcher{
		path:    absolutePath,
		watcher: watcher,
	}, nil
}

func (w *singleFileWatcher) Events() <-chan fsnotify.Event {
	return w.watcher.Events
}

func (w *singleFileWatcher) Errors() <-chan error {
	return w.watcher.Errors
}

func (w *singleFileWatcher) Close() error {
	return w.watcher.Close()
}

func (w *singleFileWatcher) Matches(event fsnotify.Event) bool {
	if w == nil {
		return false
	}
	return sameWatchPath(event.Name, w.path)
}

func sameWatchPath(left string, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
