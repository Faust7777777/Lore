package vault

import "testing"

func TestExtractAttachmentRefs(t *testing.T) {
	markdown := `
# Demo

![[assets/chart.png|240]]
[[docs/spec.pdf]]
[download](files/report.csv)
![Inline](media/photo.jpg "photo")
[site](https://example.com/demo.png)
[[note-link]]
![[assets/chart.png]]
`

	refs := ExtractAttachmentRefs(markdown)
	if len(refs) != 4 {
		t.Fatalf("len(refs) = %d, want 4", len(refs))
	}

	want := map[string]string{
		"assets/chart.png": "image",
		"docs/spec.pdf":    "pdf",
		"files/report.csv": "file",
		"media/photo.jpg":  "image",
	}

	for _, ref := range refs {
		kind, ok := want[ref.Path]
		if !ok {
			t.Fatalf("unexpected attachment path: %s", ref.Path)
		}
		if ref.Kind != kind {
			t.Fatalf("attachment %s kind = %s, want %s", ref.Path, ref.Kind, kind)
		}
	}

	if !refs[0].Embed {
		t.Fatal("expected wiki image embed to be marked as embed")
	}
}

func TestExtractAttachmentRefsIgnoresMarkdownNotes(t *testing.T) {
	markdown := `
[[Weekly Plan]]
[note](docs/weekly-plan.md)
`

	refs := ExtractAttachmentRefs(markdown)
	if len(refs) != 0 {
		t.Fatalf("len(refs) = %d, want 0", len(refs))
	}
}
