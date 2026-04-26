package lore

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestErrorTypesSupportStandardMatching(t *testing.T) {
	transportErr := &TransportError{Op: "call", Err: ErrClosed}
	if !errors.Is(transportErr, ErrClosed) {
		t.Fatalf("errors.Is(TransportError, ErrClosed) = false")
	}
	var matchedTransport *TransportError
	if !errors.As(transportErr, &matchedTransport) || matchedTransport.Op != "call" {
		t.Fatalf("errors.As(TransportError) = %+v", matchedTransport)
	}

	decodeCause := &json.SyntaxError{Offset: 3}
	decodeErr := &DecodeError{Op: "response", Err: decodeCause}
	var matchedDecode *DecodeError
	if !As(decodeErr, &matchedDecode) || matchedDecode.Op != "response" {
		t.Fatalf("As(DecodeError) = %+v", matchedDecode)
	}
	if !errors.Is(decodeErr, decodeCause) {
		t.Fatalf("errors.Is(DecodeError, cause) = false")
	}

	toolErr := &ToolError{Tool: "managed_status", Message: "boom"}
	var matchedTool *ToolError
	if !As(toolErr, &matchedTool) || matchedTool.Tool != "managed_status" {
		t.Fatalf("As(ToolError) = %+v", matchedTool)
	}

	rpcErr := &JSONRPCError{Code: -32601, Message: "method not found"}
	var matchedRPC *JSONRPCError
	if !errors.As(rpcErr, &matchedRPC) || matchedRPC.Code != -32601 {
		t.Fatalf("errors.As(JSONRPCError) = %+v", matchedRPC)
	}
}
