package journals

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// #574: ofis genelinde bekleyen aktarımlar ve son aktarımların mizan
// durumu; atanmamış personel başka mükellefi görmez.
func TestOfficeTransferOverview(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	all := OfficeScope{OrganizationID: e.scope.OrganizationID, UserID: e.actor, ReadAll: true, Reconciliation: true}

	pending := func(sc OfficeScope) []PendingTransfer {
		t.Helper()
		p, err := e.svc.ListOfficePending(ctx, sc, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != int64(len(p.Items)) {
			t.Fatalf("total %d for %d items", p.Total, len(p.Items))
		}
		return p.Items
	}
	if got := pending(all); len(got) != 0 {
		t.Fatalf("empty office %+v", got)
	}

	// Onaylı, çıktısı alınmamış fiş.
	d := e.draft(t, oct, LineInput{Account: "770.01", Debit: dec("100")}, LineInput{Account: "320.01", Credit: dec("100")})
	e.approve(t, d)
	// Taslak sayılmaz.
	e.draft(t, oct, LineInput{Account: "770.01", Debit: dec("50")}, LineInput{Account: "320.01", Credit: dec("50")})
	got := pending(all)
	if len(got) != 1 || got[0].Period != "2026-10" || got[0].Approved != 1 || got[0].InTransfer != 0 || got[0].TaxpayerTitle == "" {
		t.Fatalf("approved %+v", got)
	}

	// Yalnız indirilen çıktı bekleyen sayılmaz; ajana gönderilince
	// "aktarımda" olur.
	b := e.exported(t, "luca")
	if got := pending(all); len(got) != 1 || got[0].Approved != 1 || got[0].InTransfer != 0 {
		t.Fatalf("downloaded only %+v", got)
	}
	tr, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := pending(all); len(got) != 1 || got[0].Approved != 1 || got[0].InTransfer != 1 {
		t.Fatalf("in transfer %+v", got)
	}
	list := func(sc OfficeScope, status string) []OfficeTransfer {
		t.Helper()
		p, err := e.svc.ListOfficeTransfers(ctx, sc, status, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return p.Items
	}
	if got := list(all, ""); len(got) != 1 || got[0].UUID != tr.UUID || got[0].Status != TransferQueued || got[0].Check != nil || got[0].Period != "2026-10" {
		t.Fatalf("queued transfer %+v", got)
	}

	// Programa alındı: bekleyenden düşer; mizan karşılaştırılmadı.
	ag := e.agent(t, "machine-0574")
	if _, err := e.svc.ClaimTransfer(ctx, e.scope, tr.UUID, ag); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ReportDelivery(ctx, e.scope, tr.UUID, DeliveryInput{AgentUUID: ag, Outcome: TransferDelivered,
		Files: []TransferFile{{Name: tr.Files[0].Name, SHA256: tr.Files[0].SHA256, Size: tr.Files[0].Size}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CompleteTransfer(ctx, e.scope, e.actor, tr.UUID, CompleteInput{Outcome: TransferImported}); err != nil {
		t.Fatal(err)
	}
	if got := pending(all); len(got) != 1 || got[0].InTransfer != 0 || got[0].Approved != 1 {
		t.Fatalf("after import %+v", got)
	}
	if got := list(all, TransferImported); len(got) != 1 || got[0].Check == nil || got[0].Check.Status != CheckNone {
		t.Fatalf("imported, never compared %+v", got)
	}
	if got := list(all, TransferQueued); len(got) != 0 {
		t.Fatalf("status filter %+v", got)
	}

	// Aktarımdan önceki karşılaştırma eski sayılır; sonrakinde fark sayısı.
	run := func(age, summary string) {
		t.Helper()
		err := rls.System(ctx, e.pool, "test: reconciliation run", func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO reconciliation_runs (organization_id, engagement_id, taxpayer_id, period,
				window_from, window_to, summary, created_at)
				VALUES ($1, $2, $3, '2026-10', '2026-10', '2026-10', $4::jsonb, clock_timestamp() - $5::interval)`,
				e.scope.OrganizationID, e.scope.ID, e.scope.TaxpayerID, summary, age)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	run("1 day", `{"differences":0}`)
	if got := list(all, ""); got[0].Check == nil || got[0].Check.Status != CheckStale {
		t.Fatalf("stale %+v", got[0].Check)
	}
	run("0 seconds", `{"differences":2,"missing_in_ledger":1,"missing_in_sources":0}`)
	if got := list(all, ""); got[0].Check == nil || got[0].Check.Status != CheckDifferences || got[0].Check.Issues != 3 {
		t.Fatalf("differences %+v", got[0].Check)
	}
	// Mutabakat izni olmayan mizan durumunu görmez.
	noRec := all
	noRec.Reconciliation = false
	if got := list(noRec, ""); got[0].Check != nil {
		t.Fatalf("check without permission %+v", got[0].Check)
	}

	// read_all olmayan personel yalnız atandığı mükellefi görür.
	d2 := &testdb.DB{Pool: e.pool}
	staff := d2.User(t)
	d2.Member(t, e.scope.OrganizationID, staff, "member")
	own := OfficeScope{OrganizationID: e.scope.OrganizationID, UserID: staff}
	if got := pending(own); len(got) != 0 {
		t.Fatalf("unassigned staff pending %+v", got)
	}
	if got := list(own, ""); len(got) != 0 {
		t.Fatalf("unassigned staff transfers %+v", got)
	}
	d2.Assign(t, e.scope.OrganizationID, e.scope.ID, staff)
	if got := pending(own); len(got) != 1 {
		t.Fatalf("assigned staff pending %+v", got)
	}
	// Başka ofis hiçbir şey görmez.
	other := OfficeScope{OrganizationID: d2.Organization(t), UserID: e.actor, ReadAll: true}
	if got := pending(other); len(got) != 0 {
		t.Fatalf("other office %+v", got)
	}

	// HTTP: sayfalama ve hatalı durum.
	h := NewHandler(e.svc, e.store)
	if w := e.do(t, h.listOfficePending, "GET", "", nil); w.Code != http.StatusOK || len(data[PendingTransferPage](t, w).Items) != 1 {
		t.Fatalf("pending http %d %s", w.Code, w.Body)
	}
	if w := e.do(t, h.listOfficeTransfers, "GET", "", nil); w.Code != http.StatusOK || data[OfficeTransferPage](t, w).Total != 1 {
		t.Fatalf("transfers http %d %s", w.Code, w.Body)
	}
}

func TestOfficeTransfersRejectsUnknownStatus(t *testing.T) {
	e := setup(t)
	h := NewHandler(e.svc, e.store)
	r := e.do(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		q.Set("status", "bogus")
		r.URL.RawQuery = q.Encode()
		h.listOfficeTransfers(w, r)
	}, "GET", "", nil)
	if r.Code != http.StatusBadRequest {
		t.Fatalf("unknown status %d %s", r.Code, r.Body)
	}
}
