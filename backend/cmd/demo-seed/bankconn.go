package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/demo"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice/vakifbank"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// DemoVakifbankIBAN is the demo taxpayer's Vakıfbank account on the bank
// web-service instruction (valid check digits, bank code 00015).
var DemoVakifbankIBAN = demo.IBAN("00015", "0158007301234567")

// seedBankConnection gives the taxpayer a Vakıfbank web-service connection
// waiting at the branch (#518): the instruction was printed and handed in,
// the bank has not sent the service user yet. The Bağlantılar tab shows the
// third step (service user) as the next action. No credentials: the worker
// never calls the bank for it.
func seedBankConnection(ctx context.Context, pool *pgxpool.Pool, taxpayerID int64) error {
	return rls.System(ctx, pool, "demo-seed: bank web-service connection", func(ctx context.Context, tx pgx.Tx) error {
		q := database.QueriesFor(pool)
		now := time.Now().UTC()
		form, submitted := now.Add(-72*time.Hour), now.Add(-48*time.Hour)
		meta, _ := json.Marshal(webservice.Meta{
			BankID: "vakifbank", Environment: "production", IBANs: []string{DemoVakifbankIBAN},
			FormAt: &form, SubmittedAt: &submitted,
		})
		_, err := q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
			TaxpayerID: taxpayerID, SourceSystem: vakifbank.SourceSystem, SetupState: "submitted_to_bank", Meta: meta,
		})
		return err
	})
}
