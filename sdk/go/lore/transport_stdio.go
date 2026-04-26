package lore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type stdioTransport struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	stdoutCloser io.Closer
	mu           sync.Mutex
	nextID       atomic.Int64
	closed       bool
}

type requestEnvelope struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type responseEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

func startStdioTransport(ctx context.Context, opts Options) (*stdioTransport, error) {
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		command = "lore"
	}
	args := []string{"mcp"}
	if strings.TrimSpace(opts.WorkDir) != "" {
		args = append(args, opts.WorkDir)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ()
	if opts.ClientKey != "" {
		cmd.Env = append(cmd.Env, "LORE_CLIENT_KEY="+opts.ClientKey)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, &TransportError{Op: "stdin", Err: err}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, &TransportError{Op: "stdout", Err: err}
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, &TransportError{Op: "start", Err: err}
	}
	transport := &stdioTransport{
		cmd:          cmd,
		stdin:        stdin,
		stdout:       bufio.NewReader(stdout),
		stdoutCloser: stdout,
	}
	transport.nextID.Store(1)
	return transport, nil
}

func (t *stdioTransport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if t == nil {
		return nil, &TransportError{Op: "call", Err: ErrClosed}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, &TransportError{Op: "call", Err: ErrClosed}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := t.nextID.Add(1)
	request := requestEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, &DecodeError{Op: "request", Err: err}
	}
	if err := t.writeFrame(payload); err != nil {
		return nil, err
	}
	response, err := t.readResponse(ctx)
	if err != nil {
		if ctx.Err() != nil {
			t.closed = true
		}
		return nil, err
	}
	if response.ID != 0 && response.ID != id {
		return nil, &TransportError{Op: "response", Err: fmt.Errorf("unexpected response id %d, want %d", response.ID, id)}
	}
	if response.Error != nil {
		return nil, response.Error
	}
	return response.Result, nil
}

func (t *stdioTransport) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	stdin := t.stdin
	stdoutCloser := t.stdoutCloser
	cmd := t.cmd
	t.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if stdoutCloser != nil {
		_ = stdoutCloser.Close()
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
		return &TransportError{Op: "kill", Err: err}
	}
	_, _ = cmd.Process.Wait()
	return nil
}

func (t *stdioTransport) writeFrame(payload []byte) error {
	if _, err := fmt.Fprintf(t.stdin, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return &TransportError{Op: "write-header", Err: err}
	}
	if _, err := t.stdin.Write(payload); err != nil {
		return &TransportError{Op: "write-body", Err: err}
	}
	return nil
}

func (t *stdioTransport) readResponse(ctx context.Context) (responseEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return responseEnvelope{}, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = t.closeAfterCanceledCall()
		case <-done:
		}
	}()
	response, err := readResponseFrame(t.stdout)
	close(done)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return responseEnvelope{}, ctxErr
	}
	if err != nil {
		return responseEnvelope{}, err
	}
	return response, nil
}

func (t *stdioTransport) closeAfterCanceledCall() error {
	if t == nil {
		return nil
	}
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.stdoutCloser != nil {
		_ = t.stdoutCloser.Close()
	}
	if t.cmd == nil || t.cmd.Process == nil {
		return nil
	}
	_ = t.cmd.Process.Kill()
	_, _ = t.cmd.Process.Wait()
	return nil
}

func readResponseFrame(reader *bufio.Reader) (responseEnvelope, error) {
	payload, err := readFrame(reader)
	if err != nil {
		return responseEnvelope{}, &TransportError{Op: "read", Err: err}
	}
	var response responseEnvelope
	if err := json.Unmarshal(payload, &response); err != nil {
		return responseEnvelope{}, &DecodeError{Op: "response", Err: err}
	}
	return response, nil
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "content-length") {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, err
		}
		contentLength = parsed
	}
	if contentLength < 0 {
		return nil, io.ErrUnexpectedEOF
	}
	payload := make([]byte, contentLength)
	_, err := io.ReadFull(reader, payload)
	return payload, err
}
