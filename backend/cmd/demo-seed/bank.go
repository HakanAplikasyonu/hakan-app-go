package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	reviewrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	review "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// DemoBankIBAN is the demo taxpayer's bank account (valid check digits, no
// real account). The e2e smoke adds its own Ziraat account (TR76…), so the
// demo one differs.
const DemoBankIBAN = "TR540001000000004457200128"

// demoPayerIBAN is the account "X Yazılım Teknolojileri" pays from.
const demoPayerIBAN = "TR290006200000006299001234"

// seedBank gives the pilot demo a bank month to match (#383): an October
// statement of the taxpayer's Ziraat account, the mizan code of the
// account, the alias that says the payments of "X Yazılım Teknolojileri
// A.Ş." are Oğuz Yılmaz's cari, a supplier known from invoices (cari
// hafızası) and the suggestions of the month in the review queue. The
// payments are approved, booked and exported to Luca; one collection waits
// in the queue.
func seedBank(ctx context.Context, pool *pgxpool.Pool, sub posting.Subject, actor int64) error {
	q := db.New(pool)
	box, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{TaxpayerID: sub.TaxpayerID, Type: "bank_account", Name: "Ziraat TL vadesiz",
		BankID: pgtype.Text{String: "ziraat", Valid: true}, Iban: pgtype.Text{String: DemoBankIBAN, Valid: true}, Currency: "TRY"})
	if err != nil {
		return fmt.Errorf("cash box: %w", err)
	}
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	mv := func(id string, d int, dir banking.Direction, amount, desc, name, iban, tax string) banking.Transaction {
		return banking.Transaction{SourceSystem: "bank_file", SourceID: "demo:" + id, CaptureMethod: banking.CaptureExcel,
			BookedAt: day(d), TransactionDate: day(d), Direction: dir, Amount: amount, Currency: "TRY", Description: desc,
			CounterpartyName: name, CounterpartyIBAN: iban, CounterpartyTaxID: tax}
	}
	bank := banking.NewService(pool, nil)
	if _, err := bank.Ingest(ctx, sub.TaxpayerID, box.ID, nil, []banking.Transaction{
		mv("1", 1, banking.DirIn, "12500.00", "FAST GELEN GONDEREN: X YAZILIM TEKNOLOJILERI A.S. EKIM HIZMET BEDELI", "X Yazılım Teknolojileri A.Ş.", demoPayerIBAN, ""),
		mv("2", 2, banking.DirOut, "3250.75", "EFT GIDEN ALICI: KALEM KIRTASIYE LTD STI FATURA KRT2026000001207", "Kalem Kırtasiye Ltd. Şti.", "", "9200000035"),
		mv("3", 2, banking.DirOut, "8.50", "EFT UCRETI", "", "", ""),
		mv("4", 2, banking.DirOut, "0.43", "BSMV", "", "", ""),
		// Eylül yükümlülükleri (ledger.go): tutar ve tarihler bordro ve
		// beyanname ekranlarıyla aynı.
		mv("5", 10, banking.DirOut, sepPayrollNet, "MAAS ODEMESI EYLUL PERSONEL", "", "", ""),
		mv("6", 26, banking.DirOut, sepKDVPayable, "GIB VERGI ODEMESI KDV 2026/09", "", "", ""),
		mv("7", 26, banking.DirOut, sepMUHSGKPayable, "GIB VERGI ODEMESI MUHSGK 2026/09", "", "", ""),
		mv("8", 28, banking.DirIn, "4800.00", "HAVALE GELEN DENIZ YILDIRIM", "Deniz Yıldırım", "", ""),
		mv("9", 30, banking.DirOut, sepSGKPayable, "SGK PRIM ODEMESI 2026/09", "", "", ""),
	}); err != nil {
		return fmt.Errorf("movements: %w", err)
	}
	flow := posting.NewWorkflow(pool, review.New(reviewrepo.New(pool, q)).Register(posting.SuggestionKind, posting.Applier{}),
		journals.New(pool, seedStore))
	if err := flow.SetLedgerAccount(ctx, sub, actor, box.Uuid, "102.01"); err != nil {
		return fmt.Errorf("ledger account: %w", err)
	}
	dir := string(banking.DirIn)
	aliases := posting.NewService(pool)
	if _, err := aliases.CreateAlias(ctx, sub, posting.AliasInput{MatchType: posting.MatchIBAN, MatchValue: demoPayerIBAN,
		DisplayName: "X Yazılım Teknolojileri A.Ş.", Direction: &dir, AccountCode: "120.01.015", AccountName: "Oğuz Yılmaz"}, actor); err != nil {
		return fmt.Errorf("alias: %w", err)
	}
	if err := q.UpsertCounterpartyAccount(ctx, db.UpsertCounterpartyAccountParams{OrganizationID: sub.OrganizationID,
		EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID, CounterpartyTaxID: "9200000035", MainCode: "320",
		AccountCode: "320.01.007", AccountName: "Kalem Kırtasiye Ltd. Şti."}); err != nil {
		return fmt.Errorf("cari memory: %w", err)
	}
	if _, err := flow.Run(ctx, sub, "2026-10"); err != nil {
		return fmt.Errorf("suggestions: %w", err)
	}
	// X Yazılım tahsilatı takma adla (alias) kendiliğinden onaylanır; giden
	// ödemeler burada onaylanır. Onaylılar fişlenip Luca'ya aktarılır (Ekim
	// mizanında yer alırlar). Deniz Yıldırım havalesi kuyrukta bekler: demoda
	// cari seçilip onaylanır ve "Fiş taslağı oluştur" ile fişlenir.
	page, err := flow.List(ctx, sub, "2026-10", "suggested", 50, 0)
	if err != nil {
		return err
	}
	var out []uuid.UUID
	for _, it := range page.Items {
		if banking.Direction(it.Direction) == banking.DirOut {
			out = append(out, it.UUID)
		}
	}
	if len(out) > 0 {
		if _, err := flow.Approve(ctx, sub, actor, out); err != nil {
			return fmt.Errorf("approve: %w", err)
		}
	}
	scope := engagementctx.Scope{ID: sub.EngagementID, OrganizationID: sub.OrganizationID, TaxpayerID: sub.TaxpayerID}
	drafts, err := flow.CreateDrafts(ctx, scope, actor, "2026-10")
	if err != nil {
		return fmt.Errorf("bank drafts: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(drafts.Drafts))
	for _, d := range drafts.Drafts {
		ids = append(ids, d.UUID)
	}
	return approveAndExport(ctx, pool, scope, actor, ids)
}
