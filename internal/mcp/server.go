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
	"time"

	"obsidian-harness/internal/model"
	"obsidian-harness/internal/orchestrator"
	"obsidian-harness/internal/tools"
)

const protocolVersion = "2024-11-05"
const maxFrameContentLength = 1 * 1024 * 1024

type Server struct {
	harness  *orchestrator.Harness
	version  string
	registry *tools.Registry
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
	registry := tools.NewRegistry()
	// Read-only built-in tools migrate to the registry as part of the
	// tool-registry refactor. Proposal tools and any future MCP-visible
	// tools will join the registry in subsequent commits.
	if err := tools.RegisterReadOnly(registry, harness); err != nil {
		// RegisterReadOnly only fails on a programming error (duplicate
		// or empty tool name) which is impossible with the static
		// built-in list. Fail fast so the misconfiguration is caught
		// at server startup rather than during a tools/call dispatch.
		panic(fmt.Errorf("mcp: register read-only tools: %w", err))
	}
	return &Server{
		harness:  harness,
		version:  version,
		registry: registry,
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
		return map[string]any{"tools": s.toolListDefinitions()}, nil
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

// toolListDefinitions returns the merged tools/list definitions: built-in
// read-only tools come from the shared registry, proposal tools still
// come from the legacy toolContracts() table. Order is preserved so
// external MCP callers see the same JSON they did before the registry
// migration. Commit 3 of the tool-registry refactor will move proposal
// tools into the registry and drop the legacy fallback.
func (s *Server) toolListDefinitions() []map[string]any {
	registryDefs := tools.MCPDefinitions(s.registry.ListBySurface(tools.SurfaceMCP))
	legacyDefs := toolDefinitions()
	merged := make([]map[string]any, 0, len(registryDefs)+len(legacyDefs))
	merged = append(merged, registryDefs...)
	merged = append(merged, legacyDefs...)
	return merged
}

func (s *Server) callTool(name string, args map[string]any) (any, error) {
	if tool, ok := s.registry.Get(name); ok && tool.Surfaces().Has(tools.SurfaceMCP) {
		return tool.Call(args)
	}
	switch name {
	case "persona_update_propose":
		observedAt, err := parseObservedAt(getString(args, "observed_at"))
		if err != nil {
			return nil, err
		}
		return s.harness.ProposePersonaUpdate(model.PersonaUpdateProposal{
			Field:         getString(args, "field"),
			CurrentValue:  getString(args, "current_value"),
			ProposedValue: getString(args, "proposed_value"),
			Evidence:      getString(args, "evidence"),
			Reason:        getString(args, "reason"),
			Confidence:    getString(args, "confidence"),
			Source:        getString(args, "source"),
			ObservedAt:    observedAt,
		}, time.Now())
	case "markdown_note_propose":
		observedAt, err := parseObservedAt(getString(args, "observed_at"))
		if err != nil {
			return nil, err
		}
		return s.harness.ProposeMarkdownNote(model.MarkdownNoteProposal{
			TargetPath:  getString(args, "target_path"),
			Title:       getString(args, "title"),
			Content:     getString(args, "content"),
			SourceKind:  getString(args, "source_kind"),
			Evidence:    getString(args, "evidence"),
			Reason:      getString(args, "reason"),
			Source:      getString(args, "source"),
			ObservedAt:  observedAt,
			TaskContext: getString(args, "task_context"),
			Course:      getString(args, "course"),
			Topic:       getString(args, "topic"),
			DedupeKey:   getString(args, "dedupe_key"),
		}, time.Now())
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
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
	if contentLength > maxFrameContentLength {
		return nil, fmt.Errorf("content length exceeds %d bytes", maxFrameContentLength)
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

func getTargetPathArg(args map[string]any) string {
	if targetPath := getString(args, "target_path"); targetPath != "" {
		return targetPath
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

func parseObservedAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("observed_at is required")
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("observed_at must be RFC3339 or YYYY-MM-DD")
}

func paramsOrObject(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}
