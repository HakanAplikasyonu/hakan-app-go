package usecase

import (
	"errors"
	"fmt"
	"slices"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
)

// ErrPermissionEscalation: the caller tried to hand out a permission it does
// not hold itself (issue #167).
var ErrPermissionEscalation = errors.New("cannot grant a permission you do not hold")

// Grantor is the caller who hands out permissions: by writing them into a
// role, or by giving someone a role. A zero Grantor holds nothing, so it can
// grant nothing; system callers act as the owner.
type Grantor struct {
	Role        string
	Permissions []string
}

// OwnerGrantor acts with owner rights (platform administrators, seeding).
var OwnerGrantor = Grantor{Role: orgctx.RoleOwner}

// CheckGrant reports whether g may hand out every permission in perms. The
// owner role holds every tenant permission. Permissions outside perms that a
// role already had are not checked here; callers pass only what is added.
func CheckGrant(g Grantor, perms []string) error {
	if g.Role == orgctx.RoleOwner {
		return nil
	}
	var missing []string
	for _, p := range perms {
		if !slices.Contains(g.Permissions, p) && !slices.Contains(missing, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return fmt.Errorf("%w: %v", ErrPermissionEscalation, missing)
}

// added returns the slugs in next that are not in prev.
func added(prev, next []string) []string {
	var out []string
	for _, p := range next {
		if !slices.Contains(prev, p) {
			out = append(out, p)
		}
	}
	return out
}
