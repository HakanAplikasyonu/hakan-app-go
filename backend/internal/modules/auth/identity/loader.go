package identity

import (
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
)

// Loader hydrates Principal from JWT claims via AuthUseCase.
type Loader struct {
	UC *usecase.AuthUseCase
}

// LoadPrincipal implements middleware.IdentityLoader.
func (l Loader) LoadPrincipal(r *http.Request, claims jwt.Claims) (authctx.Principal, error) {
	ctx := r.Context()
	userUUID, err := claims.UserUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	user, err := l.UC.LoadUserByUUID(ctx, userUUID)
	if err != nil {
		return authctx.Principal{}, err
	}
	if user.Status == "disabled" {
		return authctx.Principal{}, usecase.ErrUserDisabled
	}
	isSuperAdmin := claims.IsSuperAdmin
	perms, err := l.UC.ResolvePermissions(ctx, claims.Roles, isSuperAdmin)
	if err != nil {
		return authctx.Principal{}, err
	}
	perms = withSessionBaseline(perms)
	impersonatorUUID, err := claims.ImpersonatorUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	return authctx.Principal{
		UserID:             user.UUID,
		UserInternal:       user.ID,
		Email:              user.Email,
		Roles:              claims.Roles,
		Permissions:        perms,
		IsSuperAdmin:       isSuperAdmin,
		ImpersonatorUserID: impersonatorUUID,
		SessionID:          claims.SessionUUID(),
	}, nil
}

// withSessionBaseline gives every signed-in, non-disabled user auth.session.
// With tenancy on, office and taxpayer users get no global role (invitation
// sign-up and the demo seed assign none), so without this baseline the
// self-scoped session routes (realtime tokens, /v1/me/legal-acceptances,
// search, QR approval) answered 403 for all of them (#141). Access to office
// and taxpayer data is still decided by membership, not by this permission.
func withSessionBaseline(perms []string) []string {
	for _, p := range perms {
		if p == rbac.PermAuthSession {
			return perms
		}
	}
	out := make([]string, 0, len(perms)+1)
	out = append(out, perms...)
	return append(out, rbac.PermAuthSession)
}
