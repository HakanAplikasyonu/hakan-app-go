// Package taxpayerctx carries the current taxpayer (mükellef) of a request on
// the mükellef side of the product (issue #4, G4): users who belong to a
// company, not to an office.
//
// Like X-Organization, the taxpayer travels per request in the X-Taxpayer
// header (taxpayer uuid), never in the access token, so two tabs can show two
// companies independently. RequireTaxpayer (package middleware) resolves the
// caller's taxpayer_members row and stores the Scope on the context.
package taxpayerctx

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// HeaderName is the request header the mükellef shell sends on /v1/taxpayer/*.
const HeaderName = "X-Taxpayer"

// Taxpayer member roles.
const (
	RoleOwner    = "owner"
	RoleEmployee = "employee"
)

// ErrNotMember is returned for an unknown taxpayer and for one the caller is
// not a member of: both get the same answer, so the header cannot probe
// which taxpayers exist.
var ErrNotMember = errors.New("taxpayerctx: not a member of the taxpayer")

// Scope is the resolved taxpayer membership of one request.
type Scope struct {
	ID    int64
	UUID  uuid.UUID
	Kind  string
	TaxID string
	Title string
	// Role is the caller's mükellef role (owner, employee).
	Role string
	// ContactPhone / ContactEmail: the contact details the office keeps
	// for the taxpayer (#780); "" when not set.
	ContactPhone string
	ContactEmail string
}

// Resolver loads userID's membership in the taxpayer named by uuid.
type Resolver interface {
	ResolveTaxpayer(ctx context.Context, taxpayerUUID uuid.UUID, userID int64) (Scope, error)
}

type ctxKey struct{}

// WithScope stores the scope on the context.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// From returns the scope if the request passed RequireTaxpayer.
func From(ctx context.Context) (Scope, bool) {
	s, ok := ctx.Value(ctxKey{}).(Scope)
	return s, ok
}

// Must returns the scope or panics (handlers behind RequireTaxpayer).
func Must(ctx context.Context) Scope {
	s, ok := From(ctx)
	if !ok {
		panic("taxpayerctx: scope missing from context")
	}
	return s
}
