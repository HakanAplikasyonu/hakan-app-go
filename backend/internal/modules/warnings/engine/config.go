package engine

import (
	"encoding/json"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
)

// Layer is one configuration level (catalog default, office or engagement).
// Nil fields inherit from the level below.
type Layer struct {
	Severity *catalog.Severity
	Enabled  *bool
	Params   json.RawMessage
}

// Merge resolves default ← office ← engagement for one code. The default
// layer must carry a severity. Params merge key by key; a JSON null in an
// upper layer clears the key's value to null (threshold "not set").
func Merge(code string, def Layer, upper ...Layer) Effective {
	eff := Effective{Code: code, Enabled: true, Params: map[string]any{}}
	apply := func(l Layer) {
		if l.Severity != nil && l.Severity.Valid() {
			eff.Severity = *l.Severity
		}
		if l.Enabled != nil {
			eff.Enabled = *l.Enabled
		}
		if len(l.Params) > 0 {
			var m map[string]any
			if json.Unmarshal(l.Params, &m) == nil {
				for k, v := range m {
					eff.Params[k] = v
				}
			}
		}
	}
	apply(def)
	for _, l := range upper {
		apply(l)
	}
	return eff
}
