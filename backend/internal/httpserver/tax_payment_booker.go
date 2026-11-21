package httpserver

import (
	"context"
	"errors"

	"github.com/google/uuid"

	bankposting "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/posting"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// taxPaymentBooker books a tax payment's bank movement (#575) through the
// bank posting workflow; a movement already in a journal draft keeps its
// account.
type taxPaymentBooker struct{ flow *bankposting.Workflow }

func (b taxPaymentBooker) BookBankTransaction(ctx context.Context, sc engagementctx.Scope, userID int64, txn uuid.UUID, code, name string) error {
	err := b.flow.SetAccount(ctx, bankposting.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID}, userID, txn, code, name)
	if errors.Is(err, bankposting.ErrDrafted) {
		return nil
	}
	return err
}
