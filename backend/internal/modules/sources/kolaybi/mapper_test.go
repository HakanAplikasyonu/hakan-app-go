package kolaybi

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func expenseObj(t *testing.T, s string) obj {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return obj(m)
}

// KDV kırılımı (#780): items are grouped by their own vat_rate; an expense
// without items has no breakdown even when its total VAT is known.
func TestMapExpenseVatLines(t *testing.T) {
	e, skip := mapExpense(expenseObj(t, `{"id": "7", "issue_date": "2026-10-03", "items": [
	  {"net_total": "100", "vat_rate": "20"}, {"net_total": "50", "vat_rate": "10"}, {"net_total": "200", "vat_rate": "20"}]}`))
	if skip != "" || len(e.VatLines) != 2 {
		t.Fatalf("%q %+v", skip, e.VatLines)
	}
	want := [][3]string{{"10", "50", "5"}, {"20", "300", "60"}}
	for i, w := range want {
		l := e.VatLines[i]
		if !l.Rate.Equal(decimal.RequireFromString(w[0])) || !l.Base.Equal(decimal.RequireFromString(w[1])) || !l.Tax.Equal(decimal.RequireFromString(w[2])) {
			t.Errorf("line %d %+v, want %v", i, l, w)
		}
	}
	e, _ = mapExpense(expenseObj(t, `{"id": "8", "issue_date": "2026-10-03", "grand_total": "120", "total_vat": "20"}`))
	if len(e.VatLines) != 0 {
		t.Fatalf("kırılım yok: %+v", e.VatLines)
	}
}
