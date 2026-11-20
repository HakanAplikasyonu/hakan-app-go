package matching

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// TestMatchJournalLine (#669 G): a collection with no invoice is tied to the
// cari line of an existing voucher; the uninvoiced warning resolves; a
// re-match replaces the line in one step; another engagement's line and the
// wrong side are refused.
func TestMatchJournalLine(t *testing.T) {
	d := testdb.Open(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	tp := d.Taxpayer(t, "legal", "9000000025")
	org := d.Organization(t)
	user := d.User(t)
	eng := d.Engagement(t, org, tp)
	// Another office's taxpayer: its voucher must never be reachable.
	otherOrg := d.Organization(t)
	otherTp := d.Taxpayer(t, "legal", "9000000033")
	otherEng := d.Engagement(t, otherOrg, otherTp)

	box, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{TaxpayerID: tp, Type: "bank_account", Currency: "TRY",
		Iban: pgtype.Text{String: ownIBAN, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	bank := banking.NewService(d.Pool, nil)
	if _, err := bank.Ingest(ctx, tp, box.ID, nil, []banking.Transaction{
		{SourceSystem: "bank_ziraat", SourceID: "jl-in", CaptureMethod: banking.CaptureAPI, TransactionDate: on(10),
			Direction: banking.DirIn, Amount: "1000", Currency: "TRY", Description: "FAST GELEN", CounterpartyTaxID: "9500000074"},
	}); err != nil {
		t.Fatal(err)
	}
	var txID uuid.UUID
	if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM bank_transactions WHERE taxpayer_id = $1 AND source_id = 'jl-in'`, tp).Scan(&txID); err != nil {
		t.Fatal(err)
	}

	// voucher inserts a journal draft with lines (account, debit, credit).
	voucher := func(o, e, tp int64, no int, lines ...[3]string) uuid.UUID {
		var id int64
		var u uuid.UUID
		if err := d.Pool.QueryRow(ctx, `INSERT INTO journal_drafts (organization_id, engagement_id, taxpayer_id, period, entry_no, entry_date, description)
			VALUES ($1, $2, $3, '2026-10', $4, '2026-10-08', 'Elle fiş') RETURNING id, uuid`, o, e, tp, no).Scan(&id, &u); err != nil {
			t.Fatal(err)
		}
		for i, l := range lines {
			if _, err := d.Pool.Exec(ctx, `INSERT INTO journal_draft_lines (organization_id, engagement_id, journal_draft_id, line_no, account_code, description, debit, credit)
				VALUES ($1, $2, $3, $4, $5, 'satır', $6::numeric, $7::numeric)`, o, e, id, i+1, l[0], l[1], l[2]); err != nil {
				t.Fatal(err)
			}
		}
		return u
	}
	v1 := voucher(org, eng, tp, 1, [3]string{"102.01", "1000", "0"}, [3]string{"120.01.074", "0", "1000"})
	v2 := voucher(org, eng, tp, 2, [3]string{"100.01", "1000", "0"}, [3]string{"649.01", "0", "1000"})
	foreign := voucher(otherOrg, otherEng, otherTp, 1, [3]string{"120.01.074", "0", "1000"})

	warn := engine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q))
	svc := NewService(d.Pool, nil, warn, Options{FeeTolerance: d25})
	sub := Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	if _, err := svc.Run(ctx, sub, "2026-10"); err != nil {
		t.Fatal(err)
	}
	openUninvoiced := func() int {
		var n int
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM data_warnings WHERE engagement_id = $1 AND code = $2 AND status = 'open'`,
			eng, CodeUninvoicedCollection).Scan(&n)
		return n
	}
	if openUninvoiced() != 1 {
		t.Fatalf("uninvoiced before = %d", openUninvoiced())
	}

	// Candidates: money in → credit lines of the amount, this engagement only.
	cands, err := svc.SearchLines(ctx, sub, txID, "", false)
	if err != nil || len(cands) != 2 {
		t.Fatalf("candidates: %+v %v", cands, err)
	}
	cari, err := svc.SearchLines(ctx, sub, txID, "", true)
	if err != nil || len(cari) != 1 || cari[0].AccountCode != "120.01.074" || !cari[0].Cari || cari[0].OpenAmount != "1000.00" {
		t.Fatalf("cari candidates: %+v %v", cari, err)
	}
	if found, err := svc.SearchLines(ctx, sub, txID, "649", false); err != nil || len(found) != 1 || found[0].JournalDraftUUID != v2 {
		t.Fatalf("search 649: %+v %v", found, err)
	}

	// The debit side (102) and another engagement's line are refused.
	if _, err := svc.Match(ctx, sub, txID, nil, []LineRef{{JournalDraftUUID: v1, LineNo: 1}}, user); !errors.Is(err, ErrInvalidLines) {
		t.Fatalf("wrong side: %v", err)
	}
	if _, err := svc.Match(ctx, sub, txID, nil, []LineRef{{JournalDraftUUID: foreign, LineNo: 1}}, user); !errors.Is(err, ErrInvalidLines) {
		t.Fatalf("other engagement: %v", err)
	}

	view, err := svc.Match(ctx, sub, txID, nil, []LineRef{{JournalDraftUUID: v1, LineNo: 2}}, user)
	if err != nil || view.MatchStatus != "matched" || len(view.JournalMatches) != 1 || view.JournalMatches[0].Amount != "1000.00" ||
		view.JournalMatches[0].Label != "2026-10 #1/2 120.01.074" {
		t.Fatalf("match line: %+v %v", view, err)
	}
	if openUninvoiced() != 0 {
		t.Fatal("a collection tied to a cari line must resolve the uninvoiced warning")
	}
	// Fully matched: no longer a candidate.
	if cari, _ := svc.SearchLines(ctx, sub, txID, "", true); len(cari) != 1 {
		t.Fatalf("own match stays a candidate for a re-match: %+v", cari)
	}

	// Re-match in one step: the 649 line replaces the 120 line.
	view, err = svc.Match(ctx, sub, txID, nil, []LineRef{{JournalDraftUUID: v2, LineNo: 2}}, user)
	if err != nil || len(view.JournalMatches) != 1 || view.JournalMatches[0].AccountCode != "649.01" || view.MatchStatus != "matched" {
		t.Fatalf("rematch: %+v %v", view, err)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM bank_transaction_matches WHERE taxpayer_id = $1 AND target_type = 'journal_line'`, tp).Scan(&n)
	if n != 1 {
		t.Fatalf("rematch must replace, matches = %d", n)
	}

	// Undo: open again, the warning comes back.
	view, err = svc.Unmatch(ctx, sub, txID)
	if err != nil || view.MatchStatus != "unmatched" || len(view.JournalMatches) != 0 || openUninvoiced() != 1 {
		t.Fatalf("unmatch: %+v %v open=%d", view, err, openUninvoiced())
	}
}
