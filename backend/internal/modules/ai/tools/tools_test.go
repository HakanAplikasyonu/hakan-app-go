package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

func TestValidate(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"date":   map[string]any{"type": "string", "format": "date"},
			"status": map[string]any{"type": "string", "enum": []string{"a", "b"}},
			"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
			"id":     map[string]any{"type": "string", "format": "uuid"},
			"rows":   map[string]any{"type": "array", "maxItems": 2, "items": map[string]any{"type": "object"}},
		},
		"required":             []string{"date"},
		"additionalProperties": false,
	}
	ok := []string{`{"date":"2026-09-24"}`, `{"date":"2026-09-24","status":"a","limit":5,"rows":[{},{}]}`}
	bad := []string{
		`{}`, `{"date":"24.09.2026"}`, `{"date":"2026-09-24","status":"c"}`,
		`{"date":"2026-09-24","limit":1.5}`, `{"date":"2026-09-24","limit":9}`,
		`{"date":"2026-09-24","id":"nope"}`, `{"date":"2026-09-24","x":1}`,
		`{"date":"2026-09-24","rows":[{},{},{}]}`, `not json`, `"a string"`,
	}
	for _, in := range ok {
		if err := Validate(schema, json.RawMessage(in)); err != nil {
			t.Errorf("Validate(%s) = %v, want nil", in, err)
		}
	}
	for _, in := range bad {
		if err := Validate(schema, json.RawMessage(in)); err == nil {
			t.Errorf("Validate(%s) = nil, want error", in)
		}
	}
}

type stubTool struct{ spec Spec }

func (s stubTool) Spec() Spec { return s.spec }
func (s stubTool) Run(context.Context, Env, json.RawMessage) (Result, error) {
	return Result{Content: "ok"}, nil
}

func TestRegistryAvailableFiltersByPermission(t *testing.T) {
	r := NewRegistry(
		stubTool{Spec{Name: "b_open"}},
		stubTool{Spec{Name: "a_needs_perm", Permissions: []string{"tenant.x.read"}}},
		nil,
	)
	if got := len(r.All()); got != 2 {
		t.Fatalf("All = %d", got)
	}
	if got := r.Available(orgctx.Scope{}); len(got) != 1 || got[0].Spec().Name != "b_open" {
		t.Fatalf("Available without perm = %v", got)
	}
	got := r.Available(orgctx.Scope{Permissions: []string{"tenant.x.read"}})
	if len(got) != 2 || got[0].Spec().Name != "a_needs_perm" {
		t.Fatalf("Available with perm not sorted/complete: %v", got)
	}
}
