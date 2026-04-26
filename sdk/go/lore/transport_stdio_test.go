package lore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestReadFrame(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(frame("{\"ok\":true}\nxx")))
	payload, err := readFrame(reader)
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}
	if string(payload) != "{\"ok\":true}\nxx" {
		t.Fatalf("payload = %q", string(payload))
	}
}

func TestReadResponseFrameReturnsJSONRPCError(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`
	reader := bufio.NewReader(strings.NewReader(frame(payload)))
	response, err := readResponseFrame(reader)
	if err != nil {
		t.Fatalf("readResponseFrame() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != -32601 {
		t.Fatalf("response error = %+v", response.Error)
	}
}

func TestCallToolReturnsToolError(t *testing.T) {
	client := &Client{transport: &stdioTransport{
		stdin:  nopWriteCloser{Writer: io.Discard},
		stdout: bufio.NewReader(strings.NewReader(frame(`{"jsonrpc":"2.0","id":2,"result":{"isError":true,"content":[{"type":"text","text":"boom"}]}}`))),
	}}
	client.transport.nextID.Store(1)

	err := client.CallTool(context.Background(), "managed_status", map[string]any{}, nil)
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("error = %T %[1]v, want ToolError", err)
	}
	if toolErr.Tool != "managed_status" || toolErr.Message != "boom" {
		t.Fatalf("tool error = %+v", toolErr)
	}
}

func TestCallToolDecodesStructuredContent(t *testing.T) {
	client := &Client{transport: &stdioTransport{
		stdin:  nopWriteCloser{Writer: io.Discard},
		stdout: bufio.NewReader(strings.NewReader(frame(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":{"ready":true}}}`))),
	}}
	client.transport.nextID.Store(1)

	var out struct {
		Ready bool `json:"ready"`
	}
	if err := client.CallTool(context.Background(), "managed_status", map[string]any{}, &out); err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !out.Ready {
		t.Fatal("Ready = false, want true")
	}
}

func TestCallToolDecodeError(t *testing.T) {
	client := &Client{transport: &stdioTransport{
		stdin:  nopWriteCloser{Writer: io.Discard},
		stdout: bufio.NewReader(strings.NewReader(frame(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":null}}`))),
	}}
	client.transport.nextID.Store(1)

	var out struct{}
	err := client.CallTool(context.Background(), "managed_status", map[string]any{}, &out)
	var decodeErr *DecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("error = %T %[1]v, want DecodeError", err)
	}
}

func frame(payload string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
}

type nopWriteCloser struct {
	io.Writer
}

func (n nopWriteCloser) Close() error { return nil }

func TestStartStdioTransportHonorsPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport, err := startStdioTransport(ctx, Options{Command: "definitely-missing-lore-binary"})
	if err == nil {
		_ = transport.Close()
		t.Fatal("startStdioTransport() error = nil, want canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestCallCanceledWhileReadingClosesTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stdout := &blockingReadCloser{ready: make(chan struct{}), unblock: make(chan struct{})}
	transport := &stdioTransport{
		stdin:        nopWriteCloser{Writer: io.Discard},
		stdout:       bufio.NewReader(stdout),
		stdoutCloser: stdout,
	}
	transport.nextID.Store(1)
	client := &Client{transport: transport}

	errCh := make(chan error, 1)
	go func() {
		_, err := client.Tools(ctx)
		errCh <- err
	}()
	<-stdout.ready
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Tools() error = %v, want context.Canceled", err)
	}
	if _, err := client.Tools(context.Background()); err == nil || !strings.Contains(err.Error(), ErrClosed.Error()) {
		t.Fatalf("second Tools() error = %v, want closed transport", err)
	}
}

type blockingReadCloser struct {
	ready   chan struct{}
	unblock chan struct{}
	once    sync.Once
}

func (r *blockingReadCloser) Read(_ []byte) (int, error) {
	r.once.Do(func() { close(r.ready) })
	<-r.unblock
	return 0, io.EOF
}

func (r *blockingReadCloser) Close() error {
	r.once.Do(func() { close(r.ready) })
	select {
	case <-r.unblock:
	default:
		close(r.unblock)
	}
	return nil
}
