package main

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/entegrator"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// DemoEinvoiceSystem is the demo taxpayer's e-invoice integrator.
const DemoEinvoiceSystem = entegrator.SourcePrefix + "izibiz"

// seedEinvoiceConnection gives the taxpayer an İzibiz connection that shows
// as connected (#518 A1) with one finished first sync. No credentials: the
// demo never calls the integrator, and "şimdi çek" asks for the user first.
func seedEinvoiceConnection(ctx context.Context, pool *pgxpool.Pool, taxpayerID int64) error {
	return rls.System(ctx, pool, "demo-seed: e-invoice integrator connection", func(ctx context.Context, tx pgx.Tx) error {
		q := database.QueriesFor(pool)
		conn, err := q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
			TaxpayerID: taxpayerID, SourceSystem: DemoEinvoiceSystem, SetupState: "credentials_entered",
			Meta: []byte(`{"environment":"production"}`),
		})
		if err != nil {
			return err
		}
		run, err := q.StartSyncRun(ctx, db.StartSyncRunParams{TaxpayerID: taxpayerID, ConnectionID: conn.ID, Kind: entegrator.RunBackfill})
		if err != nil {
			return err
		}
		counters, _ := json.Marshal(entegrator.Counters{Windows: 12, Pages: 6, Listed: 148, New: 148, UBLFetched: 148})
		if _, err := q.FinishSyncRun(ctx, db.FinishSyncRunParams{
			TaxpayerID: taxpayerID, ID: run.ID, Status: "success", Counters: counters,
		}); err != nil {
			return err
		}
		_, err = q.SetSourceConnectionStatus(ctx, db.SetSourceConnectionStatusParams{
			TaxpayerID: taxpayerID, ID: conn.ID, Status: "active",
		})
		return err
	})
}
