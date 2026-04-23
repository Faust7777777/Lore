package codexappserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Capabilities struct {
	ExperimentalAPI          bool     `json:"experimentalApi,omitempty"`
	OptOutNotificationMethod []string `json:"optOutNotificationMethods,omitempty"`
}

type Client struct {
	mu     sync.Mutex
	nextID int64
	reader *bufio.Reader
	writer *bufio.Writer
	rw     io.ReadWriteCloser
}

type ProcessClient struct {
	*Client
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

type ThreadStatus struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags,omitempty"`
}

type ItemContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

type Item struct {
	Type    string        `json:"type"`
	ID      string        `json:"id,omitempty"`
	Text    string        `json:"text,omitempty"`
	Phase   string        `json:"phase,omitempty"`
	Content []ItemContent `json:"content,omitempty"`
}

type TurnError struct {
	Message string `json:"message"`
}

type Turn struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	Items     []Item     `json:"items"`
	Error     *TurnError `json:"error,omitempty"`
	CreatedAt int64      `json:"createdAt,omitempty"`
	UpdatedAt int64      `json:"updatedAt,omitempty"`
}

type Thread struct {
	ID            string       `json:"id"`
	Name          string       `json:"name,omitempty"`
	Preview       string       `json:"preview,omitempty"`
	Ephemeral     bool         `json:"ephemeral"`
	ModelProvider string       `json:"modelProvider,omitempty"`
	CreatedAt     int64        `json:"createdAt,omitempty"`
	UpdatedAt     int64        `json:"updatedAt,omitempty"`
	Status        ThreadStatus `json:"status"`
	Turns         []Turn       `json:"turns,omitempty"`
}

type ListThreadsParams struct {
	Cursor         string   `json:"cursor,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	SortKey        string   `json:"sortKey,omitempty"`
	Cwd            string   `json:"cwd,omitempty"`
	SourceKinds    []string `json:"sourceKinds,omitempty"`
	SearchTerm     string   `json:"searchTerm,omitempty"`
	ModelProviders []string `json:"modelProviders,omitempty"`
	Archived       bool     `json:"archived,omitempty"`
}

type ListThreadsResult struct {
	Data            []Thread `json:"data"`
	NextCursor      string   `json:"nextCursor,omitempty"`
	BackwardsCursor string   `json:"backwardsCursor,omitempty"`
}

type ListThreadTurnsParams struct {
	ThreadID      string `json:"threadId"`
	Cursor        string `json:"cursor,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	SortDirection string `json:"sortDirection,omitempty"`
}

type ListThreadTurnsResult struct {
	Data            []Turn `json:"data"`
	NextCursor      string `json:"nextCursor,omitempty"`
	BackwardsCursor string `json:"backwardsCursor,omitempty"`
}

type ReadThreadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns bool   `json:"includeTurns,omitempty"`
}

type readThreadResult struct {
	Thread Thread `json:"thread"`
}

type initializeParams struct {
	ClientInfo   ClientInfo   `json:"clientInfo"`
	Capabilities Capabilities `json:"capabilities,omitempty"`
}

type rpcEnvelope struct {
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
	ID     *int64          `json:"id,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	errCodeMethodNotFound = -32601
)

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("codex app-server rpc error %d: %s", e.Code, e.Message)
}

func NewClient(rw io.ReadWriteCloser, info ClientInfo, caps Capabilities) (*Client, error) {
	if rw == nil {
		return nil, fmt.Errorf("codex app-server client: read-write transport is required")
	}
	client := &Client{
		reader: bufio.NewReader(rw),
		writer: bufio.NewWriter(rw),
		rw:     rw,
	}
	if err := client.initialize(info, caps); err != nil {
		_ = rw.Close()
		return nil, err
	}
	return client, nil
}

func StartProcess(ctx context.Context, workDir string, command string, args []string, info ClientInfo, caps Capabilities) (*ProcessClient, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("codex app-server process: command is required")
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	rw := &processTransport{
		reader: stdout,
		writer: stdin,
		closeFn: func() error {
			_ = stdin.Close()
			_ = stdout.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return cmd.Wait()
		},
	}

	client, err := NewClient(rw, info, caps)
	if err != nil {
		return nil, fmt.Errorf("codex app-server process: %w\nstderr:\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return &ProcessClient{
		Client: client,
		cmd:    cmd,
		stderr: &stderr,
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.rw == nil {
		return nil
	}
	return c.rw.Close()
}

func (c *ProcessClient) Close() error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Client.Close()
}

func (c *ProcessClient) Stderr() string {
	if c == nil || c.stderr == nil {
		return ""
	}
	return strings.TrimSpace(c.stderr.String())
}

func (c *Client) ListThreads(params ListThreadsParams) (ListThreadsResult, error) {
	var result ListThreadsResult
	if err := c.call("thread/list", params, &result); err != nil {
		return ListThreadsResult{}, err
	}
	return result, nil
}

func (c *Client) ReadThread(params ReadThreadParams) (Thread, error) {
	var result readThreadResult
	if err := c.call("thread/read", params, &result); err != nil {
		return Thread{}, err
	}
	return result.Thread, nil
}

func (c *Client) ListThreadTurns(params ListThreadTurnsParams) (ListThreadTurnsResult, error) {
	var result ListThreadTurnsResult
	if err := c.call("thread/turns/list", params, &result); err != nil {
		return ListThreadTurnsResult{}, err
	}
	return result, nil
}

func (c *Client) initialize(info ClientInfo, caps Capabilities) error {
	var result map[string]any
	if err := c.call("initialize", initializeParams{
		ClientInfo:   info,
		Capabilities: caps,
	}, &result); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *Client) call(method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID
	c.nextID++

	if err := c.writeEnvelope(rpcEnvelope{
		Method: method,
		Params: mustMarshalRaw(params),
		ID:     &id,
	}); err != nil {
		return err
	}

	for {
		message, err := c.readEnvelope()
		if err != nil {
			return err
		}
		if strings.TrimSpace(message.Method) != "" {
			if message.ID != nil {
				if err := c.writeEnvelope(rpcEnvelope{
					ID: message.ID,
					Error: &rpcError{
						Code:    errCodeMethodNotFound,
						Message: fmt.Sprintf("unsupported server request: %s", strings.TrimSpace(message.Method)),
					},
				}); err != nil {
					return err
				}
			}
			continue
		}
		if message.ID == nil {
			continue
		}
		if *message.ID != id {
			return fmt.Errorf("codex app-server client: unexpected response id %d while waiting for %d", *message.ID, id)
		}
		if message.Error != nil {
			return message.Error
		}
		if result == nil || len(message.Result) == 0 {
			return nil
		}
		return json.Unmarshal(message.Result, result)
	}
}

func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeEnvelope(rpcEnvelope{
		Method: method,
		Params: mustMarshalRaw(params),
	})
}

func (c *Client) writeEnvelope(envelope rpcEnvelope) error {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if _, err := c.writer.Write(append(payload, '\n')); err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *Client) readEnvelope() (rpcEnvelope, error) {
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return rpcEnvelope{}, err
	}
	var envelope rpcEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(line), &envelope); err != nil {
		return rpcEnvelope{}, err
	}
	return envelope, nil
}

func mustMarshalRaw(value any) json.RawMessage {
	if value == nil {
		return json.RawMessage([]byte("{}"))
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

type processTransport struct {
	reader  io.ReadCloser
	writer  io.WriteCloser
	closeFn func() error
}

func (t *processTransport) Read(p []byte) (int, error) {
	return t.reader.Read(p)
}

func (t *processTransport) Write(p []byte) (int, error) {
	return t.writer.Write(p)
}

func (t *processTransport) Close() error {
	if t.closeFn != nil {
		return t.closeFn()
	}
	if t.writer != nil {
		_ = t.writer.Close()
	}
	if t.reader != nil {
		return t.reader.Close()
	}
	return nil
}
