package easytrade

// Mapping unit tests (#780): synthetic Easytrade payloads, shaped like the
// detail answers Easytrade's own mobile client reads, mapped into the VAT
// return rows (invoice_tax_summaries: rate × withholding code × exemption
// code; period by issue date). No network, no database.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

func payload(t *testing.T, s string) obj {
	t.Helper()
	var o obj
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		t.Fatal(err)
	}
	return o
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type sumRow struct{ rate, base, vat, withholding, wcode, excode string }

func rows(ss []model.TaxSummary) []sumRow {
	out := make([]sumRow, 0, len(ss))
	for _, s := range ss {
		out = append(out, sumRow{s.TaxRate.String(), s.Base.StringFixed(2), s.VAT.StringFixed(2), s.Withholding.StringFixed(2), s.WithholdingCode, s.TaxExemptionCode})
	}
	return out
}

func codes(ws []model.Warning) []string {
	var out []string
	for _, w := range ws {
		f, _ := w.Params["field"].(string)
		out = append(out, w.Code+":"+f)
	}
	return out
}

// A sales e-invoice with 1 %, 10 % and 20 % lines: one VAT row per rate.
func TestMapInvoiceRates(t *testing.T) {
	inv, ws, skip := mapInvoice(payload(t, `{
	  "id": 501, "uuid": "AAAAAAAA-0000-4000-8000-000000000501", "invoice_number": "ETR2026000000501",
	  "direction": {"value": "outgoing", "label": "Giden"}, "type": {"value": "SATIS"},
	  "profile": {"value": "TICARIFATURA"}, "status": {"value": "approved", "label": "Onaylandı"},
	  "issue_date": "2026-10-31", "currency": {"code": "TRY", "symbol": "₺"},
	  "customer": {"full_name": "Yıldız Market Ltd.", "tax_or_identity_number": "9100000021"},
	  "amounts": {"tax_exclusive_amount": "350.00", "tax_amount": "41.00", "payable_amount": "391.00"},
	  "items": [
	    {"name": "Ekmek", "quantity": 1, "unit_price": 100, "line_extension_amount": 100, "tax_exclusive_amount": 100,
	     "tax_breakdowns": [{"code": "0015", "rate": 1, "amount": 1, "base_amount": 100}]},
	    {"name": "Süt", "quantity": 1, "unit_price": 100, "line_extension_amount": 100, "tax_exclusive_amount": 100,
	     "tax_breakdowns": [{"code": "0015", "rate": 10, "amount": 10, "base_amount": 100}]},
	    {"name": "Deterjan", "quantity": 1, "unit_price": 150, "line_extension_amount": 150, "tax_exclusive_amount": 150,
	     "tax_breakdowns": [{"code": "0015", "rate": 20, "amount": 30, "base_amount": 150}]}
	  ]}`), "")
	if skip != "" || len(ws) != 0 {
		t.Fatalf("skip %q, warnings %v", skip, codes(ws))
	}
	if inv.Direction != model.DirectionOutgoing || inv.Period != "2026-10" || *inv.Profile != model.ProfileTicari ||
		inv.Counterparty.TaxNumber != "9100000021" || inv.Counterparty.Name != "Yıldız Market Ltd." || inv.UUID != "aaaaaaaa-0000-4000-8000-000000000501" {
		t.Fatalf("header: %+v", inv)
	}
	want := []sumRow{{"1", "100.00", "1.00", "0.00", "", ""}, {"10", "100.00", "10.00", "0.00", "", ""}, {"20", "150.00", "30.00", "0.00", "", ""}}
	if got := rows(inv.TaxSummaries); !slices.Equal(got, want) {
		t.Fatalf("summaries %v, want %v", got, want)
	}
	if !inv.Amounts.PayableAmount.Equal(d("391")) || !inv.Amounts.TaxAmount.Equal(d("41")) {
		t.Fatalf("amounts: %+v", inv.Amounts)
	}
}

// A purchase with KDV tevkifatı (603, 7/10) and a line without its own
// rate: the rate comes from the tax row, the withholding row carries the
// code, rate and amount the return needs.
func TestMapInvoiceWithholding(t *testing.T) {
	inv, ws, _ := mapInvoice(payload(t, `{
	  "id": 502, "invoice_number": "ABC2026000000001", "type": {"value": "TEVKIFAT"},
	  "profile": {"value": "TEMELFATURA"}, "status": {"value": "approved"}, "issue_date": "2026-11-02",
	  "customer": {"trade_name": "Temizlik Hizmetleri A.Ş.", "tax_or_identity_number": "1234567890"},
	  "amounts": {"tax_exclusive_amount": "1000.00", "tax_amount": "200.00", "payable_amount": "1060.00"},
	  "items": [{"name": "Temizlik hizmeti", "quantity": 1, "unit_price": 1000, "tax_exclusive_amount": 1000,
	    "withholding_code": "603", "withholding_rate": 70, "withholding_amount": 140,
	    "tax_breakdowns": [{"code": "0015", "rate": 20, "amount": 200, "base_amount": 1000},
	                       {"code": "9015", "rate": 70, "amount": 140, "base_amount": 200}]}]}`), model.DirectionIncoming)
	if len(ws) != 0 {
		t.Fatalf("warnings %v", codes(ws))
	}
	if inv.Direction != model.DirectionIncoming || inv.Type != model.TypeTevkifat || inv.Period != "2026-11" {
		t.Fatalf("header: %s %s %s", inv.Direction, inv.Type, inv.Period)
	}
	want := []sumRow{{"20", "1000.00", "200.00", "140.00", "603", ""}}
	if got := rows(inv.TaxSummaries); !slices.Equal(got, want) {
		t.Fatalf("summaries %v, want %v", got, want)
	}
	it := inv.Items[0]
	if !it.WithholdingRate.Equal(d("70")) || len(it.TaxBreakdowns) != 2 || it.TaxBreakdowns[1].Kind != "withholding" {
		t.Fatalf("item: %+v", it)
	}
	if !inv.Amounts.WithholdingAmount.Equal(d("140")) {
		t.Fatalf("withholding total %s", inv.Amounts.WithholdingAmount)
	}
}

// An export-style 0 % invoice: the exemption code comes from the invoice
// meta (global_tax_exemption_code) and lands in the exemption column.
func TestMapInvoiceExemption(t *testing.T) {
	inv, ws, _ := mapInvoice(payload(t, `{
	  "id": 503, "direction": {"value": "outgoing"}, "type": {"value": "ISTISNA"}, "profile": {"value": "EARSIVFATURA"},
	  "status": {"value": "sent"}, "issue_date": "2026-10-05",
	  "meta": {"global_tax_exemption_code": "301", "global_tax_exemption_reason": "Mal ihracatı"},
	  "customer": {"name": "Ali Veli", "tax_or_identity_number": "10000000146"},
	  "amounts": {"tax_exclusive_amount": "500.00", "tax_amount": "0.00", "payable_amount": "500.00"},
	  "items": [{"name": "Halı", "quantity": 1, "unit_price": 500, "tax_exclusive_amount": 500,
	    "tax_breakdowns": [{"code": "0015", "rate": 0, "amount": 0, "base_amount": 500}]}]}`), "")
	if len(ws) != 0 {
		t.Fatalf("warnings %v", codes(ws))
	}
	if inv.GlobalTaxExemptionCode != "301" || inv.Counterparty.IdentityNumber != "10000000146" || *inv.Profile != model.ProfileEArsiv {
		t.Fatalf("header: %+v", inv)
	}
	if got := rows(inv.TaxSummaries); !slices.Equal(got, []sumRow{{"0", "500.00", "0.00", "0.00", "", ""}}) {
		t.Fatalf("summaries %v", got)
	}
}

// What Easytrade did not send stays empty and is flagged: no VKN, no rate
// row (derived, flagged), a 0 % line without exemption code, a withholding
// code without rate or amount.
func TestMapInvoiceFlagsMissing(t *testing.T) {
	inv, ws, _ := mapInvoice(payload(t, `{
	  "id": 504, "status": {"value": "approved"}, "issue_date": "2026-10-20",
	  "customer": {"name": "Bilinmeyen Tedarikçi"},
	  "amounts": {"tax_exclusive_amount": "300.00", "tax_amount": "20.00", "payable_amount": "320.00"},
	  "items": [
	    {"name": "A", "tax_exclusive_amount": 100, "tax_amount": 20},
	    {"name": "B", "tax_exclusive_amount": 100, "tax_amount": 0},
	    {"name": "C", "tax_exclusive_amount": 100, "tax_amount": 0, "withholding_code": "602"}
	  ]}`), model.DirectionIncoming)
	got := codes(ws)
	for _, w := range []string{"INVALID_TAX_ID:customer_tax_number", "UNKNOWN_TAX_CODE:tax_rate", "UNKNOWN_TAX_CODE:tax_exemption_code", "UNKNOWN_TAX_CODE:withholding"} {
		if !slices.Contains(got, w) {
			t.Fatalf("warnings %v lack %s", got, w)
		}
	}
	if inv.Counterparty.TaxNumber != "" || inv.Counterparty.IdentityNumber != "" {
		t.Fatalf("invented a tax id: %+v", inv.Counterparty)
	}
	if inv.Direction != model.DirectionIncoming {
		t.Fatalf("direction %s, want the list's (incoming)", inv.Direction)
	}
	it := inv.Items[2]
	if it.WithholdingCode != "602" || !it.WithholdingRate.IsZero() || !it.WithholdingAmount.IsZero() {
		t.Fatalf("withholding guessed: %+v", it)
	}
	for _, w := range ws {
		if w.Code == model.WarnUnknownTaxCode && (w.EntityType != model.EntityInvoiceItem || w.LineNo == 0) {
			t.Fatalf("item warning without line: %+v", w)
		}
	}
}

// No lines: the header estimate is flagged HEADER_ESTIMATE.
func TestMapInvoiceHeaderOnly(t *testing.T) {
	inv, ws, _ := mapInvoice(payload(t, `{"id": 505, "status": {"value": "approved"}, "issue_date": "2026-10-01",
	  "customer": {"name": "X", "tax_or_identity_number": "9100000021"},
	  "amounts": {"tax_exclusive_amount": "100.00", "tax_amount": "20.00", "payable_amount": "120.00"}}`), "")
	if inv.TaxSummarySource != model.TaxSummaryFromHeaderEstimate || !slices.Contains(codes(ws), "HEADER_ESTIMATE:") {
		t.Fatalf("%s %v", inv.TaxSummarySource, codes(ws))
	}
}

func TestMapInvoiceSkips(t *testing.T) {
	for in, want := range map[string]string{
		`{"id": 1, "issue_date": "2026-10-01", "status": {"value": "cancelled"}}`:                  "cancelled",
		`{"id": 2, "issue_date": "2026-10-01", "status": {"value": "rejected"}}`:                   "cancelled",
		`{"id": 3, "issue_date": "2026-10-01", "status": {"value": "x", "label": "İptal Edildi"}}`: "cancelled",
		`{"id": 4, "issue_date": "2026-10-01", "status": {"value": "draft"}}`:                      "draft",
		`{"id": 5, "status": {"value": "approved"}}`:                                               "no_issue_date",
	} {
		if _, _, skip := mapInvoice(payload(t, in), ""); skip != want {
			t.Errorf("%s: skip %q, want %q", in, skip, want)
		}
	}
}

// Expense receipts: rate from tax_rate ("20_kdv") or tax_rate_percent, the
// currency as a plain string, no rate → no VAT (flagged on the office
// screen, not guessed).
func TestMapExpense(t *testing.T) {
	e, skip := mapExpense(payload(t, `{"id": 9, "expense_date": "2026-10-07", "amount": 120, "currency": "TRY",
	  "tax_rate_percent": 20, "description": "Kırtasiye", "vendor_name": "Kadıköy Kırtasiye", "vendor_tax_number": "2222222222",
	  "receipt_number": "0031"}`))
	if skip != "" || e.Period != "2026-10" || !e.TaxAmount.Equal(d("20")) || e.TaxRate == nil || !e.TaxRate.Equal(d("20")) || e.VendorTaxID != "2222222222" {
		t.Fatalf("%+v %q", e, skip)
	}
	e, _ = mapExpense(payload(t, `{"id": 10, "expense_date": "2026-10-08", "amount": "50.00", "currency": "USD"}`))
	if e.Currency != "USD" || e.TaxRate != nil || !e.TaxAmount.IsZero() || e.VendorTaxID != "" {
		t.Fatalf("%+v", e)
	}
	e, _ = mapExpense(payload(t, `{"id": 11, "expense_date": "2026-10-09", "amount": "110.00", "tax_rate": {"value": "10_kdv"}}`))
	if !e.TaxAmount.Equal(d("10")) {
		t.Fatalf("10 %%: %s", e.TaxAmount)
	}
}

func TestBaseURL(t *testing.T) {
	for in, want := range map[string]string{"": DefaultBaseURL, " ": DefaultBaseURL, `""`: DefaultBaseURL, "http://x.test/api/": "http://x.test/api"} {
		if got := BaseURL(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

// The connection test against the fake: the right ID passes, another ID,
// a wrong password, no permission, a dead and a slow server each give
// their own error.
func TestProbe(t *testing.T) {
	ctx := context.Background()
	f := testfakes.EasytradeSample()
	srv := testfakes.Serve(t, f)
	svc := NewService(nil, nil, srv.URL, srv.Client())
	res, err := svc.Test(ctx, "a@b.c", testfakes.EasytradePassword, " "+testfakes.EasytradeCompanyID+" ")
	if err != nil || !res.OK || len(res.Branches) != 2 || res.CompanyName == "" {
		t.Fatalf("test: %+v %v", res, err)
	}
	if f.Refreshes == 0 {
		t.Fatal("the login token was not refreshed")
	}
	for name, c := range map[string]struct {
		pw, id string
		want   error
	}{
		"wrong password": {"nope", "42", ErrBadLogin},
		"other company":  {testfakes.EasytradePassword, "43", ErrCompanyMismatch},
		"not a number":   {testfakes.EasytradePassword, "abc", ErrBadCompanyID},
	} {
		if _, err := svc.Test(ctx, "a@b.c", c.pw, c.id); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	f.Forbidden = true
	if _, err := svc.Test(ctx, "a@b.c", testfakes.EasytradePassword, "42"); !errors.Is(err, ErrForbidden) {
		t.Errorf("forbidden: %v", err)
	}

	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if _, err := NewService(nil, nil, dead.URL, nil).Test(ctx, "a@b.c", "pw", "42"); !errors.Is(err, ErrUnreachable) {
		t.Errorf("dead server: %v", err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	t.Cleanup(slow.Close)
	if _, err := NewService(nil, nil, slow.URL, &http.Client{Timeout: 50 * time.Millisecond}).Test(ctx, "a@b.c", "pw", "42"); !errors.Is(err, ErrTimeout) {
		t.Errorf("slow server: %v", err)
	}
}

// Invoice lists are asked with the date filter, the branch filter and the
// stable created_at order (Easytrade's issue_date order repeats and loses
// rows across pages).
func TestInvoiceListQuery(t *testing.T) {
	var q []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = append(q, r.URL.Path+"?"+r.URL.RawQuery)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{"current_page": 1, "last_page": 1, "total": 0}})
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, nil, "42", Token{Value: "t"})
	c.Interval, c.BranchID = 0, "7"
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	if _, err := c.listIDs(context.Background(), "/company/invoices/sales", "issue_date", from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := c.listIDs(context.Background(), "/company/expenses", "expense_date", from, to); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"filters%5Bissue_date%5D%5Bstart%5D=2026-10-01", "filters%5Bissue_date%5D%5Bend%5D=2026-10-31", "filters%5Bbranch_id%5D=7", "sort=created_at"} {
		if !strings.Contains(q[0], want) {
			t.Errorf("invoice list %s lacks %s", q[0], want)
		}
	}
	if !strings.Contains(q[1], "date_from=2026-10-01") || !strings.Contains(q[1], "date_to=2026-10-31") {
		t.Errorf("expense list %s lacks the date range", q[1])
	}
}
