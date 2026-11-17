package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	bankstatement "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/statement"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedBankStatementExport downloads the October movements of the demo bank
// account once as a Luca "Banka Ekstreleri" CSV (#465), so the bank screen
// shows the history and the "dışa aktarıldı" marks. Idempotent: skipped
// when the engagement already has a file.
func seedBankStatementExport(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	return rls.WithScope(ctx, pool, rls.Scope{OrganizationID: scope.OrganizationID, UserID: actor, EngagementsReadAll: true, TaxpayerIDs: []int64{scope.TaxpayerID}},
		func(ctx context.Context, tx pgx.Tx) error {
			q := db.New(tx)
			done, err := q.ListBankStatementExports(ctx, db.ListBankStatementExportsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Lim: 1})
			if err != nil {
				return err
			}
			if len(done) > 0 {
				return nil
			}
			var box uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT uuid FROM cash_boxes WHERE taxpayer_id = $1 AND type = 'bank_account' ORDER BY id LIMIT 1`, scope.TaxpayerID).Scan(&box); err != nil {
				return fmt.Errorf("bank account: %w", err)
			}
			_, err = bankstatement.BuildLucaStatement(ctx, q, scope, actor, bankstatement.LucaExportInput{
				CashBoxUUID: box, DateFrom: "2026-10-01", DateTo: "2026-10-31",
			})
			return err
		})
}
