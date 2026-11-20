package matching

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// historyLimit caps the audit history of one movement.
const historyLimit = 50

// HistoryEntry is one audit event of a movement (#565): who matched,
// unmatched or booked it, when and with what.
type HistoryEntry struct {
	UUID      uuid.UUID `json:"uuid"`
	Action    string    `json:"action"` // matched | rematched | unmatched | booked
	At        time.Time `json:"at"`
	ActorName *string   `json:"actor_name"`
	Invoices  []string  `json:"invoices"`
	// JournalLines are the journal lines (#669 G) as "2026-10 #12/3 120.01".
	JournalLines []string `json:"journal_lines"`
	// Replaced: what a re-match replaced (invoices and lines), empty otherwise.
	Replaced    []string `json:"replaced"`
	AccountCode *string  `json:"account_code"`
	AccountName *string  `json:"account_name"`
}

var historyActions = map[string]string{
	ActionMatched:   "matched",
	ActionRematched: "rematched",
	ActionUnmatched: "unmatched",
	ActionBooked:    "booked",
}

// History lists the movement's audit events of this engagement, newest
// first. The movement must belong to the engagement's taxpayer; events
// written under another engagement or organization are not shown.
func (s *Service) History(ctx context.Context, sub Subject, id uuid.UUID) ([]HistoryEntry, error) {
	q := database.QueriesFor(s.pool)
	if _, err := s.transaction(ctx, q, sub, id); err != nil {
		return nil, err
	}
	rows, err := q.ListResourceActivityForEngagement(ctx, db.ListResourceActivityForEngagementParams{
		OrganizationID: pgtype.Int8{Int64: sub.OrganizationID, Valid: true},
		Resource:       "bank_transaction",
		ResourceUuid:   pgtype.UUID{Bytes: id, Valid: true},
		EngagementUuid: sub.EngagementUUID.String(),
		LimitCount:     historyLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]HistoryEntry, 0, len(rows))
	for _, r := range rows {
		action, ok := historyActions[r.Action]
		if !ok {
			continue
		}
		var p struct {
			Invoices             []string `json:"invoices"`
			JournalLines         []string `json:"journal_lines"`
			ReplacedInvoices     []string `json:"replaced_invoices"`
			ReplacedJournalLines []string `json:"replaced_journal_lines"`
			AccountCode          string   `json:"account_code"`
			AccountName          string   `json:"account_name"`
		}
		_ = json.Unmarshal(r.Payload, &p)
		e := HistoryEntry{UUID: r.Uuid, Action: action, At: r.CreatedAt.Time, ActorName: actorName(r.ActorName, r.ActorSurname), Invoices: p.Invoices}
		if e.Invoices == nil {
			e.Invoices = []string{}
		}
		e.JournalLines = append([]string{}, p.JournalLines...)
		e.Replaced = append(append([]string{}, p.ReplacedInvoices...), p.ReplacedJournalLines...)
		if p.AccountCode != "" {
			e.AccountCode = &p.AccountCode
		}
		if p.AccountName != "" {
			e.AccountName = &p.AccountName
		}
		out = append(out, e)
	}
	return out, nil
}

func actorName(name, surname pgtype.Text) *string {
	n := strings.TrimSpace(strings.TrimSpace(name.String) + " " + strings.TrimSpace(surname.String))
	if n == "" {
		return nil
	}
	return &n
}
