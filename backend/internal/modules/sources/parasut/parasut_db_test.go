package parasut

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// dataset builds a month of Paraşüt records: 30 sales documents (invoices,
// refunds, a cancelled invoice, estimates, one of September) and bills
// (detailed, e-invoice without lines, kalemsiz receipts). It returns the
// October totals a person would add up in Paraşüt.
func dataset(f *fakeParasut) (salesN int, salesSum decimal.Decimal, buyN int, buySum decimal.Decimal, expN int, expSum decimal.Decimal) {
	f.included = append(f.included,
		resource{ID: "c1", Type: "contacts", Attributes: attrs(map[string]any{"name": "Yıldız Market Ltd. Şti.", "tax_number": "9100000021", "contact_type": "company"})},
		resource{ID: "c2", Type: "contacts", Attributes: attrs(map[string]any{"name": "Ayşe Kaya", "tax_number": "10000000146", "contact_type": "person"})},
		resource{ID: "s1", Type: "contacts", Attributes: attrs(map[string]any{"name": "Kalem Kırtasiye", "tax_number": "9200000035"})},
		resource{ID: "p1", Type: "products", Attributes: attrs(map[string]any{"name": "Kola 330 ml", "code": "K330"})},
	)
	for i := 1; i <= 30; i++ {
		day := (i % 28) + 1
		issue := fmt.Sprintf("2026-10-%02d", day)
		typ := "invoice"
		switch {
		case i%10 == 0:
			typ = "refund"
		case i == 7:
			typ = "cancelled"
		case i%9 == 0:
			typ = "estimate"
		}
		if i == 30 {
			issue = "2026-09-30" // previous month: the walk must stop here
		}
		qty, unit := float64(i), 10.0
		net := qty * unit
		vat := net * 0.20
		detID := fmt.Sprintf("sd%d", i)
		contact := "c1"
		if i%2 == 0 {
			contact = "c2"
		}
		f.add(&f.sales, resource{ID: fmt.Sprint(1000 + i), Type: "sales_invoices",
			Attributes: attrs(map[string]any{"item_type": typ, "invoice_series": "ATL", "invoice_id": 100 + i, "issue_date": issue,
				"currency": "TRL", "gross_total": money(net), "total_discount": "0.00", "total_invoice_discount": "0.00",
				"before_taxes_total": money(net), "total_vat": money(vat), "net_total": money(net + vat)}),
			Relationships: map[string]rel{"contact": relOne("contacts", contact), "details": relMany("sales_invoice_details", detID),
				"active_e_document": relOne("e_archives", map[bool]string{true: "ea1", false: ""}[i%3 == 0])}},
			resource{ID: detID, Type: "sales_invoice_details", Attributes: attrs(map[string]any{"quantity": qty, "unit_price": money(unit),
				"vat_rate": 20, "net_total": money(net)}), Relationships: map[string]rel{"product": relOne("products", "p1")}})
		if issue >= "2026-10-01" && (typ == "invoice" || typ == "refund") {
			salesN++
			salesSum = salesSum.Add(decimal.NewFromFloat(net + vat).Round(2))
		}
	}
	bill := func(id, typ, issue string, detailed bool, edoc string, net, vat float64, desc string, lines bool) {
		r := resource{ID: id, Type: "purchase_bills", Attributes: attrs(map[string]any{"item_type": typ, "invoice_no": "KRT" + id,
			"issue_date": issue, "currency": "TRL", "gross_total": money(net), "total_vat": money(vat), "net_total": money(net + vat),
			"is_detailed": detailed, "description": desc}),
			Relationships: map[string]rel{"supplier": relOne("contacts", "s1"), "active_e_document": relOne("e_invoices", edoc)}}
		var inc []resource
		if lines {
			r.Relationships["details"] = relMany("purchase_bill_details", "bd"+id)
			inc = append(inc, resource{ID: "bd" + id, Type: "purchase_bill_details", Attributes: attrs(map[string]any{
				"quantity": 1, "unit_price": money(net), "vat_rate": 20, "net_total": money(net)})})
		}
		f.add(&f.bills, r, inc...)
	}
	bill("1", "purchase_bill", "2026-10-03", true, "", 1000, 200, "", true)     // invoice with lines
	bill("2", "purchase_bill", "2026-10-05", false, "ei9", 500, 100, "", false) // e-invoice without lines → invoice
	bill("3", "purchase_bill", "2026-10-07", false, "", 250, 50, "34 ABC 123 akaryakıt", false)
	bill("4", "purchase_bill", "2026-10-09", false, "", 90, 0, "otopark", false)
	bill("5", "recurring_purchase_bill", "2026-10-10", true, "", 10, 2, "", true)
	bill("6", "purchase_bill", "2026-09-28", true, "", 70, 14, "", true)
	buyN, buySum = 2, decimal.RequireFromString("1800.00")
	expN, expSum = 2, decimal.RequireFromString("390.00")
	return
}

// TestAcceptanceCountsAndTotals is #47's acceptance: after a sync the portal
// holds exactly the source's count and total per kind, and a second sync
// adds nothing.
func TestAcceptanceCountsAndTotals(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")

	f := &fakeParasut{}
	salesN, salesSum, buyN, buySum, expN, expSum := dataset(f)
	srv := f.server(t)
	app := App{BaseURL: srv.URL, ClientID: "cid", ClientSecret: "sec", RedirectURL: "https://portal.example/cb"}
	var stored Tokens
	c := NewClient(app, srv.Client(), "777", Tokens{RefreshToken: "ref0"})
	c.Interval = 0
	c.OnTokens = func(t Tokens) error { stored = t; return nil }

	cos, err := c.Companies(ctx)
	if err != nil || len(cos) != 1 || cos[0].Name != "Atlas Gıda Ticaret A.Ş." {
		t.Fatalf("companies: %+v %v", cos, err)
	}
	if stored.RefreshToken != "ref1" {
		t.Fatalf("rotated refresh token not handed back: %+v", stored)
	}

	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	b, err := c.Read(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if b.Excluded["cancelled"] != 1 || b.Excluded["not_an_invoice"] < 2 {
		t.Fatalf("excluded: %v", b.Excluded)
	}
	rep, err := sources.Write(ctx, d.Pool, tp, SourceSystem, b)
	if err != nil {
		t.Fatal(err)
	}
	if rep.InvoicesCreated != salesN+buyN || rep.ExpensesCreated != expN {
		t.Fatalf("report: %+v (want %d invoices, %d expenses)", rep, salesN+buyN, expN)
	}

	check := func(label, direction string, wantN int, wantSum decimal.Decimal) {
		var n int
		var sum string
		if err := d.Pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(payable_amount), 0)::numeric(18,2)::text FROM invoices
			WHERE taxpayer_id = $1 AND source_system = 'parasut' AND direction = $2 AND period = '2026-10'`, tp, direction).Scan(&n, &sum); err != nil {
			t.Fatal(err)
		}
		if n != wantN || sum != wantSum.StringFixed(2) {
			t.Errorf("%s: %d / %s, Paraşüt %d / %s", label, n, sum, wantN, wantSum.StringFixed(2))
		}
	}
	check("satış", "outgoing", salesN, salesSum)
	check("alış", "incoming", buyN, buySum)
	var n int
	var sum, plate string
	if err := d.Pool.QueryRow(ctx, `SELECT count(*), sum(amount)::numeric(18,2)::text, max(COALESCE(vehicle_plate, '')) FROM expenses
		WHERE taxpayer_id = $1 AND source_system = 'parasut'`, tp).Scan(&n, &sum, &plate); err != nil {
		t.Fatal(err)
	}
	if n != expN || sum != expSum.StringFixed(2) || plate != "34ABC123" {
		t.Errorf("masraf: %d / %s plate %q, Paraşüt %d / %s", n, sum, plate, expN, expSum.StringFixed(2))
	}
	// Lines and VAT summaries: Σ line VAT = header VAT (10 §3.4).
	var bad int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM invoices i
		WHERE i.taxpayer_id = $1 AND i.has_items AND abs(i.tax_amount - (SELECT COALESCE(sum(total_tax_amount), 0) FROM invoice_items WHERE invoice_id = i.id)) > 0.05`, tp).Scan(&bad); err != nil {
		t.Fatal(err)
	}
	if bad != 0 {
		t.Errorf("%d invoices whose line VAT does not add up", bad)
	}
	var refunds, tevk int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE type = 'IADE'), count(*) FILTER (WHERE profile = 'EARSIVFATURA') FROM invoices WHERE taxpayer_id = $1`, tp).Scan(&refunds, &tevk)
	if refunds != 2 || tevk == 0 {
		t.Errorf("refunds %d, e-archive profiles %d", refunds, tevk)
	}

	// Second sync: same data, nothing new (01 §6 rule 1).
	b, err = c.Read(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	rep, err = sources.Write(ctx, d.Pool, tp, SourceSystem, b)
	if err != nil {
		t.Fatal(err)
	}
	if rep.InvoicesCreated != 0 || rep.ExpensesCreated != 0 || rep.InvoicesExisting != salesN+buyN || rep.ExpensesUnchanged != expN {
		t.Fatalf("second run: %+v", rep)
	}
}

// TestWalkStopsAtWindowStart: the client never pages past the first record
// older than the window (10 §2.1 safe method).
func TestWalkStopsAtWindowStart(t *testing.T) {
	f := &fakeParasut{}
	for i := 0; i < 60; i++ {
		f.add(&f.sales, resource{ID: fmt.Sprint(i), Type: "sales_invoices", Attributes: attrs(map[string]any{
			"item_type": "invoice", "issue_date": time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i).Format("2006-01-02"),
			"net_total": "1.00", "total_vat": "0.00", "gross_total": "1.00"})})
	}
	srv := f.server(t)
	c := NewClient(App{BaseURL: srv.URL, ClientID: "cid", ClientSecret: "sec"}, srv.Client(), "777", Tokens{AccessToken: "acc0", ExpiresAt: time.Now().Add(time.Hour)})
	c.Interval = 0
	b, err := c.Read(context.Background(), time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Invoices) != 7 {
		t.Fatalf("invoices: %d", len(b.Invoices))
	}
	pages := 0
	for _, r := range f.requests {
		if u, _ := url.Parse(r); u.Path == "/v4/777/sales_invoices" {
			pages++
		}
	}
	if pages != 1 {
		t.Fatalf("read %d pages for a 7-day window", pages)
	}
}

func TestPlateAndAuth(t *testing.T) {
	if plate("yakıt 06 abc 4567 istasyon") != "06ABC4567" || plate("fatura") != "" {
		t.Fatal("plate")
	}
	f := &fakeParasut{}
	srv := f.server(t)
	c := NewClient(App{BaseURL: srv.URL, ClientID: "cid", ClientSecret: "wrong"}, srv.Client(), "777", Tokens{RefreshToken: "x"})
	c.Interval = 0
	if _, err := c.Companies(context.Background()); err == nil {
		t.Fatal("bad client secret accepted")
	}
}
