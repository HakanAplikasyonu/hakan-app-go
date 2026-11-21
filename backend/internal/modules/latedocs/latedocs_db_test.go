package latedocs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestLateDocuments: kapanmış döneme (kilit ya da verilmiş beyan) kapanıştan
// sonra gelen fatura, masraf ve Z raporu listelenir; kapanıştan önce gelen,
// iptal edilen ve açık döneme gelen belge listelenmez. Tarama bir belgeyi bir
// kez yazar; ofisin kararı kaydedilir ve geri alınır; ofis geneli sayaç
// atamaya uyar (#780 C).
func TestLateDocuments(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	tp := d.Taxpayer(t, "legal", "9000000780")
	org := d.Organization(t)
	eng := d.Engagement(t, org, tp)
	user := d.User(t)
	sub := Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	svc := New(q).WithClock(func() time.Time { return time.Date(2026, 11, 21, 10, 0, 0, 0, Istanbul) })

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	invoice := func(src, number, status, created string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := d.Pool.QueryRow(ctx, `
			INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type,
			                      issue_date, period, currency, customer_name, payable_amount, tax_summary_source,
			                      status_normalized, created_at)
			VALUES ($1, 'izibiz', $2, 'api', $3, 'incoming', 'ALIS', '2026-09-28', '2026-09', 'TRY', 'Deniz Lojistik',
			        4800, 'items', $4, $5::timestamptz)
			RETURNING uuid`, tp, src, number, status, created).Scan(&id); err != nil {
			t.Fatalf("invoice: %v", err)
		}
		return id
	}

	// Eylül 2026 3 Kasım'da kilitlendi; Ekim 2026'nın KDV1'i 20 Kasım'da verildi.
	exec(`INSERT INTO period_locks (organization_id, engagement_id, taxpayer_id, period, locked_at)
	      VALUES ($1, $2, $3, '2026-09', '2026-11-03 10:00+03')`, org, eng, tp)
	exec(`INSERT INTO tax_returns (organization_id, engagement_id, kind, period, status, snapshot, filed_on, filed_at)
	      VALUES ($1, $2, 'kdv1', '2026-10', 'filed', '{}'::jsonb, '2026-11-20', '2026-11-20 11:00+03')`, org, eng)

	invoice("ON", "GEC2026000000001", "active", "2026-10-02 09:00+03")           // kapanıştan önce
	late := invoice("LATE", "GEC2026000000002", "active", "2026-11-18 09:00+03") // geç
	invoice("CANCEL", "GEC2026000000003", "cancelled", "2026-11-18 09:00+03")    // iptal
	exec(`INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date, period, amount,
	                            description, vendor_name, created_at)
	      VALUES ($1, 'manual', 'X1', 'manual', '2026-09-30', '2026-09', 350, 'Kırtasiye', 'Ofis Kırtasiye', '2026-11-19 12:00+03'),
	             ($1, 'manual', 'X2', 'manual', '2026-08-30', '2026-08', 120, 'Taksi', NULL, '2026-11-19 12:00+03')`, tp)
	exec(`INSERT INTO retail_sales (taxpayer_id, source_system, source_id, capture_method, sale_date, period,
	                                z_report_number, total_amount, created_at)
	      VALUES ($1, 'easytrade', 'Z1', 'api', '2026-10-31', '2026-10', '0412', 9600, '2026-11-21 08:00+03')`, tp)

	list, err := svc.List(ctx, sub, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 3 || list.Counts.Pending != 3 {
		t.Fatalf("items = %d, counts = %+v", len(list.Items), list.Counts)
	}
	byType := map[string]Item{}
	for _, it := range list.Items {
		byType[it.DocumentType] = it
	}
	inv := byType["invoice"]
	if inv.DocumentUUID != late || inv.Period != "2026-09" || inv.CloseReason != "lock" || inv.Label != "GEC2026000000002" {
		t.Fatalf("invoice row = %+v", inv)
	}
	if want := time.Date(2026, 11, 3, 10, 0, 0, 0, Istanbul); !inv.ClosedAt.Equal(want) {
		t.Fatalf("closed_at = %v, want %v", inv.ClosedAt, want)
	}
	if want := time.Date(2026, 11, 18, 9, 0, 0, 0, Istanbul); !inv.ArrivedAt.Equal(want) {
		t.Fatalf("arrived_at = %v, want %v", inv.ArrivedAt, want)
	}
	if inv.Amount != "4800.00" || inv.InvoiceDirection == nil || *inv.InvoiceDirection != "incoming" {
		t.Fatalf("invoice amount/direction = %+v", inv)
	}
	if z := byType["retail_sale"]; z.CloseReason != "filed" || z.Period != "2026-10" || z.Label != "0412" {
		t.Fatalf("z row = %+v", z)
	}
	if x := byType["expense"]; x.Period != "2026-09" || x.Counterparty == nil || *x.Counterparty != "Ofis Kırtasiye" {
		t.Fatalf("expense row = %+v", x)
	}

	// İkinci tarama yeni satır yazmaz.
	if n, err := svc.Scan(ctx, sub); err != nil || n != 0 {
		t.Fatalf("rescan = %d, %v", n, err)
	}

	// Belgenin uyarısı: document_uuid süzgeci.
	one, err := svc.List(ctx, sub, Filter{DocumentUUID: &late})
	if err != nil || len(one.Items) != 1 {
		t.Fatalf("document filter = %+v, %v", one, err)
	}

	// Karar: cari döneme alındı (varsayılan bu ay), geçersiz ay, geri alma.
	got, err := svc.Decide(ctx, sub, inv.UUID, Decision{Status: StatusCurrentPeriod}, user)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusCurrentPeriod || got.TargetPeriod == nil || *got.TargetPeriod != "2026-11" ||
		got.DecidedAt == nil || got.DecidedByName == nil {
		t.Fatalf("decided = %+v", got)
	}
	if _, err := svc.Decide(ctx, sub, inv.UUID, Decision{Status: StatusCurrentPeriod, TargetPeriod: "2026-08"}, user); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past target err = %v", err)
	}
	if _, err := svc.Decide(ctx, sub, uuid.New(), Decision{Status: StatusIgnored}, user); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown err = %v", err)
	}
	if _, err := svc.Decide(ctx, sub, byType["expense"].UUID, Decision{Status: StatusCorrection, Note: "Eylül KDV1 düzeltmesi"}, user); err != nil {
		t.Fatal(err)
	}
	list, err = svc.List(ctx, sub, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Counts != (Counts{Pending: 1, Correction: 1, CurrentPeriod: 1}) {
		t.Fatalf("counts = %+v", list.Counts)
	}
	back, err := svc.Decide(ctx, sub, inv.UUID, Decision{Status: StatusPending}, user)
	if err != nil || back.Status != StatusPending || back.TargetPeriod != nil || back.DecidedAt != nil {
		t.Fatalf("undo = %+v, %v", back, err)
	}
	pending, err := svc.List(ctx, sub, Filter{Status: StatusPending, Period: "2026-09"})
	if err != nil || len(pending.Items) != 1 || pending.Items[0].UUID != inv.UUID {
		t.Fatalf("pending filter = %+v, %v", pending, err)
	}

	// Ofis geneli sayaç: read_all hepsini görür; atanmamış kullanıcı görmez.
	all, err := svc.Summary(ctx, org, true, user)
	if err != nil {
		t.Fatal(err)
	}
	if all.Pending != 2 || all.Total != 3 || len(all.Items) != 1 || all.Items[0].OldestPendingPeriod == nil ||
		*all.Items[0].OldestPendingPeriod != "2026-09" {
		t.Fatalf("summary = %+v", all)
	}
	mine, err := svc.Summary(ctx, org, false, user)
	if err != nil || len(mine.Items) != 0 {
		t.Fatalf("unassigned summary = %+v, %v", mine, err)
	}
	d.Member(t, org, user, "member")
	d.Assign(t, org, eng, user)
	mine, err = svc.Summary(ctx, org, false, user)
	if err != nil || mine.Pending != 2 {
		t.Fatalf("assigned summary = %+v, %v", mine, err)
	}

	// Başka ofisin kaydı bulunamaz.
	other := Subject{OrganizationID: d.Organization(t), EngagementID: eng, TaxpayerID: tp}
	if _, err := svc.Decide(ctx, other, inv.UUID, Decision{Status: StatusIgnored}, user); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other office err = %v", err)
	}
}
