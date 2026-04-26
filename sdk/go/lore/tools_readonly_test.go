package lore

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestReadOnlyMethodsUseStandardToolArguments(t *testing.T) {
	tests := []struct {
		name     string
		call     func(context.Context, *Client) error
		wantTool string
		wantArgs map[string]any
		result   string
	}{
		{
			name:     "managed status",
			call:     func(ctx context.Context, c *Client) error { _, err := c.ManagedStatus(ctx); return err },
			wantTool: "managed_status",
			wantArgs: map[string]any{},
			result:   `{"ready":true}`,
		},
		{
			name: "system doc get",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.SystemDocGet(ctx, SystemDocGetRequest{Name: "persona"})
				return err
			},
			wantTool: "system_doc_get",
			wantArgs: map[string]any{"name": "persona"},
			result:   `{"path":"persona.md","doc_class":"persona","content":"profile"}`,
		},
		{
			name: "vault read",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.VaultRead(ctx, VaultReadRequest{Path: "notes/a.md"})
				return err
			},
			wantTool: "vault_read",
			wantArgs: map[string]any{"path": "notes/a.md"},
			result:   `{"path":"notes/a.md","doc_class":"note","content":"A"}`,
		},
		{
			name: "vault list",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.VaultList(ctx, VaultListRequest{Dir: "notes"})
				return err
			},
			wantTool: "vault_list",
			wantArgs: map[string]any{"dir": "notes"},
			result:   `[{"path":"notes/a.md","name":"a.md","kind":"file","doc_class":"note"}]`,
		},
		{
			name: "vault search text",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.VaultSearchText(ctx, VaultSearchTextRequest{Query: "SQL", Dir: "notes", Limit: 7})
				return err
			},
			wantTool: "vault_search_text",
			wantArgs: map[string]any{"query": "SQL", "dir": "notes", "limit": float64(7)},
			result:   `[{"path":"notes/a.md","line":1,"preview":"SQL","doc_class":"note"}]`,
		},
		{
			name: "vault resolve",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.VaultResolve(ctx, VaultResolveRequest{Query: "note", Dir: "notes", Limit: 5})
				return err
			},
			wantTool: "vault_resolve",
			wantArgs: map[string]any{"query": "note", "dir": "notes", "limit": float64(5)},
			result:   `{"query":"note","status":"unique","selected_path":"notes/a.md"}`,
		},
		{
			name: "vault backlinks",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.VaultBacklinks(ctx, VaultBacklinksRequest{Path: "notes/a.md", Limit: 4})
				return err
			},
			wantTool: "vault_backlinks",
			wantArgs: map[string]any{"path": "notes/a.md", "limit": float64(4)},
			result:   `[{"path":"notes/b.md","line":2,"preview":"[[a]]"}]`,
		},
		{
			name: "context pack",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ContextPack(ctx, ContextPackRequest{TargetPath: "notes/a.md", Task: "read", Limit: 6})
				return err
			},
			wantTool: "context_pack",
			wantArgs: map[string]any{"target_path": "notes/a.md", "task": "read", "limit": float64(6)},
			result:   `{"target_path":"notes/a.md","managed_status":{"ready":true}}`,
		},
		{
			name: "doc classify",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.DocClassify(ctx, DocClassifyRequest{Path: "notes/a.md"})
				return err
			},
			wantTool: "doc_classify",
			wantArgs: map[string]any{"path": "notes/a.md"},
			result:   `{"path":"notes/a.md","doc_class":"note"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, recorder := newSingleResponseClient(t, tt.result)
			if err := tt.call(context.Background(), client); err != nil {
				t.Fatalf("call() error = %v", err)
			}
			name, args := decodeToolCallRequest(t, recorder.String())
			if name != tt.wantTool {
				t.Fatalf("tool name = %q, want %q", name, tt.wantTool)
			}
			for key, want := range tt.wantArgs {
				if got := args[key]; got != want {
					t.Fatalf("arg %s = %#v, want %#v in %#v", key, got, want, args)
				}
			}
			if _, ok := args["path"]; ok && (tt.wantTool == "vault_list" || tt.wantTool == "context_pack" || tt.wantTool == "vault_search_text" || tt.wantTool == "vault_resolve") {
				t.Fatalf("%s sent deprecated path alias in args %#v", tt.wantTool, args)
			}
		})
	}
}

func newSingleResponseClient(t *testing.T, structuredContent string) (*Client, *recordingWriteCloser) {
	t.Helper()
	response := `{"jsonrpc":"2.0","id":2,"result":{"structuredContent":` + structuredContent + `}}`
	recorder := &recordingWriteCloser{}
	transport := &stdioTransport{
		stdin:  recorder,
		stdout: bufio.NewReader(strings.NewReader(frame(response))),
	}
	transport.nextID.Store(1)
	return &Client{transport: transport}, recorder
}

func decodeToolCallRequest(t *testing.T, written string) (string, map[string]any) {
	t.Helper()
	_, body, ok := strings.Cut(written, "\r\n\r\n")
	if !ok {
		t.Fatalf("request frame missing body: %q", written)
	}
	var envelope struct {
		Method string `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(request) error = %v", err)
	}
	if envelope.Method != "tools/call" {
		t.Fatalf("method = %q, want tools/call", envelope.Method)
	}
	return envelope.Params.Name, envelope.Params.Arguments
}

type recordingWriteCloser struct {
	strings.Builder
}

func (w *recordingWriteCloser) Close() error { return nil }

func (w *recordingWriteCloser) Write(p []byte) (int, error) {
	return io.WriteString(&w.Builder, string(p))
}
