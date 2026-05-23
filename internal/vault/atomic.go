package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func ComputeSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func EnsureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func ReadFileWithHash(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, ComputeSHA256(data), nil
}

// WriteFileAtomic writes `data` to `path` durably and atomically:
//
//  1. Write to `path + tempSuffix`, fsync the file before close so
//     the bytes hit physical storage rather than just the kernel
//     page cache. Architect's infra handover called out the
//     previous WriteFile + Rename pattern as a power-loss-window
//     gap: the rename metadata could land before the data, leaving
//     a truncated managed file on recovery.
//  2. Rename the temp into place. On most filesystems this is the
//     atomic step the operator depends on; the destination either
//     refers entirely to the old file or entirely to the new file.
//  3. If the rename fails because the temp and the destination
//     live on different devices (syscall.EXDEV; happens when the
//     workdir is bind-mounted, on a Docker volume, or when the
//     vault root and tmpdir straddle Windows drive letters), fall
//     back to copy-and-sync into a sibling, then rename. Preserves
//     atomicity for callers that depend on it (ApplyDraft,
//     WriteLowRiskNote, daemon scaffolding) without surfacing the
//     cross-device error.
//  4. Best-effort fsync the parent directory so the rename metadata
//     itself is durable. Skipped silently on platforms (notably
//     Windows) where directory sync is not supported -- the data
//     is still safe; only the rename metadata might re-replay if
//     the OS crashes immediately after this call returns.
//
// Returns the sha256 of `data` so callers can record a verifiable
// content hash in the audit trail.
func WriteFileAtomic(path string, data []byte, tempSuffix string) (string, error) {
	if tempSuffix == "" {
		tempSuffix = ".tmp"
	}
	if err := EnsureParentDir(path); err != nil {
		return "", err
	}

	tempPath := path + tempSuffix
	if err := writeFileSync(tempPath, data); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, path); err != nil {
		if isCrossDeviceError(err) {
			// Cross-device rename: copy the temp into a same-fs
			// sibling next to the destination, sync it, then
			// rename. The original temp is removed on either
			// success or failure of the fallback so a partial
			// scaffold left behind never gets confused with a
			// real backup.
			fallbackTemp := path + tempSuffix + ".xdev"
			if copyErr := copyFileSync(tempPath, fallbackTemp); copyErr != nil {
				_ = os.Remove(tempPath)
				_ = os.Remove(fallbackTemp)
				return "", copyErr
			}
			if renameErr := os.Rename(fallbackTemp, path); renameErr != nil {
				_ = os.Remove(tempPath)
				_ = os.Remove(fallbackTemp)
				return "", renameErr
			}
			_ = os.Remove(tempPath)
		} else {
			_ = os.Remove(tempPath)
			return "", err
		}
	}
	syncParentDirBestEffort(path)

	return ComputeSHA256(data), nil
}

// writeFileSync writes `data` to `path` and fsyncs the file before
// closing. Replaces the previous os.WriteFile call which only
// flushed to the kernel page cache and could lose bytes on power
// loss. Uses O_TRUNC so a previous temp left over from a crashed
// run is overwritten cleanly.
func writeFileSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, writeErr := f.Write(data); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return writeErr
	}
	if syncErr := f.Sync(); syncErr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return syncErr
	}
	return f.Close()
}

// copyFileSync streams src into dst and fsyncs dst before close.
// Used by the cross-device WriteFileAtomic fallback. Source is
// opened read-only; destination is created with the same 0o644
// mode as the writeFileSync hot path so the on-disk permissions
// are uniform regardless of which branch produced the file.
func copyFileSync(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, copyErr := io.Copy(out, in); copyErr != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return copyErr
	}
	if syncErr := out.Sync(); syncErr != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return syncErr
	}
	return out.Close()
}

// isCrossDeviceError reports whether err is the standard cross-
// device rename error. Both POSIX (EXDEV = 18) and Windows
// (ERROR_NOT_SAME_DEVICE = 17) surface via syscall.Errno, which
// implements Is so errors.Is can route a wrapped *os.LinkError
// through to the underlying errno comparison without any platform-
// specific code in this file.
func isCrossDeviceError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// syncParentDirBestEffort fsyncs the directory holding `path` so
// the rename metadata persists across a power loss. Returns no
// error: filesystems that do not support dir sync (Windows on
// most setups, some FUSE drivers) silently skip the step. The
// data is already on disk via the temp file fsync; only the
// rename's metadata is at stake here, and an unsynced rename
// just means recovery might temporarily see the old file under
// the temp name -- recoverable, not a data loss.
func syncParentDirBestEffort(path string) {
	dir := filepath.Dir(path)
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	defer f.Close()
	_ = f.Sync()
}
