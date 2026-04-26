package lore

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSDKDoesNotImportLoreInternalPackages(t *testing.T) {
	t.Parallel()

	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob Go files: %v", err)
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

func isLoreInternalImport(path string) bool {
	return path == "obsidian-harness/internal" || strings.HasPrefix(path, "obsidian-harness/internal/")
}
