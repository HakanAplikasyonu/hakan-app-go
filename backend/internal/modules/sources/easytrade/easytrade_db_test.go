package easytrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fakeET struct {
	invoices map[string]map[string]any // id → detail
	branchOf map[string]string         // invoice/expense id → branch
	expenses map[string]map[string]any
	license  bool // false → LICENSE_EXPIRED
	logins   int
}

func (f *fakeET) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		js := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		if r.URL.Path == "/auth/login" {
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["password"] != "pw" {
				w.WriteHeader(401)
				return
			}
			f.logins++
			js(map[string]any{"data": map[string]any{"token": "tok", "expires_at": time.Now().Add(720 * time.Hour).Format(time.RFC3339)}})
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Company-ID") != "42" {
			w.WriteHeader(401)
			return
		}
		if !f.license {
			w.WriteHeader(403)
			js(map[string]any{"error_code": "LICENSE_EXPIRED"})
			return
		}
		br := r.Header.Get("X-Branch-ID")
		switch {
		case r.URL.Path == "/company/branches":
			js(map[string]any{"data": []map[string]any{{"id": 1, "name": "Merkez"}, {"id": 2, "name": "Kadıköy"}}})
		case r.URL.Path == "/company/invoices/sales" || r.URL.Path == "/company/invoices/purchases":
			dir := map[bool]string{true: "outgoing", false: "incoming"}[strings.HasSuffix(r.URL.Path, "sales")]
			var rows []map[string]any
			for id, d := range f.invoices {
				if d["direction"].(map[string]any)["value"] == dir && f.branchOf[id] == br {
					rows = append(rows, map[string]any{"id": json.Number(id), "issue_date": d["issue_date"]})
				}
			}
			f.page(w, r, rows, "issue_date")
		case strings.HasPrefix(r.URL.Path, "/company/invoices/"):
			js(map[string]any{"data": f.invoices[strings.TrimPrefix(r.URL.Path, "/company/invoices/")]})
		case r.URL.Path == "/company/expenses":
			var rows []map[string]any
			for id, d := range f.expenses {
				if f.branchOf["e"+id] == br {
					rows = append(rows, map[string]any{"id": json.Number(id), "expense_date": d["expense_date"]})
				}
			}
			f.page(w, r, rows, "expense_date")
		case strings.HasPrefix(r.URL.Path, "/company/expenses/"):
			js(map[string]any{"data": f.expenses[strings.TrimPrefix(r.URL.Path, "/company/expenses/")]})
		case r.URL.Path == "/company/reports/vat":
			js(map[string]any{"data": map[string]any{"sales": f.vat("outgoing"), "purchases": f.vat("incoming")}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeET) page(w http.ResponseWriter, r *http.Request, rows []map[string]any, field string) {
	// Stable: date desc, then id desc (a real ORDER BY must break ties too, or
	// rows on a page boundary can be skipped; see the PR's decision list).
	sort.Slice(rows, func(i, j int) bool {
		if rows[i][field].(string) != rows[j][field].(string) {
			return rows[i][field].(string) > rows[j][field].(string)
		}
		a, _ := strconv.Atoi(string(rows[i]["id"].(json.Number)))
		b, _ := strconv.Atoi(string(rows[j]["id"].(json.Number)))
		return a > b
	})
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	last := (len(rows) + per - 1) / per
	if last == 0 {
		last = 1
	}
	lo, hi := (page-1)*per, page*per
	if lo > len(rows) {
		lo = len(rows)
	}
	if hi > len(rows) {
		hi = len(rows)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": rows[lo:hi], "meta": map[string]any{"current_page": page, "last_page": last}})
}

// vat is Easytrade's own October VAT report: what its user sees (09 §8).
func (f *fakeET) vat(dir string) map[string]any {
	n := 0
	base, vat := decimal.Zero, decimal.Zero
	for _, d := range f.invoices {
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

func dataset() *fakeET {
	f := &fakeET{invoices: map[string]map[string]any{}, branchOf: map[string]string{}, expenses: map[string]map[string]any{}, license: true}
	for i := 1; i <= 230; i++ { // > 2 pages per branch at 100 per page
		id := fmt.Sprint(i)
		dir := "outgoing"
		if i%4 == 0 {
			dir = "incoming"
		}
		status := "approved"
		if i%37 == 0 {
			status = "cancelled"
		}
		day := i%30 + 1
		issue := fmt.Sprintf("2026-10-%02d", day)
		if i > 220 {
			issue = fmt.Sprintf("2026-09-%02d", day) // older: the walk stops before them
		}
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
		f.invoices[id] = map[string]any{"id": i, "uuid": fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "invoice_number": fmt.Sprintf("ETR2026%09d", i),
			"direction": map[string]any{"value": dir, "label": "x"}, "type": map[string]any{"value": "SATIS"},
			"profile": map[string]any{"value": "TEMELFATURA"}, "status": map[string]any{"value": status}, "issue_date": issue,
			"currency": map[string]any{"code": "TRY"}, "customer": map[string]any{"name": "Yıldız Market", "tax_number": "9100000021"},
			"amounts": map[string]any{"tax_exclusive_amount": base.StringFixed(2), "tax_amount": vat.StringFixed(2), "payable_amount": base.Add(vat).StringFixed(2)},
			"items": items}
		if dir == "incoming" {
			f.invoices[id]["type"] = map[string]any{"value": "ALIS"}
		}
		f.branchOf[id] = fmt.Sprint(i%2 + 1)
	}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprint(900 + i)
		f.expenses[id] = map[string]any{"id": 900 + i, "expense_date": fmt.Sprintf("2026-10-%02d", i), "amount": "120.00", "tax_rate": map[string]any{"value": "20_kdv"},
			"description": map[bool]string{true: "06 ABC 123 yakıt", false: "kırtasiye"}[i == 1], "is_main_expense": i == 5}
		f.branchOf["e"+id] = "1"
	}
	return f
}

// TestAcceptance is #46's acceptance: after a sync Easytrade's own VAT
// report and the portal agree on count, base and VAT, both directions.
func TestAcceptance(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	f := dataset()
	srv := f.server(t)
	kr, _ := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	svc := NewService(d.Pool, secrets.New(kr, secrets.NewPGStore(db.New(d.Pool))), srv.URL, srv.Client())
	svc.interval = 0

	if _, err := svc.Connect(ctx, tp, "a@b.c", "wrong", "42"); !errors.Is(err, ErrAuth) {
		t.Fatalf("bad password: %v", err)
	}
	c, err := svc.Connect(ctx, tp, "a@b.c", "pw", "42")
	if err != nil {
		t.Fatal(err)
	}
	if c.SetupState != "active" || len(c.Branches) != 2 || c.CompanyID != "42" {
		t.Fatalf("connect: %+v", c)
	}
	var metaText string
	_ = d.Pool.QueryRow(ctx, `SELECT meta::text FROM source_connections WHERE uuid = $1`, c.UUID).Scan(&metaText)
	if strings.Contains(metaText, "tok") || strings.Contains(metaText, "pw") {
		t.Fatalf("secret in meta: %s", metaText)
	}

	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	rep, err := svc.Sync(ctx, tp, c.UUID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Excluded["cancelled"] == 0 || rep.Excluded["recurring_template"] != 1 || rep.ExpensesCreated != 4 {
		t.Fatalf("report: %+v", rep)
	}
	rec, err := svc.Reconcile(ctx, tp, c.UUID, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Sales.Match || !rec.Purchases.Match || rec.Sales.Source.Count == 0 || rec.Purchases.Source.Count == 0 {
		t.Fatalf("VAT reconciliation: %+v", rec)
	}
	var sum, plate string
	_ = d.Pool.QueryRow(ctx, `SELECT sum(amount)::numeric(18,2)::text, max(COALESCE(vehicle_plate,'')) FROM expenses WHERE taxpayer_id = $1 AND source_system = 'easytrade'`, tp).Scan(&sum, &plate)
	if sum != "480.00" || plate != "06ABC123" {
		t.Fatalf("expenses: %s %q", sum, plate)
	}

	svc.now = func() time.Time { return time.Now().Add(SyncThrottle + time.Minute) }
	rep, err = svc.Sync(ctx, tp, c.UUID, from, to)
	if err != nil || rep.InvoicesCreated != 0 || rep.ExpensesCreated != 0 {
		t.Fatalf("second sync: %+v %v", rep, err)
	}

	// 403 LICENSE_EXPIRED: connection disabled, not retried (09 §1.3).
	f.license = false
	svc.now = func() time.Time { return time.Now().Add(2 * (SyncThrottle + time.Minute)) }
	if _, err := svc.Sync(ctx, tp, c.UUID, from, to); !errors.Is(err, ErrLicense) {
		t.Fatalf("license: %v", err)
	}
	list, _ := svc.List(ctx, tp)
	if list[0].Status != "disabled" || !strings.Contains(list[0].Error, "lisans") {
		t.Fatalf("after license: %+v", list[0])
	}
}
