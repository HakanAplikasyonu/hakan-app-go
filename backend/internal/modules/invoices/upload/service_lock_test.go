package upload

import (
	"context"
	"testing"

	periodsrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/repository"
	periodsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/usecase"
)

// Kabul (#32): kilitli döneme yüklenen fatura kaydı değiştirmiyor, talep
// oluşturuyor; aynı dosya tekrar yüklenince talep çoğalmıyor.
func TestUploadIntoLockedPeriodOpensChangeRequest(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.svc.WithPeriodGuard(periodsrepo.NewGuard())
	periods := periodsusecase.New(periodsrepo.New(e.pool))
	sub := periodsusecase.Subject{OrganizationID: e.scope.OrganizationID, EngagementID: e.scope.ID, TaxpayerID: e.scope.TaxpayerID}
	if _, err := periods.Lock(ctx, sub, "2026-03", "beyan verildi", e.actor); err != nil {
		t.Fatal(err)
	}
	xml, _ := makeInvoice(t, "temelfatura-satis.xml", 1)
	for round := 1; round <= 2; round++ {
		job, err := e.svc.Upload(ctx, e.scope, e.actor, "fatura.xml", xml)
		if err != nil {
			t.Fatal(err)
		}
		r := job.Report
		if r == nil || r.Total != 1 || r.Held != 1 || r.Added != 0 || r.Duplicate != 0 {
			t.Fatalf("round %d report: %+v", round, r)
		}
		if it := r.Items[0]; it.Status != ItemHeld || it.Outcome != "held" || it.ChangeRequestUUID == nil {
			t.Fatalf("round %d item: %+v", round, it)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 0 {
		t.Fatalf("invoices = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM change_requests WHERE taxpayer_id = $1 AND status = 'open'`); n != 1 {
		t.Fatalf("change requests = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM data_warnings WHERE taxpayer_id = $1 AND code = 'LOCKED_PERIOD_CHANGE' AND status = 'open'`); n != 1 {
		t.Fatalf("warnings = %d", n)
	}
}
