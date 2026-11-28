package agenttasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agenttasks/caritrigger"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/lucalists"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// fakeLists answers the bank list with one file; the others have nothing.
type fakeLists struct{ calls int }

func (f *fakeLists) Export(_ context.Context, _ engagementctx.Scope, _ int64, list lucalists.List, _ lucalists.Input) (lucalists.Result, error) {
	f.calls++
	if list != lucalists.BankTransactions {
		return lucalists.Result{Report: lucalists.Report{AlreadyExported: []lucalists.Row{{DocumentNo: "Z-1", Reason: "DOCUMENT_ALREADY_EXPORTED"}}}}, lucalists.ErrNothingToExport
	}
	return lucalists.Result{
		File:   lucalists.File{Name: "banka_2026-10.xlsx", ContentType: xlsxType, Data: []byte("sahte banka dosyası")},
		Export: uuid.New(),
	}, nil
}

type env struct {
	d      *testdb.DB
	svc    *Service
	lists  *fakeLists
	store  *storage.Memory
	sc     Scope
	eng    engagementctx.Scope
	engID  uuid.UUID
	taxpID int64
	actor  int64
}

func setup(t *testing.T) env {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	user := d.User(t)
	tp := d.Taxpayer(t, "legal", "1288331521")
	eng := d.Engagement(t, org, tp)
	var engUUID, tpUUID uuid.UUID
	if err := d.Pool.QueryRow(context.Background(), `SELECT e.uuid, t.uuid FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id WHERE e.id = $1`, eng).Scan(&engUUID, &tpUUID); err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemory()
	lists := &fakeLists{}
	svc := New(d.Pool, &Builder{Lists: lists, Store: store})
	return env{
		d: d, svc: svc, lists: lists, store: store, actor: user, engID: engUUID, taxpID: tp,
		sc:  Scope{OrganizationID: org, UserID: user, ReadAll: true},
		eng: engagementctx.Scope{ID: eng, UUID: engUUID, OrganizationID: org, TaxpayerID: tp, TaxpayerUUID: tpUUID, Status: "active"},
	}
}

func (e env) agent(t *testing.T, machine string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := rls.System(context.Background(), e.d.Pool, "test: desktop agent", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO desktop_agents (organization_id, machine_id, name, platform)
			VALUES ($1, $2, 'Ofis PC', 'windows') RETURNING uuid`, e.sc.OrganizationID, machine).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) enable(t *testing.T) {
	t.Helper()
	if _, err := e.svc.PutSettings(context.Background(), e.sc, Settings{LucaEnabled: true}); err != nil {
		t.Fatal(err)
	}
}

// trigger runs the new counterparty trigger as invoice ingestion does.
func (e env) trigger(t *testing.T, c caritrigger.Counterparty) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.d.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	caritrigger.Enqueue(ctx, tx, e.taxpID, c)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func (e env) pendingCari(t *testing.T) []Task {
	t.Helper()
	page, err := e.svc.List(context.Background(), e.sc, Filter{Kind: KindCariSync, Status: StatusQueued, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

// Ayar varsayılan kapalı: görev açılmaz, ajan alamaz, tetik iş açmaz.
func TestHelperOffByDefault(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	st, err := e.svc.GetSettings(ctx, e.sc)
	if err != nil || st.LucaEnabled || st.SGKEnabled {
		t.Fatalf("default settings %+v %v", st, err)
	}
	if _, err := e.svc.StartExport(ctx, e.sc, ExportInput{EngagementUUIDs: []uuid.UUID{e.engID}, Period: "2026-10"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("start while off = %v", err)
	}
	e.trigger(t, caritrigger.Counterparty{Direction: "incoming", TaxNumber: "1111111111", Name: "Tedarikçi A"})
	if got := e.pendingCari(t); len(got) != 0 {
		t.Fatalf("trigger while off opened %d tasks", len(got))
	}
	if _, err := e.svc.Claim(ctx, e.sc, uuid.New(), e.agent(t, "m-off")); !errors.Is(err, ErrDisabled) {
		t.Fatalf("claim while off = %v", err)
	}
}

// Aynı VKN'li iki fatura tek cari satırı açar; hafızadaki cari girmez.
func TestCariTriggerOneRowPerCounterparty(t *testing.T) {
	e := setup(t)
	e.enable(t)
	e.trigger(t, caritrigger.Counterparty{Direction: "incoming", TaxNumber: "1111111111", Name: "Tedarikçi A"})
	e.trigger(t, caritrigger.Counterparty{Direction: "incoming", TaxNumber: "1111111111", Name: "Tedarikçi A"})
	// Same firm as a customer is another row (120).
	e.trigger(t, caritrigger.Counterparty{Direction: "outgoing", TaxNumber: "1111111111", Name: "Tedarikçi A"})
	_, err := e.d.Pool.Exec(context.Background(), `INSERT INTO counterparty_accounts (organization_id, engagement_id, taxpayer_id, counterparty_tax_id, main_code, account_code)
		VALUES ($1, $2, $3, '2222222222', '320', '320.01.E07')`, e.sc.OrganizationID, e.eng.ID, e.taxpID)
	if err != nil {
		t.Fatal(err)
	}
	e.trigger(t, caritrigger.Counterparty{Direction: "incoming", TaxNumber: "2222222222", Name: "Bilinen"})
	e.trigger(t, caritrigger.Counterparty{Direction: "incoming", TaxNumber: "bad", Name: "VKN yok"})
	tasks := e.pendingCari(t)
	if len(tasks) != 1 {
		t.Fatalf("pending cari tasks = %d", len(tasks))
	}
	if tasks[0].Approved || len(tasks[0].Counterparties) != 2 {
		t.Fatalf("task %+v", tasks[0])
	}
	// Onaysız görevi ajan alamaz.
	ag := e.agent(t, "m-cari")
	if _, err := e.svc.Claim(context.Background(), e.sc, tasks[0].UUID, ag); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("claim unapproved = %v", err)
	}
	ap, err := e.svc.Approve(context.Background(), e.sc, tasks[0].UUID)
	if err != nil || !ap.Approved || ap.ApprovedAt == nil {
		t.Fatalf("approve %+v %v", ap, err)
	}
	// A new counterparty after the approval opens a new pending task.
	e.trigger(t, caritrigger.Counterparty{Direction: "incoming", TaxNumber: "3333333333", Name: "Yeni"})
	if got := e.pendingCari(t); len(got) != 2 {
		t.Fatalf("after approval %d queued cari tasks", len(got))
	}
}

// İş sırası: önce cari görevi, sonra aktarım; ajan aynı anda tek görev.
// Cari sonucu hafızaya yazılır, farklı karar ezilmez (conflict).
func TestRunOrderAndCariWriteBack(t *testing.T) {
	e := setup(t)
	e.enable(t)
	ctx := context.Background()
	e.trigger(t, caritrigger.Counterparty{Direction: "incoming", TaxNumber: "1111111111", Name: "Tedarikçi A"})
	e.trigger(t, caritrigger.Counterparty{Direction: "outgoing", TaxNumber: "4444444444", Name: "Müşteri B"})
	run, err := e.svc.StartExport(ctx, e.sc, ExportInput{EngagementUUIDs: []uuid.UUID{e.engID}, Period: "2026-10", Kinds: []string{FileBank, FileZ}})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Items) != 2 || run.Items[0].Kind != KindCariSync || run.Items[1].Kind != KindBulkExport || !run.Items[0].Approved || !run.Items[1].Approved {
		t.Fatalf("run %+v", run.Items)
	}
	// İkinci tıklama aynı dönemi yeniden açmaz.
	if _, err := e.svc.StartExport(ctx, e.sc, ExportInput{EngagementUUIDs: []uuid.UUID{e.engID}, Period: "2026-10"}); !errors.Is(err, ErrState) {
		t.Fatalf("second run = %v", err)
	}
	cari, bulk := run.Items[0], run.Items[1]
	ag := e.agent(t, "m-run")
	if _, err := e.svc.Claim(ctx, e.sc, bulk.UUID, ag); !errors.Is(err, ErrNotReady) {
		t.Fatalf("bulk before cari = %v", err)
	}
	if e.lists.calls != 0 {
		t.Fatal("files built before the cari task")
	}
	c, err := e.svc.Claim(ctx, e.sc, cari.UUID, ag)
	if err != nil || c.Status != StatusClaimed || c.Agent == nil {
		t.Fatalf("claim cari %+v %v", c, err)
	}
	if again, err := e.svc.Claim(ctx, e.sc, cari.UUID, ag); err != nil || again.UUID != cari.UUID {
		t.Fatalf("re-claim %v", err)
	}
	if _, err := e.svc.Progress(ctx, e.sc, cari.UUID, ProgressInput{AgentUUID: ag, Status: StatusWaitingUser, Message: "Luca'ya giriş yapın"}); err != nil {
		t.Fatal(err)
	}
	// Meanwhile the office decided 120 for Müşteri B by hand.
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO counterparty_accounts (organization_id, engagement_id, taxpayer_id, counterparty_tax_id, main_code, account_code)
		VALUES ($1, $2, $3, '4444444444', '120', '120.01.007')`, e.sc.OrganizationID, e.eng.ID, e.taxpID); err != nil {
		t.Fatal(err)
	}
	done, err := e.svc.Finish(ctx, e.sc, cari.UUID, ResultInput{AgentUUID: ag, Outcome: StatusDone, Counterparties: []CounterpartyResult{
		{TaxID: "1111111111", MainCode: "320", Status: CariCreated, AccountCode: "320.01.E11", Title: "Tedarikçi A"},
		{TaxID: "4444444444", MainCode: "120", Status: CariFound, AccountCode: "120.01.009"},
	}})
	if err != nil || done.Status != StatusDone {
		t.Fatalf("finish cari %+v %v", done, err)
	}
	byTax := map[string]CounterpartyView{}
	for _, cp := range done.Counterparties {
		byTax[cp.TaxID] = cp
	}
	if byTax["1111111111"].Status != CariCreated || byTax["4444444444"].Status != CariConflict || byTax["4444444444"].HakanAccountCode != "120.01.007" {
		t.Fatalf("counterparties %+v", done.Counterparties)
	}
	var code string
	if err := e.d.Pool.QueryRow(ctx, `SELECT account_code FROM counterparty_accounts WHERE engagement_id = $1 AND counterparty_tax_id = '1111111111' AND main_code = '320'`, e.eng.ID).Scan(&code); err != nil || code != "320.01.E11" {
		t.Fatalf("memory %q %v", code, err)
	}
	if err := e.d.Pool.QueryRow(ctx, `SELECT account_code FROM counterparty_accounts WHERE engagement_id = $1 AND counterparty_tax_id = '4444444444'`, e.eng.ID).Scan(&code); err != nil || code != "120.01.007" {
		t.Fatalf("decision overwritten: %q %v", code, err)
	}
	// Now the export: files are built at claim.
	b, err := e.svc.Claim(ctx, e.sc, bulk.UUID, ag)
	if err != nil || b.Status != StatusClaimed || len(b.Files) != 1 || b.Files[0].Kind != FileBank || len(b.Files[0].SHA256) != 64 {
		t.Fatalf("claim bulk %+v %v", b, err)
	}
	if b.ServerReport == nil || len(b.ServerReport.AlreadyExported) != 1 || b.ServerReport.AlreadyExported[0].Kind != FileZ {
		t.Fatalf("server report %+v", b.ServerReport)
	}
	ref, err := e.svc.File(ctx, e.sc, bulk.UUID, b.Files[0].UUID)
	if err != nil || ref.SHA256 != b.Files[0].SHA256 {
		t.Fatalf("file %+v %v", ref, err)
	}
	if _, _, err := e.store.Download(ctx, ref.StorageKey); err != nil {
		t.Fatalf("stored file: %v", err)
	}
}

// "Yükle"den sonra sonuç okunamazsa görev needs_check olur ve asla kendi
// kendine yeniden denenmez; kullanıcı Luca'ya bakıp kapatır.
func TestNeedsCheckIsNeverRetried(t *testing.T) {
	e := setup(t)
	e.enable(t)
	ctx := context.Background()
	run, err := e.svc.StartExport(ctx, e.sc, ExportInput{EngagementUUIDs: []uuid.UUID{e.engID}, Period: "2026-10", Kinds: []string{FileBank}})
	if err != nil || len(run.Items) != 1 {
		t.Fatalf("run %+v %v", run, err)
	}
	ag := e.agent(t, "m-check")
	task, err := e.svc.Claim(ctx, e.sc, run.Items[0].UUID, ag)
	if err != nil || len(task.Files) != 1 {
		t.Fatalf("claim %+v %v", task, err)
	}
	other := e.agent(t, "m-other")
	if _, err := e.svc.ReportFile(ctx, e.sc, task.UUID, task.Files[0].UUID, FileResultInput{AgentUUID: other, Status: FileOK}); !errors.Is(err, ErrState) {
		t.Fatalf("other agent file result = %v", err)
	}
	if _, err := e.svc.ReportFile(ctx, e.sc, task.UUID, task.Files[0].UUID, FileResultInput{AgentUUID: ag, Status: FileUnknown}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ReportFile(ctx, e.sc, task.UUID, task.Files[0].UUID, FileResultInput{AgentUUID: ag, Status: FileOK}); !errors.Is(err, ErrState) {
		t.Fatalf("second file result = %v", err)
	}
	fin, err := e.svc.Finish(ctx, e.sc, task.UUID, ResultInput{AgentUUID: ag, Outcome: StatusDone})
	if err != nil || fin.Status != StatusNeedsCheck {
		t.Fatalf("finish %+v %v", fin, err)
	}
	for _, a := range []uuid.UUID{ag, other} {
		if _, err := e.svc.Claim(ctx, e.sc, task.UUID, a); !errors.Is(err, ErrState) {
			t.Fatalf("claim needs_check = %v", err)
		}
	}
	if _, err := e.svc.Cancel(ctx, e.sc, task.UUID); !errors.Is(err, ErrState) {
		t.Fatalf("cancel needs_check = %v", err)
	}
	no := false
	res, err := e.svc.Resolve(ctx, e.sc, task.UUID, ResolveInput{Imported: &no})
	if err != nil || res.Resolution == nil || *res.Resolution != "not_imported" || res.Status != StatusNeedsCheck {
		t.Fatalf("resolve %+v %v", res, err)
	}
	if _, err := e.svc.Resolve(ctx, e.sc, task.UUID, ResolveInput{Imported: &no}); !errors.Is(err, ErrState) {
		t.Fatalf("second resolve = %v", err)
	}
	// The agent is free again for the next task.
	run2, err := e.svc.StartExport(ctx, e.sc, ExportInput{EngagementUUIDs: []uuid.UUID{e.engID}, Period: "2026-11", Kinds: []string{FileBank}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Claim(ctx, e.sc, run2.Items[0].UUID, ag); err != nil {
		t.Fatalf("next claim = %v", err)
	}
}

// Yevmiye çıktısı yardımcıyla gidince luca_web aktarımı açılır; bir
// çıktının tek açık ya da aktarılmış görevi kuralı aynen geçerli, klasör
// ajanı bu görevi üstlenemez; dosya sonucu aktarımı kapatır.
func TestJournalExportTransferRule(t *testing.T) {
	e := setup(t)
	e.enable(t)
	ctx := context.Background()
	js := journals.New(e.d.Pool, e.store)
	oct := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	d, err := js.CreateDraft(ctx, e.eng, e.actor, journals.DraftInput{Date: oct, Description: "test", Lines: []journals.LineInput{
		{Account: "770.01", Debit: decimal.RequireFromString("100")}, {Account: "320.01", Credit: decimal.RequireFromString("100")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := js.Approve(ctx, e.eng, e.actor, d.UUID); err != nil {
		t.Fatal(err)
	}
	batch, err := js.Export(ctx, e.eng, e.actor, journals.ExportInput{Driver: "luca", Period: "2026-10"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := e.svc.StartExport(ctx, e.sc, ExportInput{EngagementUUIDs: []uuid.UUID{e.engID}, Period: "2026-10", Kinds: []string{FileJournals}})
	if err != nil {
		t.Fatal(err)
	}
	ag := e.agent(t, "m-journal")
	task, err := e.svc.Claim(ctx, e.sc, run.Items[0].UUID, ag)
	if err != nil || len(task.Files) != len(batch.Files) || task.Files[0].Kind != FileJournals {
		t.Fatalf("claim %+v %v", task, err)
	}
	if _, err := js.RequestTransfer(ctx, e.eng, e.actor, batch.UUID, journals.MethodAgentFolder); !errors.Is(err, journals.ErrTransferExists) {
		t.Fatalf("folder transfer while luca_web open = %v", err)
	}
	page, err := js.ListTransfers(ctx, e.eng, journals.TransferFilter{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Method != "luca_web" || page.Items[0].Status != journals.TransferClaimed {
		t.Fatalf("transfers %+v %v", page.Items, err)
	}
	folder := e.agent(t, "m-folder")
	if _, err := js.ClaimTransfer(ctx, e.eng, page.Items[0].UUID, folder); !errors.Is(err, journals.ErrTransferState) {
		t.Fatalf("folder agent claims luca_web = %v", err)
	}
	for _, f := range task.Files {
		if _, err := e.svc.ReportFile(ctx, e.sc, task.UUID, f.UUID, FileResultInput{AgentUUID: ag, Status: FileOK}); err != nil {
			t.Fatal(err)
		}
	}
	tr, err := js.GetTransfer(ctx, e.eng, page.Items[0].UUID)
	if err != nil || tr.Status != journals.TransferImported {
		t.Fatalf("transfer after upload %+v %v", tr, err)
	}
	fin, err := e.svc.Finish(ctx, e.sc, task.UUID, ResultInput{AgentUUID: ag, Outcome: StatusDone})
	if err != nil || fin.Status != StatusDone {
		t.Fatalf("finish %+v %v", fin, err)
	}
	// Imported: the export never goes again, by any path.
	if _, err := js.RequestTransfer(ctx, e.eng, e.actor, batch.UUID, journals.MethodManual); !errors.Is(err, journals.ErrTransferExists) {
		t.Fatalf("request after import = %v", err)
	}
}
