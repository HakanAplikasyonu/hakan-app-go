// Package reviewlink connects receipt capture to the review queue (#14):
// low-confidence OCR fields become "receipt_field" suggestions, and the
// accountant's accept / correct / revert is written back to the draft.
package reviewlink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	review "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
)

// Kind is the suggestion kind of an OCR field.
const Kind = "receipt_field"

// Submitter is the review service subset.
type Submitter interface {
	Submit(ctx context.Context, sub review.Subject, in review.Input) (review.Suggestion, error)
}

// Reviewer implements receipts.Reviewer.
type Reviewer struct{ svc Submitter }

// NewReviewer builds the adapter.
func NewReviewer(svc Submitter) *Reviewer { return &Reviewer{svc: svc} }

var _ receipts.Reviewer = (*Reviewer)(nil)

// SubmitField sends one field to the queue with its provenance (model +
// the photo it was read from).
func (r *Reviewer) SubmitField(ctx context.Context, f receipts.FieldReview) error {
	val := f.Value.Value
	if !f.Value.Valid {
		val = f.Value.Raw
	}
	raw, _ := json.Marshal(val)
	targetID := f.TargetID
	why := fmt.Sprintf("Belge okuma bu alanı %%%.0f güvenle okudu (onay eşiği %%%.0f); belgeyle karşılaştırın.", f.Value.Confidence*100, receipts.ReviewBelow*100)
	if !f.Value.Valid {
		why = "OCR değeri okunamadı veya biçimi geçersiz: " + f.Value.Raw
	}
	model := f.Model
	if model == "" {
		model = "unknown"
	}
	prov := review.Provenance{Origin: review.OriginModel, Model: model, Inputs: map[string]any{
		"raw": f.Value.Raw, "valid": f.Value.Valid, "attachment_id": f.AttachmentID,
	}}
	if f.ProposedBy > 0 {
		// The taxpayer typed the value in the mobile app (#199).
		why = "Mükellef düzeltmesi"
		if f.Note != "" {
			why += ": " + f.Note
		}
		prov = review.Provenance{Origin: review.OriginInput, Inputs: map[string]any{
			"raw": f.Value.Raw, "attachment_id": f.AttachmentID, "proposed_by": "taxpayer", "user_id": f.ProposedBy,
		}}
	}
	if f.ReceiptUUID != uuid.Nil {
		prov.Inputs["receipt_uuid"] = f.ReceiptUUID.String()
	}
	_, err := r.svc.Submit(ctx, review.Subject{OrganizationID: f.OrganizationID, EngagementID: f.EngagementID, TaxpayerID: f.TaxpayerID}, review.Input{
		Kind: Kind, TargetType: f.TargetType, TargetID: &targetID, TargetField: f.Field, Period: f.Period,
		Value: raw, Confidence: decimal.NewFromFloat(f.Value.Confidence), Rationale: why, Provenance: prov,
	})
	if errors.Is(err, review.ErrInvalid) {
		// A value the draft cannot take (a correction whose VAT exceeds the
		// total): the caller's input error, not a server failure (#318).
		return fmt.Errorf("%w: %v", receipts.ErrInvalid, err)
	}
	return err
}

// Applier writes a reviewed value to the expense / retail_sale draft.
type Applier struct{}

var _ review.Applier = Applier{}

var errHandle = errors.New("reviewlink: transaction handle is not *db.Queries")

// Apply implements review.Applier.
func (Applier) Apply(ctx context.Context, tx review.Tx, s review.Suggestion, value json.RawMessage) (json.RawMessage, error) {
	q, ok := tx.Handle().(*db.Queries)
	if !ok {
		return nil, errHandle
	}
	prev, err := current(ctx, q, s)
	if err != nil {
		return nil, err
	}
	if err := write(ctx, q, s, value); err != nil {
		return nil, err
	}
	return prev, nil
}

// Revert implements review.Applier.
func (Applier) Revert(ctx context.Context, tx review.Tx, s review.Suggestion, previous json.RawMessage) error {
	q, ok := tx.Handle().(*db.Queries)
	if !ok {
		return errHandle
	}
	return write(ctx, q, s, previous)
}

func current(ctx context.Context, q *db.Queries, s review.Suggestion) (json.RawMessage, error) {
	id := *s.Input.TargetID
	var v any
	switch s.Input.TargetType {
	case "expense":
		e, err := q.GetExpense(ctx, db.GetExpenseParams{TaxpayerID: s.TaxpayerID, ID: id})
		if err != nil {
			return nil, err
		}
		v = map[string]any{
			extract.FieldDate: dateText(e.ExpenseDate), extract.FieldTotal: numText(e.Amount), extract.FieldTax: numText(e.TaxAmount),
			extract.FieldTaxRate: numText(e.TaxRate), extract.FieldVendorTaxNumber: e.VendorTaxNumber.String,
			extract.FieldVendorName: e.VendorName.String, extract.FieldReceiptNumber: e.ReceiptNumber.String,
			extract.FieldPlate: e.VehiclePlate.String, extract.FieldZNumber: e.ZReportNumber.String,
			extract.FieldOKCSerial: e.OkcSerialNumber.String, extract.FieldPaymentMethod: e.PaymentMethod.String,
		}[s.Input.TargetField]
	case "retail_sale":
		r, err := q.GetRetailSale(ctx, db.GetRetailSaleParams{TaxpayerID: s.TaxpayerID, ID: id})
		if err != nil {
			return nil, err
		}
		v = map[string]any{
			extract.FieldDate: dateText(r.SaleDate), extract.FieldTotal: numText(r.TotalAmount), extract.FieldTax: numText(r.TaxAmount),
			extract.FieldZNumber: r.ZReportNumber.String, extract.FieldOKCSerial: r.OkcSerialNumber.String,
		}[s.Input.TargetField]
	default:
		return nil, fmt.Errorf("reviewlink: target %q", s.Input.TargetType)
	}
	if v == "" {
		v = nil
	}
	return json.Marshal(v)
}

func write(ctx context.Context, q *db.Queries, s review.Suggestion, value json.RawMessage) error {
	var str *string
	if err := json.Unmarshal(value, &str); err != nil {
		return fmt.Errorf("%w: reviewlink: value must be a JSON string or null: %v", review.ErrInvalidValue, err)
	}
	field := s.Input.TargetField
	var dateV pgtype.Date
	var numV pgtype.Numeric
	var textV pgtype.Text
	if str != nil && *str != "" {
		norm, ok := extract.Normalize(field, *str)
		if !ok {
			return fmt.Errorf("%w: reviewlink: %q is not a valid %s", review.ErrInvalidValue, *str, field)
		}
		switch field {
		case extract.FieldDate:
			t, _ := time.Parse("2006-01-02", norm)
			dateV = pgtype.Date{Time: t, Valid: true}
		case extract.FieldTotal, extract.FieldTax, extract.FieldTaxRate:
			d := decimal.RequireFromString(norm)
			numV = pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
		default:
			textV = pgtype.Text{String: norm, Valid: true}
		}
	}
	id := *s.Input.TargetID
	if numV.Valid && (field == extract.FieldTotal || field == extract.FieldTax) {
		if err := checkAmounts(ctx, q, s, field, decimal.NewFromBigInt(numV.Int, numV.Exp)); err != nil {
			return err
		}
	}
	var err error
	switch s.Input.TargetType {
	case "expense":
		_, err = q.UpdateExpenseField(ctx, db.UpdateExpenseFieldParams{Field: field, DateValue: dateV, NumValue: numV, TextValue: textV, TaxpayerID: s.TaxpayerID, ID: id})
	case "retail_sale":
		_, err = q.UpdateRetailSaleField(ctx, db.UpdateRetailSaleFieldParams{Field: field, DateValue: dateV, NumValue: numV, TextValue: textV, TaxpayerID: s.TaxpayerID, ID: id})
	default:
		return fmt.Errorf("reviewlink: target %q", s.Input.TargetType)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("reviewlink: draft is no longer pending")
	}
	return err
}

// checkAmounts refuses a total or VAT that would leave the draft's VAT above
// its total: the net amount (total - VAT) must not go negative
// (chk_expenses_amounts; #318 found it as a 500).
func checkAmounts(ctx context.Context, q *db.Queries, s review.Suggestion, field string, v decimal.Decimal) error {
	var total, tax pgtype.Numeric
	switch s.Input.TargetType {
	case "expense":
		e, err := q.GetExpense(ctx, db.GetExpenseParams{TaxpayerID: s.TaxpayerID, ID: *s.Input.TargetID})
		if err != nil {
			return err
		}
		total, tax = e.Amount, e.TaxAmount
	case "retail_sale":
		r, err := q.GetRetailSale(ctx, db.GetRetailSaleParams{TaxpayerID: s.TaxpayerID, ID: *s.Input.TargetID})
		if err != nil {
			return err
		}
		total, tax = r.TotalAmount, r.TaxAmount
	default:
		return nil
	}
	t, x := numText(total), numText(tax)
	if field == extract.FieldTotal {
		t = v.String()
	} else {
		x = v.String()
	}
	if t == "" || x == "" {
		return nil
	}
	if decimal.RequireFromString(x).GreaterThan(decimal.RequireFromString(t)) {
		return fmt.Errorf("%w: reviewlink: VAT %s is more than the total %s", review.ErrInvalidValue, x, t)
	}
	return nil
}

func dateText(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func numText(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return ""
	}
	return decimal.NewFromBigInt(n.Int, n.Exp).String()
}
