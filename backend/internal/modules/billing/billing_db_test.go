package billing

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestMonthlyActiveTaxpayers is #52's acceptance: the monthly active
// taxpayer count is right on the month's edges (Türkiye time).
func TestMonthlyActiveTaxpayers(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	org, other := d.Organization(t), d.Organization(t)
	svc := NewService(d.Pool)

	// eng opens an engagement over [start, end) (end "" = still active).
	eng := func(orgID, tp int64, start, end string) {
		id := d.Engagement(t, orgID, tp)
		if _, err := d.Pool.Exec(ctx, `UPDATE engagements SET started_at = $2::timestamptz WHERE id = $1`, id, start); err != nil {
			t.Fatal(err)
		}
		if end != "" {
			if _, err := d.Pool.Exec(ctx, `UPDATE engagements SET status = 'ended', ended_at = $2::timestamptz WHERE id = $1`, id, end); err != nil {
				t.Fatal(err)
			}
		}
	}
	a := d.Taxpayer(t, "legal", "9000000101") // since September, active
	b := d.Taxpayer(t, "legal", "9000000102") // joined 20 October
	c := d.Taxpayer(t, "legal", "9000000103") // left 2 October
	e := d.Taxpayer(t, "legal", "9000000104") // left 30 Sep 23:30 TR (= 20:30 UTC)
	f := d.Taxpayer(t, "legal", "9000000105") // joins 1 November 00:10 TR (= 31 Oct 21:10 UTC)
	g := d.Taxpayer(t, "legal", "9000000106") // left 5 October, came back 20 October
	h := d.Taxpayer(t, "legal", "9000000107") // another office's taxpayer
	eng(org, a, "2026-09-10T09:00:00+03:00", "")
	eng(org, b, "2026-10-20T09:00:00+03:00", "")
	eng(org, c, "2026-08-01T09:00:00+03:00", "2026-10-02T12:00:00+03:00")
	eng(org, e, "2026-08-01T09:00:00+03:00", "2026-09-30T23:30:00+03:00")
	eng(org, f, "2026-11-01T00:10:00+03:00", "")
	eng(org, g, "2026-01-01T09:00:00+03:00", "2026-10-05T09:00:00+03:00")
	eng(org, g, "2026-10-20T09:00:00+03:00", "")
	eng(other, h, "2026-01-01T09:00:00+03:00", "")

	u, err := svc.Usage(ctx, org, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if u.ActiveTaxpayers != 4 { // a, b, c, g (g once)
		t.Fatalf("October: %d %+v", u.ActiveTaxpayers, u.Taxpayers)
	}
	// Inside a tenant request scope of a member who sees no engagement, the
	// bill still counts the whole office (issue #166).
	err = rls.WithScope(ctx, d.Pool, rls.Scope{OrganizationID: org, UserID: d.User(t)}, func(ctx context.Context, _ pgx.Tx) error {
		u, err := svc.Usage(ctx, org, "2026-10")
		if err != nil {
			return err
		}
		if u.ActiveTaxpayers != 4 {
			t.Errorf("October in a member's scope: %d, want 4", u.ActiveTaxpayers)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := svc.Usage(ctx, org, "2026-09"); u.ActiveTaxpayers != 4 { // a, c, e, g
		t.Fatalf("September: %+v", u)
	}
	if u, _ := svc.Usage(ctx, org, "2026-11"); u.ActiveTaxpayers != 4 { // a, b, f, g
		t.Fatalf("November: %+v", u)
	}

	// Priced plan: 1 taxpayer included in the base.
	if _, err := svc.SavePlan(ctx, Plan{Code: "pro", Name: "Pro", BaseFee: d2("1000"), PerTaxpayerFee: d2("50"), IncludedTaxpayers: 1, VATRate: d2("20"), Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetAccount(ctx, org, "pro", false, "", 0); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Generate(ctx, org, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "draft" || st.Amounts.ActiveTaxpayers != 4 || st.Amounts.BillableTaxpayers != 3 || st.Amounts.Total.String() != "1380" || len(st.Taxpayers) != 4 {
		t.Fatalf("statement: %+v", st)
	}
	if _, err := svc.SetStatus(ctx, org, "2026-10", "final"); err != nil {
		t.Fatal(err)
	}
	// A final statement stays as it was, even if the plan changes.
	if _, err := svc.Generate(ctx, org, "2026-10"); !errors.Is(err, ErrFinal) {
		t.Fatalf("regenerate final: %v", err)
	}
	// The default plan (unpriced) counts but charges nothing.
	if st, err := svc.Generate(ctx, other, "2026-10"); err != nil || st.PlanCode != "standart" || st.Amounts.ActiveTaxpayers != 1 || !st.Amounts.Total.IsZero() {
		t.Fatalf("default plan: %+v %v", st, err)
	}

	// AccessPolicy: read-only mode refuses writes, lets reads through.
	scope := orgctx.Scope{ID: org}
	if err := svc.Check(ctx, scope, httptest.NewRequest("POST", "/v1/tenant/x", nil)); err != nil {
		t.Fatalf("write before read-only: %v", err)
	}
	if _, err := svc.SetAccount(ctx, org, "pro", true, "Ödeme bekleniyor", 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.Check(ctx, scope, httptest.NewRequest("POST", "/v1/tenant/x", nil)); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("write in read-only: %v", err)
	}
	if err := svc.Check(ctx, scope, httptest.NewRequest("GET", "/v1/tenant/x", nil)); err != nil {
		t.Fatalf("read in read-only: %v", err)
	}
	if err := svc.Check(ctx, orgctx.Scope{ID: other}, httptest.NewRequest("DELETE", "/v1/tenant/x", nil)); err != nil {
		t.Fatalf("other office: %v", err)
	}
	ov, err := svc.Overview(ctx, org, time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	if err != nil || !ov.Account.ReadOnly || ov.Current.ActiveTaxpayers != 4 || len(ov.Statements) != 1 {
		t.Fatalf("overview: %+v %v", ov, err)
	}
}
