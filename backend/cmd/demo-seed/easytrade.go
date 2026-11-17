package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources/easytrade"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedEasytrade gives the taxpayer an active Easytrade connection with a
// 4-hour schedule, its sync history (a partial run that was retried, then
// successes) and October 2026's expenses, financial transactions and Z
// reports (#385), written through sources.Write as a sync does. The
// connection has no credentials: the worker never calls Easytrade for it.
func seedEasytrade(ctx context.Context, pool *pgxpool.Pool, taxpayerID int64) error {
	return rls.System(ctx, pool, "demo-seed: Easytrade connection and data", func(ctx context.Context, tx pgx.Tx) error {
		q := database.QueriesFor(pool)
		now := time.Now().UTC()
		branches := make([]easytrade.Branch, 0, len(demo.EasytradeBranches))
		for _, b := range demo.EasytradeBranches {
			branches = append(branches, easytrade.Branch{ID: b.ID, Name: b.Name})
		}
		rep, err := sources.Write(ctx, pool, taxpayerID, easytrade.SourceSystem, demo.EasytradeBatch())
		if err != nil {
			return err
		}
		meta, _ := json.Marshal(map[string]any{"branches": branches, "last_sync_at": now.Add(-40 * time.Minute), "last_report": rep})
		row, err := q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
			TaxpayerID: taxpayerID, SourceSystem: easytrade.SourceSystem,
			ExternalCompanyID: pgtype.Text{String: demo.EasytradeCompanyID, Valid: true}, SetupState: "active", Meta: meta,
		})
		if err != nil {
			return err
		}
		if _, err := q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
			SetupState: "active", Status: "active", MetaPatch: []byte("{}"), MarkSuccess: true, TaxpayerID: taxpayerID, ID: row.ID,
		}); err != nil {
			return err
		}
		if _, err := q.SetSourceConnectionSchedule(ctx, db.SetSourceConnectionScheduleParams{
			SyncIntervalMinutes: pgtype.Int4{Int32: int32(easytrade.DefaultSyncInterval / time.Minute), Valid: true},
			NextSyncAt:          pgtype.Timestamptz{Time: now.Add(easytrade.DefaultSyncInterval - 40*time.Minute), Valid: true},
			TaxpayerID:          taxpayerID, ID: row.ID,
		}); err != nil {
			return err
		}
		// History, oldest first: a full first read, a partial run (expenses
		// answered 500) retried five minutes later, then a quiet run.
		first := easytrade.Counters{Report: rep, WindowsDone: 8}
		partial := easytrade.Counters{Report: sources.Report{InvoicesExisting: 0, Excluded: sources.Excluded{}}, WindowsDone: 6, WindowsFailed: 2}
		quiet := easytrade.Counters{Report: sources.Report{ExpensesUnchanged: 3, FinancialUnchanged: 8, ZReportsUnchanged: 5, Excluded: sources.Excluded{}}, WindowsDone: 8}
		for _, r := range []struct {
			kind, status string
			c            easytrade.Counters
			ago          time.Duration
			err          string
		}{
			{easytrade.RunManual, "success", first, 9 * time.Hour, ""},
			{easytrade.RunScheduled, "partial", partial, 4*time.Hour + 45*time.Minute, ""},
			{easytrade.RunScheduled, "success", quiet, 4*time.Hour + 40*time.Minute, ""},
			{easytrade.RunScheduled, "success", quiet, 40 * time.Minute, ""},
		} {
			run, err := q.StartSyncRun(ctx, db.StartSyncRunParams{TaxpayerID: taxpayerID, ConnectionID: row.ID, Kind: r.kind})
			if err != nil {
				return err
			}
			counters, _ := json.Marshal(r.c)
			if _, err := q.FinishSyncRun(ctx, db.FinishSyncRunParams{TaxpayerID: taxpayerID, ID: run.ID, Status: r.status, Counters: counters}); err != nil {
				return err
			}
			started := now.Add(-r.ago)
			if _, err := tx.Exec(ctx, `UPDATE sync_runs SET started_at = $1, finished_at = $2 WHERE id = $3`,
				started, started.Add(40*time.Second), run.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
