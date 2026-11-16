package testfakes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// Easytrade credentials the fake accepts (docs/kaynak 09).
const (
	EasytradePassword  = "pw"
	EasytradeCompanyID = "42"
)

// Easytrade fakes the Easytrade API: /auth/login → token, Bearer +
// X-Company-ID (+ X-Branch-ID) headers, branches, paged sales / purchases /
// expenses (date desc, id desc), their details and the VAT report.
type Easytrade struct {
	mu       sync.Mutex
	Invoices map[string]map[string]any // id → detail
	BranchOf map[string]string         // invoice id (expense: "e"+id) → branch
	Expenses map[string]map[string]any
	License  bool // false → 403 LICENSE_EXPIRED
	Logins   int
}

func (f *Easytrade) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/auth/login" {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["password"] != EasytradePassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.Logins++
		writeJSON(w, map[string]any{"data": map[string]any{"token": "tok", "expires_at": time.Now().Add(720 * time.Hour).Format(time.RFC3339)}})
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Company-ID") != EasytradeCompanyID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !f.License {
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]any{"error_code": "LICENSE_EXPIRED"})
		return
	}
	br := r.Header.Get("X-Branch-ID")
	switch {
	case r.URL.Path == "/company/branches":
		writeJSON(w, map[string]any{"data": []map[string]any{{"id": 1, "name": "Merkez"}, {"id": 2, "name": "Kadıköy"}}})
	case r.URL.Path == "/company/invoices/sales" || r.URL.Path == "/company/invoices/purchases":
		dir := map[bool]string{true: "outgoing", false: "incoming"}[strings.HasSuffix(r.URL.Path, "sales")]
		var rows []map[string]any
		for id, d := range f.Invoices {
			if d["direction"].(map[string]any)["value"] == dir && f.BranchOf[id] == br {
				rows = append(rows, map[string]any{"id": json.Number(id), "issue_date": d["issue_date"]})
			}
		}
		page(w, r, rows, "issue_date")
	case strings.HasPrefix(r.URL.Path, "/company/invoices/"):
		writeJSON(w, map[string]any{"data": f.Invoices[strings.TrimPrefix(r.URL.Path, "/company/invoices/")]})
	case r.URL.Path == "/company/expenses":
		var rows []map[string]any
		for id, d := range f.Expenses {
			if f.BranchOf["e"+id] == br {
				rows = append(rows, map[string]any{"id": json.Number(id), "expense_date": d["expense_date"]})
			}
		}
		page(w, r, rows, "expense_date")
	case strings.HasPrefix(r.URL.Path, "/company/expenses/"):
		writeJSON(w, map[string]any{"data": f.Expenses[strings.TrimPrefix(r.URL.Path, "/company/expenses/")]})
	case r.URL.Path == "/company/reports/vat":
		writeJSON(w, map[string]any{"data": map[string]any{"sales": f.VAT("outgoing"), "purchases": f.VAT("incoming")}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// page answers one page, date desc then id desc (a real ORDER BY must break
// ties too, or rows on a page boundary can be skipped).
func page(w http.ResponseWriter, r *http.Request, rows []map[string]any, field string) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i][field].(string) != rows[j][field].(string) {
			return rows[i][field].(string) > rows[j][field].(string)
		}
		a, _ := strconv.Atoi(string(rows[i]["id"].(json.Number)))
		b, _ := strconv.Atoi(string(rows[j]["id"].(json.Number)))
		return a > b
	})
	pg, _ := strconv.Atoi(r.URL.Query().Get("page"))
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if per <= 0 {
		per = 100
	}
	last := max((len(rows)+per-1)/per, 1)
	lo, hi := min(max(pg-1, 0)*per, len(rows)), min(max(pg, 1)*per, len(rows))
	writeJSON(w, map[string]any{"data": rows[lo:hi], "meta": map[string]any{"current_page": pg, "last_page": last}})
}

// VAT is Easytrade's own October VAT report: what its user sees (09 §8).
func (f *Easytrade) VAT(dir string) map[string]any {
	n := 0
	base, vat := decimal.Zero, decimal.Zero
	for _, d := range f.Invoices {
		if d["direction"].(map[string]any)["value"] != dir || !strings.HasPrefix(d["issue_date"].(string), "2026-10") ||
			d["status"].(map[string]any)["value"] == "cancelled" {
			continue
		}
		n++
		am := d["amounts"].(map[string]any)
		base = base.Add(decimal.RequireFromString(am["tax_exclusive_amount"].(string)))
		vat = vat.Add(decimal.RequireFromString(am["tax_amount"].(string)))
	}
	return map[string]any{"invoice_count": n, "totals": map[string]any{"base": base.StringFixed(2), "vat": vat.StringFixed(2)}}
}

// EasytradeInvoice is one invoice detail as Easytrade returns it: two lines
// at 20 % and 10 %.
func EasytradeInvoice(i int, dir, status, issue string) map[string]any {
	var items []map[string]any
	base, vat := decimal.Zero, decimal.Zero
	for l, rate := range []int64{20, 10} {
		b := decimal.NewFromInt(int64(i*10 + l))
		v := b.Mul(decimal.NewFromInt(rate)).Div(decimal.NewFromInt(100)).Round(2)
		base, vat = base.Add(b), vat.Add(v)
		items = append(items, map[string]any{"name": "Kalem", "quantity": "1", "unit_price": b.StringFixed(2),
			"line_extension_amount": b.StringFixed(2), "tax_exclusive_amount": b.StringFixed(2), "total_tax_amount": v.StringFixed(2),
			"tax_inclusive_amount": b.Add(v).StringFixed(2), "unit": map[string]any{"code": "C62"}})
	}
	typ := "SATIS"
	if dir == "incoming" {
		typ = "ALIS"
	}
	return map[string]any{"id": i, "uuid": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "invoice_number": fmt.Sprintf("ETR2026%09d", i),
		"direction": map[string]any{"value": dir, "label": "x"}, "type": map[string]any{"value": typ},
		"profile": map[string]any{"value": "TEMELFATURA"}, "status": map[string]any{"value": status}, "issue_date": issue,
		"currency": map[string]any{"code": "TRY"}, "customer": map[string]any{"name": "Yıldız Market", "tax_number": "9100000021"},
		"amounts": map[string]any{"tax_exclusive_amount": base.StringFixed(2), "tax_amount": vat.StringFixed(2), "payable_amount": base.Add(vat).StringFixed(2)},
		"items":   items}
}

// EasytradeExpense is one expense detail.
func EasytradeExpense(id int, date, description string, main bool) map[string]any {
	return map[string]any{"id": id, "expense_date": date, "amount": "120.00", "tax_rate": map[string]any{"value": "20_kdv"},
		"description": description, "is_main_expense": main}
}

// EasytradeSample is a small October data set over both branches.
func EasytradeSample() *Easytrade {
	f := &Easytrade{Invoices: map[string]map[string]any{}, BranchOf: map[string]string{}, Expenses: map[string]map[string]any{}, License: true}
	for i, dir := range []string{"outgoing", "outgoing", "incoming", "outgoing"} {
		id := strconv.Itoa(i + 1)
		f.Invoices[id] = EasytradeInvoice(i+1, dir, "approved", fmt.Sprintf("2026-10-%02d", 3+i*5))
		f.BranchOf[id] = strconv.Itoa(i%2 + 1)
	}
	f.Expenses["901"] = EasytradeExpense(901, "2026-10-05", "kırtasiye", false)
	f.BranchOf["e901"] = "1"
	return f
}
