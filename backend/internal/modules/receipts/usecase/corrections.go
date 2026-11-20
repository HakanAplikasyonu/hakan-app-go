package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// ErrNoReviewQueue: corrections need the review queue and it is not wired.
var ErrNoReviewQueue = errors.New("receipts: review queue is not configured")

// Correction is a value the taxpayer proposes for an OCR field (#199).
type Correction struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
}

// MaxCorrections bounds one request.
const MaxCorrections = 20

// Correct sends the taxpayer's proposed values to the office's review queue
// as suggestions on the capture's draft. The accountant accepts or rejects
// them there; nothing is written to the draft here. A proposal replaces a
// pending suggestion on the same field (the low-confidence OCR one).
func (s *Service) Correct(ctx context.Context, sc engagementctx.Scope, captureUUID uuid.UUID, userID int64, in []Correction) ([]string, error) {
	if len(in) == 0 || len(in) > MaxCorrections {
		return nil, fmt.Errorf("%w: send 1 to %d corrections", ErrInvalid, MaxCorrections)
	}
	type item struct {
		field string
		value extract.Value
		note  string
	}
	items := make([]item, 0, len(in))
	seen := map[string]bool{}
	for _, c := range in {
		f := strings.TrimSpace(c.Field)
		raw := strings.TrimSpace(c.Value)
		note := strings.TrimSpace(c.Note)
		switch {
		case !slices.Contains(extract.Fields, f):
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalid, c.Field)
		case seen[f]:
			return nil, fmt.Errorf("%w: field %q appears twice", ErrInvalid, f)
		case raw == "" || utf8.RuneCountInString(raw) > 200:
			return nil, fmt.Errorf("%w: %s value must be 1 to 200 characters", ErrInvalid, f)
		case utf8.RuneCountInString(note) > 500:
			return nil, fmt.Errorf("%w: note is longer than 500 characters", ErrInvalid)
		}
		norm, ok := extract.Normalize(f, raw)
		if !ok {
			return nil, fmt.Errorf("%w: %s value %q is not valid", ErrInvalid, f, raw)
		}
		seen[f] = true
		items = append(items, item{field: f, value: extract.Value{Raw: raw, Value: norm, Confidence: 1, Valid: true}, note: note})
	}
	if s.reviewer == nil {
		return nil, ErrNoReviewQueue
	}
	c, err := s.get(ctx, sc, captureUUID)
	if err != nil {
		return nil, err
	}
	target, id, period, err := s.draftOf(ctx, sc, c)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if err := s.reviewer.SubmitField(ctx, FieldReview{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
			TargetType: target, TargetID: id, Period: period, Field: it.field, Value: it.value,
			AttachmentID: c.AttachmentID, ReceiptUUID: c.Uuid, ProposedBy: userID, Note: it.note,
		}); err != nil {
			return out, fmt.Errorf("receipts: submit correction %s: %w", it.field, err)
		}
		out = append(out, it.field)
	}
	return out, nil
}

// draftOf is the expense / retail sale draft of a capture and its period.
func (s *Service) draftOf(ctx context.Context, sc engagementctx.Scope, c db.ReceiptCapture) (string, int64, string, error) {
	switch {
	case c.ExpenseID.Valid:
		e, err := s.q.GetExpense(ctx, db.GetExpenseParams{TaxpayerID: sc.TaxpayerID, ID: c.ExpenseID.Int64})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, "", ErrNotFound
		}
		return "expense", e.ID, e.Period.String, err
	case c.RetailSaleID.Valid:
		r, err := s.q.GetRetailSale(ctx, db.GetRetailSaleParams{TaxpayerID: sc.TaxpayerID, ID: c.RetailSaleID.Int64})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, "", ErrNotFound
		}
		return "retail_sale", r.ID, r.Period.String, err
	default:
		// Failed or duplicate captures have no draft to correct.
		return "", 0, "", fmt.Errorf("%w: the receipt has no draft (status %s)", ErrState, c.Status)
	}
}
