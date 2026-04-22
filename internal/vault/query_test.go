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
