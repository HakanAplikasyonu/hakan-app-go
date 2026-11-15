package banking

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

func day(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
func ptr[T any](v T) *T   { return &v }

func setup(t *testing.T) (*Service, *db.Queries, int64, int64) {
	t.Helper()
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	q := db.New(d.Pool)
	box, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{
		TaxpayerID: tp, Type: "bank_account", Name: "Ziraat TL", Currency: "TRY",
		BankID: pgtype.Text{String: "ziraat", Valid: true},
		Iban:   pgtype.Text{String: ownIBAN, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewService(d.Pool, nil), q, tp, box.ID
}

func TestIngestDedupeAndClassify(t *testing.T) {
	svc, q, tp, box := setup(t)
	ctx := context.Background()

	api := []Transaction{
		{SourceSystem: "bank_ziraat", SourceID: "Z-1", CaptureMethod: CaptureAPI, TransactionDate: day(3),
			Direction: DirOut, Amount: "12.50", Currency: "TL", BalanceAfter: ptr("987.50"),
			BankTxnCode: "218F", Description: "Serbest masraf"},
		{SourceSystem: "bank_ziraat", SourceID: "Z-2", CaptureMethod: CaptureAPI, TransactionDate: day(4),
			Direction: DirIn, Amount: "500", BalanceAfter: ptr("1487.50"),
			Description: "FAST gelen", CounterpartyIBAN: "TR33 0006 1005 1978 6457 8413 26"},
	}
	res, err := svc.Ingest(ctx, tp, box, nil, api)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 2 || res.Merged != 0 {
		t.Fatalf("first ingest: %+v", res)
	}

	// Same API window again: rule 1, nothing new.
	res, err = svc.Ingest(ctx, tp, box, nil, api)
	if err != nil || res.Skipped != 2 || res.Inserted != 0 {
		t.Fatalf("re-ingest: %+v %v", res, err)
	}

	// The same movement from an Excel upload: rule 2 (running balance) merges,
	// the extra column fills an empty field, the API stays primary.
	imp := uuid.New()
	res, err = svc.Ingest(ctx, tp, box, &imp, []Transaction{{
		SourceSystem: "bank_file", CaptureMethod: CaptureExcel, TransactionDate: day(3),
		Direction: DirOut, Amount: "12.5000", BalanceAfter: ptr("987.5"),
		Description: "SERBEST MASRAF", ReceiptNumber: "DK-77",
	}})
	if err != nil || res.Merged != 1 || res.Inserted != 0 {
		t.Fatalf("excel merge: %+v %v", res, err)
	}

	// Without a balance, a lookalike is stored but flagged (rule 3).
	noBal := Transaction{SourceSystem: "bank_file", CaptureMethod: CapturePDF, TransactionDate: day(10),
		Direction: DirOut, Amount: "75", Description: "Kasko poliçesi Ekim taksidi"}
	if _, err := svc.Ingest(ctx, tp, box, &imp, []Transaction{noBal}); err != nil {
		t.Fatal(err)
	}
	noBal.SourceID = "other-pdf-row"
	res, err = svc.Ingest(ctx, tp, box, &imp, []Transaction{noBal})
	if err != nil || res.PossibleDuplicates != 1 || res.Inserted != 1 {
		t.Fatalf("lookalike: %+v %v", res, err)
	}

	rows, err := q.ListBankTransactionsToClassify(ctx, db.ListBankTransactionsToClassifyParams{TaxpayerID: tp})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d", len(rows))
	}
	fee := rows[0]
	if fee.SourceID != "Z-1" || fee.CaptureMethod != CaptureAPI || fee.ReceiptNumber.String != "DK-77" || fee.Currency != "TRY" {
		t.Errorf("merged row: source=%s/%s receipt=%q currency=%s", fee.SourceID, fee.CaptureMethod, fee.ReceiptNumber.String, fee.Currency)
	}
	if fee.Kind != string(KindUcretKomisyonFaiz) || fee.KindDetail != string(DetailBankFee) || fee.KindSource.String != string(SourceBankCode) {
		t.Errorf("fee classification: %s/%s/%s", fee.Kind, fee.KindDetail, fee.KindSource.String)
	}
	if rows[1].KindDetail != string(DetailOwnTransfer) {
		t.Errorf("own transfer: %s", rows[1].KindDetail)
	}
	if !rows[3].PossibleDuplicateOf.Valid || rows[3].PossibleDuplicateOf.Int64 != rows[2].ID {
		t.Errorf("possible duplicate not linked: %+v", rows[3].PossibleDuplicateOf)
	}
}

func TestCorrectLearnsRule(t *testing.T) {
	svc, q, tp, box := setup(t)
	ctx := context.Background()
	in := []Transaction{{SourceSystem: "bank_ziraat", SourceID: "1", CaptureMethod: CaptureAPI, TransactionDate: day(5),
		Direction: DirOut, Amount: "3000", Description: "ODEME", CounterpartyName: "Kalem Kırtasiye Ltd"}}
	if _, err := svc.Ingest(ctx, tp, box, nil, in); err != nil {
		t.Fatal(err)
	}
	rows, _ := q.ListBankTransactionsToClassify(ctx, db.ListBankTransactionsToClassifyParams{TaxpayerID: tp})
	if rows[0].KindDetail != string(DetailUnknown) {
		t.Fatalf("expected unknown first, got %s", rows[0].KindDetail)
	}
	firstID := rows[0].ID
	rule, err := svc.Correct(ctx, tp, firstID, KindEFT, DetailSupplierPayment, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rule.MatchType != "description" || rule.MatchValue != "KALEM KIRTASIYE LTD" {
		t.Fatalf("rule: %s=%q", rule.MatchType, rule.MatchValue)
	}
	// Same correction again does not duplicate the rule.
	if _, err := svc.Correct(ctx, tp, firstID, KindEFT, DetailSupplierPayment, nil); err != nil {
		t.Fatal(err)
	}
	if rules, _ := q.ListBankRules(ctx, tp); len(rules) != 1 {
		t.Fatalf("rules = %d", len(rules))
	}

	// The next payment to the same counterparty is classified by the rule.
	in[0].SourceID, in[0].Amount, in[0].TransactionDate = "2", "1200", day(20)
	if _, err := svc.Ingest(ctx, tp, box, nil, in); err != nil {
		t.Fatal(err)
	}
	rows, _ = q.ListBankTransactionsToClassify(ctx, db.ListBankTransactionsToClassifyParams{TaxpayerID: tp})
	var second db.BankTransaction
	for _, r := range rows {
		if r.SourceID == "2" {
			second = r
		}
	}
	if second.KindSource.String != string(SourceCompanyRule) || second.KindDetail != string(DetailSupplierPayment) {
		t.Fatalf("learned rule not applied: %s/%s", second.KindSource.String, second.KindDetail)
	}
	// A manual correction survives a full reclassification.
	if _, err := svc.Classify(ctx, tp, "2026-10", false); err != nil {
		t.Fatal(err)
	}
	first, _ := q.GetBankTransaction(ctx, db.GetBankTransactionParams{TaxpayerID: tp, ID: firstID})
	if first.KindSource.String != string(SourceManual) {
		t.Fatalf("manual overwritten: %s", first.KindSource.String)
	}
}

func TestIngestRejectsBadInput(t *testing.T) {
	svc, _, tp, box := setup(t)
	_, err := svc.Ingest(context.Background(), tp, box, nil, []Transaction{{SourceSystem: "bank_ziraat",
		CaptureMethod: CaptureAPI, TransactionDate: day(1), Direction: DirOut, Amount: "-5"}})
	if err == nil {
		t.Fatal("negative amount accepted")
	}
}
