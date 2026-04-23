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
const replayAnchorLimit int64 = 4096

type Cursor struct {
	Version            int       `json:"v,omitempty"`
	Fingerprint        string    `json:"fingerprint,omitempty"`
	Offset             int64     `json:"offset,omitempty"`
	ReplayOffset       int64     `json:"replay_offset,omitempty"`
	ReplayAnchorStart  int64     `json:"replay_anchor_start,omitempty"`
	ReplayAnchorEnd    int64     `json:"replay_anchor_end,omitempty"`
	ReplayAnchorSHA256 string    `json:"replay_anchor_sha256,omitempty"`
	LastWindowStart    time.Time `json:"last_window_start,omitempty"`
	LastEventAt        time.Time `json:"last_event_at,omitempty"`
	FileSize           int64     `json:"file_size,omitempty"`
	FileModTimeNS      int64     `json:"file_mtime_ns,omitempty"`
	FileHeadSHA256     string    `json:"file_head_sha256,omitempty"`
	AgentID            string    `json:"agent_id,omitempty"`
	SessionID          string    `json:"session_id,omitempty"`
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

func (c Cursor) MatchesReplayAnchor(source SourceState) bool {
	if c.ReplayOffset <= 0 {
		return true
	}
	if c.ReplayAnchorSHA256 == "" || c.ReplayAnchorEnd != c.ReplayOffset {
		return false
	}
	if c.ReplayAnchorStart < 0 || c.ReplayAnchorStart > c.ReplayAnchorEnd {
		return false
	}
	if c.ReplayAnchorEnd > source.Size {
		return false
	}
	sha, err := spanSHA256(source.Path, c.ReplayAnchorStart, c.ReplayAnchorEnd)
	if err != nil {
		return false
	}
	return sha == c.ReplayAnchorSHA256
}

func BuildReplayAnchor(path string, replayOffset int64) (int64, int64, string, error) {
	if replayOffset <= 0 {
		return 0, 0, "", nil
	}
	start := replayOffset - replayAnchorLimit
	if start < 0 {
		start = 0
	}
	sha, err := spanSHA256(path, start, replayOffset)
	if err != nil {
		return 0, 0, "", err
	}
	return start, replayOffset, sha, nil
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

func spanSHA256(path string, start int64, end int64) (string, error) {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return "", err
	}

	hasher := sha256.New()
	length := end - start
	if length > 0 {
		_, err = io.CopyN(hasher, file, length)
		if err == io.EOF {
			err = nil
		}
		if err != nil {
			return "", err
		}
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
