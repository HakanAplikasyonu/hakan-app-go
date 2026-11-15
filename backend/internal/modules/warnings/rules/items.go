package rules

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// Codes checked on invoice lines (05 §3.3, 06 T3).
const (
	CodeFixedAsset    = "FIXED_ASSET_CANDIDATE"
	CodeUnusualRecord = "UNUSUAL_RECORD_TYPE"
)

// ItemInput is one invoice line with the taxpayer's history.
type ItemInput struct {
	ItemID int64
	Period string
	// Incoming is true for purchase invoices (fixed assets are purchases).
	Incoming bool
	Name     string
	// Amount is the line's tax-exclusive amount.
	Amount decimal.Decimal
	// History is the taxpayer's item names in the same direction over the
	// last 12 months (distinct), excluding this period; HistoryLines is how
	// many lines those names cover.
	History      []string
	HistoryLines int
}

// Items checks FIXED_ASSET_CANDIDATE (params keywords, amount_limit) and
// UNUSUAL_RECORD_TYPE (params min_history = past lines needed, min_overlap).
type Items struct{}

// Producer implements engine.Rule.
func (Items) Producer() string { return "control:invoice_items" }

// Codes implements engine.Rule.
func (Items) Codes() []string { return []string{CodeFixedAsset, CodeUnusualRecord} }

// Check implements engine.Rule.
func (Items) Check(in ItemInput, cfg map[string]engine.Effective) ([]engine.Outcome, error) {
	id := in.ItemID
	o := engine.Outcome{
		Entity: engine.Entity{Type: catalog.EntityInvoiceItem, ID: &id, Period: in.Period},
		Owned:  []string{CodeFixedAsset, CodeUnusualRecord},
	}
	tokens := Tokens(in.Name)

	if in.Incoming {
		keywords, _, err := paramStrings(cfg, CodeFixedAsset, "keywords")
		if err != nil {
			return nil, err
		}
		limit, limitSet, err := paramDecimal(cfg, CodeFixedAsset, "amount_limit")
		if err != nil {
			return nil, err
		}
		if kw := matchKeyword(in.Name, keywords); kw != "" {
			o.Findings = append(o.Findings, engine.Finding{Code: CodeFixedAsset, Params: map[string]any{"keyword": kw, "name": in.Name}})
		} else if limitSet && in.Amount.GreaterThan(limit) {
			o.Findings = append(o.Findings, engine.Finding{Code: CodeFixedAsset, Params: map[string]any{
				"amount": money(in.Amount), "limit": money(limit), "name": in.Name}})
		}
	}

	minHist, histSet, err := paramDecimal(cfg, CodeUnusualRecord, "min_history")
	if err != nil {
		return nil, err
	}
	minOverlap, overlapSet, err := paramDecimal(cfg, CodeUnusualRecord, "min_overlap")
	if err != nil {
		return nil, err
	}
	if histSet && overlapSet && len(tokens) > 0 &&
		decimal.NewFromInt(int64(in.HistoryLines)).GreaterThanOrEqual(minHist) {
		known := map[string]bool{}
		for _, h := range in.History {
			for _, t := range Tokens(h) {
				known[stem(t)] = true
			}
		}
		hit := 0
		for _, t := range tokens {
			if known[stem(t)] {
				hit++
			}
		}
		overlap := decimal.NewFromInt(int64(hit)).Div(decimal.NewFromInt(int64(len(tokens))))
		if overlap.LessThan(minOverlap) {
			o.Findings = append(o.Findings, engine.Finding{Code: CodeUnusualRecord, Params: map[string]any{
				"name": in.Name, "overlap": overlap.StringFixed(2), "history_lines": in.HistoryLines}})
		}
	}
	return []engine.Outcome{o}, nil
}

// Tokens splits a line name into normalized words (Turkish folded, 3+
// letters, digits and noise dropped).
func Tokens(s string) []string {
	s = fold(s)
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) < 3 || stopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// stem keeps the first five letters: Turkish suffixes ("toner" /
// "toneri", "danışmanlık" / "danışmanlığı") then compare equal.
func stem(w string) string {
	r := []rune(w)
	if len(r) > 5 {
		return string(r[:5])
	}
	return w
}

var stopwords = map[string]bool{"adet": true, "ile": true, "icin": true, "olarak": true, "kdv": true, "dahil": true, "haric": true, "bedeli": true, "ait": true}

func matchKeyword(name string, keywords []string) string {
	n := " " + strings.Join(strings.FieldsFunc(fold(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ") + " "
	for _, k := range keywords {
		kk := strings.TrimSpace(fold(k))
		if kk != "" && strings.Contains(n, " "+kk) {
			return k
		}
	}
	return ""
}

func fold(s string) string {
	s = strings.NewReplacer("İ", "i", "I", "ı").Replace(s)
	s = strings.ToLower(s)
	return strings.NewReplacer("ı", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c").Replace(s)
}

var _ engine.Rule[ItemInput] = Items{}
