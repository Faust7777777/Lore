package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicWritesAndHashes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "note.md")

	hash, err := WriteFileAtomic(path, []byte("hello"), ".tmp")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	data, readHash, err := ReadFileWithHash(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected content: %q", string(data))
	}
	if hash != readHash {
		t.Fatalf("hash mismatch: write %q read %q", hash, readHash)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file still exists")
	}
}
