package kolaybi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// dataset: 40 sale invoices, 6 sale returns, 12 purchase invoices and 5
// general expenses in October, plus September records; it returns the
// October count and grand total per direction a KolayBi user would see.
func dataset() (*testfakes.KolayBi, map[string]int, map[string]decimal.Decimal) {
	f := &testfakes.KolayBi{Docs: map[string][]map[string]any{}}
	n := map[string]int{}
	sum := map[string]decimal.Decimal{}
	id := 0
	add := func(kind string, count int, dir string) {
		for i := 1; i <= count+2; i++ {
			id++
			date := fmt.Sprintf("2026-10-%02d", i%28+1)
			if i > count {
				date = "2026-09-15"
			}
			qty, price := decimal.NewFromInt(int64(i)), decimal.RequireFromString("12.50")
			net := qty.Mul(price)
			vat := net.Mul(decimal.NewFromInt(20)).Div(decimal.NewFromInt(100)).Round(2)
			d := map[string]any{"id": id, "serial_no": fmt.Sprintf("KB%07d", id), "order_date": date, "currency": "TRY",
				"document_type": "SATIS", "document_scenario": "EARSIVFATURA", "description": "kayıt",
				"associate":   map[string]any{"name": "Yıldız Market", "identity_no": "9100000021", "is_corporate": true},
				"items":       []map[string]any{{"description": "Ürün", "quantity": qty.String(), "unit_price": price.String(), "vat_rate": 20}},
				"grand_total": net.Add(vat).StringFixed(2), "total_vat": vat.StringFixed(2)}
			if kind == "general_expense" {
				d["description"] = "34 KB 1234 akaryakıt"
			}
			f.Docs[kind] = append(f.Docs[kind], d)
			if i <= count {
				n[dir]++
				sum[dir] = sum[dir].Add(net.Add(vat))
			}
		}
	}
	add("sale_invoice", 40, "outgoing")
	add("sale_return_invoice", 6, "outgoing")
	add("purchase_invoice", 12, "incoming")
	add("general_expense", 5, "expense")
	return f, n, sum
}

// TestAcceptance is #48's acceptance: counts and totals equal KolayBi's.
func TestAcceptance(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	f, wantN, wantSum := dataset()
	srv := testfakes.Serve(t, f)
	kr, _ := secrets.ParseKeyring("", "", "test-legacy-key-32-bytes-long!!!")
	vault := secrets.New(kr, secrets.NewPGStore(db.New(d.Pool)))

	if _, _, err := NewService(d.Pool, vault, App{BaseURL: srv.URL}, srv.Client()).Connect(ctx, tp, "key-1", ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no channel: %v", err)
	}
	svc := NewService(d.Pool, vault, App{BaseURL: srv.URL, Channel: "HAKAN"}, srv.Client())
	svc.interval = 0
	if _, _, err := svc.Connect(ctx, tp, "wrong", ""); !errors.Is(err, ErrAuth) {
		t.Fatalf("bad key: %v", err)
	}
	c, cos, err := svc.Connect(ctx, tp, "key-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cos) != 2 || c.SetupState != "credentials_entered" {
		t.Fatalf("two companies must be chosen: %+v", c)
	}
	if _, err := svc.SelectCompany(ctx, tp, c.UUID, "99"); !errors.Is(err, ErrUnknownCo) {
		t.Fatalf("foreign company: %v", err)
	}
	if c, err = svc.SelectCompany(ctx, tp, c.UUID, "5"); err != nil || c.SetupState != "active" || c.CompanyName != "Atlas Gıda" {
		t.Fatalf("select: %+v %v", c, err)
	}
	var metaText string
	_ = d.Pool.QueryRow(ctx, `SELECT meta::text FROM source_connections WHERE uuid = $1`, c.UUID).Scan(&metaText)
	if strings.Contains(metaText, "key-1") {
		t.Fatal("api key outside the vault")
	}

	f.Expired = true // token expired: a new one is taken with the api key
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	rep, err := svc.Sync(ctx, tp, c.UUID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if rep.InvoicesCreated != wantN["outgoing"]+wantN["incoming"] || rep.ExpensesCreated != wantN["expense"] {
		t.Fatalf("report: %+v want %v", rep, wantN)
	}
	for _, dir := range []string{"outgoing", "incoming"} {
		var n int
		var s string
		_ = d.Pool.QueryRow(ctx, `SELECT count(*), sum(payable_amount)::numeric(18,2)::text FROM invoices
			WHERE taxpayer_id = $1 AND source_system = 'kolaybi' AND direction = $2 AND period = '2026-10'`, tp, dir).Scan(&n, &s)
		if n != wantN[dir] || s != wantSum[dir].StringFixed(2) {
			t.Errorf("%s: %d / %s, KolayBi %d / %s", dir, n, s, wantN[dir], wantSum[dir].StringFixed(2))
		}
	}
	var en int
	var es, plate string
	_ = d.Pool.QueryRow(ctx, `SELECT count(*), sum(amount)::numeric(18,2)::text, max(COALESCE(vehicle_plate,'')) FROM expenses WHERE taxpayer_id = $1 AND source_system = 'kolaybi'`, tp).Scan(&en, &es, &plate)
	if en != wantN["expense"] || es != wantSum["expense"].StringFixed(2) || plate != "34KB1234" {
		t.Errorf("expenses: %d / %s %q, KolayBi %d / %s", en, es, plate, wantN["expense"], wantSum["expense"].StringFixed(2))
	}
	var iade int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1 AND type = 'IADE'`, tp).Scan(&iade)
	if iade != 6 {
		t.Errorf("returns: %d", iade)
	}

	svc.now = func() time.Time { return time.Now().Add(SyncThrottle + time.Minute) }
	rep, err = svc.Sync(ctx, tp, c.UUID, from, to)
	if err != nil || rep.InvoicesCreated != 0 || rep.ExpensesCreated != 0 {
		t.Fatalf("second sync: %+v %v", rep, err)
	}
}
