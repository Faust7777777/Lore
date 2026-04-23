package codexappserver

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestClientInitializeThenListThreads(t *testing.T) {
	client, serverDone := newScriptedClient(t, func(t *testing.T, rw net.Conn) {
		reader := bufio.NewReader(rw)
		writer := bufio.NewWriter(rw)

		req := readEnvelopeForTest(t, reader)
		if req.Method != "initialize" {
			t.Fatalf("initialize method = %q, want initialize", req.Method)
		}
		var params initializeParams
		decodeRawForTest(t, req.Params, &params)
		if params.ClientInfo.Name != "lore-test" {
			t.Fatalf("clientInfo.name = %q, want lore-test", params.ClientInfo.Name)
		}
		writeEnvelopeForTest(t, writer, rpcEnvelope{
			ID:     req.ID,
			Result: mustMarshalRaw(map[string]any{"serverInfo": map[string]any{"name": "codex"}}),
		})

		initialized := readEnvelopeForTest(t, reader)
		if initialized.Method != "initialized" || initialized.ID != nil {
			t.Fatalf("initialized envelope = %+v, want initialized notification", initialized)
		}

		listReq := readEnvelopeForTest(t, reader)
		if listReq.Method != "thread/list" {
			t.Fatalf("list method = %q, want thread/list", listReq.Method)
		}
		var listParams ListThreadsParams
		decodeRawForTest(t, listReq.Params, &listParams)
		if listParams.Cwd != "C:/repo" || listParams.Limit != 5 {
			t.Fatalf("list params = %+v", listParams)
		}
		writeEnvelopeForTest(t, writer, rpcEnvelope{
			ID: listReq.ID,
			Result: mustMarshalRaw(ListThreadsResult{
				Data: []Thread{{
					ID:        "thread-1",
					Preview:   "latest task",
					Ephemeral: true,
					Status:    ThreadStatus{Type: "idle"},
				}},
				NextCursor: "next",
			}),
		})
	})
	defer serverDone()
	defer client.Close()

	result, err := client.ListThreads(ListThreadsParams{
		Cwd:   "C:/repo",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("ListThreads() error = %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].ID != "thread-1" || result.NextCursor != "next" {
		t.Fatalf("ListThreads() result = %+v", result)
	}
}

func TestClientReadThreadDecodesTurns(t *testing.T) {
	client, serverDone := newScriptedClient(t, func(t *testing.T, rw net.Conn) {
		reader := bufio.NewReader(rw)
		writer := bufio.NewWriter(rw)
		respondInitializeForTest(t, reader, writer)

		req := readEnvelopeForTest(t, reader)
		if req.Method != "thread/read" {
			t.Fatalf("method = %q, want thread/read", req.Method)
		}
		var params ReadThreadParams
		decodeRawForTest(t, req.Params, &params)
		if params.ThreadID != "thread-42" || !params.IncludeTurns {
			t.Fatalf("params = %+v", params)
		}
		writeEnvelopeForTest(t, writer, rpcEnvelope{
			ID: req.ID,
			Result: mustMarshalRaw(readThreadResult{
				Thread: Thread{
					ID: "thread-42",
					Turns: []Turn{{
						ID:        "turn-1",
						Status:    "completed",
						CreatedAt: 1710000000,
						Items: []Item{{
							Type: "user_message",
							Text: "inspect the repo",
						}},
					}},
				},
			}),
		})
	})
	defer serverDone()
	defer client.Close()

	thread, err := client.ReadThread(ReadThreadParams{
		ThreadID:     "thread-42",
		IncludeTurns: true,
	})
	if err != nil {
		t.Fatalf("ReadThread() error = %v", err)
	}
	if thread.ID != "thread-42" || len(thread.Turns) != 1 || thread.Turns[0].ID != "turn-1" {
		t.Fatalf("ReadThread() thread = %+v", thread)
	}
}

func TestClientListThreadTurns(t *testing.T) {
	client, serverDone := newScriptedClient(t, func(t *testing.T, rw net.Conn) {
		reader := bufio.NewReader(rw)
		writer := bufio.NewWriter(rw)
		respondInitializeForTest(t, reader, writer)

		req := readEnvelopeForTest(t, reader)
		if req.Method != "thread/turns/list" {
			t.Fatalf("method = %q, want thread/turns/list", req.Method)
		}
		var params ListThreadTurnsParams
		decodeRawForTest(t, req.Params, &params)
		if params.ThreadID != "thread-7" || params.SortDirection != "asc" {
			t.Fatalf("params = %+v", params)
		}
		writeEnvelopeForTest(t, writer, rpcEnvelope{
			ID: req.ID,
			Result: mustMarshalRaw(ListThreadTurnsResult{
				Data: []Turn{{
					ID:        "turn-a",
					Status:    "completed",
					CreatedAt: 1710000001,
				}},
			}),
		})
	})
	defer serverDone()
	defer client.Close()

	result, err := client.ListThreadTurns(ListThreadTurnsParams{
		ThreadID:      "thread-7",
		SortDirection: "asc",
	})
	if err != nil {
		t.Fatalf("ListThreadTurns() error = %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].ID != "turn-a" {
		t.Fatalf("ListThreadTurns() result = %+v", result)
	}
}

func TestClientRejectsUnexpectedServerRequestAndContinues(t *testing.T) {
	client, serverDone := newScriptedClient(t, func(t *testing.T, rw net.Conn) {
		reader := bufio.NewReader(rw)
		writer := bufio.NewWriter(rw)
		respondInitializeForTest(t, reader, writer)

		req := readEnvelopeForTest(t, reader)
		if req.Method != "thread/list" {
			t.Fatalf("method = %q, want thread/list", req.Method)
		}

		serverRequestID := int64(99)
		writeEnvelopeForTest(t, writer, rpcEnvelope{
			ID:     &serverRequestID,
			Method: "item/commandExecution/requestApproval",
			Params: mustMarshalRaw(map[string]any{"command": "git status"}),
		})

		rejection := readEnvelopeForTest(t, reader)
		if rejection.ID == nil || *rejection.ID != serverRequestID || rejection.Error == nil {
			t.Fatalf("rejection = %+v, want error response for server request", rejection)
		}
		if rejection.Error.Code != errCodeMethodNotFound || !strings.Contains(rejection.Error.Message, "unsupported server request") {
			t.Fatalf("rejection error = %+v", rejection.Error)
		}

		writeEnvelopeForTest(t, writer, rpcEnvelope{
			ID: req.ID,
			Result: mustMarshalRaw(ListThreadsResult{
				Data: []Thread{{ID: "thread-after-reject", Status: ThreadStatus{Type: "idle"}}},
			}),
		})
	})
	defer serverDone()
	defer client.Close()

	result, err := client.ListThreads(ListThreadsParams{})
	if err != nil {
		t.Fatalf("ListThreads() error = %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].ID != "thread-after-reject" {
		t.Fatalf("ListThreads() result = %+v", result)
	}
}

func newScriptedClient(t *testing.T, server func(t *testing.T, rw net.Conn)) (*Client, func()) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverConn.Close()
		server(t, serverConn)
	}()

	client, err := NewClient(clientConn, ClientInfo{
		Name:    "lore-test",
		Title:   "Lore Test",
		Version: "test",
	}, Capabilities{
		ExperimentalAPI: true,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client, func() {
		_ = client.Close()
		<-done
	}
}

func respondInitializeForTest(t *testing.T, reader *bufio.Reader, writer *bufio.Writer) {
	t.Helper()
	req := readEnvelopeForTest(t, reader)
	if req.Method != "initialize" {
		t.Fatalf("initialize method = %q, want initialize", req.Method)
	}
	writeEnvelopeForTest(t, writer, rpcEnvelope{
		ID:     req.ID,
		Result: mustMarshalRaw(map[string]any{"serverInfo": map[string]any{"name": "codex"}}),
	})

	initialized := readEnvelopeForTest(t, reader)
	if initialized.Method != "initialized" || initialized.ID != nil {
		t.Fatalf("initialized envelope = %+v, want initialized notification", initialized)
	}
}

func readEnvelopeForTest(t *testing.T, reader *bufio.Reader) rpcEnvelope {
	t.Helper()
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("ReadBytes() error = %v", err)
	}
	var envelope rpcEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v, payload = %q", err, string(line))
	}
	return envelope
}

func writeEnvelopeForTest(t *testing.T, writer *bufio.Writer, envelope rpcEnvelope) {
	t.Helper()
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if _, err := writer.Write(append(data, '\n')); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
}

func decodeRawForTest[T any](t *testing.T, raw json.RawMessage, target *T) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode raw error = %v, raw = %s", err, string(raw))
	}
}
