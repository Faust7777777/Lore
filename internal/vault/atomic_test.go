package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestWriteFileAtomicOverwritesExisting(t *testing.T) {
	// Overwriting an existing file is the common ApplyDraft path
	// once a draft's target already exists in the vault. The temp
	// file must use O_TRUNC so a previous crashed-run leftover
	// does not contaminate the new payload.
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if _, err := WriteFileAtomic(path, []byte("first"), ".tmp"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := WriteFileAtomic(path, []byte("second"), ".tmp"); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("content = %q, want second", string(got))
	}
}

func TestWriteFileAtomicDefaultsTempSuffix(t *testing.T) {
	// Empty tempSuffix must default to ".tmp" rather than letting
	// the temp path collide with the target.
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if _, err := WriteFileAtomic(path, []byte("body"), ""); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp leftover after default-suffix write")
	}
}

func TestWriteFileAtomicCustomTempSuffixCleanedUp(t *testing.T) {
	// A custom suffix configured via Config.Vault.TempSuffix must
	// be removed after the rename succeeds; a leftover would
	// confuse the daemon's vault scan.
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if _, err := WriteFileAtomic(path, []byte("payload"), ".obsidian-harness-tmp"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path + ".obsidian-harness-tmp"); !os.IsNotExist(err) {
		t.Fatalf(".obsidian-harness-tmp leftover")
	}
}

func TestWriteFileAtomicEmptyDataYieldsEmptyFile(t *testing.T) {
	// Empty payload is a legal write -- ApplyDraft can produce one
	// after a deletion patch. The file must exist with zero bytes.
	root := t.TempDir()
	path := filepath.Join(root, "empty.md")
	hash, err := WriteFileAtomic(path, []byte(""), ".tmp")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("size = %d, want 0", info.Size())
	}
	if hash != ComputeSHA256([]byte("")) {
		t.Fatalf("hash = %q, want sha256 of empty input", hash)
	}
}

func TestWriteFileAtomicLargePayloadIsDurable(t *testing.T) {
	// Larger payload exercises the writeFileSync hot path more
	// thoroughly than the short string cases. The architect's
	// concern about the previous os.WriteFile call was that it
	// could leak large payloads on power loss; this test does
	// not simulate power loss but at minimum proves the buffer
	// is flushed and the readback matches.
	root := t.TempDir()
	path := filepath.Join(root, "large.md")
	data := make([]byte, 1<<20) // 1 MiB
	for i := range data {
		data[i] = byte(i % 251)
	}
	hash, err := WriteFileAtomic(path, data, ".tmp")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	got, readHash, err := ReadFileWithHash(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != len(data) {
		t.Fatalf("len = %d, want %d", len(got), len(data))
	}
	if hash != readHash {
		t.Fatalf("hash drifted across write -> read for 1MiB payload")
	}
}

func TestIsCrossDeviceErrorMatchesEXDEV(t *testing.T) {
	// The cross-device fallback path in WriteFileAtomic depends
	// on isCrossDeviceError correctly recognising EXDEV wrapped
	// in *os.LinkError (the shape os.Rename returns). syscall.Errno
	// implements Is on every supported platform, so errors.Is
	// routes via that without any GOOS-specific code in atomic.go.
	wrapped := &os.LinkError{
		Op:  "rename",
		Old: "/a/old",
		New: "/b/new",
		Err: syscall.EXDEV,
	}
	if !isCrossDeviceError(wrapped) {
		t.Fatalf("isCrossDeviceError should match *os.LinkError with syscall.EXDEV; got false")
	}
	notExdev := &os.LinkError{
		Op:  "rename",
		Old: "/a/old",
		New: "/b/new",
		Err: syscall.EPERM,
	}
	if isCrossDeviceError(notExdev) {
		t.Fatalf("isCrossDeviceError should not match EPERM")
	}
	if isCrossDeviceError(errors.New("generic error")) {
		t.Fatalf("isCrossDeviceError should not match a bare error")
	}
	if isCrossDeviceError(nil) {
		t.Fatalf("isCrossDeviceError should not match nil")
	}
}

func TestWriteFileAtomicLeavesNoArtifactsOnTempWriteFailure(t *testing.T) {
	// If the writeFileSync step fails (e.g. EnsureParentDir
	// succeeded but the file open returns an error), the operator
	// should not be left with a stray .tmp pointing nowhere. We
	// drive the failure by pointing at a parent that exists as a
	// FILE instead of a directory -- creating the temp under it
	// fails with ENOTDIR / NotADirectory.
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}
	target := filepath.Join(blocker, "would-be-note.md")
	_, err := WriteFileAtomic(target, []byte("body"), ".tmp")
	if err == nil {
		t.Fatal("WriteFileAtomic should fail when the parent is a file, not a directory")
	}
	// On both POSIX and Windows the underlying error is
	// "not a directory" / "directory name is invalid". We do not
	// assert the specific message; the contract is that no
	// artifact is left under the file-blocker.
	if _, statErr := os.Stat(target + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("temp leftover after failed write: stat err = %v", statErr)
	}
	// Sanity: the blocker file itself is untouched.
	got, readErr := os.ReadFile(blocker)
	if readErr != nil {
		t.Fatalf("blocker readback: %v", readErr)
	}
	if !strings.Contains(string(got), "not a dir") {
		t.Fatalf("blocker content corrupted: %q", string(got))
	}
}
