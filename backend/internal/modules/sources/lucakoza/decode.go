package lucakoza

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Koza's answers are read tolerantly: numbers may come as strings ("1.234,56"
// or "1234.56"), unknown fields are ignored and a missing field is empty.
// Numbers are decoded as json.Number, never float64, so amounts keep every
// digit Koza sent.

type obj map[string]any

// decode reads a JSON answer keeping numbers exact.
func decode(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func asObj(v any) obj {
	m, _ := v.(map[string]any)
	return obj(m)
}

// get returns the field k; a different letter case is accepted.
func (o obj) get(k string) (any, bool) {
	if v, ok := o[k]; ok {
		return v, true
	}
	for name, v := range o {
		if strings.EqualFold(name, k) {
			return v, true
		}
	}
	return nil, false
}

// sub is the first nested object among keys.
func (o obj) sub(keys ...string) obj {
	for _, k := range keys {
		if v, ok := o.get(k); ok {
			if m, ok := v.(map[string]any); ok {
				return obj(m)
			}
		}
	}
	return obj{}
}

// list is the first array of objects among keys.
func (o obj) list(keys ...string) []obj {
	for _, k := range keys {
		if v, ok := o.get(k); ok {
			if xs, ok := v.([]any); ok {
				return objs(xs)
			}
		}
	}
	return nil
}

func objs(xs []any) []obj {
	out := make([]obj, 0, len(xs))
	for _, x := range xs {
		if m, ok := x.(map[string]any); ok {
			out = append(out, obj(m))
		}
	}
	return out
}

// rows reads a list answer: a top-level array, or the first array of
// objects under one of keys, else under any field.
func rows(v any, keys ...string) []obj {
	if xs, ok := v.([]any); ok {
		return objs(xs)
	}
	o := asObj(v)
	if l := o.list(keys...); l != nil {
		return l
	}
	for _, x := range o {
		if xs, ok := x.([]any); ok && len(objs(xs)) > 0 {
			return objs(xs)
		}
	}
	return nil
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

// str is the first non-empty scalar among keys.
func (o obj) str(keys ...string) string {
	for _, k := range keys {
		if v, ok := o.get(k); ok {
			if s := scalar(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// dec is the first amount among keys that parses; ok is false when none does.
func (o obj) dec(keys ...string) (decimal.Decimal, bool) {
	for _, k := range keys {
		if v, ok := o.get(k); ok {
			if d, ok := parseDecimal(v); ok {
				return d, true
			}
		}
	}
	return decimal.Zero, false
}

// parseDecimal reads an amount exactly: a JSON number, or a string in either
// "1234.56", "1,234.56", "1234,56" or "1.234,56" form.
func parseDecimal(v any) (decimal.Decimal, bool) {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case string:
		s = normalizeNumber(x)
	default:
		return decimal.Zero, false
	}
	if s == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

func normalizeNumber(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	comma, dot := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	switch {
	case comma >= 0 && dot >= 0 && comma > dot: // 1.234,56
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
	case comma >= 0 && dot >= 0: // 1,234.56
		s = strings.ReplaceAll(s, ",", "")
	case comma >= 0 && strings.Count(s, ",") == 1: // 1234,56
		s = strings.Replace(s, ",", ".", 1)
	case comma >= 0: // 1,234,567
		s = strings.ReplaceAll(s, ",", "")
	case strings.Count(s, ".") > 1: // 1.234.567
		s = strings.ReplaceAll(s, ".", "")
	}
	return s
}

// turkey is Türkiye's fixed offset (UTC+3, no daylight saving since 2016).
var turkey = time.FixedZone("TRT", 3*60*60)

// parseDate reads a document date: dd/MM/yyyy, dd.MM.yyyy, yyyy-MM-dd (a
// time part is ignored) or epoch milliseconds. It returns the calendar day.
func parseDate(v any) (time.Time, bool) {
	if n, ok := v.(json.Number); ok {
		ms, err := n.Int64()
		if err != nil || ms <= 0 {
			return time.Time{}, false
		}
		if ms < 100_000_000_000 { // seconds
			ms *= 1000
		}
		t := time.UnixMilli(ms).In(turkey)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
	}
	s := scalar(v)
	if len(s) < 10 {
		return time.Time{}, false
	}
	for _, layout := range []string{"02/01/2006", "02.01.2006", "2006-01-02", "02-01-2006"} {
		if t, err := time.Parse(layout, s[:10]); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
