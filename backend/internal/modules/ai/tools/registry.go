// Package tools holds the AI tool registry (ported from otopoly-go's
// internal/modules/ai/tools, without the auto-service tools).
//
// A tool declares a Spec (name, JSON schema, required tenant permissions,
// kind) and a Run function. The agent loop offers a tool to the model only
// when the caller's organization scope grants every permission in
// Spec.Permissions. Every input is validated against Spec.InputSchema before
// Run. Write tools are not run by the loop directly: they become suggestions
// for the review queue (docs/01 §3) instead.
package tools

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

// Kind classifies a tool.
type Kind string

const (
	// KindRead tools only read data and run immediately.
	KindRead Kind = "read"
	// KindWrite tools propose a change; the loop does not run them.
	KindWrite Kind = "write"
)

// Spec describes a tool.
type Spec struct {
	Name        string
	Description string
	InputSchema map[string]any
	// Permissions are all required (tenant.* slugs).
	Permissions []string
	Kind        Kind
}

// Env is the execution environment for a tool call.
type Env struct {
	Scope      orgctx.Scope
	UserID     int64
	TaxpayerID *int64
}

// Result is the outcome of a tool call. Content goes back to the model (it
// is masked by the caller before it leaves the server).
type Result struct {
	Content string
	IsError bool
}

// Tool is an executable tool.
type Tool interface {
	Spec() Spec
	Run(ctx context.Context, env Env, input json.RawMessage) (Result, error)
}

// Registry is an ordered set of tools.
type Registry struct {
	byName map[string]Tool
}

// NewRegistry builds a registry; nil tools are skipped.
func NewRegistry(ts ...Tool) *Registry {
	r := &Registry{byName: map[string]Tool{}}
	for _, t := range ts {
		r.Register(t)
	}
	return r
}

// Register adds (or replaces) a tool.
func (r *Registry) Register(t Tool) {
	if t == nil {
		return
	}
	r.byName[t.Spec().Name] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// All returns every registered tool sorted by name (a stable order keeps the
// prompt-cache prefix byte-identical between requests).
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.byName))
	for _, t := range r.byName {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec().Name < out[j].Spec().Name })
	return out
}

// Allowed reports whether the scope may use a tool.
func Allowed(spec Spec, scope orgctx.Scope) bool {
	for _, perm := range spec.Permissions {
		if !scope.Has(perm) {
			return false
		}
	}
	return true
}

// Available returns the tools the scope may use, sorted by name.
func (r *Registry) Available(scope orgctx.Scope) []Tool {
	var out []Tool
	for _, t := range r.All() {
		if Allowed(t.Spec(), scope) {
			out = append(out, t)
		}
	}
	return out
}

// ErrorResult is a tool_result error returned to the model.
func ErrorResult(msg string) Result { return Result{Content: msg, IsError: true} }

// JSONResult marshals v as compact JSON content.
func JSONResult(v any) Result {
	raw, err := json.Marshal(v)
	if err != nil {
		return ErrorResult("failed to encode result")
	}
	return Result{Content: string(raw)}
}
