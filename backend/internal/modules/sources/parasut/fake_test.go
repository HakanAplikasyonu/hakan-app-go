package parasut

import (
	"encoding/json"
	"fmt"
)

// The Paraşüt fake is testfakes.Parasut (shared with the write sweep,
// #318); these helpers build its JSON:API resources with the adapter's types.

func attrs(m map[string]any) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}

func relOne(typ, id string) rel {
	if id == "" {
		return rel{Data: json.RawMessage("null")}
	}
	b, _ := json.Marshal(ref{ID: id, Type: typ})
	return rel{Data: b}
}

func relMany(typ string, ids ...string) rel {
	refs := make([]ref, len(ids))
	for i, id := range ids {
		refs[i] = ref{ID: id, Type: typ}
	}
	b, _ := json.Marshal(refs)
	return rel{Data: b}
}

// money formats like Paraşüt's string decimals.
func money(v float64) string { return fmt.Sprintf("%.2f", v) }
