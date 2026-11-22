package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// closeChainPeriod is the month the demo closes (Sıradaki iş's default).
const closeChainPeriod = "2026-10"

// seedCloseChain (#780, ay sonu kapanış) adds a few new rows so the
// October chain shows a spread of steps across the demo taxpayers:
//
//   - selin (bilanço): the Luca mizan is uploaded → next is "KDV bildirildi";
//   - kaya (işletme): one masraf fişi waits for approval → "Fişler onaylı";
//   - deniz (işletme): has an employee but no Luca payroll → "Luca'dan alındı";
//   - arslan (serbest meslek): the VAT notice was sent → only the report.
//
// Atlas keeps the state its own seeders leave. Idempotent: every insert is
// skipped when its row already exists.
func seedCloseChain(ctx context.Context, pool *pgxpool.Pool, orgID int64) error {
	return rls.System(ctx, pool, "demo-seed: month-end close chain", func(ctx context.Context, tx pgx.Tx) error {
		eng := func(key string) (engID, tpID int64, ok bool, err error) {
			var taxID string
			for _, tp := range demo.Taxpayers {
				if tp.Key == key {
					taxID = tp.TaxID
				}
			}
			for _, tp := range demo.OperatingTaxpayers {
				if tp.Key == key {
					taxID = tp.TaxID
				}
			}
			err = tx.QueryRow(ctx, `SELECT e.id, e.taxpayer_id FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
				WHERE e.organization_id = $1 AND t.tax_id = $2 AND e.ended_at IS NULL
				ORDER BY e.id DESC LIMIT 1`, orgID, taxID).Scan(&engID, &tpID)
			if err == pgx.ErrNoRows {
				return 0, 0, false, nil
			}
			return engID, tpID, err == nil, err
		}
		if e, _, ok, err := eng("selin"); err != nil {
			return fmt.Errorf("selin: %w", err)
		} else if ok {
			if _, err := tx.Exec(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, target, source_format,
					source_kind, file_name, sha256, account_count, period)
				SELECT $1, $2, 'luca', 'xlsx', 'trial_balance', 'selin-mizan-2026-10.xlsx', repeat('5', 64), 0, $3
				WHERE NOT EXISTS (SELECT 1 FROM account_plan_imports
					WHERE organization_id = $1 AND engagement_id = $2 AND source_kind = 'trial_balance' AND period = $3)`,
				orgID, e, closeChainPeriod); err != nil {
				return fmt.Errorf("selin mizan: %w", err)
			}
		}
		if _, tp, ok, err := eng("kaya"); err != nil {
			return fmt.Errorf("kaya: %w", err)
		} else if ok {
			if _, err := tx.Exec(ctx, `INSERT INTO expenses (taxpayer_id, source_system, source_id, capture_method, expense_date,
					period, amount, tax_rate, tax_exclusive_amount, tax_amount, description, receipt_type, vendor_name)
				VALUES ($1, 'demo', 'close-chain-2026-10', 'ocr', '2026-10-28', $2, 1200, 20, 1000, 200,
					'Kırtasiye toptancısı', 'okc_receipt', 'Demo Toptan Kırtasiye')
				ON CONFLICT (taxpayer_id, source_system, source_id) DO NOTHING`, tp, closeChainPeriod); err != nil {
				return fmt.Errorf("kaya expense: %w", err)
			}
		}
		if _, tp, ok, err := eng("deniz"); err != nil {
			return fmt.Errorf("deniz: %w", err)
		} else if ok {
			if _, err := tx.Exec(ctx, `INSERT INTO employees (taxpayer_id, full_name, job_title, hire_date)
				SELECT $1, 'Murat Şahin', 'Kasiyer', '2026-03-01'
				WHERE NOT EXISTS (SELECT 1 FROM employees WHERE taxpayer_id = $1 AND full_name = 'Murat Şahin')`, tp); err != nil {
				return fmt.Errorf("deniz employee: %w", err)
			}
		}
		if e, tp, ok, err := eng("arslan"); err != nil {
			return fmt.Errorf("arslan: %w", err)
		} else if ok {
			if _, err := tx.Exec(ctx, `INSERT INTO vat_notices (organization_id, engagement_id, taxpayer_id, period, outcome,
					output_vat, input_vat, carried_in, payable, carry_out, due_date, message, created_at)
				SELECT $1, $2, $3, $4, 'payment', 6000, 1500, 0, 4500, 0, '2026-11-26',
					'Ekim KDV ödemeniz 4.500 TL, son gün 26 Kasım.', '2026-11-20 10:00:00+03'
				WHERE NOT EXISTS (SELECT 1 FROM vat_notices WHERE organization_id = $1 AND engagement_id = $2 AND period = $4)`,
				orgID, e, tp, closeChainPeriod); err != nil {
				return fmt.Errorf("arslan vat notice: %w", err)
			}
		}
		return nil
	})
}
