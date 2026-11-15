package rls

import (
	"context"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

// FromRequest builds the scope of an API request from what the middleware
// chain resolved: the caller, the organization (X-Organization) with its
// read_all permission, the engagement's taxpayer, or the mükellef
// (X-Taxpayer). Anything the chain did not resolve stays empty, so the
// database shows nothing for it.
func FromRequest(ctx context.Context) Scope {
	s := Scope{Role: RoleApp}
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		s.UserID = p.UserInternal
	}
	if o, ok := orgctx.From(ctx); ok {
		s.OrganizationID = o.ID
		s.EngagementsReadAll = o.Has(rbac.PermTenantEngagementsReadAll)
	}
	if e, ok := engagementctx.From(ctx); ok && e.TaxpayerID > 0 {
		s.TaxpayerIDs = append(s.TaxpayerIDs, e.TaxpayerID)
	}
	if t, ok := taxpayerctx.From(ctx); ok && t.ID > 0 {
		s.TaxpayerIDs = append(s.TaxpayerIDs, t.ID)
	}
	return s
}
