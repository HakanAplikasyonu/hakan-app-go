package journals

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ErrNotExported: only an entry in a live export can be cancelled.
var ErrNotExported = errors.New("journals: entry is not exported")

// CancelExport cancels an entry already transferred to the program
// completely (#710, office decision A.4): before the e-defter there is no
// correction entry. In one transaction the entry leaves its export (who,
// when, why on the export item) and goes back to draft with the same
// reason, so it can be corrected, approved and exported again. The entry
// in the program is deleted by the user; the screen says which one. A
// locked period refuses (the e-defter is closed).
func (s *Service) CancelExport(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrReasonRequired
	}
	row, err := s.q.GetJournalDraft(ctx, db.GetJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.Status != "exported" {
		return fmt.Errorf("%w: entry %s/%d is %s", ErrNotExported, row.Period, row.EntryNo, row.Status)
	}
	issues, err := s.draftIssues(ctx, scope, []int64{row.ID})
	if err != nil {
		return err
	}
	for _, is := range issues[row.ID] {
		if is.Code == IssuePeriodLocked {
			return &DraftBlockedError{Issues: []DraftIssue{is}}
		}
	}
	tx, err := database.Begin(ctx, s.db)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	n, err := q.ReverseExportBatchItem(ctx, db.ReverseExportBatchItemParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, JournalDraftID: row.ID,
		ReversedBy: actor(actorID), Reason: repository.Text(reason),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: entry %s/%d", ErrNotExported, row.Period, row.EntryNo)
	}
	n, err = q.RevertJournalDraftApproval(ctx, db.RevertJournalDraftApprovalParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: row.ID,
		RevertedBy: actor(actorID), Reason: repository.Text(reason),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: entry %s/%d", ErrNotExported, row.Period, row.EntryNo)
	}
	return tx.Commit(ctx)
}
