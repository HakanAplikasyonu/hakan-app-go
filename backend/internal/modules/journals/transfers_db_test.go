package journals

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// agent registers a desktop agent of the test office.
func (e env) agent(t *testing.T, machine string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := rls.System(context.Background(), e.pool, "test: desktop agent", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO desktop_agents (organization_id, machine_id, name, platform)
			VALUES ($1, $2, 'Ofis PC', 'windows') RETURNING uuid`, e.scope.OrganizationID, machine).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) exported(t *testing.T, driver string) BatchView {
	t.Helper()
	d := e.draft(t, oct, LineInput{Account: "770.01", Debit: dec("100")}, LineInput{Account: "320.01", Credit: dec("100")})
	e.approve(t, d)
	b, err := e.svc.Export(context.Background(), e.scope, e.actor, ExportInput{Driver: driver, Drafts: []uuid.UUID{d.UUID}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// #391: çıktı → ajana gönder → ajan üstlenir → teslim → kullanıcı
// "aktarıldı" der; ikinci gönderim reddedilir.
func TestTransferFlow(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	b := e.exported(t, "luca")
	ag := e.agent(t, "machine-0001")

	tr, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, "")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != TransferQueued || tr.Driver != "luca" || tr.Method != MethodAgentFolder || len(tr.Files) != 1 {
		t.Fatalf("queued %+v", tr)
	}
	if _, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, ""); !errors.Is(err, ErrTransferExists) {
		t.Fatalf("second request = %v", err)
	}
	if _, err := e.svc.ClaimTransfer(ctx, e.scope, tr.UUID, uuid.New()); !errors.Is(err, ErrAgentUnknown) {
		t.Fatalf("unknown agent claim = %v", err)
	}
	// Delivery before claim: wrong state.
	in := DeliveryInput{AgentUUID: ag, Outcome: TransferDelivered, FolderLabel: "Luca aktarım",
		Files: []TransferFile{{Name: tr.Files[0].Name, SHA256: tr.Files[0].SHA256, Size: tr.Files[0].Size}}}
	if _, err := e.svc.ReportDelivery(ctx, e.scope, tr.UUID, in); !errors.Is(err, ErrTransferState) {
		t.Fatalf("delivery before claim = %v", err)
	}
	c, err := e.svc.ClaimTransfer(ctx, e.scope, tr.UUID, ag)
	if err != nil || c.Status != TransferClaimed || c.Agent == nil || c.Agent.UUID != ag || c.ClaimedAt == nil {
		t.Fatalf("claim %+v %v", c, err)
	}
	// Another agent cannot take a fresh claim.
	other := e.agent(t, "machine-0002")
	if _, err := e.svc.ClaimTransfer(ctx, e.scope, tr.UUID, other); !errors.Is(err, ErrTransferState) {
		t.Fatalf("second agent claim = %v", err)
	}
	if _, err := e.svc.ReportDelivery(ctx, e.scope, tr.UUID, DeliveryInput{AgentUUID: ag, Outcome: TransferDelivered}); !errors.Is(err, ErrTransferInvalid) {
		t.Fatalf("delivery without files = %v", err)
	}
	dl, err := e.svc.ReportDelivery(ctx, e.scope, tr.UUID, in)
	if err != nil || dl.Status != TransferDelivered || len(dl.DeliveredFiles) != 1 || dl.FolderLabel != "Luca aktarım" || dl.DeliveredAt == nil {
		t.Fatalf("delivered %+v %v", dl, err)
	}
	if _, err := e.svc.CompleteTransfer(ctx, e.scope, e.actor, tr.UUID, CompleteInput{Outcome: TransferFailed}); !errors.Is(err, ErrTransferInvalid) {
		t.Fatalf("failed without message = %v", err)
	}
	done, err := e.svc.CompleteTransfer(ctx, e.scope, e.actor, tr.UUID, CompleteInput{Outcome: TransferImported})
	if err != nil || done.Status != TransferImported || done.CompletedAt == nil {
		t.Fatalf("imported %+v %v", done, err)
	}
	// Imported: never again (Luca would cut the vouchers twice).
	if _, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, ""); !errors.Is(err, ErrTransferExists) {
		t.Fatalf("request after import = %v", err)
	}
	if _, err := e.svc.RetryTransfer(ctx, e.scope, e.actor, tr.UUID); !errors.Is(err, ErrTransferState) {
		t.Fatalf("retry imported = %v", err)
	}
	if _, err := e.svc.CancelTransfer(ctx, e.scope, e.actor, tr.UUID); !errors.Is(err, ErrTransferState) {
		t.Fatalf("cancel imported = %v", err)
	}
}

// Ajan hatası geçmişte kalır, yeniden deneme yeni görev açar.
func TestTransferFailureAndRetry(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	b := e.exported(t, "zirve")
	ag := e.agent(t, "machine-0003")

	tr, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, MethodAgentFolder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ClaimTransfer(ctx, e.scope, tr.UUID, ag); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ReportDelivery(ctx, e.scope, tr.UUID, DeliveryInput{AgentUUID: ag, Outcome: TransferFailed, ErrorCode: "bad code"}); !errors.Is(err, ErrTransferInvalid) {
		t.Fatalf("bad error code = %v", err)
	}
	f, err := e.svc.ReportDelivery(ctx, e.scope, tr.UUID, DeliveryInput{AgentUUID: ag, Outcome: TransferFailed,
		ErrorCode: "FOLDER_MISSING", ErrorMessage: "Aktarım klasörü bulunamadı"})
	if err != nil || f.Status != TransferFailed || f.ErrorCode != "FOLDER_MISSING" || f.ErrorMessage != "Aktarım klasörü bulunamadı" {
		t.Fatalf("failed %+v %v", f, err)
	}
	again, err := e.svc.RetryTransfer(ctx, e.scope, e.actor, tr.UUID)
	if err != nil || again.Status != TransferQueued || again.UUID == tr.UUID || again.BatchUUID != b.UUID {
		t.Fatalf("retry %+v %v", again, err)
	}
	if _, err := e.svc.CancelTransfer(ctx, e.scope, e.actor, again.UUID); err != nil {
		t.Fatal(err)
	}
	page, err := e.svc.ListTransfers(ctx, e.scope, TransferFilter{Limit: 50})
	if err != nil || page.Total != 2 || page.Items[0].Status != TransferCancelled || page.Items[1].Status != TransferFailed {
		t.Fatalf("history %+v %v", page, err)
	}
	failed, err := e.svc.ListTransfers(ctx, e.scope, TransferFilter{Status: TransferFailed, Limit: 50})
	if err != nil || failed.Total != 1 {
		t.Fatalf("failed filter %+v %v", failed, err)
	}
}

// Formüllü Excel programa gitmez; bütün fişleri geri alınmış çıktı da.
func TestTransferRefusals(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	xl := e.exported(t, "excel_formula")
	if _, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, xl.UUID, ""); !errors.Is(err, ErrTransferDriver) {
		t.Fatalf("excel transfer = %v", err)
	}
	if _, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, uuid.New(), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown batch = %v", err)
	}
	b := e.exported(t, "luca")
	if _, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, "luca_portal"); !errors.Is(err, ErrTransferInvalid) {
		t.Fatalf("unknown method = %v", err)
	}
	det, err := e.svc.GetBatch(ctx, e.scope, b.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Reverse(ctx, e.scope, e.actor, det.Items[0].Draft, "yanlış dönem"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, ""); !errors.Is(err, ErrTransferReversed) {
		t.Fatalf("reversed batch = %v", err)
	}
}

// HTTP: istek kapsamındaki RLS işleminde akış ve hata kodları.
func TestTransferHTTP(t *testing.T) {
	e := setup(t)
	h := NewHandler(e.svc, e.store)
	b := e.exported(t, "luca")
	ag := e.agent(t, "machine-0004")

	w := e.do(t, h.requestTransfer, "POST", "", map[string]string{"batchUuid": b.UUID.String()})
	if w.Code != http.StatusCreated {
		t.Fatalf("request %d %s", w.Code, w.Body)
	}
	tr := data[Transfer](t, w)
	if w := e.do(t, h.requestTransfer, "POST", "{}", map[string]string{"batchUuid": b.UUID.String()}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "TRANSFER_EXISTS") {
		t.Fatalf("duplicate %d %s", w.Code, w.Body)
	}
	pv := map[string]string{"transferUuid": tr.UUID.String()}
	if w := e.do(t, h.claimTransfer, "POST", `{}`, pv); w.Code != http.StatusBadRequest {
		t.Fatalf("claim without agent %d", w.Code)
	}
	if w := e.do(t, h.claimTransfer, "POST", `{"agent_uuid":"`+ag.String()+`"}`, pv); w.Code != http.StatusOK {
		t.Fatalf("claim %d %s", w.Code, w.Body)
	}
	w = e.do(t, h.deliverTransfer, "POST", `{"agent_uuid":"`+ag.String()+`","outcome":"failed","error_code":"WRITE_DENIED","error_message":"Klasöre yazma izni yok"}`, pv)
	if w.Code != http.StatusOK || data[Transfer](t, w).ErrorMessage != "Klasöre yazma izni yok" {
		t.Fatalf("deliver %d %s", w.Code, w.Body)
	}
	if w := e.do(t, h.completeTransfer, "POST", `{"outcome":"imported"}`, pv); w.Code != http.StatusConflict {
		t.Fatalf("complete failed transfer %d %s", w.Code, w.Body)
	}
	w = e.do(t, h.retryTransfer, "POST", "", pv)
	if w.Code != http.StatusCreated {
		t.Fatalf("retry %d %s", w.Code, w.Body)
	}
	if w := e.do(t, h.listTransfers, "GET", "", nil); w.Code != http.StatusOK || data[TransferPage](t, w).Total != 2 {
		t.Fatalf("list %d %s", w.Code, w.Body)
	}
	if w := e.do(t, h.getTransfer, "GET", "", map[string]string{"transferUuid": uuid.NewString()}); w.Code != http.StatusNotFound {
		t.Fatalf("unknown transfer %d", w.Code)
	}
}

// #574: fiş listesi aktarılan fişin çıktısını ve aktarım durumunu gösterir.
func TestDraftListShowsExportAndTransfer(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	b := e.exported(t, "luca")
	find := func() *DraftExport {
		t.Helper()
		page, err := e.svc.ListDrafts(ctx, e.scope, DraftFilter{Period: oct.Format("2006-01"), Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range page.Items {
			if d.Status == "exported" {
				return d.Export
			}
		}
		t.Fatal("no exported draft")
		return nil
	}
	x := find()
	if x == nil || x.BatchUUID != b.UUID || x.Driver != "luca" || x.TransferStatus != "" {
		t.Fatalf("after export %+v", x)
	}
	if _, err := e.svc.RequestTransfer(ctx, e.scope, e.actor, b.UUID, ""); err != nil {
		t.Fatal(err)
	}
	if x := find(); x == nil || x.TransferStatus != TransferQueued {
		t.Fatalf("after transfer %+v", x)
	}
}
