package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// RequireTaxpayer resolves the current taxpayer from the X-Taxpayer header
// (taxpayer uuid) and the caller's taxpayer_members row (issue #4). It must
// run after Authenticate. Office membership gives no access here, and
// super_admin is not exempt.
//
// A malformed uuid, an unknown taxpayer and a missing membership all get the
// same answer (403 NO_TAXPAYER_MEMBERSHIP).
func RequireTaxpayer(resolver taxpayerctx.Resolver) func(http.Handler) http.Handler {
	if resolver == nil {
		panic("middleware: RequireTaxpayer needs a resolver")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			ref := strings.TrimSpace(r.Header.Get(taxpayerctx.HeaderName))
			if ref == "" {
				response.BadRequest(w, r, response.CodeTaxpayerRequired, taxpayerctx.HeaderName+" header is required")
				return
			}
			id, err := uuid.Parse(ref)
			if err != nil {
				writeNoTaxpayerMembership(w, r, err)
				return
			}
			scope, err := resolver.ResolveTaxpayer(r.Context(), id, p.UserInternal)
			if err != nil {
				if errors.Is(err, taxpayerctx.ErrNotMember) {
					writeNoTaxpayerMembership(w, r, err)
					return
				}
				response.InternalErr(w, r, err, "")
				return
			}
			next.ServeHTTP(w, r.WithContext(taxpayerctx.WithScope(r.Context(), scope)))
		})
	}
}

func writeNoTaxpayerMembership(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	response.Error(w, r, http.StatusForbidden, response.CodeNoTaxpayerMembership, "You are not a member of this taxpayer")
}
