package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/taxreturns"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// seedTaxReturns (#390): Atlas Gıda'nın Eylül 2026 beyanları verilmiş
// (KDV1, MUHSGK tahakkuk fiş numaralarıyla), Ba/Bs hazırlanmış; Ekim KDV1'i
// taslak ve devreden KDV'yi Eylül beyanından alır. Kontrol listesindeki
// uyarılar Eylül için onaylanır (müşavir gözden geçirmiş gibi); Ekim
// uyarıları açık kalır ki ekranda onay akışı görünsün.
func seedTaxReturns(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	svc := taxreturns.New(pool)
	return rls.System(ctx, pool, "demo-seed: tax returns (#390)", func(ctx context.Context, _ pgx.Tx) error {
		const sep = "2026-09"
		for _, r := range []struct {
			kind, accrual string
			file          bool
		}{
			{taxreturns.KindKDV1, "KDV1-2026-09-4471", true},
			{taxreturns.KindMUHSGK, "MUH-2026-09-1186", true},
			{taxreturns.KindBABS, "", false},
		} {
			p, err := svc.Preview(ctx, scope, r.kind, sep)
			if err != nil {
				return fmt.Errorf("%s preview: %w", r.kind, err)
			}
			if _, err := svc.Prepare(ctx, scope, actor, r.kind, sep, p.OpenWarnings); err != nil {
				return fmt.Errorf("%s prepare: %w", r.kind, err)
			}
			if !r.file {
				continue
			}
			accrual := r.accrual
			if _, err := svc.File(ctx, scope, actor, r.kind, sep, taxreturns.FileInput{
				FiledOn: "2026-10-24", AccrualNumber: &accrual,
			}); err != nil {
				return fmt.Errorf("%s file: %w", r.kind, err)
			}
		}
		note := "Ekim beyanı: bekleyen masraflar onaylanınca hazırlanacak."
		_, err := svc.SaveDraft(ctx, scope, actor, taxreturns.KindKDV1, "2026-10", taxreturns.DraftInput{Note: &note})
		return err
	})
}
