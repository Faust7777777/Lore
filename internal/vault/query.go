package vault

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ListEntry struct {
	Path    string
	Name    string
	IsDir   bool
	ModTime time.Time
}

type TextHit struct {
	Path    string
	Line    int
	Preview string
}

func ListEntries(root string, relDir string) ([]ListEntry, error) {
	absolute, normalized, err := resolveUnderRoot(root, relDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absolute)
	if err != nil {
		return nil, err
	}

	out := make([]ListEntry, 0, len(entries))
	for _, entry := range entries {
		childPath := joinNormalized(normalized, entry.Name())
		if shouldIgnoreRelativePath(childPath) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, ListEntry{
			Path:    childPath,
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func SearchText(root string, relDir string, query string, limit int) ([]TextHit, error) {
	if limit <= 0 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	lowerQuery := strings.ToLower(query)

	hits := make([]TextHit, 0, limit)
	err := walkMarkdownFiles(root, relDir, func(relPath string, _ fs.DirEntry) error {
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(relPath)))
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), lowerQuery) {
				hits = append(hits, TextHit{
					Path:    relPath,
					Line:    lineNumber,
					Preview: trimPreview(line),
				})
				if len(hits) >= limit {
					return fs.SkipAll
				}
				break
			}
		}
		return scanner.Err()
	})
	if err == fs.SkipAll {
		err = nil
	}
	return hits, err
}

func FindBacklinks(root string, relDir string, targetPath string, limit int) ([]TextHit, error) {
	if limit <= 0 {
		limit = 10
	}

	normalizedTarget := normalizeRelativePath(targetPath)
	baseName := strings.TrimSuffix(pathBase(normalizedTarget), filepath.Ext(normalizedTarget))
	targetWithoutExt := strings.TrimSuffix(normalizedTarget, filepath.Ext(normalizedTarget))
	patterns := []string{
		"[[" + baseName + "]]",
		"[[" + baseName + "|",
		"[[" + targetWithoutExt + "]]",
		"[[" + targetWithoutExt + "|",
	}

	hits := make([]TextHit, 0, limit)
	err := walkMarkdownFiles(root, relDir, func(relPath string, _ fs.DirEntry) error {
		if relPath == normalizedTarget {
			return nil
		}

		file, err := os.Open(filepath.Join(root, filepath.FromSlash(relPath)))
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			for _, pattern := range patterns {
				if strings.Contains(line, pattern) {
					hits = append(hits, TextHit{
						Path:    relPath,
						Line:    lineNumber,
						Preview: trimPreview(line),
					})
					if len(hits) >= limit {
						return fs.SkipAll
					}
					return nil
				}
			}
		}
		return scanner.Err()
	})
	if err == fs.SkipAll {
		err = nil
	}
	return hits, err
}

func WalkMarkdownPaths(root string, relDir string) ([]string, error) {
	paths := make([]string, 0)
	err := walkMarkdownFiles(root, relDir, func(relPath string, _ fs.DirEntry) error {
		paths = append(paths, relPath)
		return nil
	})
	return paths, err
}

func ReadRelativeWithHash(root string, relPath string) ([]byte, string, string, error) {
	absolute, normalized, err := resolveUnderRoot(root, relPath)
	if err != nil {
		return nil, "", "", err
	}
	data, hash, err := ReadFileWithHash(absolute)
	if err != nil {
		return nil, "", "", err
	}
	return data, hash, normalized, nil
}

func walkMarkdownFiles(root string, relDir string, visit func(relPath string, entry fs.DirEntry) error) error {
	absolute, normalized, err := resolveUnderRoot(root, relDir)
	if err != nil {
		return err
	}

	return filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relToRoot, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relToRoot = normalizeRelativePath(relToRoot)
		if relToRoot == "." {
			relToRoot = normalized
		}

		if entry.IsDir() {
			if relToRoot != "" && shouldIgnoreRelativePath(relToRoot) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnoreRelativePath(relToRoot) {
			return nil
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}
		return visit(relToRoot, entry)
	})
}

func resolveUnderRoot(root string, rel string) (string, string, error) {
	normalized := normalizeRelativePath(rel)
	if normalized == "." {
		normalized = ""
	}
	absolute := filepath.Join(root, filepath.FromSlash(normalized))
	cleanRoot := filepath.Clean(root)
	cleanAbsolute := filepath.Clean(absolute)
	if cleanAbsolute != cleanRoot && !strings.HasPrefix(cleanAbsolute, cleanRoot+string(filepath.Separator)) {
		return "", "", fs.ErrPermission
	}
	return absolute, normalized, nil
}

func normalizeRelativePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	value = strings.TrimPrefix(value, "./")
	return value
}

func shouldIgnoreRelativePath(rel string) bool {
	rel = normalizeRelativePath(rel)
	if rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		switch part {
		case ".obsidian", ".trash", ".git", ".tools", "state":
			return true
		}
	}
	return strings.HasSuffix(rel, ".obsidian-harness-tmp")
}

func joinNormalized(base string, child string) string {
	if base == "" {
		return normalizeRelativePath(child)
	}
	return normalizeRelativePath(base + "/" + child)
}

func pathBase(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "/")
	return parts[len(parts)-1]
}

func trimPreview(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 180 {
		return value[:180] + "..."
	}
	return value
}
