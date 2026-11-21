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
	// LinkCustomer: the taxpayer's cari card (müşteri / tedarikçi).
	LinkCustomer = "customer"
)

// SetManualLink links (on) or unlinks (off) a record to a journal draft by
// hand (#710): "bu fişe bağlı başka kayıt yok" is fixed by the user. The
// link lives in the draft's origin (manual_links); lines, amounts and the
// export are not touched, so it is allowed in every status. It shows in the
// related records of both sides. Adding an existing link or removing a
// missing one is a no-op; the result tells whether the link exists now.
func (s *Service) SetManualLink(ctx context.Context, scope engagementctx.Scope, draft uuid.UUID, kind string, target uuid.UUID, on bool) (bool, error) {
	if kind != LinkInvoice && kind != LinkBankTransaction && kind != LinkCustomer {
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

// LinkCustomerCandidate is a cari card that can be linked to a draft.
type LinkCustomerCandidate struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	// Type: customer, supplier or both.
	Type string `json:"type"`
}

// CustomerCandidates lists the taxpayer's cari cards by name (q searches
// the name only, never the VKN / TCKN), at most limit (1..200; anything
// else means 200).
func (s *Service) CustomerCandidates(ctx context.Context, scope engagementctx.Scope, q string, limit int32) ([]LinkCustomerCandidate, error) {
	if limit < 1 || limit > 200 {
		limit = 200
	}
	rows, err := s.q.ListEngagementCustomers(ctx, db.ListEngagementCustomersParams{TaxpayerID: scope.TaxpayerID, Q: q, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]LinkCustomerCandidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, LinkCustomerCandidate{UUID: r.Uuid, Name: r.Name, Type: r.Type})
	}
	return out, nil
}
