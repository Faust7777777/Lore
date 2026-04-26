package lore

import (
	"errors"
	"fmt"
)

var ErrClosed = errors.New("lore: client closed")

func As(err error, target any) bool {
	return errors.As(err, target)
}

type TransportError struct {
	Op  string
	Err error
}

func (e *TransportError) Error() string {
	if e == nil {
		return "lore transport error"
	}
	if e.Op == "" {
		return "lore transport error: " + e.Err.Error()
	}
	return "lore transport " + e.Op + ": " + e.Err.Error()
}

func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *JSONRPCError) Error() string {
	if e == nil {
		return "lore jsonrpc error"
	}
	return fmt.Sprintf("lore jsonrpc error %d: %s", e.Code, e.Message)
}

type ToolError struct {
	Tool    string
	Message string
}

func (e *ToolError) Error() string {
	if e == nil {
		return "lore tool error"
	}
	if e.Tool == "" {
		return "lore tool error: " + e.Message
	}
	return "lore tool " + e.Tool + ": " + e.Message
}

type DecodeError struct {
	Op  string
	Err error
}

func (e *DecodeError) Error() string {
	if e == nil {
		return "lore decode error"
	}
	if e.Op == "" {
		return "lore decode error: " + e.Err.Error()
	}
	return "lore decode " + e.Op + ": " + e.Err.Error()
}

func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
