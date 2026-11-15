package taxpayers

import (
	"context"
	"errors"
)

// SignupGate is the auth module's InvitationGate.
type SignupGate interface {
	CheckSignupInvitation(ctx context.Context, token, email string) error
}

// ChainGates lets an invitation sign-up through for office invitations and
// mükellef invitations alike: each gate is asked in order and the first one
// that knows the token decides. A token none of them knows is
// ErrInvitationNotFound.
func ChainGates(gates ...SignupGate) SignupGate { return chain(gates) }

type chain []SignupGate

func (c chain) CheckSignupInvitation(ctx context.Context, token, email string) error {
	for _, g := range c {
		if g == nil {
			continue
		}
		err := g.CheckSignupInvitation(ctx, token, email)
		if errors.Is(err, ErrInvitationNotFound) || errors.Is(err, ErrInvitationsDisabled) {
			continue
		}
		return err
	}
	return ErrInvitationNotFound
}
