package journals

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// #566: hesap planında olmayan kod, ana hesaba kayıt ve kilitli dönem taslak
// fişi engeller; liste engeli gösterir, onay reddedilir.
func TestDraftIssuesBlockApproval(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := rls.System(ctx, e.pool, "test: account plan", func(ctx context.Context, tx pgx.Tx) error {
		var imp int64
		if err := tx.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256)
			VALUES ($1, $2, 'xlsx', 'account_plan', repeat('a', 64)) RETURNING id`, e.scope.OrganizationID, e.scope.ID).Scan(&imp); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf)
			VALUES ($1, $2, $3, '770', 'GENEL YÖNETİM GİDERLERİ', '770', 1, false),
			       ($1, $2, $3, '770.01', 'KİRA', '770', 2, true),
			       ($1, $2, $3, '320.01', 'SATICI', '320', 2, true)`, e.scope.OrganizationID, e.scope.ID, imp)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	ok := e.draft(t, oct, debit("770.01", "100"), credit("320.01", "100"))
	bad := e.draft(t, oct, debit("770", "100"), credit("320.99", "100"))

	page, err := e.svc.ListDrafts(ctx, e.scope, DraftFilter{Period: "2026-10", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]DraftIssue{}
	for _, d := range page.Items {
		got[d.UUID.String()] = d.Issues
	}
	if len(got[ok.UUID.String()]) != 0 {
		t.Fatalf("clean draft issues %+v", got[ok.UUID.String()])
	}
	is := got[bad.UUID.String()]
	if len(is) != 2 || is[0].Code != IssueParentAccount || is[0].Account != "770" || is[1].Code != IssueUnknownAccount || is[1].Account != "320.99" {
		t.Fatalf("bad draft issues %+v", is)
	}

	if is[0].Blocking || is[1].Blocking {
		t.Fatalf("account issues are warnings, not blockers: %+v", is)
	}
	var blocked *DraftBlockedError

	// Kontrol ekranında hesaplar düzeltilir, engel kalkar, onay geçer.
	if err := e.svc.SetLineAccount(ctx, e.scope, bad.UUID, 1, "770.01"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetLineAccount(ctx, e.scope, bad.UUID, 2, "320.01"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetLineAccount(ctx, e.scope, bad.UUID, 2, "kasa"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid code err = %v", err)
	}
	det, err := e.svc.GetDraft(ctx, e.scope, bad.UUID)
	if err != nil || len(det.Issues) != 0 {
		t.Fatalf("fixed draft issues %+v err %v", det.Issues, err)
	}
	e.approve(t, bad)
	if err := e.svc.SetLineAccount(ctx, e.scope, bad.UUID, 1, "770"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("approved draft line change err = %v", err)
	}

	// Kilitli dönemde temiz fiş de onaylanmaz.
	err = rls.System(ctx, e.pool, "test: lock", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO period_locks (organization_id, engagement_id, taxpayer_id, period) VALUES ($1, $2, $3, '2026-10')`,
			e.scope.OrganizationID, e.scope.ID, e.scope.TaxpayerID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Approve(ctx, e.scope, e.actor, ok.UUID); !errors.As(err, &blocked) || blocked.Issues[0].Code != IssuePeriodLocked || !blocked.Issues[0].Blocking {
		t.Fatalf("approve in locked period err = %v", err)
	}
}
