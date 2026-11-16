package identity

import (
	"net/http"
	"slices"

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
	// A signed-out, revoked or password-reset session's access tokens stop
	// at once, not when they expire (#323).
	ended, err := l.UC.SessionEnded(ctx, user.ID, claims.SessionUUID())
	if err != nil {
		return authctx.Principal{}, err
	}
	if ended {
		return authctx.Principal{}, usecase.ErrSessionEnded
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

// withSessionBaseline gives every signed-in, non-disabled user auth.session
// and notifications.read. With tenancy on, office and taxpayer users get no
// global role (invitation sign-up and the demo seed assign none), so without
// this baseline the self-scoped routes answered 403 for all of them: the
// session routes (realtime tokens, /v1/me/legal-acceptances, search, QR
// approval; #141) and their own notification inbox (/v1/notifications*,
// every query of it is by the caller's user id; #323). Access to office and
// taxpayer data is still decided by membership, not by these permissions.
func withSessionBaseline(perms []string) []string {
	out := make([]string, 0, len(perms)+2)
	out = append(out, perms...)
	for _, base := range []string{rbac.PermAuthSession, rbac.PermNotificationsRead} {
		if !slices.Contains(perms, base) {
			out = append(out, base)
		}
	}
	return out
}
