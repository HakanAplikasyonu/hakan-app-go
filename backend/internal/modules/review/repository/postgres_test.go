package repository

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type mapApplier struct {
	v    map[int64]string
	fail bool
	tx   []any
}

func (m *mapApplier) Apply(_ context.Context, tx usecase.Tx, s usecase.Suggestion, val json.RawMessage) (json.RawMessage, error) {
	m.tx = append(m.tx, tx.Handle())
	if m.fail {
		return nil, errors.New("target locked")
	}
	prev, _ := json.Marshal(m.v[*s.Input.TargetID])
	var x string
	_ = json.Unmarshal(val, &x)
	m.v[*s.Input.TargetID] = x
	return prev, nil
}

func (m *mapApplier) Revert(_ context.Context, _ usecase.Tx, s usecase.Suggestion, prev json.RawMessage) error {
	var x string
	_ = json.Unmarshal(prev, &x)
	m.v[*s.Input.TargetID] = x
	return nil
}

// TestReviewAgainstPostgres runs with REVIEW_TEST_DATABASE_URL (skipped in CI).
func TestReviewAgainstPostgres(t *testing.T) {
	url := os.Getenv("REVIEW_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("REVIEW_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var org, tp, eng, user int64
	tag := time.Now().Format("150405.000000")
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'R') RETURNING id`, "r-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', lpad((floor(random() * 9000000000) + 1000000000)::bigint::text, 10, '0'), 'T') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org)
		_, _ = pool.Exec(ctx, `DELETE FROM taxpayers WHERE id = $1`, tp)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	})

	store := New(pool, db.New(pool))
	ap := &mapApplier{v: map[int64]string{10: "770.01"}}
	svc := usecase.New(store).Register("account_code", ap)
	sub := usecase.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	target := int64(10)
	in := func(conf, val string) usecase.Input {
		return usecase.Input{
			Kind: "account_code", TargetType: "invoice_item", TargetID: &target, TargetField: "account_code",
			Period: "2026-10", Value: json.RawMessage(`"` + val + `"`), Confidence: decimal.RequireFromString(conf),
			Rationale: "geçmiş eşleşme", Provenance: usecase.Provenance{Origin: usecase.OriginModel, Model: "claude-opus-5-5", Inputs: map[string]any{"line": "Kırtasiye"}},
		}
	}

	low, err := svc.Submit(ctx, sub, in("0.6000", "760.01"))
	must(err)
	if low.Status != usecase.StatusPending || !low.Input.Confidence.Equal(decimal.RequireFromString("0.6")) {
		t.Fatalf("low = %+v", low)
	}
	// A second pending suggestion supersedes the first (unique pending index).
	low2, err := svc.Submit(ctx, sub, in("0.7", "760.02"))
	must(err)
	got, log, err := store.Get(ctx, sub, low.UUID)
	must(err)
	if got.Status != usecase.StatusSuperseded || len(log) != 1 {
		t.Fatalf("superseded = %s log %d", got.Status, len(log))
	}

	// Office threshold 0.65 -> the next one auto-applies.
	thr := decimal.RequireFromString("0.65")
	must(store.UpsertSetting(ctx, org, 0, usecase.SettingRow{Kind: usecase.AnyKind, Threshold: &thr}, user))
	must(store.UpsertSetting(ctx, org, 0, usecase.SettingRow{Kind: usecase.AnyKind, Threshold: &thr}, user))
	hi, err := svc.Submit(ctx, sub, in("0.66", "760.03"))
	must(err)
	if hi.Status != usecase.StatusAutoAccepted || ap.v[10] != "760.03" || string(hi.PreviousValue) != `"770.01"` {
		t.Fatalf("hi = %s value %s prev %s", hi.Status, ap.v[10], hi.PreviousValue)
	}
	if _, ok := ap.tx[0].(*db.Queries); !ok {
		t.Fatalf("applier handle = %T", ap.tx[0])
	}
	// Revert, with who/when/why in the log.
	_, err = svc.Revert(ctx, sub, hi.UUID, user, "yanlış")
	must(err)
	_, log, _ = store.Get(ctx, sub, hi.UUID)
	if ap.v[10] != "770.01" || len(log) != 2 || log[1].ActorUUID == nil || log[1].Reason != "yanlış" || log[0].Threshold == nil {
		t.Fatalf("revert log = %+v value %s", log, ap.v[10])
	}

	// low2 was superseded by hi (same target field).
	if g, _, _ := store.Get(ctx, sub, low2.UUID); g.Status != usecase.StatusSuperseded {
		t.Fatalf("low2 = %s", g.Status)
	}
	// Correction of a pending suggestion on another target marks a rule candidate.
	target = 11
	ap.v[11] = "100"
	p11, err := svc.Submit(ctx, sub, in("0.10", "760.09"))
	must(err)
	c, err := svc.Correct(ctx, sub, p11.UUID, user, json.RawMessage(`"740.01"`), "kırtasiye 740")
	must(err)
	if ap.v[11] != "740.01" || string(c.PreviousValue) != `"100"` || c.Status != usecase.StatusCorrected || !c.RuleCandidate || c.DecidedBy == nil || c.DecidedAt == nil {
		t.Fatalf("correct = %+v", c)
	}
	rc := true
	rows, total, err := store.List(ctx, usecase.Filter{OrganizationID: org, AllEngagements: true, RuleCandidate: &rc, Limit: 20})
	must(err)
	if total != 1 || rows[0].UUID != p11.UUID || rows[0].EngagementUUID.String() == "" {
		t.Fatalf("rule candidates = %d", total)
	}
	// Not assigned and no read_all: nothing.
	if _, total, _ := store.List(ctx, usecase.Filter{OrganizationID: org, UserID: user, Limit: 20}); total != 0 {
		t.Fatalf("unassigned saw %d", total)
	}

	// Apply failure rolls back the insert.
	ap.fail = true
	before, _, _ := store.List(ctx, usecase.Filter{OrganizationID: org, AllEngagements: true, Limit: 100})
	if _, err := svc.Submit(ctx, sub, in("0.99", "1")); !errors.Is(err, usecase.ErrApplyFailed) {
		t.Fatalf("err = %v", err)
	}
	after, _, _ := store.List(ctx, usecase.Filter{OrganizationID: org, AllEngagements: true, Limit: 100})
	if len(after) != len(before) {
		t.Fatalf("rollback failed: %d -> %d", len(before), len(after))
	}
	ok, err := store.DeleteSetting(ctx, org, 0, usecase.AnyKind)
	must(err)
	if !ok {
		t.Fatal("delete setting")
	}
}
