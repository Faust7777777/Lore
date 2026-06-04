package vault

import (
	"bufio"
	"errors"
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

func NormalizeRelativePath(value string) string {
	return normalizeRelativePath(value)
}

func ShouldIgnoreRelativePath(rel string) bool {
	return shouldIgnoreRelativePath(rel)
}

func ListEntries(root string, relDir string) ([]ListEntry, error) {
	absolute, normalized, err := ResolveExistingUnderRoot(root, relDir)
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
		if _, _, err := ResolveExistingUnderRoot(root, childPath); err != nil {
			if errors.Is(err, fs.ErrPermission) || os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !entry.IsDir() && !info.Mode().IsRegular() {
			continue
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
		absolute, _, err := ResolveExistingUnderRoot(root, relPath)
		if err != nil {
			return err
		}
		file, err := os.Open(absolute)
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
	wikiPatterns := []string{
		"[[" + baseName + "]]",
		"[[" + baseName + "|",
		"[[" + targetWithoutExt + "]]",
		"[[" + targetWithoutExt + "|",
	}
	// Markdown-link suffix candidates. We accept any URL that ends
	// in one of these, so `[doc](./notes/refactor.md)` and
	// `[doc](../notes/refactor.md)` both resolve as backlinks
	// without per-source-file path arithmetic. The full path is
	// the most precise match; the basename-with-ext catches
	// cross-folder relative links like `../refactor.md` that share
	// the same filename. A bare-basename match (no extension)
	// would be ambiguous and is intentionally NOT included.
	markdownSuffixes := []string{normalizedTarget}
	baseWithExt := pathBase(normalizedTarget)
	if baseWithExt != "" && baseWithExt != normalizedTarget {
		markdownSuffixes = append(markdownSuffixes, baseWithExt)
	}

	hits := make([]TextHit, 0, limit)
	err := walkMarkdownFiles(root, relDir, func(relPath string, _ fs.DirEntry) error {
		if relPath == normalizedTarget {
			return nil
		}

		absolute, _, err := ResolveExistingUnderRoot(root, relPath)
		if err != nil {
			return err
		}
		file, err := os.Open(absolute)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			matched := false
			for _, pattern := range wikiPatterns {
				if strings.Contains(line, pattern) {
					matched = true
					break
				}
			}
			if !matched && lineHasMarkdownLinkTo(line, markdownSuffixes) {
				matched = true
			}
			if !matched {
				continue
			}
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
		return scanner.Err()
	})
	if err == fs.SkipAll {
		err = nil
	}
	return hits, err
}

// lineHasMarkdownLinkTo reports whether the line contains a
// CommonMark inline link `](URL)` whose URL resolves to one of the
// `suffixes` (typically the target's full normalized path and its
// basename-with-extension). The URL extends from `](` up to the
// first space, `)`, `#` (anchor), or `"` (title), and can optionally
// be wrapped in angle brackets `](<URL>)`. A leading `./` is
// stripped before suffix comparison so `./notes/x.md` matches
// `notes/x.md` and `../notes/x.md` matches `notes/x.md` via the
// full-path suffix. A bare basename like `[doc](other-folder.md)`
// only matches when the source URL ends in the basename suffix --
// callers should pass the basename explicitly when they want that
// fallback. No regex is used: lineHasMarkdownLinkTo runs once per
// scanned line in the backlink walk, and a hand-rolled scan is
// faster and easier to reason about than backtracking patterns.
func lineHasMarkdownLinkTo(line string, suffixes []string) bool {
	rest := line
	for {
		idx := strings.Index(rest, "](")
		if idx == -1 {
			return false
		}
		urlStart := idx + 2
		url := rest[urlStart:]
		if strings.HasPrefix(url, "<") {
			closing := strings.Index(url, ">")
			if closing == -1 {
				return false
			}
			url = url[1:closing]
		} else {
			end := len(url)
			for i, ch := range url {
				if ch == ')' || ch == ' ' || ch == '\t' || ch == '#' || ch == '"' {
					end = i
					break
				}
			}
			url = url[:end]
		}
		url = strings.TrimPrefix(url, "./")
		url = strings.TrimSpace(url)
		for _, suffix := range suffixes {
			if suffix == "" {
				continue
			}
			if url == suffix || strings.HasSuffix(url, "/"+suffix) {
				return true
			}
		}
		rest = rest[urlStart:]
	}
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
	absolute, normalized, err := ResolveExistingUnderRoot(root, relPath)
	if err != nil {
		return nil, "", "", err
	}
	data, hash, err := ReadFileWithHash(absolute)
	if err != nil {
		return nil, "", "", err
	}
	return data, hash, normalized, nil
}

func WriteRelativeAtomic(root string, relPath string, data []byte, tempSuffix string) (string, string, error) {
	absolute, normalized, err := ResolveWriteUnderRoot(root, relPath)
	if err != nil {
		return "", "", err
	}
	hash, err := WriteFileAtomic(absolute, data, tempSuffix)
	if err != nil {
		return "", "", err
	}
	return hash, normalized, nil
}

func StatRelative(root string, relPath string) (fs.FileInfo, string, error) {
	absolute, normalized, err := ResolveExistingUnderRoot(root, relPath)
	if err != nil {
		return nil, "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, "", err
	}
	return info, normalized, nil
}

func walkMarkdownFiles(root string, relDir string, visit func(relPath string, entry fs.DirEntry) error) error {
	absolute, normalized, err := ResolveExistingUnderRoot(root, relDir)
	if err != nil {
		return err
	}
	realRoot, err := canonicalRoot(root, false)
	if err != nil {
		return err
	}

	return filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			if entry.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			return err
		}
		if !isPathUnderRoot(realRoot, filepath.Clean(realPath)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relToRoot, err := filepath.Rel(realRoot, path)
		if err != nil {
			return err
		}
		relToRoot = normalizeRelativePath(relToRoot)
		if relToRoot == "." {
			relToRoot = normalized
		}

		if entry.IsDir() {
			if entry.Type()&fs.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			if relToRoot != "" && shouldIgnoreRelativePath(relToRoot) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
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

func ResolveExistingUnderRoot(root string, rel string) (string, string, error) {
	absolute, normalized, err := resolveLexicalUnderRoot(root, rel)
	if err != nil {
		return "", "", err
	}
	if err := rejectUnsafeExistingParents(absolute); err != nil {
		return "", "", err
	}
	realRoot, err := canonicalRoot(root, false)
	if err != nil {
		return "", "", err
	}
	realAbsolute, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", "", err
	}
	realAbsolute = filepath.Clean(realAbsolute)
	if !isPathUnderRoot(realRoot, realAbsolute) {
		return "", "", fs.ErrPermission
	}
	return realAbsolute, normalized, nil
}

func ResolveWriteUnderRoot(root string, rel string) (string, string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", err
	}
	absolute, normalized, err := resolveLexicalUnderRoot(root, rel)
	if err != nil {
		return "", "", err
	}
	realRoot, err := canonicalRoot(root, true)
	if err != nil {
		return "", "", err
	}

	if info, err := os.Lstat(absolute); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return "", "", fs.ErrPermission
	} else if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	if realAbsolute, err := filepath.EvalSymlinks(absolute); err == nil {
		realAbsolute = filepath.Clean(realAbsolute)
		if !isPathUnderRoot(realRoot, realAbsolute) {
			return "", "", fs.ErrPermission
		}
		return absolute, normalized, nil
	} else if !os.IsNotExist(err) {
		return "", "", err
	}

	parent, err := nearestExistingParent(absolute)
	if err != nil {
		return "", "", err
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", "", err
	}
	realParent = filepath.Clean(realParent)
	if !isPathUnderRoot(realRoot, realParent) {
		return "", "", fs.ErrPermission
	}
	return absolute, normalized, nil
}

func resolveLexicalUnderRoot(root string, rel string) (string, string, error) {
	normalized := normalizeRelativePath(rel)
	if normalized == "." {
		normalized = ""
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", err
	}
	cleanRoot = filepath.Clean(cleanRoot)
	absolute := filepath.Join(cleanRoot, filepath.FromSlash(normalized))
	cleanAbsolute := filepath.Clean(absolute)
	if !isPathUnderRoot(cleanRoot, cleanAbsolute) {
		return "", "", fs.ErrPermission
	}
	return cleanAbsolute, normalized, nil
}

func canonicalRoot(root string, create bool) (string, error) {
	if create {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return filepath.Clean(absolute), nil
		}
		return "", err
	}
	return filepath.Clean(realRoot), nil
}

func nearestExistingParent(path string) (string, error) {
	dir := filepath.Dir(filepath.Clean(path))
	for {
		info, err := os.Lstat(dir)
		if err == nil {
			if info.Mode()&fs.ModeSymlink == 0 && !info.IsDir() {
				return "", fs.ErrPermission
			}
			return dir, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", os.ErrNotExist
		}
		dir = next
	}
}

func rejectUnsafeExistingParents(path string) error {
	dir := filepath.Dir(filepath.Clean(path))
	parts := strings.Split(dir, string(filepath.Separator))
	if filepath.IsAbs(dir) {
		volume := filepath.VolumeName(dir)
		current := volume + string(filepath.Separator)
		rest := strings.TrimPrefix(dir, current)
		if rest == "" {
			return nil
		}
		parts = strings.Split(rest, string(filepath.Separator))
		for _, part := range parts {
			if part == "" {
				continue
			}
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if info.Mode()&fs.ModeSymlink == 0 && !info.IsDir() {
				return fs.ErrPermission
			}
		}
		return nil
	}
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink == 0 && !info.IsDir() {
			return fs.ErrPermission
		}
	}
	return nil
}

func isPathUnderRoot(root string, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
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
	// Rune-safe: search/backlink previews include Chinese; a 180-byte slice
	// could split a multibyte character into an invalid UTF-8 sequence.
	runes := []rune(value)
	if len(runes) > 180 {
		return string(runes[:180]) + "..."
	}
	return value
}
