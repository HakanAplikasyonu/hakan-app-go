package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const ownVKN = "1288331521"

func ublInvoice(t *testing.T) model.Invoice {
	t.Helper()
	b, err := os.ReadFile("../ublmap/testdata/temelfatura-satis.xml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ublmap.MapXML(b, ublmap.Options{OwnTaxID: ownVKN})
	if err != nil {
		t.Fatal(err)
	}
	return r.Invoice
}

// programCopy is the same invoice as a program (Paraşüt API) delivers it:
// other source key, lower priority, a different payable to tell contents apart.
func programCopy(inv model.Invoice, sourceID string) model.Invoice {
	p := inv
	p.SourceSystem, p.SourceID, p.CaptureMethod = "parasut", sourceID, model.CaptureAPI
	p.Notes = "programdan"
	p.Items = inv.Items[:1]
	return p
}

type fx struct {
	pool *pgxpool.Pool
	tp   int64
}

func setup(t *testing.T) fx {
	d := testdb.Open(t)
	return fx{pool: d.Pool, tp: d.Taxpayer(t, "legal", ownVKN)}
}

func (f fx) save(t *testing.T, inv model.Invoice) SaveResult {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := SaveInvoice(ctx, tx, f.tp, inv, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return r
}

func (f fx) one(t *testing.T, sql string, args ...any) (out string) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(), sql, append([]any{f.tp}, args...)...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Kabul: aynı ETTN iki kaynaktan → tek invoice, iki kaynak bağlantısı. Daha
// yüksek öncelikli kaynak (UBL) sonra gelirse içerik onunla yeniden yazılır.
func TestSameETTNProgramThenUBL(t *testing.T) {
	f := setup(t)
	ubl := ublInvoice(t)
	r1 := f.save(t, programCopy(ubl, "P-1"))
	if r1.Outcome != OutcomeCreated {
		t.Fatalf("first: %+v", r1)
	}
	r2 := f.save(t, ubl)
	if r2.Outcome != OutcomeReplaced || r2.MatchRule != model.MatchETTN || r2.InvoiceID != r1.InvoiceID || !r2.Written {
		t.Fatalf("second: %+v", r2)
	}
	if n := f.one(t, `SELECT count(*)::text FROM invoices WHERE taxpayer_id = $1`); n != "1" {
		t.Fatalf("invoices = %s", n)
	}
	if n := f.one(t, `SELECT count(*)::text FROM invoice_sources WHERE taxpayer_id = $1`); n != "2" {
		t.Fatalf("sources = %s", n)
	}
	if s := f.one(t, `SELECT source_system || '/' || capture_method || '/' || COALESCE(notes, '') FROM invoices WHERE taxpayer_id = $1`); s != "manual_upload/ubl_upload/" {
		t.Fatalf("winner = %s", s)
	}
	if n := f.one(t, `SELECT count(*)::text FROM invoice_items WHERE taxpayer_id = $1`); n != "2" {
		t.Fatalf("items after replace = %s (UBL has 2 lines)", n)
	}
	if n := f.one(t, `SELECT count(*)::text FROM invoice_tax_summaries WHERE taxpayer_id = $1`); n != "2" {
		t.Fatalf("summaries after replace = %s", n)
	}
	// The same UBL again is a known source.
	if r3 := f.save(t, ubl); r3.Outcome != OutcomeSeen || r3.Written {
		t.Fatalf("third: %+v", r3)
	}
}

func TestSameETTNUBLThenProgramKeepsUBL(t *testing.T) {
	f := setup(t)
	ubl := ublInvoice(t)
	f.save(t, ubl)
	r := f.save(t, programCopy(ubl, "P-1"))
	if r.Outcome != OutcomeMerged || r.Written {
		t.Fatalf("program after UBL: %+v", r)
	}
	if s := f.one(t, `SELECT source_system || '/' || COALESCE(notes, '') FROM invoices WHERE taxpayer_id = $1`); s != "manual_upload/" {
		t.Fatalf("content changed: %s", s)
	}
	if s := f.one(t, `SELECT string_agg(source_system || ':' || priority || ':' || match_rule, ',' ORDER BY priority DESC) FROM invoice_sources WHERE taxpayer_id = $1`); s != "manual_upload:3:new,parasut:2:ettn" {
		t.Fatalf("sources = %s", s)
	}
}

// Kabul: ETTN'siz olası mükerrer uyarı üretiyor, kayıt birleşmiyor.
func TestPossibleDuplicateWithoutETTN(t *testing.T) {
	f := setup(t)
	ubl := ublInvoice(t)
	first := f.save(t, ubl)
	ocr := ubl
	ocr.UUID, ocr.SourceSystem, ocr.SourceID, ocr.CaptureMethod = "", "manual_upload", "ocr-"+uuid.NewString(), model.CaptureOCR
	ocr.InvoiceNumber = "FIS2026000000099"
	r := f.save(t, ocr)
	if r.Outcome != OutcomeCreated || len(r.PossibleDuplicates) != 1 || r.PossibleDuplicates[0] != first.InvoiceUUID {
		t.Fatalf("ocr: %+v", r)
	}
	if n := f.one(t, `SELECT count(*)::text FROM invoices WHERE taxpayer_id = $1`); n != "2" {
		t.Fatalf("invoices = %s", n)
	}
	// Different amount: not a candidate.
	other := ocr
	other.SourceID, other.InvoiceNumber = "ocr-"+uuid.NewString(), "FIS2026000000100"
	other.Amounts.PayableAmount = other.Amounts.PayableAmount.Add(decimal.RequireFromString("0.02"))
	if r := f.save(t, other); len(r.PossibleDuplicates) != 0 {
		t.Fatalf("different amount flagged: %+v", r)
	}
}

func TestRuleTwoNumberMatch(t *testing.T) {
	f := setup(t)
	ubl := ublInvoice(t)
	first := f.save(t, ubl)
	// A program export without ETTN but the same party, number and date.
	p := programCopy(ubl, "P-9")
	p.UUID = ""
	r := f.save(t, p)
	if r.Outcome != OutcomeMerged || r.MatchRule != model.MatchNumber || r.InvoiceID != first.InvoiceID {
		t.Fatalf("number match: %+v", r)
	}
	// Same number but a different ETTN: a different document.
	q := ubl
	q.UUID, q.SourceID = uuid.NewString(), "x-"+uuid.NewString()
	q.SourceSystem = "entegrator_uyumsoft"
	if r := f.save(t, q); r.Outcome != OutcomeCreated {
		t.Fatalf("different ETTN merged: %+v", r)
	}
}
