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

// Easytrade fakes the Easytrade API: /auth/login → token and the user's
// company (data.user.company.id), /auth/refresh, Bearer + X-Company-ID
// (+ X-Branch-ID) headers, branches, paged sales / purchases / inbox /
// expenses / financial transactions (date desc, id desc, meta.total), their
// details and the VAT report. Z reports (GET /company/z-reports) are the
// endpoint the portal asks Easytrade for (issue #385): nil ZReports answers
// 404 like Easytrade today. A wrong password answers 401 and a one-letter
// password 422, like Easytrade (checked against the real API).
type Easytrade struct {
	mu        sync.Mutex
	Invoices  map[string]map[string]any // id → detail
	BranchOf  map[string]string         // invoice id (expense "e"+id, financial "f"+id, Z "z"+id) → branch; missing → "1"
	Inbox     map[string]bool           // invoice ids listed in /company/invoices/inbox instead of purchases (GİB e-invoices)
	Expenses  map[string]map[string]any
	Financial map[string]map[string]any // financial-transactions id → detail
	ZReports  map[string]map[string]any // nil → 404
	License   bool                      // false → 403 LICENSE_EXPIRED
	Forbidden bool                      // true → 403 without a license code (no permission)
	Logins    int
	Refreshes int
}

func (f *Easytrade) branchOf(key string) string {
	if b, ok := f.BranchOf[key]; ok {
		return b
	}
	return "1"
}

func (f *Easytrade) listOf(r *http.Request, w http.ResponseWriter, rows map[string]map[string]any, prefix, field string) {
	var out []map[string]any
	for id, d := range rows {
		if f.branchOf(prefix+id) == r.Header.Get("X-Branch-ID") {
			out = append(out, map[string]any{"id": json.Number(id), field: d[field]})
		}
	}
	page(w, r, out, field)
}

func (f *Easytrade) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user := map[string]any{"company": map[string]any{"id": 42, "name": "Atlas Gıda"}}
	if r.URL.Path == "/auth/login" {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if pw, _ := in["password"].(string); len(pw) < 2 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			writeJSON(w, map[string]any{"message": "Gönderdiğiniz veriler geçersiz.", "errors": map[string]any{"password": []string{"Şifre en az 4 karakter olmalıdır."}}})
			return
		}
		if in["password"] != EasytradePassword {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]any{"message": "E-posta veya şifre hatalı."})
			return
		}
		f.Logins++
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"token": "tok", "expires_at": time.Now().Add(720 * time.Hour).Format(time.RFC3339), "user": user}})
		return
	}
	if r.URL.Path == "/auth/refresh" {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.Refreshes++
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"token": "tok", "expires_at": time.Now().Add(720 * time.Hour).Format(time.RFC3339)}, "user": user})
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Company-ID") != EasytradeCompanyID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.Forbidden {
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]any{"message": "Bu işlem için yetkiniz yok."})
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
	case r.URL.Path == "/company/invoices/sales" || r.URL.Path == "/company/invoices/purchases" || r.URL.Path == "/company/invoices/inbox":
		dir := map[bool]string{true: "outgoing", false: "incoming"}[strings.HasSuffix(r.URL.Path, "sales")]
		inbox := strings.HasSuffix(r.URL.Path, "inbox")
		var rows []map[string]any
		for id, d := range f.Invoices {
			if d["direction"].(map[string]any)["value"] == dir && f.BranchOf[id] == br && (dir == "outgoing" || f.Inbox[id] == inbox) {
				rows = append(rows, map[string]any{"id": json.Number(id), "issue_date": d["issue_date"]})
			}
		}
		page(w, r, rows, "issue_date")
	case strings.HasPrefix(r.URL.Path, "/company/invoices/"):
		writeJSON(w, map[string]any{"data": f.Invoices[strings.TrimPrefix(r.URL.Path, "/company/invoices/")]})
	case r.URL.Path == "/company/expenses":
		var rows []map[string]any
		for id, d := range f.Expenses {
			if f.branchOf("e"+id) == br {
				rows = append(rows, map[string]any{"id": json.Number(id), "expense_date": d["expense_date"]})
			}
		}
		page(w, r, rows, "expense_date")
	case strings.HasPrefix(r.URL.Path, "/company/expenses/"):
		writeJSON(w, map[string]any{"data": f.Expenses[strings.TrimPrefix(r.URL.Path, "/company/expenses/")]})
	case r.URL.Path == "/company/financial-transactions":
		f.listOf(r, w, f.Financial, "f", "transaction_date")
	case strings.HasPrefix(r.URL.Path, "/company/financial-transactions/"):
		writeJSON(w, map[string]any{"data": f.Financial[strings.TrimPrefix(r.URL.Path, "/company/financial-transactions/")]})
	case r.URL.Path == "/company/z-reports" && f.ZReports != nil:
		f.listOf(r, w, f.ZReports, "z", "report_date")
	case strings.HasPrefix(r.URL.Path, "/company/z-reports/") && f.ZReports != nil:
		writeJSON(w, map[string]any{"data": f.ZReports[strings.TrimPrefix(r.URL.Path, "/company/z-reports/")]})
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
	writeJSON(w, map[string]any{"data": rows[lo:hi], "meta": map[string]any{"current_page": pg, "last_page": last, "total": len(rows)}})
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
			"tax_inclusive_amount": b.Add(v).StringFixed(2), "unit": map[string]any{"code": "C62"},
			"tax_breakdowns": []map[string]any{{"code": "0015", "rate": rate, "amount": v.InexactFloat64(), "base_amount": b.InexactFloat64()}}})
	}
	typ := "SATIS"
	if dir == "incoming" {
		typ = "ALIS"
	}
	return map[string]any{"id": i, "uuid": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "invoice_number": fmt.Sprintf("ETR2026%09d", i),
		"direction": map[string]any{"value": dir, "label": "x"}, "type": map[string]any{"value": typ},
		"profile": map[string]any{"value": "TEMELFATURA"}, "status": map[string]any{"value": status}, "issue_date": issue,
		"currency": map[string]any{"code": "TRY"}, "customer": map[string]any{"name": "Yıldız Market", "tax_or_identity_number": "9100000021"},
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
	f := &Easytrade{Invoices: map[string]map[string]any{}, BranchOf: map[string]string{}, Inbox: map[string]bool{"3": true},
		Expenses: map[string]map[string]any{}, License: true}
	for i, dir := range []string{"outgoing", "outgoing", "incoming", "outgoing"} {
		id := strconv.Itoa(i + 1)
		f.Invoices[id] = EasytradeInvoice(i+1, dir, "approved", fmt.Sprintf("2026-10-%02d", 3+i*5))
		f.BranchOf[id] = strconv.Itoa(i%2 + 1)
	}
	f.Expenses["901"] = EasytradeExpense(901, "2026-10-05", "kırtasiye", false)
	f.BranchOf["e901"] = "1"
	f.Financial = EasytradeFinancialSample()
	f.ZReports = EasytradeZReportSample()
	return f
}

// EasytradeFinancial is one financial transaction detail (01 §4.9).
func EasytradeFinancial(id int, date, typ, method, amount, customer, taxNumber string, invoiceID int) map[string]any {
	d := map[string]any{"id": id, "transaction_date": date, "transaction_type": map[string]any{"value": typ, "label": typ},
		"payment_method_type": map[string]any{"value": method, "label": method}, "amount": amount, "currency": map[string]any{"code": "TRY"},
		"description": typ + " " + customer, "status": map[string]any{"value": "completed"},
		"customer": map[string]any{"id": 500 + id, "name": customer, "tax_number": taxNumber},
		"cash_box": map[string]any{"id": 7, "name": "Merkez Kasa", "type": "cash"}}
	if invoiceID > 0 {
		d["referenceable_type"] = `App\Models\Invoice`
		d["referenceable_id"] = invoiceID
	}
	return d
}

// EasytradeFinancialSample: October collections and payments of two
// people (Ahmet, Mehmet) by cash, card and bank transfer; two reference
// sample invoices.
func EasytradeFinancialSample() map[string]map[string]any {
	return map[string]map[string]any{
		"801": EasytradeFinancial(801, "2026-10-04", "income", "cash", "120.00", "Ahmet Yılmaz", "12345678950", 1),
		"802": EasytradeFinancial(802, "2026-10-09", "income", "credit_card", "250.00", "Mehmet Demir", "", 2),
		"803": EasytradeFinancial(803, "2026-10-14", "expense", "bank_transfer", "300.00", "Yıldız Market", "9100000021", 3),
		"804": EasytradeFinancial(804, "2026-10-20", "retail_sale", "credit_card", "80.00", "Ahmet Yılmaz", "12345678950", 0),
		"805": EasytradeFinancial(805, "2026-10-21", "income", "cash", "40.00", "Mehmet Demir", "", 0),
	}
}

// EasytradeZReport is one Z report in the proposed shape
// (docs/easytrade-senkron.md §4).
func EasytradeZReport(id int, date, no, cash, card string) map[string]any {
	c, k := decimal.RequireFromString(cash), decimal.RequireFromString(card)
	total := c.Add(k)
	tax := total.Mul(decimal.NewFromInt(20)).Div(decimal.NewFromInt(120)).Round(2)
	return map[string]any{"id": id, "report_date": date, "z_report_number": no, "okc_serial_number": "AB12345678",
		"total_amount": total.StringFixed(2), "tax_amount": tax.StringFixed(2), "subtotal_amount": total.Sub(tax).StringFixed(2),
		"currency":      map[string]any{"code": "TRY"},
		"payments":      map[string]any{"cash": c.StringFixed(2), "credit_card": k.StringFixed(2)},
		"vat_breakdown": []map[string]any{{"tax_rate": "20", "taxable_amount": total.Sub(tax).StringFixed(2), "tax_amount": tax.StringFixed(2)}}}
}

// EasytradeZReportSample: three October days.
func EasytradeZReportSample() map[string]map[string]any {
	return map[string]map[string]any{
		"71": EasytradeZReport(71, "2026-10-01", "0041", "1200.00", "2400.00"),
		"72": EasytradeZReport(72, "2026-10-02", "0042", "800.00", "3100.00"),
		"73": EasytradeZReport(73, "2026-10-03", "0043", "950.50", "2049.50"),
	}
}
