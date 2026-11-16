package rls_test

import (
	"context"
	"sort"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestTaxpayerPartyScope (issue #166, migration 000080): a taxpayer scope
// (mükellef panel: no office) sees its own taxpayer's rows of the office
// tables at every office, and nothing of another taxpayer; an office scope
// with the same taxpayer never sees another office's rows.
func TestTaxpayerPartyScope(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var oldEng int64
	err := rls.System(ctx, f.d.Pool, "test fixture", func(ctx context.Context, tx pgx.Tx) error {
		// Office B served taxpayer A before (ended engagement).
		if err := tx.QueryRow(ctx, `
			INSERT INTO engagements (organization_id, taxpayer_id, status, ended_at)
			VALUES ($1, $2, 'ended', NOW()) RETURNING id`, f.orgB, f.tpA).Scan(&oldEng); err != nil {
			return err
		}
		// Office B asks to add taxpayer A again (found by tax id only).
		_, err := tx.Exec(ctx, `
			INSERT INTO taxpayer_onboarding_requests (organization_id, kind, tax_id, title, contact_email, token_hash, expires_at)
			SELECT $1, 'legal', t.tax_id, 'A', 'a@example.test', md5(random()::text), NOW() + interval '1 day'
			FROM taxpayers t WHERE t.id = $2`, f.orgB, f.tpA)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := func(ctx context.Context, tx pgx.Tx, sql string) []int64 {
		rows, err := tx.Query(ctx, sql)
		if err != nil {
			t.Fatal(err)
		}
		out, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}

	// Taxpayer A's panel.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{UserID: f.ownerA, TaxpayerIDs: []int64{f.tpA}}, func(ctx context.Context, tx pgx.Tx) error {
		got := ids(ctx, tx, `SELECT id FROM engagements`)
		want := []int64{f.engA, oldEng}
		sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("taxpayer A sees engagements %v, want its own %v", got, want)
		}
		if n := len(ids(ctx, tx, `SELECT organization_id FROM taxpayer_onboarding_requests`)); n != 1 {
			t.Errorf("taxpayer A sees %d onboarding requests, want 1 (by tax id)", n)
		}
		sp, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		_, err = sp.Exec(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2)`, f.orgA, f.tpB)
		if !isRLSError(err) {
			t.Errorf("taxpayer A writing taxpayer B's engagement: %v, want a row level security violation", err)
		}
		return sp.Rollback(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Taxpayer B's panel sees none of A's.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{UserID: f.ownerB, TaxpayerIDs: []int64{f.tpB}}, func(ctx context.Context, tx pgx.Tx) error {
		if got := ids(ctx, tx, `SELECT id FROM engagements WHERE taxpayer_id = `+strconv.FormatInt(f.tpA, 10)); len(got) != 0 {
			t.Errorf("taxpayer B sees taxpayer A's engagements %v", got)
		}
		if n := len(ids(ctx, tx, `SELECT organization_id FROM taxpayer_onboarding_requests`)); n != 0 {
			t.Errorf("taxpayer B sees %d onboarding requests of A", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Office A, with taxpayer A in scope: still only its own office's rows.
	err = rls.WithScope(ctx, f.d.Pool, rls.Scope{OrganizationID: f.orgA, UserID: f.ownerA, EngagementsReadAll: true, TaxpayerIDs: []int64{f.tpA}}, func(ctx context.Context, tx pgx.Tx) error {
		for _, id := range ids(ctx, tx, `SELECT organization_id FROM engagements`) {
			if id != f.orgA {
				t.Errorf("office A sees an engagement of office %d", id)
			}
		}
		if n := len(ids(ctx, tx, `SELECT organization_id FROM taxpayer_onboarding_requests`)); n != 0 {
			t.Errorf("office A sees %d of office B's onboarding requests", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
