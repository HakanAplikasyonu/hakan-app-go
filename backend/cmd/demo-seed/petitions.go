package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/petitions"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// seedPetitions gives the engagement two petitions (#780: the Dilekçeler
// tab is not empty in the demo): a borç durum yazısı for a public tender
// and a mükellefiyet yazısı. A second seed run adds nothing.
func seedPetitions(ctx context.Context, pool *pgxpool.Pool, scope engagementctx.Scope, actor int64) error {
	svc := petitions.New(pool)
	// The service takes its row-level scope from the request: the office
	// (with every engagement visible) and the engagement.
	ctx = orgctx.WithScope(ctx, orgctx.Scope{ID: scope.OrganizationID, Permissions: []string{rbac.PermTenantEngagementsReadAll}})
	ctx = engagementctx.WithScope(ctx, scope)
	have, err := svc.List(ctx, scope, 1, 0)
	if err != nil {
		return fmt.Errorf("petitions: %w", err)
	}
	if len(have) > 0 {
		return nil
	}
	for _, in := range []petitions.Input{
		{TemplateKey: "vd_borc_durum_yazisi", Fields: map[string]string{"amac": "Ankara Büyükşehir Belediyesi gıda ihalesine"}},
		{TemplateKey: "vd_mukellefiyet_yazisi", Fields: map[string]string{"adet": "2"}},
	} {
		if _, err := svc.Create(ctx, scope, actor, in); err != nil {
			return fmt.Errorf("petition %s: %w", in.TemplateKey, err)
		}
	}
	return nil
}
