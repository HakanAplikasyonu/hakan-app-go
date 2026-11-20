package journals

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Kinds a journal draft can be linked to by hand (#710).
const (
	LinkInvoice         = "invoice"
	LinkBankTransaction = "bank_transaction"
)

// SetManualLink links (on) or unlinks (off) a record to a journal draft by
// hand (#710): "bu fişe bağlı başka kayıt yok" is fixed by the user. The
// link lives in the draft's origin (manual_links); lines, amounts and the
// export are not touched, so it is allowed in every status. It shows in the
// related records of both sides. Adding an existing link or removing a
// missing one is a no-op; the result tells whether the link exists now.
func (s *Service) SetManualLink(ctx context.Context, scope engagementctx.Scope, draft uuid.UUID, kind string, target uuid.UUID, on bool) (bool, error) {
	if kind != LinkInvoice && kind != LinkBankTransaction {
		return false, fmt.Errorf("%w: unknown link kind %q", ErrInvalid, kind)
	}
	row, err := s.q.GetJournalDraft(ctx, db.GetJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: draft})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	id, err := s.q.ResolveRelatedSource(ctx, db.ResolveRelatedSourceParams{
		Kind: kind, TaxpayerID: scope.TaxpayerID, Uuid: target, EngagementID: scope.ID, OrganizationID: scope.OrganizationID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if on {
		_, err = s.q.AddJournalDraftManualLink(ctx, db.AddJournalDraftManualLinkParams{
			Kind: kind, TargetID: id, OrganizationID: scope.OrganizationID, EngagementID: scope.ID, DraftID: row.ID,
		})
	} else {
		_, err = s.q.RemoveJournalDraftManualLink(ctx, db.RemoveJournalDraftManualLinkParams{
			Kind: kind, TargetID: id, OrganizationID: scope.OrganizationID, EngagementID: scope.ID, DraftID: row.ID,
		})
	}
	if err != nil {
		return false, err
	}
	return on, nil
}
