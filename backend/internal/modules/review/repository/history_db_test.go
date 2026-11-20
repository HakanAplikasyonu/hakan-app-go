package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/shopspring/decimal"
)

// TestCounterpartyHistory (#669 L): karşı tarafın geçmiş kodları kuyruğun
// ilk sayfasıyla sınırlı değil; bütün kararlardan, en çok kullanılan önce.
// Bekleyen, reddedilen, başka karşı taraf ve KDV hesabı sayılmaz.
func TestCounterpartyHistory(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng, user int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "rh-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})

	store := New(pool, db.New(pool))
	svc := usecase.New(store).Register("account_code", &mapApplier{v: map[int64]string{}})
	sub := usecase.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	const cp = "vkn:1234567890"
	target := int64(0)
	submit := func(key, field, code string) usecase.Suggestion {
		t.Helper()
		target++
		id := target
		s, err := svc.Submit(ctx, sub, usecase.Input{
			Kind: "account_code", TargetType: "invoice_item", TargetID: &id, TargetField: field,
			Period: "2026-10", Value: json.RawMessage(`{"account_code":"` + code + `"}`), Confidence: decimal.RequireFromString("0.5"),
			Rationale: "kural", Provenance: usecase.Provenance{Origin: usecase.OriginRule, Inputs: map[string]any{"counterparty_key": key}},
		})
		must(err)
		return s
	}
	// 120 kabul edilen satır: kuyruğun 100'lük ilk sayfasından fazlası.
	for range 120 {
		_, err := svc.Accept(ctx, sub, submit(cp, "account_code", "770.01").UUID, user, "")
		must(err)
	}
	// İki satır düzeltildi: düzeltilen kod sayılır, önerilen değil.
	for range 2 {
		_, err := svc.Correct(ctx, sub, submit(cp, "account_code", "770.01").UUID, user, json.RawMessage(`{"account_code":"760.01"}`), "düzelt")
		must(err)
	}
	submit(cp, "account_code", "999.01") // bekliyor: sayılmaz
	_, err := svc.Reject(ctx, sub, submit(cp, "account_code", "998.01").UUID, user, "yanlış")
	must(err)
	_, err = svc.Accept(ctx, sub, submit(cp, "vat_account_code", "191.01").UUID, user, "")
	must(err)
	_, err = svc.Accept(ctx, sub, submit("vkn:9999999999", "account_code", "740.01").UUID, user, "")
	must(err)

	got, err := svc.CounterpartyHistory(ctx, sub, cp, 5)
	must(err)
	if len(got) != 2 || got[0].AccountCode != "770.01" || got[0].Uses != 120 || got[1].AccountCode != "760.01" || got[1].Uses != 2 {
		t.Fatalf("history = %+v", got)
	}
	if _, err := svc.CounterpartyHistory(ctx, sub, " ", 5); err == nil {
		t.Fatal("empty counterparty accepted")
	}
}
