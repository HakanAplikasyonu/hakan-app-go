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
	if rate, part, ok := extract.VatField(s.Input.TargetField); ok && s.Input.TargetType == "expense" {
		total, tax, found, err := vatLine(ctx, q, s.TaxpayerID, id, rate)
		if err != nil || !found {
			return json.RawMessage("null"), err
		}
		if part == "total" {
			return json.Marshal(total.String())
		}
		return json.Marshal(tax.String())
	}
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
	if rate, part, ok := extract.VatField(field); ok {
		return writeVat(ctx, q, s, rate, part, str)
	}
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

// vatLine is an expense's breakdown line for one rate: its total incl. VAT
// and its VAT.
func vatLine(ctx context.Context, q *db.Queries, taxpayerID, expenseID int64, rate string) (total, tax decimal.Decimal, found bool, err error) {
	lines, err := q.ListExpenseVatLines(ctx, db.ListExpenseVatLinesParams{TaxpayerID: taxpayerID, Ids: []int64{expenseID}})
	if err != nil {
		return decimal.Zero, decimal.Zero, false, err
	}
	r := decimal.RequireFromString(rate)
	for _, l := range lines {
		if numDec(l.TaxRate).Equal(r) {
			tax = numDec(l.TaxAmount)
			return numDec(l.TaxBase).Add(tax), tax, true, nil
		}
	}
	return decimal.Zero, decimal.Zero, false, nil
}

// writeVat writes one rate of an expense's breakdown (the taxpayer's
// correction in the mobile preview, Oğuz'un geri bildirimi). A total incl.
// VAT keeps the rate's VAT; a VAT keeps the rate's total; a zero or empty
// total removes the rate. The matrah is total - VAT.
func writeVat(ctx context.Context, q *db.Queries, s review.Suggestion, rate, part string, str *string) error {
	if s.Input.TargetType != "expense" {
		return fmt.Errorf("%w: reviewlink: %s is corrected on the Z report screen", review.ErrInvalidValue, s.Input.TargetField)
	}
	id := *s.Input.TargetID
	e, err := q.GetExpense(ctx, db.GetExpenseParams{TaxpayerID: s.TaxpayerID, ID: id})
	if err != nil {
		return err
	}
	if e.ApprovalStatus != "pending" {
		return errors.New("reviewlink: draft is no longer pending")
	}
	var v decimal.Decimal
	has := str != nil && *str != ""
	if has {
		norm, ok := extract.Normalize(s.Input.TargetField, *str)
		if !ok {
			return fmt.Errorf("%w: reviewlink: %q is not a valid %s", review.ErrInvalidValue, *str, s.Input.TargetField)
		}
		v = decimal.RequireFromString(norm)
	}
	total, tax, found, err := vatLine(ctx, q, s.TaxpayerID, id, rate)
	if err != nil {
		return err
	}
	r := decimal.RequireFromString(rate)
	switch {
	case part == "total" && (!has || v.IsZero()):
		if found {
			if err := q.DeleteExpenseVatLine(ctx, db.DeleteExpenseVatLineParams{TaxpayerID: s.TaxpayerID, ExpenseID: id, TaxRate: toNum(r)}); err != nil {
				return err
			}
		}
		return markReviewed(ctx, q, s, id)
	case part == "total":
		total = v
	case !has:
		if !found {
			return markReviewed(ctx, q, s, id)
		}
		tax = decimal.Zero
	default:
		tax = v
		if !found {
			total = v // the rate's total comes with its own correction
		}
	}
	if tax.IsNegative() || tax.GreaterThan(total) {
		return fmt.Errorf("%w: reviewlink: VAT %s of rate %s is more than its total %s", review.ErrInvalidValue, tax, rate, total)
	}
	if err := q.UpsertExpenseVatLine(ctx, db.UpsertExpenseVatLineParams{
		TaxpayerID: s.TaxpayerID, ExpenseID: id, TaxRate: toNum(r), TaxBase: toNum(total.Sub(tax)), TaxAmount: toNum(tax),
	}); err != nil {
		return err
	}
	return markReviewed(ctx, q, s, id)
}

func markReviewed(ctx context.Context, q *db.Queries, s review.Suggestion, id int64) error {
	return q.MarkExpenseOcrFieldReviewed(ctx, db.MarkExpenseOcrFieldReviewedParams{Field: s.Input.TargetField, TaxpayerID: s.TaxpayerID, ID: id})
}

func numDec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func toNum(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
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
