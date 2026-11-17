package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Atlas Gıda'nın demo hikâyesi (#456): modüller aynı rakamları anlatır.
//
//   - Eylül 2026 kapanmıştır: bordro tahakkuku, KDV1 ve MUHSGK beyanları
//     Eylül mizanında (30.09) borç olarak durur.
//   - Ekim banka ekstresi Eylül yükümlülüklerini öder: net maaşlar, SGK
//     primi (bordrodaki ödeme tarihiyle aynı gün), KDV1 ve MUHSGK
//     tahakkukları. Tutarlar bordro ve beyanname ekranındakilerle aynıdır.
//   - Ekim mizanı (Luca) = Eylül mizanı + portaldan Luca'ya aktarılmış Ekim
//     fişleri + Luca'da elle girilmiş tek bir kur farkı fişi.
//   - Bu yüzden Ekim mutabakatında farklar yalnızca iki nedenden gelir:
//     onaylanmış ama Luca'ya aktarılmamış Z raporu taslakları (100, 108,
//     391, 600) ve Luca'daki elle fiş (120). Diğer hesaplar eşleşir.
//
// Tutarlar sabittir; demo-seed testi (TestDemoStoryIsConsistent) bordro,
// beyanname ve banka kayıtlarının bu sabitlerle tuttuğunu denetler.
const (
	// Eylül bordrosu: dört personelin net ücretleri toplamı (demoPayrollRows).
	sepPayrollNet = "124741.51"
	// Eylül SGK: tahakkuk − teşvik; 30.10.2026'da ödendi.
	sepSGKPayable = "50701.79"
	sepSGKPaidOn  = "2026-10-30"
	// Eylül KDV1 ödenecek tutarı (Z raporları ve masraflardan).
	sepKDVPayable = "10150.00"
	// Eylül MUHSGK: gelir vergisi 7.468,60 + damga vergisi 394,56.
	sepMUHSGKPayable = "7863.16"
)

// lucaManualEntry is the only October entry made in Luca by hand (no portal
// source): a foreign-exchange gain on a customer. The reconciliation item of
// 120 is explained with lucaManualNote.
var lucaManualEntry = []struct{ code, name, debit, credit string }{
	{"120.01.001", "ÇEŞİTLİ ALICILAR", "1200.00", "0"},
	{"646.01", "KAMBİYO KARLARI", "0", "1200.00"},
}

const lucaManualNote = "Kur farkı fişi Luca'da elle girildi (Ekim, 1.200,00 TL); portalda karşılığı yok."

// approveAndExport approves the drafts (when still drafts) and exports them
// to Luca in one batch, as the office would before the month's transfer.
func approveAndExport(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	svc := journals.New(pool, seedStore)
	for _, id := range ids {
		if err := svc.Approve(ctx, scope, actor, id); err != nil {
			return fmt.Errorf("approve draft %s: %w", id, err)
		}
	}
	if _, err := svc.Export(ctx, scope, actor, journals.ExportInput{Driver: "luca", Drafts: ids}); err != nil {
		return fmt.Errorf("luca export: %w", err)
	}
	return nil
}

// draftsWhere lists the engagement's draft-status journal drafts matching
// cond (a SQL condition on journal_drafts d).
func draftsWhere(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, cond string, args ...any) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := rls.System(ctx, pool, "demo-seed: drafts", func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT d.uuid FROM journal_drafts d
			WHERE d.organization_id = $1 AND d.engagement_id = $2 AND d.status = 'draft' AND (`+cond+`)
			ORDER BY d.entry_date, d.id`, append([]any{scope.OrganizationID, scope.ID}, args...)...)
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		return err
	})
	return ids, err
}

// exportedLines sums the lines of the period's exported mahsup drafts per
// account code: what Luca received from the portal.
func exportedLines(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, period string) (map[string]mizanAmt, error) {
	out := map[string]mizanAmt{}
	err := rls.System(ctx, pool, "demo-seed: exported lines", func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT l.account_code::text, COALESCE(SUM(l.debit), 0)::text, COALESCE(SUM(l.credit), 0)::text
			FROM journal_draft_lines l
			JOIN journal_drafts d ON d.engagement_id = l.engagement_id AND d.id = l.journal_draft_id
			WHERE d.organization_id = $1 AND d.engagement_id = $2 AND d.status = 'exported'
			  AND d.entry_type = 'mahsup' AND d.period = $3
			GROUP BY 1`, scope.OrganizationID, scope.ID, period)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var code, d, c string
			if err := rows.Scan(&code, &d, &c); err != nil {
				return err
			}
			a := out[code]
			a.debit = a.debit.Add(decimal.RequireFromString(d))
			a.credit = a.credit.Add(decimal.RequireFromString(c))
			out[code] = a
		}
		return rows.Err()
	})
	return out, err
}
