package lore

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSDKDoesNotImportLoreInternalPackages(t *testing.T) {
	t.Parallel()

	matches, err := sdkGoFiles()
	if err != nil {
		t.Fatalf("list Go files: %v", err)
	}
	for _, file := range matches {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports from %s: %v", file, err)
		}
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import %s in %s: %v", spec.Path.Value, file, err)
			}
			if isLoreInternalImport(path) {
				t.Fatalf("SDK file %s imports Lore internal package %q", file, path)
			}
		}
	}
}

func sdkGoFiles() ([]string, error) {
	var matches []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != "." && shouldSkipSDKBoundaryDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func shouldSkipSDKBoundaryDir(name string) bool {
	switch name {
	case ".git", ".idea", ".vscode", "node_modules", "vendor":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func isLoreInternalImport(path string) bool {
	return path == "obsidian-harness/internal" || strings.HasPrefix(path, "obsidian-harness/internal/")
}
