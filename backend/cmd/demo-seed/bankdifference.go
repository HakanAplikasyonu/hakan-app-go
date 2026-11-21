package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	reviewrepo "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	review "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// DemoDiffIBAN is the demo taxpayer's second bank account (Garanti, valid
// check digits, no real account): its October statement and its 102.02
// balance differ by two reasons (#780 C).
const DemoDiffIBAN = "TR980006200000000099124518"

// seedBankDifference gives the bank screen's "Muhasebe farkı" tab a story:
// a Garanti account with an October statement (balances 0 → 23.150 TL),
// two movements booked to 102.02, a card payment not booked yet (−650) and
// a manual bank-fee line on 102.02 without a bank movement (90). Statement
// 23.150, ledger 23.710: the −560 difference is the two reasons. The drafts
// stay drafts (the default trial balance counts approved entries only).
// Idempotent: skipped when the account exists; adds new rows only.
func seedBankDifference(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	q := db.New(pool)
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cash_boxes WHERE taxpayer_id = $1 AND iban = $2)`,
		scope.TaxpayerID, DemoDiffIBAN).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	box, err := q.InsertCashBox(ctx, db.InsertCashBoxParams{TaxpayerID: scope.TaxpayerID, Type: "bank_account", Name: "Garanti TL vadesiz",
		BankID: pgtype.Text{String: "garanti", Valid: true}, Iban: pgtype.Text{String: DemoDiffIBAN, Valid: true}, Currency: "TRY"})
	if err != nil {
		return fmt.Errorf("cash box: %w", err)
	}
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	str := func(s string) *string { return &s }
	mv := func(id string, d int, dir banking.Direction, amount, desc, before, after string) banking.Transaction {
		t := banking.Transaction{SourceSystem: "bank_file", SourceID: "demo-fark:" + id, CaptureMethod: banking.CaptureMT940,
			BookedAt: day(d), TransactionDate: day(d), Direction: dir, Amount: amount, Currency: "TRY", Description: desc,
			BalanceAfter: str(after)}
		if before != "" {
			t.BalanceBefore = str(before)
		}
		return t
	}
	if _, err := banking.NewService(pool, nil).Ingest(ctx, scope.TaxpayerID, box.ID, nil, []banking.Transaction{
		mv("1", 1, banking.DirIn, "25000.00", "HAVALE GELEN OGUZ YILMAZ EKIM TAHSILAT", "0.00", "25000.00"),
		mv("2", 8, banking.DirOut, "1200.00", "EFT GIDEN KALEM KIRTASIYE EKIM", "", "23800.00"),
		mv("3", 20, banking.DirOut, "650.00", "KREDI KARTI ODEMESI GARANTI BONUS", "", "23150.00"),
	}); err != nil {
		return fmt.Errorf("movements: %w", err)
	}
	sub := posting.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID}
	js := journals.New(pool, seedStore)
	flow := posting.NewWorkflow(pool, review.New(reviewrepo.New(pool, q)).Register(posting.SuggestionKind, posting.Applier{}), js)
	if err := flow.SetLedgerAccount(ctx, sub, actor, box.Uuid, "102.02"); err != nil {
		return fmt.Errorf("ledger account: %w", err)
	}
	type booked struct {
		desc, counter string
		in            bool
		amount        string
	}
	page, err := flow.List(ctx, sub, "2026-10", "", 200, 0)
	if err != nil {
		return err
	}
	for _, b := range []booked{
		{"HAVALE GELEN OGUZ YILMAZ EKIM TAHSILAT", "120.01.015", true, "25000.00"},
		{"EFT GIDEN KALEM KIRTASIYE EKIM", "320.01.007", false, "1200.00"},
	} {
		var it *posting.Item
		for i := range page.Items {
			if page.Items[i].Description == b.desc {
				it = &page.Items[i]
			}
		}
		if it == nil {
			return fmt.Errorf("movement %q not found", b.desc)
		}
		if err := flow.SetAccount(ctx, sub, actor, it.UUID, b.counter, ""); err != nil {
			return fmt.Errorf("account %s: %w", b.desc, err)
		}
		t, err := q.GetBankTransactionForPosting(ctx, db.GetBankTransactionForPostingParams{TaxpayerID: scope.TaxpayerID,
			Uuid: pgtype.UUID{Bytes: it.UUID, Valid: true}})
		if err != nil {
			return err
		}
		amt := decimal.RequireFromString(b.amount)
		src := strconv.FormatInt(t.ID, 10)
		bank := journals.LineInput{Account: "102.02", Description: b.desc, SourceKind: posting.DraftSourceKind, SourceID: src}
		counter := journals.LineInput{Account: b.counter, Description: b.desc, SourceKind: posting.DraftSourceKind, SourceID: src}
		if b.in {
			bank.Debit, counter.Credit = amt, amt
		} else {
			counter.Debit, bank.Credit = amt, amt
		}
		d, err := js.CreateDraft(ctx, scope, actor, journals.DraftInput{Type: accountingexport.EntryMahsup, Date: t.TransactionDate.Time,
			Description: "Garanti TL vadesiz hareketleri " + t.TransactionDate.Time.Format("02.01.2006"),
			Origin:      map[string]any{"source": "bank_transactions", "transactions": 1}, Lines: []journals.LineInput{bank, counter}})
		if err != nil {
			return fmt.Errorf("draft %s: %w", b.desc, err)
		}
		if err := q.SetBankPostingsDraft(ctx, db.SetBankPostingsDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
			JournalDraftUuid: pgtype.UUID{Bytes: d.UUID, Valid: true}, BankTransactionIds: []int64{t.ID}}); err != nil {
			return err
		}
	}
	fee := decimal.RequireFromString("90.00")
	if _, err := js.CreateDraft(ctx, scope, actor, journals.DraftInput{Type: accountingexport.EntryMahsup, Date: day(25),
		Description: "Elle girilen banka masrafı (ekstrede yok)",
		Lines: []journals.LineInput{{Account: "780", Description: "Hesap işletim ücreti", Debit: fee},
			{Account: "102.02", Description: "Hesap işletim ücreti", Credit: fee}}}); err != nil {
		return fmt.Errorf("manual draft: %w", err)
	}
	return nil
}
