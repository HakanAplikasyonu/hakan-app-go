package inbox

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

type fakeQueries struct {
	rows   []db.ListInboxItemsRow
	counts db.CountInboxItemsRow
	got    db.ListInboxItemsParams
	gotC   db.CountInboxItemsParams
}

func (f *fakeQueries) ListInboxItems(_ context.Context, arg db.ListInboxItemsParams) ([]db.ListInboxItemsRow, error) {
	f.got = arg
	n := int(arg.Lim)
	if n > len(f.rows) {
		n = len(f.rows)
	}
	return f.rows[:n], nil
}

func (f *fakeQueries) CountInboxItems(_ context.Context, arg db.CountInboxItemsParams) (db.CountInboxItemsRow, error) {
	f.gotC = arg
	return f.counts, nil
}

func TestClamp(t *testing.T) {
	cases := []struct{ l, o, wl, wo int32 }{
		{0, 0, DefaultLimit, 0},
		{-5, -1, DefaultLimit, 0},
		{1, 3, 1, 3},
		{1000000, 99999, MaxLimit, MaxOffset},
	}
	for _, c := range cases {
		l, o := Clamp(c.l, c.o)
		if l != c.wl || o != c.wo {
			t.Errorf("Clamp(%d,%d) = %d,%d want %d,%d", c.l, c.o, l, o, c.wl, c.wo)
		}
	}
}

func TestValidType(t *testing.T) {
	for _, v := range append([]string{""}, Types...) {
		if !ValidType(v) {
			t.Errorf("%q should be valid", v)
		}
	}
	if ValidType("suggestion") {
		t.Error("suggestion is not an inbox type")
	}
}

func TestListPagesAndMapsRows(t *testing.T) {
	eng := uuid.New()
	fq := &fakeQueries{
		rows: []db.ListInboxItemsRow{
			{ItemType: TypeExpense, ItemUuid: uuid.New(), EngagementUuid: eng, TaxpayerTitle: "A", Amount: "10.00", Currency: "TRY", ItemDate: "2026-11-03"},
			{ItemType: TypeInvoice, ItemUuid: uuid.New(), EngagementUuid: eng, TaxpayerTitle: "A", Amount: "", Currency: "TRY", Reference: "ABC2026"},
			{ItemType: TypeBankTransaction, ItemUuid: uuid.New(), EngagementUuid: eng, TaxpayerTitle: "A"},
		},
		counts: db.CountInboxItemsRow{Invoice: 1, BankTransaction: 2, Expense: 3, RetailSale: 4, ForeignInvoice: 5, LateDocument: 6},
	}
	s := New(fq)
	s.now = func() time.Time { return time.Date(2026, 11, 30, 22, 0, 0, 0, time.UTC) } // 1 Dec in Istanbul
	out, err := s.List(context.Background(), Filter{OrganizationID: 7, Limit: 2, Engagement: &eng})
	if err != nil {
		t.Fatal(err)
	}
	if out.Period != "2026-12" || fq.got.Period != "2026-12" || fq.gotC.Period != "2026-12" {
		t.Fatalf("period = %q, want the Istanbul month 2026-12", out.Period)
	}
	if fq.got.Lim != 3 || fq.got.Cap != 3 || fq.got.Off != 0 {
		t.Fatalf("params = %+v", fq.got)
	}
	if !fq.got.EngagementUuid.Valid || fq.got.OrganizationID != 7 {
		t.Fatalf("engagement / org filter not passed: %+v", fq.got)
	}
	if len(out.Items) != 2 || out.NextCursor == nil || *out.NextCursor != "2" {
		t.Fatalf("items = %d, next = %v", len(out.Items), out.NextCursor)
	}
	first, second := out.Items[0], out.Items[1]
	if !first.CanApprove || first.Target.Tab != "expenses" || first.Amount == nil || *first.Amount != "10.00" {
		t.Fatalf("expense row = %+v", first)
	}
	if second.CanApprove || second.Target.Tab != "invoices" || second.Amount != nil || second.Date != nil {
		t.Fatalf("invoice row = %+v", second)
	}
	if out.Counts.Total != 21 {
		t.Fatalf("total = %d", out.Counts.Total)
	}
}

func TestListRejectsUnknownType(t *testing.T) {
	if _, err := New(&fakeQueries{}).List(context.Background(), Filter{Type: "x"}); err == nil {
		t.Fatal("want ErrInvalid")
	}
}
