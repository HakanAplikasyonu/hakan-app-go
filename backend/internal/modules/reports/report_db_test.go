package reports

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestReportNumbersMatchPortalData checks every total of a monthly and a
// yearly report against hand-computed sums.
func TestReportNumbersMatchPortalData(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 5100000000+testdb.Next()))
	eng := d.Engagement(t, org, tp)
	n := 0
	inv := func(dir, typ, date, cur, rate, base, vat, status string) {
		n++
		if _, err := d.Pool.Exec(ctx, `INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number,
			direction, type, issue_date, period, currency, exchange_rate, tax_summary_source, tax_exclusive_amount, tax_amount, payable_amount, status_normalized)
			VALUES ($1, 'test', $2::text, 'manual', $2::text, $3::text, $4::text, $5::date, to_char($5::date, 'YYYY-MM'), $6::text, nullif($7, '')::numeric, 'items',
			        $8::numeric, $9::numeric, $8::numeric + $9::numeric, $10::text)`,
			tp, fmt.Sprintf("R-%d", n), dir, typ, date, cur, rate, base, vat, status); err != nil {
			t.Fatal(err)
		}
	}
	inv("outgoing", "SATIS", "2026-09-05", "TRY", "", "80000", "16000", "active")
	inv("outgoing", "SATIS", "2026-10-03", "TRY", "", "50000", "10000", "active")
	inv("outgoing", "SATIS", "2026-10-20", "USD", "40", "1000", "200", "active") // 40000 / 8000 TRY
	inv("outgoing", "SATIS", "2026-10-21", "TRY", "", "7777", "0", "cancelled")  // not counted
	inv("incoming", "IADE", "2026-10-22", "TRY", "", "5000", "1000", "active")   // sales return
	inv("incoming", "ALIS", "2026-10-07", "TRY", "", "30000", "6000", "active")
	inv("outgoing", "IADE", "2026-10-08", "TRY", "", "2000", "400", "active") // purchase return
	inv("incoming", "ALIS", "2026-09-07", "TRY", "", "20000", "4000", "active")
	if _, err := d.Pool.Exec(ctx, `INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount, tax_exclusive_amount, tax_amount, receipt_type, approval_status)
		VALUES ($1, 'test', 'e1', 'manual', '2026-10-09', '2026-10', 1200, 1000, 200, 'fuel_receipt', 'approved'),
		       ($1, 'test', 'e2', 'manual', '2026-10-10', '2026-10', 500, NULL, NULL, 'other', 'approved'),
		       ($1, 'test', 'e3', 'manual', '2026-10-11', '2026-10', 999, 999, 0, 'other', 'pending')`, tp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period, subtotal_amount, tax_amount, total_amount, approval_status)
		VALUES ($1, 'test', 'z1', 'manual', '2026-10-31', '2026-10', 10000, 2000, 12000, 'approved'),
		       ($1, 'test', 'z2', 'manual', '2026-10-30', '2026-10', 3000, 600, 3600, 'pending')`, tp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO period_summaries (organization_id, engagement_id, taxpayer_id, period, facts, template, body, status)
		VALUES ($1, $2, $3, '2026-10', '[]', 't', 'Taslak yorum', 'draft')`, org, eng, tp); err != nil {
		t.Fatal(err)
	}
	q := db.New(d.Pool)
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

	rg, _ := ParseRange("2026-10", "")
	rep, err := Build(ctx, q, tp, rg, Options{OrganizationID: org, EngagementID: eng}, now)
	if err != nil {
		t.Fatal(err)
	}
	tot := rep.Totals
	// Revenue 50000 + 40000 − 5000 + 10000 = 95000; expenses 30000 − 2000 + 1000 + 500 = 29500.
	want := Totals{Revenue: "95000.00", Expenses: "29500.00", Net: "65500.00",
		VATCalculated: "19000.00", VATDeductible: "5800.00", VATPayable: "13200.00",
		SalesInvoices: 2, PurchaseInvoices: 1, ExpenseRecords: 2, RetailReports: 1}
	if tot != want {
		t.Fatalf("totals\n got %+v\nwant %+v", tot, want)
	}
	if rep.Previous.Revenue != "80000.00" || rep.Previous.Expenses != "20000.00" {
		t.Fatalf("previous: %+v", rep.Previous)
	}
	if rep.Change.Revenue == nil || *rep.Change.Revenue != "18.8" {
		t.Fatalf("revenue change: %v", rep.Change.Revenue)
	}
	if len(rep.Months) != 12 || rep.Months[11].Period != "2026-10" || rep.Months[10].Revenue != "80000.00" {
		t.Fatalf("months: %+v", rep.Months)
	}
	if len(rep.Categories) != 3 || rep.Categories[0].Key != "purchase_invoices" || rep.Categories[0].Amount != "28000.00" {
		t.Fatalf("categories: %+v", rep.Categories)
	}
	if rep.Label != "Ekim 2026" || rep.Taxpayer.TaxID == "" {
		t.Fatalf("header: %q %+v", rep.Label, rep.Taxpayer)
	}
	if rep.Commentary == nil || rep.Commentary.Status != "draft" || rep.Commentary.Body != "Taslak yorum" {
		t.Fatalf("commentary: %+v", rep.Commentary)
	}
	approvedOnly, err := Build(ctx, q, tp, rg, Options{OrganizationID: org, EngagementID: eng, ApprovedOnly: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if approvedOnly.Commentary != nil {
		t.Fatal("a draft comment must not reach the taxpayer")
	}

	yr, _ := ParseRange("", "2026")
	year, err := Build(ctx, q, tp, yr, Options{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if year.Totals.Revenue != "175000.00" || year.Totals.Expenses != "49500.00" || len(year.Months) != 12 || year.Change.Revenue != nil {
		t.Fatalf("year: %+v change %v", year.Totals, year.Change.Revenue)
	}
}
