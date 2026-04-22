package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
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

func WriteFileAtomic(path string, data []byte, tempSuffix string) (string, error) {
	if tempSuffix == "" {
		tempSuffix = ".tmp"
	}
	if err := EnsureParentDir(path); err != nil {
		return "", err
	}

	tempPath := path + tempSuffix
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	return ComputeSHA256(data), nil
}
