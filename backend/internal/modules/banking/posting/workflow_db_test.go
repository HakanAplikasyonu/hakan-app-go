package posting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

const ownIBAN = "TR760001000519786457841326"

type flowFixture struct {
	aliasFixture
	flow   *Workflow
	review *reviewusecase.Service
	bank   *banking.Service
	box    db.CashBox
	user   int64
}

func setupFlow(t *testing.T) flowFixture {
	t.Helper()
	f := setupAliases(t)
	ctx := context.Background()
	box, err := f.q.InsertCashBox(ctx, db.InsertCashBoxParams{TaxpayerID: f.sub.TaxpayerID, Type: "bank_account", Currency: "TRY",
		Name: "Ziraat TL", Iban: pgtype.Text{String: ownIBAN, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	rv := reviewusecase.New(reviewrepository.New(f.d.Pool, f.q)).Register(SuggestionKind, Applier{})
	return flowFixture{aliasFixture: f, flow: NewWorkflow(f.d.Pool, rv, journals.New(f.d.Pool, nil)), review: rv,
		bank: banking.NewService(f.d.Pool, nil), box: box, user: f.d.User(t)}
}

func (f flowFixture) ingest(t *testing.T, txns ...banking.Transaction) {
	t.Helper()
	if _, err := f.bank.Ingest(context.Background(), f.sub.TaxpayerID, f.box.ID, nil, txns); err != nil {
		t.Fatal(err)
	}
}

func mv(id string, day time.Time, dir banking.Direction, amount, desc, name, iban, tax string) banking.Transaction {
	return banking.Transaction{SourceSystem: "bank_file", SourceID: id, CaptureMethod: banking.CaptureExcel, BookedAt: day,
		TransactionDate: day, Direction: dir, Amount: amount, Currency: "TRY", Description: desc,
		CounterpartyName: name, CounterpartyIBAN: iban, CounterpartyTaxID: tax}
}

func (f flowFixture) item(t *testing.T, period, desc string) Item {
	t.Helper()
	p, err := f.flow.List(context.Background(), f.sub, period, "", 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range p.Items {
		if it.Description == desc {
			return it
		}
	}
	t.Fatalf("no movement %q in %+v", desc, p.Items)
	return Item{}
}

func TestPostingWorkflow(t *testing.T) {
	f := setupFlow(t)
	ctx := context.Background()
	oct := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	// The supplier's cari is known from invoices (cari hafızası).
	if err := f.q.UpsertCounterpartyAccount(ctx, db.UpsertCounterpartyAccountParams{OrganizationID: f.sub.OrganizationID,
		EngagementID: f.sub.EngagementID, TaxpayerID: f.sub.TaxpayerID, CounterpartyTaxID: "9200000035", MainCode: "320",
		AccountCode: "320.01.007", AccountName: "Kalem Kırtasiye"}); err != nil {
		t.Fatal(err)
	}
	f.ingest(t,
		mv("1", oct(1), banking.DirIn, "12500", "FAST GELEN GONDEREN X YAZILIM FATURA ODEMESI", "X Yazılım Teknolojileri A.Ş.", payerIBAN, ""),
		mv("2", oct(2), banking.DirOut, "3250.75", "EFT GIDEN KALEM KIRTASIYE", "Kalem Kırtasiye", "", "9200000035"),
		mv("3", oct(2), banking.DirOut, "8.50", "EFT UCRETI", "", "", ""),
		mv("4", oct(5), banking.DirOut, "18420", "SGK PRIM ODEMESI", "", "", ""),
	)
	res, err := f.flow.Run(ctx, f.sub, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if res.Movements != 4 || res.Suggested+res.AutoApproved+res.NoSuggestion != 4 {
		t.Fatalf("run %+v", res)
	}
	sup := f.item(t, "2026-10", "EFT GIDEN KALEM KIRTASIYE")
	if sup.AccountCode != "320.01.007" || sup.Source != SourceCounterpartyAccount || sup.Status != "suggested" || sup.SuggestionUUID == nil {
		t.Fatalf("supplier %+v", sup)
	}
	fee := f.item(t, "2026-10", "EFT UCRETI")
	if fee.AccountCode != "780" || fee.Source != SourceKindDefault {
		t.Fatalf("fee %+v", fee)
	}
	// Running again changes nothing.
	res2, err := f.flow.Run(ctx, f.sub, "2026-10")
	if err != nil || res2.Unchanged != res.Suggested {
		t.Fatalf("second run %+v (first %+v) %v", res2, res, err)
	}

	// The payer is Oğuz Yılmaz's cari: the accountant picks it once.
	payer := f.item(t, "2026-10", "FAST GELEN GONDEREN X YAZILIM FATURA ODEMESI")
	if err := f.flow.SetAccount(ctx, f.sub, f.user, payer.UUID, "120.01.015", "Oğuz Yılmaz"); err != nil {
		t.Fatal(err)
	}
	payer = f.item(t, "2026-10", "FAST GELEN GONDEREN X YAZILIM FATURA ODEMESI")
	if payer.Status != "approved" || payer.AccountCode != "120.01.015" || payer.Source != SourceManual {
		t.Fatalf("payer %+v", payer)
	}
	// Bulk approval of the rest (one unknown uuid is skipped).
	ap, err := f.flow.Approve(ctx, f.sub, f.user, []uuid.UUID{sup.UUID, fee.UUID, f.item(t, "2026-10", "SGK PRIM ODEMESI").UUID, uuid.New()})
	if err != nil || ap.Approved != 3 || ap.Skipped != 1 {
		t.Fatalf("approve %+v %v", ap, err)
	}
	p, _ := f.flow.List(ctx, f.sub, "2026-10", "", 50, 0)
	if p.Summary.Approved != 4 || p.Summary.Suggested != 0 || p.Summary.Total != 4 {
		t.Fatalf("summary %+v", p.Summary)
	}

	// Revert one approval: back to the queue, alias lesson taken back.
	if _, err := f.review.Revert(ctx, f.sub.review(), *sup.SuggestionUUID, f.user, "test"); err != nil {
		t.Fatal(err)
	}
	if it := f.item(t, "2026-10", "EFT GIDEN KALEM KIRTASIYE"); it.Status != "suggested" {
		t.Fatalf("after revert %+v", it)
	}
	if m, _ := Resolve(ctx, f.q, f.sub, Counterparty{Name: "Kalem Kırtasiye", TaxID: "9200000035", Direction: banking.DirOut}); m != nil {
		t.Fatalf("reverted decision still taught the alias: %+v", m.Alias)
	}

	// Bank account's ledger code, then the journal drafts.
	if err := f.flow.SetLedgerAccount(ctx, f.sub, f.user, f.box.Uuid, "102.01"); err != nil {
		t.Fatal(err)
	}
	scope := engagementctx.Scope{ID: f.sub.EngagementID, OrganizationID: f.sub.OrganizationID, TaxpayerID: f.sub.TaxpayerID}
	dr, err := f.flow.CreateDrafts(ctx, scope, f.user, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	// Oct 1 (payer), Oct 2 (fee; the supplier is back in the queue), Oct 5 (SGK).
	if len(dr.Drafts) != 3 || dr.Transactions != 3 {
		t.Fatalf("drafts %+v", dr)
	}
	for _, d := range dr.Drafts {
		if !d.Debit.Equal(d.Credit) || d.Lines != 2 {
			t.Fatalf("draft %+v", d)
		}
	}
	var bankSide, counter string
	if err := f.d.Pool.QueryRow(ctx, `SELECT
		  max(account_code) FILTER (WHERE debit > 0), max(account_code) FILTER (WHERE credit > 0)
		FROM journal_draft_lines l JOIN journal_drafts j ON j.id = l.journal_draft_id
		WHERE j.uuid = $1 AND l.source_kind = 'bank_transaction'`, dr.Drafts[0].UUID).Scan(&bankSide, &counter); err != nil {
		t.Fatal(err)
	}
	if bankSide != "102.01" || counter != "120.01.015" {
		t.Fatalf("incoming lines: debit %s credit %s", bankSide, counter)
	}
	if it := f.item(t, "2026-10", "SGK PRIM ODEMESI"); it.Status != "drafted" || it.JournalDraftUUID == nil {
		t.Fatalf("drafted %+v", it)
	}
	// A drafted movement cannot be re-booked.
	if err := f.flow.SetAccount(ctx, f.sub, f.user, payer.UUID, "120.01.016", ""); !errors.Is(err, ErrDrafted) {
		t.Fatalf("err = %v", err)
	}
	// Nothing left to draft.
	if dr, err := f.flow.CreateDrafts(ctx, scope, f.user, "2026-10"); err != nil || len(dr.Drafts) != 0 {
		t.Fatalf("second draft run %+v %v", dr, err)
	}

	// Next month the same payer is recognised by the learned alias.
	f.ingest(t, mv("5", time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), banking.DirIn, "9000", "HAVALE X YAZILIM", "X YAZILIM TEKNOLOJILERI AS", payerIBAN, ""))
	if _, err := f.flow.Run(ctx, f.sub, "2026-11"); err != nil {
		t.Fatal(err)
	}
	nov := f.item(t, "2026-11", "HAVALE X YAZILIM")
	if nov.AccountCode != "120.01.015" || nov.Source != SourceAlias || nov.Confidence == nil || nov.Confidence.String() != "0.9" {
		t.Fatalf("november %+v", nov)
	}
	// A rule alias clears the default threshold (0.95): approved at once,
	// and the automatic approval does not count as a decision.
	als, _ := f.svc.Aliases(ctx, f.sub, "")
	for _, a := range als {
		if a.MatchType == MatchIBAN {
			if _, err := f.svc.Promote(ctx, f.sub, a.UUID, f.user); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.ingest(t, mv("6", time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), banking.DirIn, "500", "FAST X YAZILIM", "X Yazılım", payerIBAN, ""))
	res, err = f.flow.Run(ctx, f.sub, "2026-11")
	if err != nil || res.AutoApproved != 2 {
		t.Fatalf("rule run %+v %v", res, err)
	}
	als, _ = f.svc.Aliases(ctx, f.sub, StatusRule)
	if len(als) != 1 || als[0].Accepts+als[0].Corrections != 1 {
		t.Fatalf("automatic approvals counted: %+v", als)
	}
}
