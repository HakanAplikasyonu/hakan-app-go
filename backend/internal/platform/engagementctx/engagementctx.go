// Package engagementctx carries the current engagement (ofis <-> mükellef
// hizmet ilişkisi) of a tenant request: the second scope level inside an
// organization (tenant-core faz 6a, issue #2).
//
// A member reaches an engagement when they are assigned to it, or when their
// tenant role grants tenant.engagements.read_all. RequireEngagement (package
// middleware) resolves the engagement named in the route and stores the Scope
// on the context; handlers read it with Must.
package engagementctx

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// PathParam is the route wildcard that names the engagement.
const PathParam = "engagementUuid"

// Engagement statuses.
const (
	StatusActive = "active"
	StatusEnded  = "ended"
)

// Resolution errors. Resolvers return these (possibly wrapped).
var (
	// ErrNotFound: no engagement with that uuid in the organization.
	ErrNotFound = errors.New("engagementctx: engagement not found")
	// ErrEnded: the engagement exists but has ended.
	ErrEnded = errors.New("engagementctx: engagement ended")
)

// Access is what a Resolver returns: the engagement row and whether userID
// is assigned to it. The middleware decides with the caller's permissions.
type Access struct {
	ID             int64
	UUID           uuid.UUID
	OrganizationID int64
	TaxpayerID     int64
	TaxpayerUUID   uuid.UUID
	Status         string
	StartedAt      time.Time
	EndedAt        *time.Time
	Assigned       bool
}

// Scope is the resolved engagement of one request.
type Scope struct {
	ID             int64
	UUID           uuid.UUID
	OrganizationID int64
	TaxpayerID     int64
	TaxpayerUUID   uuid.UUID
	Status         string
	StartedAt      time.Time
	// ViaReadAll is true when the caller is not assigned and reached the
	// engagement through tenant.engagements.read_all.
	ViaReadAll bool
}

// Resolver loads an engagement of organizationID by uuid and reports whether
// userID is assigned to it. It returns ErrNotFound for an unknown uuid or one
// that belongs to another organization.
type Resolver interface {
	ResolveEngagement(ctx context.Context, organizationID int64, engagementUUID uuid.UUID, userID int64) (Access, error)
}

type ctxKey struct{}

// WithScope stores the scope on the context.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// From returns the scope if the request passed RequireEngagement.
func From(ctx context.Context) (Scope, bool) {
	s, ok := ctx.Value(ctxKey{}).(Scope)
	return s, ok
}

// Must returns the scope or panics (handlers behind RequireEngagement).
func Must(ctx context.Context) Scope {
	s, ok := From(ctx)
	if !ok {
		panic("engagementctx: scope missing from context")
	}
	return s
}
