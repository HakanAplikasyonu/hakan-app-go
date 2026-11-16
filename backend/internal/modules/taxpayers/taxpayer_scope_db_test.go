package taxpayers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

// TestOwnerDecisionsInTaxpayerScope (issue #281): the company owner's
// approve and reject from the mükellef panel run in the panel's taxpayer
// scope (no office), as rls.Middleware opens it in production. Approval
// moves the company from office B to office A (ends B's engagement, opens
// A's); rejection declines another office's request.
func TestOwnerDecisionsInTaxpayerScope(t *testing.T) {
	e := newOnboardEnv(t, time.Millisecond)
	ctx := context.Background()
	orgA, orgB, orgC := e.d.Organization(t), e.d.Organization(t), e.d.Organization(t)
	tp := e.d.Taxpayer(t, KindLegal, "1234567890")
	engB := e.d.Engagement(t, orgB, tp)
	owner := e.d.User(t)
	if _, err := e.d.Pool.Exec(ctx, `INSERT INTO taxpayer_members (taxpayer_id, user_id, role) VALUES ($1, $2, 'owner')`, tp, owner); err != nil {
		t.Fatal(err)
	}
	var tpUUID uuid.UUID
	_ = e.d.Pool.QueryRow(ctx, `SELECT uuid FROM taxpayers WHERE id = $1`, tp).Scan(&tpUUID)
	e.request(t, orgA, "1234567890", "muhasebe@alfa.test")
	e.request(t, orgC, "1234567890", "muhasebe@gama.test")
	var uuidA, uuidC uuid.UUID
	_ = e.d.Pool.QueryRow(ctx, `SELECT uuid FROM taxpayer_onboarding_requests WHERE organization_id = $1`, orgA).Scan(&uuidA)
	_ = e.d.Pool.QueryRow(ctx, `SELECT uuid FROM taxpayer_onboarding_requests WHERE organization_id = $1`, orgC).Scan(&uuidC)

	ownerScope := taxpayerctx.Scope{ID: tp, UUID: tpUUID, TaxID: "1234567890", Role: taxpayerctx.RoleOwner}
	panel := rls.Scope{UserID: owner, TaxpayerIDs: []int64{tp}}
	err := rls.WithScope(ctx, e.d.Pool, panel, func(ctx context.Context, _ pgx.Tx) error {
		pending, err := e.flow.PendingForTaxpayer(ctx, ownerScope)
		if err != nil || len(pending) != 2 {
			t.Fatalf("pending in panel scope: %d %v", len(pending), err)
		}
		for _, p := range pending {
			switch p.UUID {
			case uuidA:
				out, err := e.flow.ApproveAsOwner(ctx, ownerScope, p.UUID, owner)
				if err != nil || out.Status != OutcomeApproved {
					t.Fatalf("approve in panel scope: %+v %v", out, err)
				}
			case uuidC:
				if err := e.flow.RejectAsOwner(ctx, ownerScope, p.UUID, owner); err != nil {
					t.Fatalf("reject in panel scope: %v", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var endedB, activeA, rejectedC int
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE id = $1 AND ended_at IS NOT NULL`, engB).Scan(&endedB)
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM engagements WHERE organization_id = $1 AND taxpayer_id = $2 AND ended_at IS NULL`, orgA, tp).Scan(&activeA)
	_ = e.d.Pool.QueryRow(ctx, `SELECT count(*) FROM taxpayer_onboarding_requests WHERE organization_id = $1 AND status = 'rejected'`, orgC).Scan(&rejectedC)
	if endedB != 1 || activeA != 1 || rejectedC != 1 {
		t.Fatalf("endedB=%d activeA=%d rejectedC=%d", endedB, activeA, rejectedC)
	}
}
