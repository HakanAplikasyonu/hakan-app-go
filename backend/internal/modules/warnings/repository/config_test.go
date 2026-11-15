package repository

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestConfigLayersAgainstPostgres runs against a migrated database named by
// WARNINGS_TEST_DATABASE_URL; skipped otherwise (CI has no database).
func TestConfigLayersAgainstPostgres(t *testing.T) {
	url := os.Getenv("WARNINGS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WARNINGS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var orgID, otherOrg, tpID, engID int64
	slug := "warn-" + time.Now().Format("150405.000000")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'W') RETURNING id`, slug).Scan(&orgID))
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'W2') RETURNING id`, slug+"b").Scan(&otherOrg))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'Test') RETURNING id`).Scan(&tpID))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, orgID, tpID).Scan(&engID))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id IN ($1, $2)`, orgID, otherOrg)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tpID)
	})

	cfg := NewConfig(db.New(pool))
	subj := engine.Subject{OrganizationID: orgID, EngagementID: engID, TaxpayerID: tpID}
	code := "TAX_TOTAL_MISMATCH"

	eff, err := cfg.Resolve(ctx, subj, []string{code})
	must(err)
	if e := eff[code]; e.Severity != catalog.SeverityWarning || !e.Enabled || e.Params["tolerance"] != "0.05" {
		t.Fatalf("default = %+v", e)
	}

	blocker := catalog.SeverityBlocker
	must(cfg.Upsert(ctx, orgID, 0, code, engine.Layer{Severity: &blocker, Params: json.RawMessage(`{"tolerance":"0.10"}`)}, 0))
	// Another office's override must not leak.
	info := catalog.SeverityInfo
	must(cfg.Upsert(ctx, otherOrg, 0, code, engine.Layer{Severity: &info}, 0))
	eff, _ = cfg.Resolve(ctx, subj, []string{code})
	if e := eff[code]; e.Severity != catalog.SeverityBlocker || e.Params["tolerance"] != "0.10" {
		t.Fatalf("office = %+v", e)
	}

	off := false
	must(cfg.Upsert(ctx, orgID, engID, code, engine.Layer{Enabled: &off}, 0))
	// Upsert twice: still one row.
	must(cfg.Upsert(ctx, orgID, engID, code, engine.Layer{Enabled: &off, Params: json.RawMessage(`{"tolerance":"1"}`)}, 0))
	eff, _ = cfg.Resolve(ctx, subj, []string{code})
	if e := eff[code]; e.Enabled || e.Severity != catalog.SeverityBlocker || e.Params["tolerance"] != "1" {
		t.Fatalf("engagement = %+v", e)
	}
	// Office-only resolution ignores the engagement row.
	eff, _ = cfg.Resolve(ctx, engine.Subject{OrganizationID: orgID}, []string{code})
	if !eff[code].Enabled {
		t.Fatal("office resolution saw the engagement override")
	}

	layers, err := cfg.Layers(ctx, orgID, engID, nil)
	must(err)
	if len(layers) != len(catalog.Codes) {
		t.Fatalf("layers = %d, catalog = %d", len(layers), len(catalog.Codes))
	}

	ok, err := cfg.Delete(ctx, orgID, engID, code)
	must(err)
	if !ok {
		t.Fatal("delete engagement override")
	}
	if ok, _ := cfg.Delete(ctx, orgID, engID, code); ok {
		t.Fatal("second delete should report nothing deleted")
	}
	eff, _ = cfg.Resolve(ctx, subj, []string{code})
	if !eff[code].Enabled || eff[code].Severity != catalog.SeverityBlocker {
		t.Fatalf("after delete = %+v", eff[code])
	}
}
