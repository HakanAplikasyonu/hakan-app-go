package parasut

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"
)

// Paraşüt API v4 speaks JSON:API (10 §2): data + included, relationships by
// {id, type}.

type ref struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// rel is a relationship; data is one ref, a list or null.
type rel struct {
	Data json.RawMessage `json:"data"`
}

func (r rel) one() (ref, bool) {
	var x ref
	if len(r.Data) == 0 || bytes.Equal(r.Data, []byte("null")) || json.Unmarshal(r.Data, &x) != nil || x.ID == "" {
		return ref{}, false
	}
	return x, true
}

func (r rel) many() []ref {
	var xs []ref
	_ = json.Unmarshal(r.Data, &xs)
	return xs
}

type resource struct {
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Attributes    map[string]json.RawMessage `json:"attributes"`
	Relationships map[string]rel             `json:"relationships"`
}

type document struct {
	Data     json.RawMessage `json:"data"`
	Included []resource      `json:"included"`
	Meta     map[string]any  `json:"meta"`
}

func (d document) list() ([]resource, error) {
	var out []resource
	if len(d.Data) == 0 || bytes.Equal(d.Data, []byte("null")) {
		return nil, nil
	}
	err := json.Unmarshal(d.Data, &out)
	return out, err
}

// index of included resources by type/id.
type index map[string]resource

func newIndex(rs []resource) index {
	ix := index{}
	for _, r := range rs {
		ix[r.Type+"/"+r.ID] = r
	}
	return ix
}

func (ix index) get(r ref) (resource, bool) {
	x, ok := ix[r.Type+"/"+r.ID]
	return x, ok
}

func (r resource) str(key string) string {
	raw, ok := r.Attributes[key]
	if !ok || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return strings.Trim(string(raw), `"`)
}

func (r resource) boolean(key string) bool {
	var b bool
	_ = json.Unmarshal(r.Attributes[key], &b)
	return b
}

// dec reads a number given as JSON number or string ("118.0"); missing → 0.
func (r resource) dec(key string) decimal.Decimal {
	s := r.str(key)
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

func (r resource) rel(name string) rel { return r.Relationships[name] }

// attrs is the raw attribute map kept in raw_payload.
func (r resource) attrs() map[string]any {
	out := map[string]any{"id": r.ID, "type": r.Type}
	for k, v := range r.Attributes {
		var x any
		if json.Unmarshal(v, &x) == nil {
			out[k] = x
		}
	}
	return out
}
