package easytrade

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// TestFinancialAndZ: a sync writes financial transactions (payment type,
// cari, invoice link) and Z reports (retail_sales), and the period views
// total them. Without Easytrade's Z endpoint the item is unsupported, not a
// failure.
func TestFinancialAndZ(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	f := testfakes.EasytradeSample()
	f.Expenses["902"] = map[string]any{"id": 902, "expense_date": "2026-10-07", "amount": "240.00", "tax_rate": map[string]any{"value": "20_kdv"},
		"description": "yakıt 34 XYZ 789", "payment_method_type": map[string]any{"value": "credit_card"}, "receipt_number": "F-0042",
		"customer": map[string]any{"name": "Petrol A.Ş.", "tax_number": "1234567890"}}
	srv := testfakes.Serve(t, f)
	kr, _ := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	svc := NewService(d.Pool, secrets.New(kr, secrets.NewPGStore(db.New(d.Pool))), srv.URL, srv.Client())
	svc.interval = 0
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	c, err := svc.Connect(ctx, tp, "a@b.c", testfakes.EasytradePassword, testfakes.EasytradeCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.RunScheduled(ctx, tp, c.UUID)
	if err != nil || res.Status != "success" || res.Counters.FinancialCreated != 5 || res.Counters.ZReportsCreated != 3 || len(res.Counters.Unsupported) != 0 {
		t.Fatalf("run: %+v %v", res, err)
	}

	fin, err := svc.FinancialTransactions(ctx, tp, c.UUID, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(fin.Items) != 5 || fin.InvoiceRefs != 3 || fin.InvoiceMatched != 3 {
		t.Fatalf("financial: %d items, %d/%d invoice refs matched", len(fin.Items), fin.InvoiceMatched, fin.InvoiceRefs)
	}
	totals := map[string]string{}
	for _, p := range fin.ByPaymentType {
		totals[p.PaymentMethodType+":"+p.Direction] = p.Total
	}
	if totals["cash:in"] != "160.00" || totals["credit_card:in"] != "330.00" || totals["bank_transfer:out"] != "300.00" {
		t.Fatalf("by payment type: %v", totals)
	}
	var ahmet, mehmet CustomerTotal
	for _, cu := range fin.ByCustomer {
		switch cu.CustomerName {
		case "Ahmet Yılmaz":
			ahmet = cu
		case "Mehmet Demir":
			mehmet = cu
		}
	}
	if ahmet.TotalIn != "200.00" || ahmet.CustomerTaxNumber != "12345678950" || mehmet.TotalIn != "290.00" || mehmet.Count != 2 {
		t.Fatalf("by customer: ahmet %+v mehmet %+v", ahmet, mehmet)
	}
	for _, it := range fin.Items {
		if it.ReferenceableType == "Invoice" && it.InvoiceUUID == nil {
			t.Fatalf("invoice reference not linked: %+v", it)
		}
	}

	z, err := svc.ZReports(ctx, tp, c.UUID, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(z.Items) != 3 || z.Total != "10500.00" || z.Payments["cash"] != "2950.50" || z.Payments["credit_card"] != "7549.50" || !z.Supported {
		t.Fatalf("Z: %+v", z)
	}
	var zn string
	_ = d.Pool.QueryRow(ctx, `SELECT string_agg(z_report_number, ',' ORDER BY sale_date) FROM retail_sales WHERE taxpayer_id = $1 AND source_system = 'easytrade'`, tp).Scan(&zn)
	if zn != "0041,0042,0043" {
		t.Fatalf("retail_sales z numbers %q", zn)
	}

	var pm, rn, vkn string
	_ = d.Pool.QueryRow(ctx, `SELECT COALESCE(payment_method,''), COALESCE(receipt_number,''), COALESCE(vendor_tax_number,'') FROM expenses WHERE taxpayer_id = $1 AND source_id = '902'`, tp).Scan(&pm, &rn, &vkn)
	if pm != "credit_card" || rn != "F-0042" || vkn != "1234567890" {
		t.Fatalf("detailed expense: %q %q %q", pm, rn, vkn)
	}

	// Easytrade without a Z endpoint: the item is unsupported, the run succeeds.
	f.ZReports = nil
	now = now.Add(5 * time.Hour)
	res, err = svc.RunScheduled(ctx, tp, c.UUID)
	if err != nil || res.Status != "success" || !slices.Contains(res.Counters.Unsupported, ItemZReports) || res.Counters.FinancialUnchanged != 5 {
		t.Fatalf("no Z endpoint: %+v %v", res, err)
	}
	if z, _ := svc.ZReports(ctx, tp, c.UUID, "2026-10"); z.Supported || len(z.Items) != 3 {
		t.Fatalf("Z after unsupported: %+v", z)
	}
}

func TestPaymentMethodType(t *testing.T) {
	for in, want := range map[string]string{
		"cash": "cash", "Nakit": "cash", "bank_transfer": "bank_transfer", "Havale": "bank_transfer", "EFT": "bank_transfer",
		"credit_card": "credit_card", "Kredi Kartı": "credit_card", "POS": "credit_card", "Çek": "check", "senet": "promissory_note",
		"credit": "credit", "": "unknown", "barter": "unknown",
	} {
		if got := PaymentMethodType(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	for flow, typ := range map[[2]string]string{{"", "income"}: "in", {"", "expense"}: "out", {"out", "income"}: "out", {"", "opening_balance"}: "unknown"} {
		if got := direction(flow[0], flow[1]); got != typ {
			t.Errorf("direction(%q, %q) = %q, want %q", flow[0], flow[1], got, typ)
		}
	}
}
