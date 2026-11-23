package officedash

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ublunit"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// ReviewQueries is what the review context route reads.
type ReviewQueries interface {
	GetSuggestionReviewContext(ctx context.Context, arg db.GetSuggestionReviewContextParams) (db.GetSuggestionReviewContextRow, error)
}

// ReviewHandler serves the review context of the müşavir mobil queue (issue #239).
type ReviewHandler struct {
	Q ReviewQueries
}

// ReviewContext is the document behind a suggestion.
type ReviewContext struct {
	SuggestionUUID uuid.UUID      `json:"suggestion_uuid"`
	Invoice        *ReviewInvoice `json:"invoice"`
	Item           *ReviewItem    `json:"item"`
	Receipt        *ReviewReceipt `json:"receipt"`
	// Expense is the expense draft of a receipt field suggestion: the
	// mobile approval screen reads and edits its account code there.
	Expense *ReviewExpense `json:"expense"`
}

// ReviewInvoice is the invoice of an invoice item suggestion.
type ReviewInvoice struct {
	UUID             uuid.UUID `json:"uuid"`
	Number           string    `json:"invoice_number"`
	Direction        string    `json:"direction"`
	IssueDate        string    `json:"issue_date"`
	Currency         string    `json:"currency"`
	PayableAmount    string    `json:"payable_amount"`
	CounterpartyName string    `json:"counterparty_name"`
}

// ReviewItem is the invoice line the suggestion targets.
type ReviewItem struct {
	LineNo   int32   `json:"line_no"`
	Name     string  `json:"name"`
	Quantity string  `json:"quantity"`
	UnitCode *string `json:"unit_code"`
	// UnitName is the Turkish name of UnitCode (#378).
	UnitName   *string `json:"unit_name"`
	LineAmount string  `json:"line_amount"`
	TaxRate    string  `json:"tax_rate"`
}

// ReviewReceipt points at the receipt photo (…/receipts/{uuid}/image).
type ReviewReceipt struct {
	UUID      uuid.UUID `json:"uuid"`
	MediaType string    `json:"media_type"`
}

// ReviewExpense points at the expense draft (…/expenses/{uuid}).
type ReviewExpense struct {
	UUID uuid.UUID `json:"uuid"`
}

// Context: GET /v1/tenant/engagements/{engagementUuid}/suggestions/{suggestionUuid}/context.
func (h *ReviewHandler) Context(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e := engagementctx.Must(ctx)
	id, err := uuid.Parse(r.PathValue("suggestionUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "suggestionUuid is invalid")
		return
	}
	row, err := h.Q.GetSuggestionReviewContext(ctx, db.GetSuggestionReviewContextParams{
		OrganizationID: e.OrganizationID, EngagementID: e.ID, SuggestionUuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "Suggestion not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "Could not load the suggestion")
		return
	}
	out := ReviewContext{SuggestionUUID: row.Uuid}
	if row.InvoiceUuid.Valid {
		inv := &ReviewInvoice{
			UUID: uuid.UUID(row.InvoiceUuid.Bytes), Number: row.InvoiceNumber.String, Direction: row.Direction.String,
			Currency: row.Currency.String, PayableAmount: row.PayableAmount, CounterpartyName: row.CounterpartyName,
		}
		if row.IssueDate.Valid {
			inv.IssueDate = row.IssueDate.Time.Format(time.DateOnly)
		}
		out.Invoice = inv
	}
	if row.LineNo.Valid {
		it := &ReviewItem{LineNo: row.LineNo.Int32, Name: row.ItemName.String, Quantity: row.Quantity, LineAmount: row.LineAmount, TaxRate: row.TaxRate}
		if row.UnitCode.Valid {
			u := row.UnitCode.String
			it.UnitCode = &u
			n := ublunit.Name(u)
			it.UnitName = &n
		}
		out.Item = it
	}
	if row.ReceiptUuid.Valid {
		out.Receipt = &ReviewReceipt{UUID: uuid.UUID(row.ReceiptUuid.Bytes), MediaType: row.ReceiptMediaType.String}
	}
	if row.ExpenseUuid.Valid {
		out.Expense = &ReviewExpense{UUID: uuid.UUID(row.ExpenseUuid.Bytes)}
	}
	response.JSON(w, r, http.StatusOK, out)
}
