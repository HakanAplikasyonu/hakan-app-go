// Package rules holds controls that need no table of their own: they take
// a plain input struct, so the UBL mapper (#10/#11), adapters and the sync
// engine can feed them once the shared data model (#9) exists. Every
// threshold is read from the effective configuration (warning_codes
// default ← office ← engagement), never from a constant.
package rules

import (
	"encoding/json"
	"fmt"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/shopspring/decimal"
)

// paramDecimal reads a decimal param. ok=false when the code has no such
// param or it is null ("not set": the check does not run).
func paramDecimal(cfg map[string]engine.Effective, code, key string) (decimal.Decimal, bool, error) {
	eff, ok := cfg[code]
	if !ok {
		return decimal.Zero, false, nil
	}
	v, ok := eff.Params[key]
	if !ok || v == nil {
		return decimal.Zero, false, nil
	}
	switch t := v.(type) {
	case string:
		d, err := decimal.NewFromString(t)
		if err != nil {
			return decimal.Zero, false, fmt.Errorf("%s.%s: %w", code, key, err)
		}
		return d, true, nil
	case float64:
		return decimal.NewFromFloat(t), true, nil
	case json.Number:
		d, err := decimal.NewFromString(t.String())
		return d, err == nil, err
	default:
		return decimal.Zero, false, fmt.Errorf("%s.%s: unsupported type %T", code, key, v)
	}
}

func money(d decimal.Decimal) string { return d.StringFixed(2) }

// paramStrings reads a list-of-strings param (keyword lists). ok=false when
// absent or null.
func paramStrings(cfg map[string]engine.Effective, code, key string) ([]string, bool, error) {
	eff, ok := cfg[code]
	if !ok {
		return nil, false, nil
	}
	v, ok := eff.Params[key]
	if !ok || v == nil {
		return nil, false, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s.%s: want a list of strings", code, key)
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		s, ok := x.(string)
		if !ok {
			return nil, false, fmt.Errorf("%s.%s: want a list of strings", code, key)
		}
		out = append(out, s)
	}
	return out, true, nil
}
