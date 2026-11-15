package statement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type fixture struct {
	im    *Importer
	bank  *banking.Service
	q     *db.Queries
	d     *testdb.DB
	tp    int64
	org   int64
	eng   int64
	box   int64
	other int64
}

func setupImport(t *testing.T) fixture {
	t.Helper()
	d := testdb.Open(t)
	ctx := context.Background()
	tp := d.Taxpayer(t, "legal", "9000000017")
	org := d.Organization(t)
	eng := d.Engagement(t, org, tp)
	q := db.New(d.Pool)
	mk := func(iban, cur string) int64 {
		b, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{
			TaxpayerID: tp, Type: "bank_account", Currency: cur,
			Iban: pgtype.Text{String: iban, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b.ID
	}
	bank := banking.NewService(d.Pool, nil)
	return fixture{im: NewImporter(d.Pool, bank), bank: bank, q: q, d: d, tp: tp, org: org, eng: eng,
		box: mk(ibanZiraat, "TRY"), other: mk(ibanAkbank, "USD")}
}

func (f fixture) count(t *testing.T, box int64) int {
	t.Helper()
	var n int
	if err := f.d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM bank_transactions WHERE cash_box_id = $1`, box).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const goodMT940 = ":20:S1\n:25:" + ibanZiraat + "\n:60F:C261001TRY1000,00\n" +
	":61:2610011001C500,00NTRFNONREF//R1\n:86:FAST GELEN GONDEREN: YILDIZ MARKET\n" +
	":61:2610011001D12,50NCOMNONREF//R2\n:86:EFT UCRETI BSMV\n" +
	":62F:C261001TRY1487,50\n:64:C261001TRY1487,50\n" +
	":20:S2\n:25:" + ibanAkbank + "\n:60F:C261001USD10,00\n:61:2610011001C5,NTRFNONREF\n:86:SWIFT\n:62F:C261001USD15,00\n"

func TestImportMT940TwiceAndPreview(t *testing.T) {
	f := setupImport(t)
	ctx := context.Background()
	req := Request{TaxpayerID: f.tp, OrganizationID: f.org, EngagementID: f.eng, FileName: "ekim.sta", Data: []byte(goodMT940)}

	// Preview: full report, nothing written.
	pr := req
	pr.Preview = true
	rep, err := f.im.Import(ctx, pr)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Accounts) != 2 || rep.Accounts[0].Inserted != 2 || rep.Accounts[1].Inserted != 1 || f.count(t, f.box) != 0 {
		t.Fatalf("preview: %+v rows=%d", rep.Accounts, f.count(t, f.box))
	}

	// Test 4: one file, two accounts → two cash boxes.
	rep, err = f.im.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if f.count(t, f.box) != 2 || f.count(t, f.other) != 1 || len(rep.Accounts[0].Gaps) != 0 {
		t.Fatalf("import: ziraat=%d akbank=%d gaps=%v", f.count(t, f.box), f.count(t, f.other), rep.Accounts[0].Gaps)
	}
	// Test 5: the same MT940 again creates nothing.
	rep, err = f.im.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if f.count(t, f.box) != 2 || rep.Accounts[0].Skipped != 2 {
		t.Fatalf("re-import: rows=%d %+v", f.count(t, f.box), rep.Accounts[0])
	}
	// Classified on import: the fee is a commission with BSMV.
	rows, _ := f.q.ListBankTransactionsToClassify(ctx, db.ListBankTransactionsToClassifyParams{TaxpayerID: f.tp})
	for _, r := range rows {
		if r.Mt940Code.String == "COM" && r.KindDetail != string(banking.DetailBSMV) {
			t.Errorf("COM line: %s/%s", r.Kind, r.KindDetail)
		}
		if r.CashBoxID == f.other && r.Currency != "USD" {
			t.Errorf("currency: %s", r.Currency)
		}
	}
}

// Test 6: a day that came from the bank service is uploaded again as MT940:
// no duplicate, empty fields are filled, the service row stays primary.
func TestImportMergesWithServiceRows(t *testing.T) {
	f := setupImport(t)
	ctx := context.Background()
	bal1, bal2 := "1500", "1487.5"
	_, err := f.bank.Ingest(ctx, f.tp, f.box, nil, []banking.Transaction{
		{SourceSystem: "bank_ziraat", SourceID: "Z1", CaptureMethod: banking.CaptureAPI, TransactionDate: day(1),
			Direction: banking.DirIn, Amount: "500", BalanceAfter: &bal1, Description: "FAST GELEN"},
		{SourceSystem: "bank_ziraat", SourceID: "Z2", CaptureMethod: banking.CaptureAPI, TransactionDate: day(1),
			Direction: banking.DirOut, Amount: "12.50", BalanceAfter: &bal2, Description: "EFT UCRETI"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := f.im.Import(ctx, Request{TaxpayerID: f.tp, FileName: "ekim.sta", Data: []byte(goodMT940)})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Accounts[0].Merged != 2 || rep.Accounts[0].Inserted != 0 || f.count(t, f.box) != 2 {
		t.Fatalf("merge: %+v rows=%d", rep.Accounts[0], f.count(t, f.box))
	}
	rows, _ := f.q.ListBankTransactionsToClassify(ctx, db.ListBankTransactionsToClassifyParams{TaxpayerID: f.tp})
	for _, r := range rows {
		if r.CashBoxID != f.box {
			continue
		}
		if r.CaptureMethod != banking.CaptureAPI || !r.Mt940Code.Valid || !r.BankReference.Valid {
			t.Errorf("row %s: capture=%s mt940=%v ref=%v", r.SourceID, r.CaptureMethod, r.Mt940Code, r.BankReference)
		}
	}

	// Undo the MT940 upload: the service rows stay (their MT940 sources go),
	// the Akbank movement that only came from the file is deleted.
	n, err := f.bank.UndoImport(ctx, f.tp, rep.ImportID)
	if err != nil || n != 1 || f.count(t, f.box) != 2 {
		t.Fatalf("undo: deleted=%d err=%v rows=%d", n, err, f.count(t, f.box))
	}
	// The Akbank block had no service rows: undo deletes it.
	if f.count(t, f.other) != 0 {
		t.Fatalf("akbank rows after undo: %d", f.count(t, f.other))
	}
}

// Test 10: an account the taxpayer does not have → nothing is imported.
func TestImportUnknownAccount(t *testing.T) {
	f := setupImport(t)
	text := ":20:X\n:25:TR940001000999990123456789\n:61:2610011001C5,00NTRFNONREF\n:86:X\n"
	_, err := f.im.Import(context.Background(), Request{TaxpayerID: f.tp, FileName: "x.sta", Data: []byte(text)})
	var ue *UnknownAccountError
	if !errors.As(err, &ue) || ue.Accounts[0] != "TR940001000999990123456789" {
		t.Fatalf("want unknown account error, got %v", err)
	}
	if f.count(t, f.box) != 0 {
		t.Fatal("rows written")
	}
}

// A statement whose balance does not add up is imported with BNK_GAP; so is
// one whose opening differs from the previous statement's closing.
func TestImportRaisesGap(t *testing.T) {
	f := setupImport(t)
	ctx := context.Background()
	day1 := ":20:A\n:25:" + ibanZiraat + "\n:60F:C261001TRY1000,00\n:61:2610011001C100,00NTRFNONREF\n:86:X\n:62F:C261001TRY1100,00\n"
	day3 := ":20:B\n:25:" + ibanZiraat + "\n:60F:C261003TRY1250,00\n:61:2610031003D50,00NTRFNONREF\n:86:Y\n:62F:C261003TRY1200,00\n"
	for _, s := range []string{day1, day3} {
		if _, err := f.im.Import(ctx, Request{TaxpayerID: f.tp, OrganizationID: f.org, EngagementID: f.eng, FileName: "g.sta", Data: []byte(s)}); err != nil {
			t.Fatal(err)
		}
	}
	var code, severity, period string
	err := f.d.Pool.QueryRow(ctx, `SELECT code, severity, period FROM data_warnings WHERE engagement_id = $1 AND status = 'open'`, f.eng).Scan(&code, &severity, &period)
	if err != nil || code != "BNK_GAP" || severity != "blocker" || period != "2026-10" {
		t.Fatalf("warning: %s %s %s %v", code, severity, period, err)
	}
}

// CSV account picked by the uploader when the file names none.
func TestImportCSVWithChosenAccount(t *testing.T) {
	f := setupImport(t)
	csvNoIBAN := "Tarih;Açıklama;Tutar;Bakiye\n01.10.2026;FAST GELEN;1.000,00;2.000,00\n02.10.2026;MASRAF;-12,50;1.987,50\n"
	ctx := context.Background()
	if _, err := f.im.Import(ctx, Request{TaxpayerID: f.tp, FileName: "h.csv", Data: []byte(csvNoIBAN)}); !errors.Is(err, ErrAccountRequired) {
		t.Fatalf("no account and no choice: %v", err)
	}
	rep, err := f.im.Import(ctx, Request{TaxpayerID: f.tp, FileName: "h.csv", Data: []byte(csvNoIBAN), CashBoxID: f.box})
	if err != nil || rep.Accounts[0].Inserted != 2 || rep.Accounts[0].TotalDebit != "12.50" {
		t.Fatalf("csv: %+v %v", rep.Accounts, err)
	}
}

func day(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
