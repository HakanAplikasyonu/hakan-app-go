package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exchangerates"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TCMB günlük kurları (#780): demoda ağ yok; son dört ayın iş günleri için
// USD, EUR ve GBP kurları sabit bir formülle yazılır (her gün biraz artar).
// Gerçek TCMB satırı varsa dokunulmaz; tekrar çalıştırmada çoğalmaz. Böylece
// Çelik Robotik'in kuru eksik yabancı faturaları TCMB kuruyla dolar.

// demoRateBases: 1 Ocak 2026 döviz alış kuru ve günlük artış (TL).
var demoRateBases = []struct {
	currency    string
	base, daily float64
}{
	{"USD", 34.20, 0.012},
	{"EUR", 36.90, 0.013},
	{"GBP", 43.40, 0.015},
}

// demoRateFrom is the first day with demo rates: the first day of the
// month four months before now.
func demoRateFrom(now time.Time) time.Time {
	t := now.In(exchangerates.Istanbul)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -4, 0)
}

// demoRateDays lists the business days from demoRateFrom up to yesterday.
func demoRateDays(now time.Time) []time.Time {
	t := now.In(exchangerates.Istanbul)
	yesterday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return exchangerates.MissingDays(demoRateFrom(now), yesterday, nil)
}

// seedExchangeRates writes the demo bulletins (source "demo").
func seedExchangeRates(ctx context.Context, pool *pgxpool.Pool, now time.Time) error {
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	batch := &pgx.Batch{}
	for _, d := range demoRateDays(now) {
		days := d.Sub(epoch).Hours() / 24
		for _, c := range demoRateBases {
			buy := c.base + c.daily*days
			batch.Queue(`INSERT INTO exchange_rates (date, currency, unit, forex_buying, forex_selling,
					banknote_buying, banknote_selling, source)
				VALUES ($1, $2, 1, ROUND(($3::float8)::numeric, 4), ROUND(($3::float8)::numeric * 1.0018, 4),
					ROUND(($3::float8)::numeric * 0.9993, 4), ROUND(($3::float8)::numeric * 1.0028, 4), 'demo')
				ON CONFLICT (date, currency) DO NOTHING`,
				pgtype.Date{Time: d, Valid: true}, c.currency, buy)
		}
	}
	if batch.Len() == 0 {
		return nil
	}
	return pool.SendBatch(ctx, batch).Close()
}

// fillDemoForeignRates fills the office's draft foreign invoices without a
// kur from the table (the worker does this after each TCMB fetch).
func fillDemoForeignRates(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: foreign invoice TCMB rates", func(ctx context.Context, _ pgx.Tx) error {
		_, err := database.QueriesFor(pool).FillForeignInvoiceExchangeRates(ctx, pgtype.Int8{Int64: orgID, Valid: true})
		return err
	})
}
