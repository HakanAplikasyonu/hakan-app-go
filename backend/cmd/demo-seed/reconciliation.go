package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reconciliation"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedReconciliation (#389): Atlas Gıda'nın Ekim 2026 mutabakatı. Ekim
// faturalarından Form BA / BS beyan satırları (biri bilerek eksik tutarlı),
// mizan – kaynak karşılaştırması (farklar ledger.go'daki iki nedenden: 120
// açıklandı, 600 inceleme kuyruğunda); en büyük cari farkına mektup
// taslağı, eşleşen bir cariye yanıtlanmış (mutabık) mektup.
func seedReconciliation(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	if err := seedBabsDeclarations(ctx, pool, scope, actor); err != nil {
		return err
	}
	review := reviewusecase.New(reviewrepository.New(pool, db.New(pool))).Register(reconciliation.SuggestionKind, reconciliation.Applier{})
	svc := reconciliation.New(database.Ambient{Pool: pool}, review).
		WithLetters(reconciliation.LetterConfig{Secret: payrollSecret, FrontendURL: "http://localhost:3000"})
	det, err := svc.Run(ctx, scope, actor, reconciliation.RunInput{Period: "2026-10"})
	if err != nil {
		return fmt.Errorf("reconciliation run: %w", err)
	}
	// 120: Luca'daki elle kur farkı fişi, açıklandı. 600: Luca'ya
	// aktarılmamış Z raporu taslakları, inceleme kuyruğunda (ledger.go).
	for _, it := range det.Items {
		switch {
		case it.Result == reconciliation.ResultMatched:
		case it.MainCode == "120":
			note := lucaManualNote
			if _, err := svc.Decide(ctx, scope, it.UUID, actor, reconciliation.DecisionInput{Status: reconciliation.StatusExplained, Note: &note}); err != nil {
				return fmt.Errorf("reconciliation decide: %w", err)
			}
		case it.MainCode == "600":
			if _, err := svc.SendToReview(ctx, scope, it.UUID, actor, nil); err != nil {
				return fmt.Errorf("reconciliation review: %w", err)
			}
		}
	}
	page, err := svc.Counterparties(ctx, scope, det.Run.UUID.String(), "")
	if err != nil {
		return fmt.Errorf("reconciliation counterparties: %w", err)
	}
	// Mektup taslağı en büyük cari farkına (yoksa ilk cariye), yanıtlanmış
	// mektup bir başka cariye.
	var diff, matched *reconciliation.CounterpartyView
	for i := range page.Items {
		c := &page.Items[i]
		if c.Result == reconciliation.CPDifference && (diff == nil || absStr(*c.Difference).GreaterThan(absStr(*diff.Difference))) {
			diff = c
		}
	}
	for i := range page.Items {
		c := &page.Items[i]
		if diff == nil {
			diff = c
			continue
		}
		if c.UUID != diff.UUID {
			matched = c
			break
		}
	}
	email := "muhasebe@ornek-cari.example"
	if diff != nil {
		if _, err := svc.CreateLetter(ctx, scope, actor, reconciliation.LetterInput{CounterpartyUUID: diff.UUID, Email: &email}); err != nil {
			return fmt.Errorf("reconciliation letter: %w", err)
		}
	}
	if matched != nil {
		l, err := svc.CreateLetter(ctx, scope, actor, reconciliation.LetterInput{CounterpartyUUID: matched.UUID, Email: &email})
		if err != nil {
			return fmt.Errorf("reconciliation letter: %w", err)
		}
		name := "Ayşe Demir"
		if _, err := svc.RecordResponse(ctx, scope, l.UUID, reconciliation.ResponseInput{Response: "agreed", ResponderName: &name}); err != nil {
			return fmt.Errorf("reconciliation response: %w", err)
		}
	}
	return nil
}

func absStr(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d.Abs()
}

// demoBabs are October form lines of counterparties the demo knows: the
// supplier in the cari memory (bank demo) and the customer of the UBL set.
// Until the UBL set is uploaded they show as lines without invoices.
var demoBabs = []struct {
	form, taxID, name string
	count             int
	amount            string
}{
	{"ba", "9200000035", "Kalem Kırtasiye Ltd. Şti.", 6, "18450"},
	{"bs", "9100000021", "Örnek Alıcı Ltd. Şti.", 4, "27300"},
}

// seedBabsDeclarations writes Form BA / BS lines for October from Atlas's
// invoices (counterparties at or above 5.000 TL; the largest BA line is
// declared 1.000 TL short so the screen shows an amount mismatch) and the
// demo lines above.
func seedBabsDeclarations(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	return rls.System(ctx, pool, "demo-seed: BA/BS declarations", func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT CASE WHEN direction = 'incoming' THEN 'ba' ELSE 'bs' END,
			       COALESCE(NULLIF(customer_tax_number, ''), customer_identity_number),
			       MAX(COALESCE(customer_name, '')), COUNT(*)::int,
			       TRUNC(SUM(CASE WHEN currency = 'TRY' THEN tax_exclusive_amount ELSE tax_exclusive_amount * COALESCE(exchange_rate, 0) END))
			FROM invoices
			WHERE taxpayer_id = $1 AND period = '2026-10' AND status_normalized = 'active'
			  AND COALESCE(NULLIF(customer_tax_number, ''), customer_identity_number) ~ '^[0-9]{10,11}$'
			GROUP BY 1, 2
			HAVING SUM(CASE WHEN currency = 'TRY' THEN tax_exclusive_amount ELSE tax_exclusive_amount * COALESCE(exchange_rate, 0) END) >= 5000
			ORDER BY 1, 5 DESC`, scope.TaxpayerID)
		if err != nil {
			return err
		}
		type line struct {
			form, taxID, name string
			count             int
			amount            decimal.Decimal
		}
		var lines []line
		for rows.Next() {
			var l line
			if err := rows.Scan(&l.form, &l.taxID, &l.name, &l.count, &l.amount); err != nil {
				return err
			}
			lines = append(lines, l)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		short := false
		for _, l := range lines {
			amt := l.amount
			if l.form == "ba" && !short && amt.GreaterThan(decimal.NewFromInt(6000)) {
				amt, short = amt.Sub(decimal.NewFromInt(1000)), true
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO babs_declarations (organization_id, engagement_id, taxpayer_id, period, form_type, counterparty_tax_id,
					counterparty_name, document_count, amount, source, created_by)
				VALUES ($1, $2, $3, '2026-10', $4, $5, $6, $7, $8, 'demo', $9)
				ON CONFLICT (engagement_id, period, form_type, counterparty_tax_id) DO NOTHING`,
				scope.OrganizationID, scope.ID, scope.TaxpayerID, l.form, l.taxID, l.name, l.count, amt, actor); err != nil {
				return fmt.Errorf("babs line: %w", err)
			}
		}
		for _, l := range demoBabs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO babs_declarations (organization_id, engagement_id, taxpayer_id, period, form_type, counterparty_tax_id,
					counterparty_name, document_count, amount, source, created_by)
				VALUES ($1, $2, $3, '2026-10', $4, $5, $6, $7, $8::numeric, 'demo', $9)
				ON CONFLICT (engagement_id, period, form_type, counterparty_tax_id) DO NOTHING`,
				scope.OrganizationID, scope.ID, scope.TaxpayerID, l.form, l.taxID, l.name, l.count, l.amount, actor); err != nil {
				return fmt.Errorf("babs demo line: %w", err)
			}
		}
		return nil
	})
}
