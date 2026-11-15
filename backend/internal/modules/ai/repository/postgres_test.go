package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRecordAndReport runs against a migrated database named by
// AI_TEST_DATABASE_URL; it is skipped otherwise (CI has no database).
func TestPostgresRecordAndReport(t *testing.T) {
	url := os.Getenv("AI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AI_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var orgID int64
	slug := "ai-test-" + time.Now().Format("150405.000000")
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'AI test') RETURNING id`, slug).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID) })

	var tp int64
	if err := pool.QueryRow(ctx, `INSERT INTO taxpayers DEFAULT VALUES RETURNING id`).Scan(&tp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp) })

	store := NewPostgres(db.New(pool))
	recs := []usecase.UsageRecord{
		{OrganizationID: orgID, TaxpayerID: &tp, Purpose: "t", Provider: "anthropic", Model: "claude-opus-5-5",
			Usage: provider.Usage{InputTokens: 1000, OutputTokens: 100}, CostUSD: "0.006000", MaskedValues: 3},
		{OrganizationID: orgID, TaxpayerID: &tp, Purpose: "t", Provider: "anthropic", Model: "claude-opus-5-5",
			Usage: provider.Usage{InputTokens: 1000, OutputTokens: 100}, CostUSD: "0.006000"},
		{OrganizationID: orgID, Purpose: "t", Provider: "openai_compatible", Model: "local", Fallback: true,
			Usage: provider.Usage{InputTokens: 5, OutputTokens: 5}},
	}
	for _, r := range recs {
		if err := store.RecordUsage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	rows, err := store.UsageReport(ctx, orgID, now.AddDate(0, -1, 0), now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	// NULL taxpayer sorts first.
	if rows[0].TaxpayerID != nil || rows[0].UnpricedCalls != 1 || rows[0].FallbackCalls != 1 || rows[0].CostUSD != "0.000000" {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].TaxpayerID == nil || *rows[1].TaxpayerID != tp || rows[1].Calls != 2 || rows[1].CostUSD != "0.012000" || rows[1].InputTokens != 2000 {
		t.Fatalf("row1 = %+v", rows[1])
	}
	if rows[1].Month != now.In(time.FixedZone("TRT", 3*3600)).Format("2006-01") {
		t.Fatalf("month = %q", rows[1].Month)
	}
}
