// Package tools is the agent's tool harness. Each Tool exposes a name and
// JSON schema for the model's tool-calling, plus a Run function executed
// locally when the model requests it.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Call is one tool invocation requested by the model.
type Call struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Result is what a tool returns to the model.
type Result struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// Tool is a callable capability the model can invoke by name.
type Tool interface {
	Name() string
	Description() string
	// Parameters is a JSON-schema object describing arguments.
	Parameters() json.RawMessage
	Run(ctx context.Context, args json.RawMessage) (Result, error)
}

// Registry holds the available tools keyed by name.
type Registry struct {
	byName map[string]Tool
}

// NewRegistry builds a registry from the given tools.
func NewRegistry(ts ...Tool) *Registry {
	r := &Registry{byName: make(map[string]Tool, len(ts))}
	for _, t := range ts {
		r.byName[t.Name()] = t
	}
	return r
}

// Get returns the tool with the given name, if registered.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// Add registers one more tool.
func (r *Registry) Add(t Tool) { r.byName[t.Name()] = t }

// Names returns tool names sorted alphabetically.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.byName))
	for n := range r.byName {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Definitions renders the OpenAI-style tool descriptions sent to the model,
// in name order for stable prompts.
func (r *Registry) Definitions() []map[string]any {
	out := make([]map[string]any, 0, len(r.byName))
	for _, name := range r.Names() {
		t := r.byName[name]
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  json.RawMessage(t.Parameters()),
			},
		})
	}
	return out
}

// Execute resolves and runs a call, normalizing errors into Result.
func (r *Registry) Execute(ctx context.Context, c Call) Result {
	t, ok := r.byName[c.Name]
	if !ok {
		return Result{Content: fmt.Sprintf("unknown tool %q; available: %s", c.Name, strings.Join(r.Names(), ", ")), IsError: true}
	}
	res, err := t.Run(ctx, c.Arguments)
	if err != nil {
		return Result{Content: fmt.Sprintf("tool %s failed: %v", c.Name, err), IsError: true}
	}
	return res
}

// Approver is implemented by tools that need explicit user confirmation
// before they run.
type Approver interface {
	RequiresApproval() bool
}

// NeedsApproval reports whether the named tool must be confirmed by the
// user before running.
func (r *Registry) NeedsApproval(name string) bool {
	t, ok := r.byName[name]
	if !ok {
		return false // unknown tools fail fast in Execute
	}
	a, ok := t.(Approver)
	return ok && a.RequiresApproval()
}
