package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	invoicerepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const ownVKN = "1288331521"

type fx struct {
	d     *testdb.DB
	pool  *pgxpool.Pool
	svc   *usecase.Service
	sub   usecase.Subject
	user  int64
	guard *Guard
}

func setup(t *testing.T) fx {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", ownVKN)
	eng := d.Engagement(t, org, tp)
	return fx{
		d: d, pool: d.Pool, svc: usecase.New(New(d.Pool)),
		sub:  usecase.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp},
		user: d.User(t), guard: NewGuard(),
	}
}

func ublInvoice(t *testing.T) model.Invoice {
	t.Helper()
	b, err := os.ReadFile("../../invoices/ublmap/testdata/temelfatura-satis.xml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ublmap.MapXML(b, ublmap.Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	return r.Invoice
}

// programCopy: the same invoice from a program API (lower priority than UBL).
func programCopy(inv model.Invoice) model.Invoice {
	p := inv
	p.SourceSystem, p.SourceID, p.CaptureMethod = "parasut", "P-1", model.CaptureAPI
	p.Notes = "programdan"
	p.ContentHash = "1111111111111111111111111111111111111111111111111111111111111111"
	p.Items = inv.Items[:1]
	return p
}

func (f fx) save(t *testing.T, inv model.Invoice) invoicerepo.SaveResult {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := invoicerepo.SaveInvoiceGuarded(ctx, tx, f.sub.TaxpayerID, inv, 0, f.guard)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return r
}

func (f fx) str(t *testing.T, sql string, args ...any) (out string) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(), sql, append([]any{f.sub.TaxpayerID}, args...)...).Scan(&out); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return out
}

// Kabul (#32): kilitli döneme gelen fatura kaydı değiştirmiyor, talep oluşturuyor.
func TestLockedPeriodChangeOpensRequest(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ubl := ublInvoice(t)
	period := ubl.Period

	first := f.save(t, programCopy(ubl))
	if first.Outcome != invoicerepo.OutcomeCreated {
		t.Fatalf("first save: %+v", first)
	}
	r, err := f.svc.Readiness(ctx, f.sub, period)
	if err != nil || !r.Ready || r.Locked {
		t.Fatalf("readiness before lock: %+v %v", r, err)
	}
	if r, err = f.svc.Lock(ctx, f.sub, period, "KDV beyanı verildi", f.user); err != nil || !r.Locked || !r.Ready {
		t.Fatalf("lock: %+v %v", r, err)
	}
	before := f.str(t, `SELECT source_system || '/' || COALESCE(notes, '') || '/' || content_hash || '/' || updated_at::text FROM invoices WHERE taxpayer_id = $1`)

	// UBL has a higher priority: unlocked it would rewrite the invoice.
	held := f.save(t, ubl)
	if held.Outcome != invoicerepo.OutcomeHeld || held.Written || held.ChangeRequestUUID == uuid.Nil || held.InvoiceID != first.InvoiceID {
		t.Fatalf("held: %+v", held)
	}
	if after := f.str(t, `SELECT source_system || '/' || COALESCE(notes, '') || '/' || content_hash || '/' || updated_at::text FROM invoices WHERE taxpayer_id = $1`); after != before {
		t.Fatalf("invoice changed in a locked period:\n%s\n%s", before, after)
	}
	if n := f.str(t, `SELECT count(*)::text FROM invoice_sources WHERE taxpayer_id = $1`); n != "1" {
		t.Fatalf("invoice sources = %s (held content must not add a source)", n)
	}
	cr, err := f.svc.GetChangeRequest(ctx, f.sub, held.ChangeRequestUUID)
	if err != nil {
		t.Fatal(err)
	}
	if cr.Status != usecase.StatusOpen || cr.Kind != KindUpdate || cr.Period != period || cr.InvoiceUUID == nil || *cr.InvoiceUUID != first.InvoiceUUID {
		t.Fatalf("change request: %+v", cr)
	}
	if cr.OldHash == nil || *cr.OldHash != programCopy(ubl).ContentHash || cr.NewHash == nil || *cr.NewHash != ubl.ContentHash {
		t.Fatalf("hashes: old=%v new=%v", cr.OldHash, cr.NewHash)
	}
	var diff Diff
	if err := json.Unmarshal(cr.Diff, &diff); err != nil || diff.Kind != KindUpdate || diff.Fields["source"].New != "manual_upload/ubl_upload" {
		t.Fatalf("diff: %s %v", cr.Diff, err)
	}
	var payload model.Invoice
	if err := json.Unmarshal(cr.NewPayload, &payload); err != nil || payload.ContentHash != ubl.ContentHash {
		t.Fatalf("payload: %v", err)
	}
	if w := f.str(t, `SELECT severity || '/' || status || '/' || entity_type || '/' || producer FROM data_warnings WHERE taxpayer_id = $1 AND code = 'LOCKED_PERIOD_CHANGE'`); w != "blocker/open/invoice/control:period_lock" {
		t.Fatalf("warning = %s", w)
	}
	r, err = f.svc.Readiness(ctx, f.sub, period)
	if err != nil || r.Ready || r.OpenBlockers != 1 || r.OpenChangeRequests != 1 || len(r.Reasons) != 2 || r.Reasons[0].WarningCode != CodeLockedPeriodChange {
		t.Fatalf("readiness with request: %+v %v", r, err)
	}

	// The same content again reuses the open request.
	if again := f.save(t, ubl); again.Outcome != invoicerepo.OutcomeHeld || again.ChangeRequestUUID != held.ChangeRequestUUID {
		t.Fatalf("again: %+v", again)
	}
	if n := f.str(t, `SELECT count(*)::text FROM change_requests WHERE taxpayer_id = $1`); n != "1" {
		t.Fatalf("change requests = %s", n)
	}

	// Apply: written through the upsert path, period stays locked.
	applied, err := f.svc.Apply(ctx, f.sub, held.ChangeRequestUUID, f.user)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != usecase.StatusApplied || applied.DecidedAt == nil || applied.DecidedByUUID == nil {
		t.Fatalf("applied: %+v", applied)
	}
	if s := f.str(t, `SELECT source_system || '/' || COALESCE(notes, '') || '/' || content_hash FROM invoices WHERE taxpayer_id = $1`); s != "manual_upload//"+ubl.ContentHash {
		t.Fatalf("invoice after apply = %s", s)
	}
	if n := f.str(t, `SELECT count(*)::text FROM invoice_items WHERE taxpayer_id = $1`); n != "2" {
		t.Fatalf("items after apply = %s", n)
	}
	if w := f.str(t, `SELECT status FROM data_warnings WHERE taxpayer_id = $1 AND code = 'LOCKED_PERIOD_CHANGE'`); w != "auto_resolved" {
		t.Fatalf("warning after apply = %s", w)
	}
	r, err = f.svc.Readiness(ctx, f.sub, period)
	if err != nil || !r.Ready || !r.Locked {
		t.Fatalf("readiness after apply: %+v %v", r, err)
	}
	if _, err := f.svc.Reject(ctx, f.sub, held.ChangeRequestUUID, f.user); !errors.Is(err, usecase.ErrDecided) {
		t.Fatalf("second decision: %v", err)
	}
	// Another engagement cannot see the request.
	other := f.sub
	other.EngagementID = f.d.Engagement(t, f.d.Organization(t), f.d.Taxpayer(t, "legal", "9876543210"))
	if _, err := f.svc.GetChangeRequest(ctx, other, held.ChangeRequestUUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("other engagement: %v", err)
	}
}

func TestLockedPeriodNewInvoiceRejected(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ubl := ublInvoice(t)
	if _, err := f.svc.Lock(ctx, f.sub, ubl.Period, "", f.user); err != nil {
		t.Fatal(err)
	}
	held := f.save(t, ubl)
	if held.Outcome != invoicerepo.OutcomeHeld || held.InvoiceID != 0 || held.ChangeRequestUUID == uuid.Nil {
		t.Fatalf("held: %+v", held)
	}
	if n := f.str(t, `SELECT count(*)::text FROM invoices WHERE taxpayer_id = $1`); n != "0" {
		t.Fatalf("invoices = %s", n)
	}
	if n := f.str(t, `SELECT count(*)::text FROM customers WHERE taxpayer_id = $1`); n != "0" {
		t.Fatalf("customers = %s", n)
	}
	if w := f.str(t, `SELECT entity_type || '/' || status || '/' || period FROM data_warnings WHERE taxpayer_id = $1 AND code = 'LOCKED_PERIOD_CHANGE'`); w != "period/open/"+ubl.Period {
		t.Fatalf("warning = %s", w)
	}
	list, total, err := f.svc.ListChangeRequests(ctx, f.sub, usecase.ChangeRequestFilter{Status: usecase.StatusOpen, Limit: 10})
	if err != nil || total != 1 || len(list) != 1 || list[0].Kind != KindCreate || list[0].InvoiceUUID != nil || list[0].OldHash != nil {
		t.Fatalf("list: %+v %d %v", list, total, err)
	}
	rej, err := f.svc.Reject(ctx, f.sub, held.ChangeRequestUUID, f.user)
	if err != nil || rej.Status != usecase.StatusRejected {
		t.Fatalf("reject: %+v %v", rej, err)
	}
	if n := f.str(t, `SELECT count(*)::text FROM invoices WHERE taxpayer_id = $1`); n != "0" {
		t.Fatalf("invoices after reject = %s", n)
	}
	if w := f.str(t, `SELECT status FROM data_warnings WHERE taxpayer_id = $1 AND code = 'LOCKED_PERIOD_CHANGE'`); w != "auto_resolved" {
		t.Fatalf("warning after reject = %s", w)
	}
	// After unlocking, the invoice is written normally.
	if _, err := f.svc.Unlock(ctx, f.sub, ubl.Period, "düzeltme beyannamesi", f.user); err != nil {
		t.Fatal(err)
	}
	if r := f.save(t, ubl); r.Outcome != invoicerepo.OutcomeCreated {
		t.Fatalf("after unlock: %+v", r)
	}
}

func TestApplyNewInvoice(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ubl := ublInvoice(t)
	if _, err := f.svc.Lock(ctx, f.sub, ubl.Period, "", f.user); err != nil {
		t.Fatal(err)
	}
	held := f.save(t, ubl)
	if _, err := f.svc.Apply(ctx, f.sub, held.ChangeRequestUUID, f.user); err != nil {
		t.Fatal(err)
	}
	if n := f.str(t, `SELECT count(*)::text FROM invoices WHERE taxpayer_id = $1 AND content_hash = $2`, ubl.ContentHash); n != "1" {
		t.Fatalf("invoices after apply = %s", n)
	}
	// The applied source is now known: the next delivery is "seen".
	if r := f.save(t, ubl); r.Outcome != invoicerepo.OutcomeSeen {
		t.Fatalf("after apply: %+v", r)
	}
}

func TestLockUnlockAudit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	const p = "2026-04"
	if _, err := f.svc.Lock(ctx, f.sub, p, "ilk", f.user); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Lock(ctx, f.sub, p, "", f.user); !errors.Is(err, usecase.ErrAlreadyLocked) {
		t.Fatalf("double lock: %v", err)
	}
	if _, err := f.svc.Unlock(ctx, f.sub, p, "  ", f.user); !errors.Is(err, usecase.ErrReasonRequired) {
		t.Fatalf("unlock without reason: %v", err)
	}
	r, err := f.svc.Unlock(ctx, f.sub, p, "SGK düzeltmesi", f.user)
	if err != nil || r.Locked {
		t.Fatalf("unlock: %+v %v", r, err)
	}
	if _, err := f.svc.Unlock(ctx, f.sub, p, "yine", f.user); !errors.Is(err, usecase.ErrNotLocked) {
		t.Fatalf("double unlock: %v", err)
	}
	if _, err := f.svc.Readiness(ctx, f.sub, "2026-13"); !errors.Is(err, usecase.ErrInvalid) {
		t.Fatalf("bad period: %v", err)
	}
	ev, err := f.svc.LockEvents(ctx, f.sub, p)
	if err != nil || len(ev) != 2 || ev[0].Action != usecase.ActionUnlock || ev[0].Reason != "SGK düzeltmesi" || ev[1].Action != usecase.ActionLock || ev[0].ActorUUID == nil {
		t.Fatalf("events: %+v %v", ev, err)
	}
	// Template roles: owner (automatic), admin and accountant may lock.
	var roles string
	if err := f.pool.QueryRow(ctx, `
		SELECT string_agg(r.slug, ',' ORDER BY r.slug) FROM roles r
		JOIN role_permissions rp ON rp.role_id = r.id JOIN permissions p ON p.id = rp.permission_id
		WHERE r.scope = 'tenant' AND r.organization_id IS NULL AND p.slug = 'tenant.periods.lock'`).Scan(&roles); err != nil {
		t.Fatal(err)
	}
	if roles != "accountant,admin,owner" {
		t.Fatalf("tenant.periods.lock roles = %s", roles)
	}
}

// An ended engagement's lock does not hold the taxpayer's data (D1, D2).
func TestEndedEngagementLockIgnored(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ubl := ublInvoice(t)
	if _, err := f.svc.Lock(ctx, f.sub, ubl.Period, "", f.user); err != nil {
		t.Fatal(err)
	}
	f.d.EndEngagement(t, f.sub.EngagementID)
	if r := f.save(t, ubl); r.Outcome != invoicerepo.OutcomeCreated {
		t.Fatalf("after engagement ended: %+v", r)
	}
}

func TestMatrix(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ubl := ublInvoice(t)
	tp2 := f.d.Taxpayer(t, "legal", "1234567890")
	eng2 := f.d.Engagement(t, f.sub.OrganizationID, tp2)
	if _, err := f.svc.Lock(ctx, f.sub, ubl.Period, "", f.user); err != nil {
		t.Fatal(err)
	}
	f.save(t, ubl) // held: one open request + one blocker in the period
	// A non-blocking open warning on the other engagement: it counts as an
	// open warning (#529) but does not hold the period back.
	if _, err := f.d.Pool.Exec(ctx, `INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, code, severity, producer, status)
		VALUES ($1, $2, $3, $4, 'period', 'VAT_DEADLINE_NEAR', 'info', 'control:test', 'open')`, f.sub.OrganizationID, eng2, tp2, ubl.Period); err != nil {
		t.Fatal(err)
	}

	m, err := f.svc.Matrix(ctx, usecase.MatrixFilter{OrganizationID: f.sub.OrganizationID, AllEngagements: true, Limit: 10}, ubl.Period, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Periods) != 2 || m.Periods[1] != ubl.Period || m.Total != 2 || len(m.Items) != 2 {
		t.Fatalf("matrix: %+v", m)
	}
	cells := map[int64]usecase.Cell{}
	for _, row := range m.Items {
		c := row.Cells[1]
		switch row.TaxpayerTaxID {
		case ownVKN:
			cells[f.sub.EngagementID] = c
		default:
			cells[eng2] = c
		}
	}
	if c := cells[f.sub.EngagementID]; c.Ready || !c.Locked || c.OpenBlockers != 1 || c.OpenWarnings < 1 || c.OpenChangeRequests != 1 {
		t.Fatalf("locked cell: %+v", c)
	}
	if c := cells[eng2]; !c.Ready || c.Locked || c.OpenBlockers != 0 || c.OpenWarnings != 1 {
		t.Fatalf("other cell: %+v", c)
	}
	if c := cells[eng2]; len(c.Returns) != 0 || c.OpenDocumentTasks != 0 {
		t.Fatalf("no returns yet: %+v", c)
	}
	// #575: beyan kayıtları ve mükelleften beklenen belgeler hücrede.
	if _, err := f.d.Pool.Exec(ctx, `INSERT INTO tax_returns (organization_id, engagement_id, kind, period, status, payable_amount, filed_on, accrual_number, snapshot)
		VALUES ($1, $2, 'kdv1', $3, 'filed', 1234.5, DATE '2026-11-26', 'T-1', '{}'), ($1, $2, 'muhsgk', $3, 'draft', NULL, NULL, NULL, NULL)`,
		f.sub.OrganizationID, eng2, ubl.Period); err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.Pool.Exec(ctx, `INSERT INTO document_tasks (organization_id, engagement_id, taxpayer_id, code, kind, period, status, title, note,
			due_at, next_reminder_at, created_by, submitted_at)
		SELECT $1, $2, $3, 'DOC_REQUEST', 'document_request', $4, s, 'Kira sözleşmesi', '', now(), now(), $5,
		       CASE WHEN s = 'submitted' THEN now() END
		FROM unnest(ARRAY['open', 'open', 'submitted']) s`, f.sub.OrganizationID, eng2, tp2, ubl.Period, f.user); err != nil {
		t.Fatal(err)
	}
	// #568: a bank account with movements last month but none in this
	// (closed) month misses its statement; one with movements does not.
	prev, _ := time.Parse("2006-01", ubl.Period)
	prevPeriod := prev.AddDate(0, -1, 0).Format("2006-01")
	for i, b := range []struct{ name, period string }{{"Ziraat TL vadesiz", prevPeriod}, {"Garanti TL vadesiz", ubl.Period}} {
		var box int64
		if err := f.d.Pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, $2, 'TRY') RETURNING id`, tp2, b.name).Scan(&box); err != nil {
			t.Fatal(err)
		}
		if _, err := f.d.Pool.Exec(ctx, `INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method, booked_at,
				transaction_date, debit_credit, amount, currency, description, period)
			VALUES ($1, $2, 'bank_test', $3, 'api', $4::date, $4::date, 'A', 100, 'TRY', 'x', $5)`,
			tp2, box, fmt.Sprintf("m%d", i), b.period+"-10", b.period); err != nil {
			t.Fatal(err)
		}
	}
	m, err = f.svc.Matrix(ctx, usecase.MatrixFilter{OrganizationID: f.sub.OrganizationID, AllEngagements: true, Limit: 10}, ubl.Period, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range m.Items {
		c := row.Cells[0]
		if row.TaxpayerTaxID == ownVKN {
			if len(c.Returns) != 0 || len(c.MissingBankStatements) != 0 {
				t.Fatalf("own cell: %+v", c)
			}
			continue
		}
		if len(c.MissingBankStatements) != 1 || c.MissingBankStatements[0] != "Ziraat TL vadesiz" {
			t.Fatalf("missing statements: %+v", c.MissingBankStatements)
		}
		got := map[string]usecase.CellReturn{}
		for _, r := range c.Returns {
			got[r.Kind] = r
		}
		k := got["kdv1"]
		if k.Status != "filed" || k.PayableAmount == nil || *k.PayableAmount != "1234.50" || k.FiledOn == nil || *k.FiledOn != "2026-11-26" ||
			got["muhsgk"].Status != "draft" || got["muhsgk"].PayableAmount != nil || c.OpenDocumentTasks != 2 || c.SubmittedDocuments != 1 {
			t.Fatalf("returns cell: %+v", c)
		}
	}
	// Without read_all a member sees only assigned engagements.
	m, err = f.svc.Matrix(ctx, usecase.MatrixFilter{OrganizationID: f.sub.OrganizationID, UserID: f.user, Limit: 10}, ubl.Period, 1)
	if err != nil || m.Total != 0 || len(m.Items) != 0 {
		t.Fatalf("unassigned matrix: %+v %v", m, err)
	}
}
