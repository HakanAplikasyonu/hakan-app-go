package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Eksik ekstre takibi (Oğuz'un geri bildirimi): Çelik Robotik'in üç banka
// hesabı (Akbank TRY, Ziraat TRY, Kuveyt Türk USD). Akbank ile Ziraat'in
// geçen ay ve önceki ay hareketi var; Kuveyt Türk'ün yalnız önceki ay
// hareketi var, geçen ayki ekstresi eksik görünür. IBAN'lar uydurmadır,
// kontrol haneleri tutar. Hesap IBAN'la, hareket kaynak anahtarıyla bir kez
// eklenir; yeniden çalıştırmada çoğalmaz.
const bankCoverageSource = "demo-eksik-ekstre"

type bankCoverageAccount struct {
	name, bank, iban, currency string
	// lastMonth: the account has last month's statement.
	lastMonth bool
}

var bankCoverageAccounts = []bankCoverageAccount{
	{name: "Akbank TL", bank: "akbank", iban: "TR570004600000780000012001", currency: "TRY", lastMonth: true},
	{name: "Ziraat TL", bank: "ziraat", iban: "TR090001000000780000012002", currency: "TRY", lastMonth: true},
	{name: "Kuveyt Türk Dolar", bank: "kuveyt_turk", iban: "TR230020500000780000012003", currency: "USD"},
}

// seedBankCoverage adds Çelik's three accounts and their movements; it only
// adds what is missing.
func seedBankCoverage(ctx context.Context, pool *pgxpool.Pool, now time.Time) error {
	var taxID string
	for _, tp := range demo.OperatingTaxpayers {
		if tp.Key == demo.CelikKey {
			taxID = tp.TaxID
		}
	}
	t := now.In(istanbul)
	first := time.Date(t.Year(), t.Month(), 1, 12, 0, 0, 0, time.UTC)
	last, before := first.AddDate(0, -1, 0), first.AddDate(0, -2, 0)
	return rls.System(ctx, pool, "demo-seed: bank statement follow-up", func(ctx context.Context, tx pgx.Tx) error {
		var tp int64
		err := tx.QueryRow(ctx, `SELECT id FROM taxpayers WHERE tax_id = $1`, taxID).Scan(&tp)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("celik taxpayer: %w", err)
		}
		for _, a := range bankCoverageAccounts {
			var box int64
			err := tx.QueryRow(ctx, `SELECT id FROM cash_boxes WHERE taxpayer_id = $1 AND iban = $2`, tp, a.iban).Scan(&box)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, type, name, bank_id, iban, currency)
					VALUES ($1, 'bank_account', $2, $3, $4, $5) RETURNING id`, tp, a.name, a.bank, a.iban, a.currency).Scan(&box)
			}
			if err != nil {
				return fmt.Errorf("bank account %s: %w", a.name, err)
			}
			months := []time.Time{before}
			if a.lastMonth {
				months = append(months, last)
			}
			for _, m := range months {
				period := m.Format("2006-01")
				day := time.Date(m.Year(), m.Month(), 12, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
				src := fmt.Sprintf("%s-%s-%s", bankCoverageSource, a.bank, period)
				if _, err := tx.Exec(ctx, `
					INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method,
						booked_at, transaction_date, debit_credit, amount, currency, description, period, counterparty_name)
					SELECT $1, $2, 'bank_demo', $3, 'excel_import', $4::date + time '10:00', $4::date, 'A', 4250, $5,
					       'FAST GELEN TAHSILAT', $6, 'Robot Parça Ltd.'
					WHERE NOT EXISTS (SELECT 1 FROM bank_transactions WHERE taxpayer_id = $1 AND source_system = 'bank_demo' AND source_id = $3)`,
					tp, box, src, day, a.currency, period); err != nil {
					return fmt.Errorf("bank movement %s: %w", src, err)
				}
			}
		}
		return nil
	})
}
