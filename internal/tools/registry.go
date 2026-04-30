// Package tools defines the shared Tool interface, Surface bitmask, and
// Registry used by Lore's MCP server, the local operator agent's tool
// runtime, and any future surfaces that need a uniform tool catalogue.
//
// Design boundaries:
//
//   - Harness-only tools (read-only context, proposal intake) live in this
//     package and depend only on a Harness handle. They are exposed on both
//     MCP and Console surfaces.
//
//   - Session-aware tools (draft actions, low-governance vault writes,
//     local-exec) belong to packages that own the relevant state, for
//     example internal/console/consoletools, and register themselves into a
//     Registry instance built per session. Putting them in their own
//     package keeps this package free of any console.Session dependency
//     and makes it impossible for the MCP server to surface them by accident
//     (MCP only ever filters by SurfaceMCP).
//
//   - This package never imports operatoragent, mcp, or console. It is a
//     leaf package so that all higher-level surfaces can depend on it.
//
// The Registry is intentionally not goroutine-safe: registration happens
// during runtime construction, before any tool dispatch begins. Lookups
// during dispatch are read-only and safe to share across goroutines once
// registration is complete.
package tools

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Surface is a bitmask describing which dispatch surfaces a tool is exposed
// on. A tool may live on multiple surfaces; for example managed_status is
// available through both SurfaceMCP (external callers via JSON-RPC) and
// SurfaceConsole (the local operator agent). draft_supersede on the other
// hand is SurfaceConsole only because the governance design forbids exposing
// draft mutation through MCP.
type Surface uint8

const (
	// SurfaceMCP marks the tool as reachable through the MCP server's
	// tools/list and tools/call endpoints.
	SurfaceMCP Surface = 1 << iota

	// SurfaceConsole marks the tool as reachable through the local operator
	// agent's ToolRuntime, which drives the console and TUI surfaces.
	SurfaceConsole

	// SurfaceProposal annotates a tool as belonging to the proposal-intake
	// category. It is informational only (used for documentation,
	// observability, and future filtering); it does not gate dispatch by
	// itself. SurfaceProposal is always combined with one or more dispatch
	// surfaces, typically SurfaceMCP|SurfaceConsole|SurfaceProposal.
	SurfaceProposal

	// SurfaceLocalExec annotates a tool as a local-execution side-effect
	// tool that touches the developer's working directory outside the
	// managed vault, for example git_status or shell_exec. It is always
	// combined with SurfaceConsole; the gate that decides whether the tool
	// is actually surfaced lives in the calling package, not here.
	SurfaceLocalExec
)

// Has reports whether s contains every bit in flag. A zero flag is treated
// as "no requirement" and always returns true.
func (s Surface) Has(flag Surface) bool {
	if flag == 0 {
		return true
	}
	return s&flag == flag
}

// ArgumentType is the JSON schema primitive type used in MCP tools/list
// definitions. Lore tools are deliberately limited to primitive types today;
// no nested objects or arrays appear in the live schema.
type ArgumentType string

const (
	ArgString  ArgumentType = "string"
	ArgInteger ArgumentType = "integer"
	ArgNumber  ArgumentType = "number"
	ArgBoolean ArgumentType = "boolean"
)

// Argument describes one input parameter of a tool. It mirrors the existing
// internal/mcp/tool_contract.go shape so the registry can produce
// byte-equivalent JSON schemas during the migration.
type Argument struct {
	// Name is the JSON property name as seen by callers.
	Name string

	// Type is the JSON schema primitive.
	Type ArgumentType

	// Description, if non-empty, becomes the schema "description" field.
	// When DeprecatedAlias is also set, Description takes precedence.
	Description string

	// Enum, if non-empty, restricts the value to one of the listed strings.
	// Only meaningful for ArgString today.
	Enum []string

	// DeprecatedAlias, if non-empty, marks this argument as a deprecated
	// alias for another standard argument. The schema description is
	// auto-generated as "deprecated alias for <name>" when Description is
	// not set explicitly. This preserves the live MCP behavior for fields
	// such as the "path" alias on vault_list/vault_search_text.
	DeprecatedAlias string
}

// Tool is the contract every Lore tool implements. Implementations should
// be value types or small handles so a Registry can hold them by interface
// without indirection.
type Tool interface {
	// Name is the dispatch key. Must be unique within a Registry and stable
	// across releases (it appears in MCP tools/list output and trace logs).
	Name() string

	// Description is the human-readable summary shown in tools/list and
	// in the operator agent system prompt. Should describe behavior and
	// any governance constraints in a single sentence.
	Description() string

	// Surfaces returns the bitmask of surfaces the tool is exposed on.
	// A Registry filters by surface when building per-surface catalogues.
	Surfaces() Surface

	// Arguments returns the input parameter schema in declaration order.
	// Order is preserved when emitting MCP definitions to keep tools/list
	// output stable for snapshot tests and callers that index by position.
	Arguments() []Argument

	// Required returns the names of arguments that must be present in a
	// call. Names must reference Arguments() entries; the registry does
	// not validate this today but will in a follow-up.
	Required() []string

	// Call dispatches the tool. The returned value is the structured
	// result; surfaces (MCP, console) handle serialization or rendering.
	// Tool implementations are responsible for argument validation and
	// for translating into harness or session calls.
	Call(args map[string]any) (any, error)
}

// Registry is an in-memory map of tool name to Tool. It is constructed
// per-runtime (one for the MCP server, one per console session) so that
// session-scoped tools can hold closures over their session without
// affecting other surfaces.
//
// Registration is expected to happen during construction; lookups during
// dispatch are read-only. Registry is not goroutine-safe across the
// registration boundary; concurrent registration is not supported.
type Registry struct {
	mu    sync.RWMutex
	byKey map[string]Tool
	order []string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		byKey: make(map[string]Tool),
		order: make([]string, 0, 16),
	}
}

// Register adds tool to r. It returns an error if a tool with the same
// Name() is already registered, or if Name() is empty after trimming.
//
// Registration order is preserved; ListBySurface and All return tools in
// registration order to keep MCP tools/list output stable for snapshot
// tests.
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("tools: cannot register nil tool")
	}
	name := strings.TrimSpace(tool.Name())
	if name == "" {
		return fmt.Errorf("tools: tool name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byKey[name]; exists {
		return fmt.Errorf("tools: duplicate registration for %q", name)
	}
	r.byKey[name] = tool
	r.order = append(r.order, name)
	return nil
}

// MustRegister is the panicking variant of Register. Intended for
// initialization code where a duplicate registration is a programmer error.
func (r *Registry) MustRegister(tool Tool) {
	if err := r.Register(tool); err != nil {
		panic(err)
	}
}

// Get returns the tool with the given name and a true ok flag, or zero
// values if no such tool is registered.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.byKey[strings.TrimSpace(name)]
	return tool, ok
}

// All returns every registered tool in registration order.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byKey[name])
	}
	return out
}

// ListBySurface returns every tool whose Surfaces() bitmask includes every
// bit in surface. Order is registration order. A zero surface is treated
// as "no requirement" and matches all tools.
func (r *Registry) ListBySurface(surface Surface) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		tool := r.byKey[name]
		if tool.Surfaces().Has(surface) {
			out = append(out, tool)
		}
	}
	return out
}

// Names returns every registered tool name in lexicographic order. Useful
// for diagnostics and tests; do not rely on this for tools/list output
// (use ListBySurface(SurfaceMCP) for stable registration order).
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
