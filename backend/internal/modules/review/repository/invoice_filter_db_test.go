package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TestListByInvoice: fatura detayı (#669 F) yalnız o faturadan okunan
// önerileri, her durumda, listeyle aynı sayımla alır.
func TestListByInvoice(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "rf-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
	})

	store := New(pool, db.New(pool))
	svc := usecase.New(store).Register("account_code", &mapApplier{v: map[int64]string{}})
	sub := usecase.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	invA, invB := uuid.NewString(), uuid.NewString()
	submit := func(target int64, invoice string) {
		t.Helper()
		id := target
		_, err := svc.Submit(ctx, sub, usecase.Input{
			Kind: "account_code", TargetType: "invoice_item", TargetID: &id, TargetField: "account_code",
			Period: "2026-10", Value: json.RawMessage(`"770.01"`), Confidence: decimal.RequireFromString("0.5"),
			Rationale: "kural", Provenance: usecase.Provenance{Origin: usecase.OriginRule, Inputs: map[string]any{"invoice_uuid": invoice}},
		})
		must(err)
	}
	submit(1, invA)
	submit(2, invA)
	submit(3, invB)

	rows, total, err := store.List(ctx, usecase.Filter{OrganizationID: org, EngagementID: eng, AllEngagements: true, Invoice: invA, Limit: 20})
	must(err)
	if total != 2 || len(rows) != 2 {
		t.Fatalf("invoice A: total %d rows %d, want 2", total, len(rows))
	}
	for _, r := range rows {
		if r.Input.Provenance.Inputs["invoice_uuid"] != invA {
			t.Fatalf("row from another invoice: %v", r.Input.Provenance.Inputs)
		}
	}
	if _, total, err := store.List(ctx, usecase.Filter{OrganizationID: org, EngagementID: eng, AllEngagements: true, Limit: 20}); err != nil || total != 3 {
		t.Fatalf("no invoice filter: total %d err %v, want 3", total, err)
	}
}
