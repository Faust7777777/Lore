package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRelativeWithHashBlocksTraversal(t *testing.T) {
	root := t.TempDir()
	parentFile := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(parentFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile(parentFile) error = %v", err)
	}

	if _, _, _, err := ReadRelativeWithHash(root, "..\\outside.txt"); err == nil {
		t.Fatal("ReadRelativeWithHash() error = nil, want permission failure")
	}
}

// FindBacklinks previously only matched Obsidian wiki-link syntax
// `[[name]]`. Real notes from external editors (and many of this
// project's own docs under /docs/) use markdown inline links of the
// shape `[label](relative/path/to/note.md)`. Without this support
// the backlink list under-reports references and the operator's
// "what links to this doc" view is incomplete.
//
// Cases covered:
//   - bare basename link in a sibling file
//   - full-path link with `./` prefix
//   - relative `../` link from a subfolder back to the root target
//   - anchor suffix is stripped before suffix comparison
//   - angle-bracketed URL form `](<path>)`
//   - mismatched extension does NOT match (foo.md vs foo.txt)
//
// The wiki-link path is regression-checked alongside so the new
// markdown branch did not displace it.
func TestFindBacklinksRecognisesMarkdownLinks(t *testing.T) {
	root := t.TempDir()
	target := "notes/refactor.md"
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "deep", "sub"), 0o755); err != nil {
		t.Fatalf("mkdir deep/sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "refactor.md"), []byte("# target\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	files := map[string]string{
		"basename-link.md":       "see [doc](refactor.md) for more",
		"full-path-link.md":      "the writeup lives at [here](./notes/refactor.md)",
		"deep/sub/relative.md":   "back to [the plan](../../notes/refactor.md#section)",
		"angle-bracket.md":       "wrapped form: [w](<notes/refactor.md>)",
		"wrong-ext.md":           "[other](refactor.txt) should NOT match",
		"wiki-still-works.md":    "linking [[refactor]] via wiki syntax",
		"wiki-with-pipe.md":      "linking [[refactor|aliased]] via wiki+pipe",
	}
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	hits, err := FindBacklinks(root, "", target, 50)
	if err != nil {
		t.Fatalf("FindBacklinks: %v", err)
	}
	gotPaths := make(map[string]bool, len(hits))
	for _, hit := range hits {
		gotPaths[hit.Path] = true
	}

	wantPaths := []string{
		"basename-link.md",
		"full-path-link.md",
		"deep/sub/relative.md",
		"angle-bracket.md",
		"wiki-still-works.md",
		"wiki-with-pipe.md",
	}
	for _, want := range wantPaths {
		if !gotPaths[want] {
			t.Errorf("missing backlink hit for %q (got paths %v)", want, keys(gotPaths))
		}
	}
	if gotPaths["wrong-ext.md"] {
		t.Errorf("wrong-ext.md matched but refactor.txt is a different file from refactor.md")
	}
}

// Backlinks honour the global limit even when many files reference
// the target. Without the limit, a heavily-referenced doc would
// flood the operator's view; the cap keeps the call bounded.
func TestFindBacklinksRespectsLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.md"), []byte("# target\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	for i := 0; i < 5; i++ {
		name := filepath.Join(root, "ref-"+string(rune('a'+i))+".md")
		body := "links to [t](target.md)"
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	hits, err := FindBacklinks(root, "", "target.md", 3)
	if err != nil {
		t.Fatalf("FindBacklinks: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits = %d, want 3 (limit cap)", len(hits))
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// broke after the first match). The architect's P2 list called this
// out: skimming long notes for a recurring term would only surface
// the first occurrence, which made vault search useless for "where
// is X discussed in this doc?". This test pins the corrected
// behaviour: every matching line is returned, capped only by the
// global limit, and ordering follows scan order within a file.
func TestSearchTextReturnsAllMatchesInFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.md")
	body := "" +
		"intro line\n" +
		"alpha keyword first\n" +
		"unrelated\n" +
		"alpha keyword second\n" +
		"alpha keyword third\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	hits, err := SearchText(root, "", "keyword", 10)
	if err != nil {
		t.Fatalf("SearchText: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits = %d, want 3 (one per matching line)", len(hits))
	}
	wantLines := []int{2, 4, 5}
	for i, want := range wantLines {
		if hits[i].Line != want {
			t.Fatalf("hits[%d].Line = %d, want %d", i, hits[i].Line, want)
		}
		if hits[i].Path != "notes.md" {
			t.Fatalf("hits[%d].Path = %q, want notes.md", i, hits[i].Path)
		}
	}
}

// The global limit must still trim results even when a single file
// holds more matches than the cap allows. Walking continues across
// files until we hit the limit, then short-circuits via fs.SkipAll.
func TestSearchTextRespectsGlobalLimitAcrossFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.md", "b.md"} {
		body := "alpha hit\nalpha hit\nalpha hit\n"
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	hits, err := SearchText(root, "", "alpha", 4)
	if err != nil {
		t.Fatalf("SearchText: %v", err)
	}
	if len(hits) != 4 {
		t.Fatalf("hits = %d, want 4 (limit cap)", len(hits))
	}
}
