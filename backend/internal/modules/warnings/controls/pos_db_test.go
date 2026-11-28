package controls

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// posFixture is one taxpayer month (uydurma veri): Z reports with card
// sales and the bank's POS collections.
type posFixture struct {
	d    *testdb.DB
	t    *testing.T
	subj engine.Subject
	box  int64
	n    int
}

func newPOSFixture(t *testing.T, d *testdb.DB) *posFixture {
	ctx := context.Background()
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", fmt.Sprintf("%010d", 4100000000+testdb.Next()))
	eng := d.Engagement(t, org, tp)
	f := &posFixture{d: d, t: t, subj: engine.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}}
	if err := d.Pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Örnek POS Banka', 'TRY') RETURNING id`, tp).Scan(&f.box); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *posFixture) z(date, card string) {
	f.t.Helper()
	f.n++
	if _, err := f.d.Pool.Exec(context.Background(), `INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method,
		sale_date, period, z_report_number, okc_serial_number, total_amount, card_amount, cash_amount)
		VALUES ($1, 'test', $2::text, 'manual', $3::date, to_char($3::date, 'YYYY-MM'), $4::text, 'AB00000001', $5::numeric, $5::numeric, 0)`,
		f.subj.TaxpayerID, fmt.Sprintf("z-%d", f.n), date, fmt.Sprintf("%04d", f.n), card); err != nil {
		f.t.Fatal(err)
	}
}

// bank inserts a movement; detail is its kind_detail.
func (f *posFixture) bank(date, dc, amount, detail string) int64 {
	f.t.Helper()
	f.n++
	var id int64
	if err := f.d.Pool.QueryRow(context.Background(), `INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id,
		capture_method, booked_at, transaction_date, debit_credit, amount, currency, description, period, kind, kind_detail)
		VALUES ($1, $2, 'bank_test', $3::text, 'api', $4::date, $4::date, $5, $6::numeric, 'TRY', 'POS GUN SONU', to_char($4::date, 'YYYY-MM'),
			CASE WHEN $7::text = 'pos_settlement' THEN 'UYE_ISYERI_ISLEMLERI' ELSE 'DIGER' END, $7::text)
		RETURNING id`, f.subj.TaxpayerID, f.box, fmt.Sprintf("b-%d", f.n), date, dc, amount, detail).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// commission records the POS commission the accountant entered (#565).
func (f *posFixture) commission(txn int64, amount string) {
	f.t.Helper()
	if _, err := f.d.Pool.Exec(context.Background(), `INSERT INTO bank_transaction_postings (organization_id, engagement_id, taxpayer_id,
		bank_transaction_id, account_code, account_name, status, confidence, source, reason, approved_at, commission_amount, commission_account_code)
		VALUES ($1, $2, $3, $4, '108.01', 'POS Kredi Kartı', 'approved', 1, 'manual', '', now(), $5::numeric, '653.01')`,
		f.subj.OrganizationID, f.subj.EngagementID, f.subj.TaxpayerID, txn, amount); err != nil {
		f.t.Fatal(err)
	}
}

// open returns the params of the open BNK_POS_VS_SALES warning of 2026-10,
// nil when there is none.
func (f *posFixture) open() map[string]any {
	f.t.Helper()
	var raw []byte
	err := f.d.Pool.QueryRow(context.Background(), `SELECT params FROM data_warnings
		WHERE engagement_id = $1 AND entity_type = 'period' AND period = '2026-10' AND code = 'BNK_POS_VS_SALES' AND status = 'open'`,
		f.subj.EngagementID).Scan(&raw)
	if err != nil {
		return nil
	}
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// TestPOSVsSalesAgainstPostgres: müşavirin örneği. Z raporlarında kartla
// 64.000 TL satış, bankaya 62.000 TL POS tahsilatı girdi: fark 2.000 TL,
// eşik (100 TL ile %1 = 640 TL'nin büyüğü) üstünde, uyarı çıkar. Müşavir
// harekete 2.000 TL komisyon girince fark açıklanır, uyarı kendiliğinden
// kapanır. Eşik altı fark (500 TL) uyarı vermez.
func TestPOSVsSalesAgainstPostgres(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	eng := engine.New(repository.NewWarnings(q), repository.NewConfig(q))
	check := NewPOSCheck(q, eng)

	// 64.000 / 62.000.
	f := newPOSFixture(t, d)
	f.z("2026-10-10", "30000")
	f.z("2026-10-20", "34000")
	f.bank("2026-10-01", "A", "5000", "pos_settlement") // eylülün satışı: pencere dışında (valör 1 gün)
	a := f.bank("2026-10-11", "A", "29000", "pos_settlement")
	f.bank("2026-10-21", "A", "33000", "pos_settlement")
	f.bank("2026-11-02", "B", "150", "unknown") // ekstre ayın sonrasına uzanıyor
	rep, err := check.Check(ctx, f.subj, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	p := f.open()
	if rep.Created != 1 || p == nil {
		t.Fatalf("64.000 / 62.000: no warning (%+v)", rep)
	}
	for k, want := range map[string]any{
		"z_card_total": "64000.00", "bank_gross": "62000.00", "bank_commission": "0.00", "difference": "2000.00",
		"threshold": "640.00", "bank_from": "2026-10-02", "bank_to": "2026-11-01",
	} {
		if p[k] != want {
			t.Errorf("param %s = %v, want %v", k, p[k], want)
		}
	}
	// Kontrolleri çalıştır da aynı uyarıyı tazeler, kopya açmaz.
	res, err := New(q, eng).Run(ctx, f.subj, "2026-10")
	if err != nil || res.Report.Created != 0 {
		t.Fatalf("controls run: %+v %v", res.Report, err)
	}

	// Komisyon girildi: 62.000 net + 2.000 komisyon = 64.000 brüt.
	f.commission(a, "2000")
	rep, err = check.Check(ctx, f.subj, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if rep.AutoResolved != 1 || f.open() != nil {
		t.Fatalf("commission explains the gap: %+v %v", rep, f.open())
	}

	// Komisyonla açıklanan fark baştan uyarı vermez.
	g := newPOSFixture(t, d)
	g.z("2026-10-15", "64000")
	g.commission(g.bank("2026-10-16", "A", "62080", "pos_settlement"), "1920")
	g.bank("2026-11-05", "B", "10", "unknown")
	if rep, err := check.Check(ctx, g.subj, "2026-10"); err != nil || rep.Created != 0 || g.open() != nil {
		t.Fatalf("commission entered: %+v %v", rep, err)
	}

	// Eşik altı: 500 TL fark, %1 = 640 TL'nin altında.
	h := newPOSFixture(t, d)
	h.z("2026-10-15", "64000")
	h.bank("2026-10-16", "A", "63500", "pos_settlement")
	h.bank("2026-11-05", "B", "10", "unknown")
	if rep, err := check.Check(ctx, h.subj, "2026-10"); err != nil || rep.Created != 0 || h.open() != nil {
		t.Fatalf("below threshold: %+v %v", rep, err)
	}

	// Ekstre ayın son gününün ödemesine ulaşmıyor: henüz karşılaştırılmaz.
	k := newPOSFixture(t, d)
	k.z("2026-10-15", "64000")
	k.bank("2026-10-16", "A", "40000", "pos_settlement")
	if rep, err := check.Check(ctx, k.subj, "2026-10"); err != nil || rep.Created != 0 || k.open() != nil {
		t.Fatalf("statement incomplete: %+v %v", rep, err)
	}
}
