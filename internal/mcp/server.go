package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"obsidian-harness/internal/orchestrator"
)

const protocolVersion = "2024-11-05"

type Server struct {
	harness *orchestrator.Harness
	version string
}

type requestEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type responseEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewServer(harness *orchestrator.Harness, version string) *Server {
	return &Server{
		harness: harness,
		version: version,
	}
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	if err := validateProcessAuth(); err != nil {
		return err
	}

	reader := bufio.NewReader(in)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		payload, err := readMessage(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req requestEnvelope
		if err := json.Unmarshal(payload, &req); err != nil {
			if err := writeResponse(out, responseEnvelope{
				JSONRPC: "2.0",
				Error:   &responseError{Code: -32700, Message: "parse error"},
			}); err != nil {
				return err
			}
			continue
		}

		if req.Method == "notifications/initialized" {
			continue
		}

		result, rpcErr := s.handle(ctx, req.Method, req.Params)
		response := responseEnvelope{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}
		if rpcErr != nil {
			response.Result = nil
			response.Error = rpcErr
		}
		if err := writeResponse(out, response); err != nil {
			return err
		}
	}
}

func (s *Server) handle(_ context.Context, method string, rawParams json.RawMessage) (any, *responseError) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{
					"listChanged": false,
				},
			},
			"serverInfo": map[string]any{
				"name":    "lore",
				"version": s.version,
			},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		var callParams struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(paramsOrObject(rawParams), &callParams); err != nil {
			return nil, &responseError{Code: -32602, Message: "invalid tool call params"}
		}
		result, err := s.callTool(callParams.Name, callParams.Arguments)
		if err != nil {
			return map[string]any{
				"isError": true,
				"content": []map[string]string{
					{"type": "text", "text": err.Error()},
				},
			}, nil
		}
		jsonResult, marshalErr := json.MarshalIndent(result, "", "  ")
		if marshalErr != nil {
			return nil, &responseError{Code: -32603, Message: marshalErr.Error()}
		}
		return map[string]any{
			"isError": false,
			"content": []map[string]string{
				{"type": "text", "text": string(jsonResult)},
			},
			"structuredContent": result,
		}, nil
	default:
		return nil, &responseError{Code: -32601, Message: "method not found"}
	}
}

func (s *Server) callTool(name string, args map[string]any) (any, error) {
	switch name {
	case "managed_status":
		return s.harness.ManagedStatus()
	case "system_doc_get":
		return s.harness.SystemDocGet(getString(args, "name"))
	case "vault_read":
		return s.harness.VaultRead(getString(args, "path"))
	case "vault_list":
		return s.harness.VaultList(getString(args, "path"))
	case "vault_search_text":
		return s.harness.VaultSearchText(getString(args, "query"), getDirArg(args), getInt(args, "limit", 10))
	case "vault_resolve":
		return s.harness.VaultResolve(getString(args, "query"), getDirArg(args), getInt(args, "limit", 5))
	case "vault_backlinks":
		return s.harness.VaultBacklinks(getString(args, "path"), getInt(args, "limit", 10))
	case "doc_classify":
		return s.harness.DocClassify(getString(args, "path")), nil
	case "context_pack":
		return s.harness.ContextPack(getString(args, "path"), getString(args, "task"), getInt(args, "limit", 6))
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func toolDefinitions() []map[string]any {
	return []map[string]any{
		toolDefinition("managed_status", "Return managed mode and core document status.", map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		toolDefinition("system_doc_get", "Read one of the managed core documents.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "enum": []string{"system", "progress", "persona", "agent", "identity"}},
			},
			"required": []string{"name"},
		}),
		toolDefinition("vault_read", "Read a markdown document from the vault.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		}),
		toolDefinition("vault_list", "List files under a vault directory.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		}),
		toolDefinition("vault_search_text", "Search vault text content.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"dir":   map[string]any{"type": "string"},
				"path":  map[string]any{"type": "string", "description": "deprecated alias for dir"},
				"limit": map[string]any{"type": "integer"},
			},
			"required": []string{"query"},
		}),
		toolDefinition("vault_resolve", "Resolve a natural-language note reference to vault markdown paths. Returns unique, ambiguous, or not_found.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"dir":   map[string]any{"type": "string"},
				"path":  map[string]any{"type": "string", "description": "deprecated alias for dir"},
				"limit": map[string]any{"type": "integer"},
			},
			"required": []string{"query"},
		}), toolDefinition("vault_backlinks", "Find backlinks to a vault document.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			},
			"required": []string{"path"},
		}),
		toolDefinition("doc_classify", "Classify a vault document by the current rules.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		}),
		toolDefinition("context_pack", "Assemble a read-only context pack for a task or document.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string"},
				"task":  map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			},
		}),
	}
}

func toolDefinition(name string, description string, schema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": schema,
	}
}

func validateProcessAuth() error {
	expected := firstNonEmptyEnv("LORE_MCP_API_KEY", "OBSIDIAN_HARNESS_MCP_API_KEY")
	if expected == "" {
		return nil
	}
	provided := firstNonEmptyEnv("LORE_CLIENT_KEY", "OBSIDIAN_HARNESS_CLIENT_KEY")
	if provided == expected {
		return nil
	}
	return fmt.Errorf("mcp authentication failed: client key mismatch")
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func readMessage(reader *bufio.Reader) ([]byte, error) {
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
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			_, value, found := strings.Cut(line, ":")
			if !found {
				return nil, io.ErrUnexpectedEOF
			}
			value = strings.TrimSpace(value)
			length, err := strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
			contentLength = length
		}
	}
	if contentLength < 0 {
		return nil, io.ErrUnexpectedEOF
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeResponse(writer io.Writer, response responseEnvelope) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}

func getString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, _ := args[key]
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func getDirArg(args map[string]any) string {
	if dir := getString(args, "dir"); dir != "" {
		return dir
	}
	return getString(args, "path")
}

func getInt(args map[string]any, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func paramsOrObject(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}
