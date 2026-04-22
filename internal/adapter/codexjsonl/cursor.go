package codexjsonl

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const sourceHeadLimit int64 = 4096

type Cursor struct {
	Version         int       `json:"v,omitempty"`
	Fingerprint     string    `json:"fingerprint,omitempty"`
	Offset          int64     `json:"offset,omitempty"`
	ReplayOffset    int64     `json:"replay_offset,omitempty"`
	LastWindowStart time.Time `json:"last_window_start,omitempty"`
	LastEventAt     time.Time `json:"last_event_at,omitempty"`
	FileSize        int64     `json:"file_size,omitempty"`
	FileModTimeNS   int64     `json:"file_mtime_ns,omitempty"`
	FileHeadSHA256  string    `json:"file_head_sha256,omitempty"`
	AgentID         string    `json:"agent_id,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
}

type SourceState struct {
	Path        string
	Size        int64
	ModTimeNS   int64
	HeadSHA256  string
	Fingerprint string
}

func DecodeCursor(raw string) (Cursor, error) {
	if raw == "" {
		return Cursor{}, nil
	}
	if raw[0] != '{' {
		return Cursor{Version: 0, Fingerprint: raw}, nil
	}
	var cursor Cursor
	if err := json.Unmarshal([]byte(raw), &cursor); err != nil {
		return Cursor{}, err
	}
	return cursor, nil
}

func EncodeCursor(cursor Cursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FileFingerprint(info os.FileInfo) string {
	return "size=" + itoa64(info.Size()) + "|mtime=" + itoa64(info.ModTime().UTC().UnixNano())
}

func itoa64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func ReadSourceState(path string) (SourceState, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return SourceState{}, err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return SourceState{}, err
	}
	headSHA, err := headSHA256(absolutePath, sourceHeadLimit)
	if err != nil {
		return SourceState{}, err
	}
	return SourceState{
		Path:        absolutePath,
		Size:        info.Size(),
		ModTimeNS:   info.ModTime().UTC().UnixNano(),
		HeadSHA256:  headSHA,
		Fingerprint: FileFingerprint(info),
	}, nil
}

func (c Cursor) MatchesSource(source SourceState) bool {
	if c.Offset == 0 && c.FileSize == 0 && c.FileModTimeNS == 0 && c.FileHeadSHA256 == "" {
		return c.Fingerprint != "" && c.Fingerprint == source.Fingerprint
	}
	if c.FileSize > source.Size {
		return false
	}
	if c.FileHeadSHA256 != "" {
		headSHA := source.HeadSHA256
		if c.FileSize > 0 && c.FileSize < sourceHeadLimit && source.Size > c.FileSize {
			prefixSHA, err := headSHA256(source.Path, c.FileSize)
			if err != nil {
				return false
			}
			headSHA = prefixSHA
		}
		if c.FileHeadSHA256 != headSHA {
			return false
		}
	}
	return true
}

func headSHA256(path string, limit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if limit <= 0 {
		_, err = io.Copy(hasher, file)
	} else {
		_, err = io.CopyN(hasher, file, limit)
		if err == io.EOF {
			err = nil
		}
	}
	if err != nil {
		return "", err
	}
	return stringHex(hasher.Sum(nil)), nil
}

func stringHex(bytes []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(bytes)*2)
	for i, b := range bytes {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out)
}
