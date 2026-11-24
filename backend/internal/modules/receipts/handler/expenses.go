package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Expenses: GET .../expenses (#384).
func (h *Handler) Expenses(w http.ResponseWriter, r *http.Request) {
	period, ok := periodq.Parse(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	page, err := h.svc.Expenses(r.Context(), engagementctx.Must(r.Context()), usecase.ExpenseFilter{
		Period: period, Status: qs.Get("status"), Category: qs.Get("category"), Q: qs.Get("q"),
		Limit:  intconv.ParseInt32(qs.Get("limit"), apiquery.DefaultLimit, 1, apiquery.MaxLimit),
		Offset: intconv.ParseInt32(qs.Get("offset"), 0, 0, 1_000_000),
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// Expense: GET .../expenses/{expenseUuid}.
func (h *Handler) Expense(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "expenseUuid")
	if !ok {
		return
	}
	v, err := h.svc.Expense(r.Context(), engagementctx.Must(r.Context()), id)
	replyAny(w, r, v, err)
}

// CreateExpense: POST .../expenses — an expense typed in the office with its
// VAT breakdown per rate (#780).
func (h *Handler) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var in usecase.ManualExpenseInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.CreateManualExpense(r.Context(), engagementctx.Must(r.Context()), userID(r), in)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// SetExpensePosting: PATCH .../expenses/{expenseUuid} {category_code, account_code}.
func (h *Handler) SetExpensePosting(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "expenseUuid")
	if !ok {
		return
	}
	var in usecase.PostingInput
	if !decode(w, r, &in) {
		return
	}
	v, err := h.svc.SetExpensePosting(r.Context(), engagementctx.Must(r.Context()), id, userID(r), in)
	replyAny(w, r, v, err)
}

// ApproveExpense / RejectExpense: POST .../expenses/{expenseUuid}/approve|reject.
func (h *Handler) ApproveExpense(w http.ResponseWriter, r *http.Request) { h.decideExpense(w, r, true) }
func (h *Handler) RejectExpense(w http.ResponseWriter, r *http.Request)  { h.decideExpense(w, r, false) }

func (h *Handler) decideExpense(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := pathID(w, r, "expenseUuid")
	if !ok {
		return
	}
	v, err := h.svc.DecideExpense(r.Context(), engagementctx.Must(r.Context()), id, userID(r), approve)
	replyAny(w, r, v, err)
}

// EngagementCategories: GET .../expense-categories (taxpayer > office > platform).
func (h *Handler) EngagementCategories(w http.ResponseWriter, r *http.Request) {
	sc := engagementctx.Must(r.Context())
	items, err := h.svc.Categories(r.Context(), sc.OrganizationID, sc.ID)
	listReply(w, r, items, err)
}

// SaveEngagementCategory: PUT .../expense-categories (taxpayer's own).
func (h *Handler) SaveEngagementCategory(w http.ResponseWriter, r *http.Request) {
	sc := engagementctx.Must(r.Context())
	h.saveCategory(w, r, sc.OrganizationID, sc.ID)
}

// DeleteEngagementCategory: DELETE .../expense-categories/{categoryUuid}.
func (h *Handler) DeleteEngagementCategory(w http.ResponseWriter, r *http.Request) {
	sc := engagementctx.Must(r.Context())
	h.deleteCategory(w, r, sc.OrganizationID, sc.ID)
}

// OfficeCategories: GET /v1/tenant/expense-categories (office template).
func (h *Handler) OfficeCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Categories(r.Context(), orgctx.Must(r.Context()).ID, 0)
	listReply(w, r, items, err)
}

// SaveOfficeCategory: PUT /v1/tenant/expense-categories.
func (h *Handler) SaveOfficeCategory(w http.ResponseWriter, r *http.Request) {
	h.saveCategory(w, r, orgctx.Must(r.Context()).ID, 0)
}

// DeleteOfficeCategory: DELETE /v1/tenant/expense-categories/{categoryUuid}.
func (h *Handler) DeleteOfficeCategory(w http.ResponseWriter, r *http.Request) {
	h.deleteCategory(w, r, orgctx.Must(r.Context()).ID, 0)
}

func (h *Handler) saveCategory(w http.ResponseWriter, r *http.Request, orgID, engagementID int64) {
	var in usecase.CategoryInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.SaveCategory(r.Context(), orgID, engagementID, userID(r), in)
	replyAny(w, r, c, err)
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request, orgID, engagementID int64) {
	id, ok := pathID(w, r, "categoryUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), orgID, engagementID, id); err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func replyAny(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func listReply[T any](w http.ResponseWriter, r *http.Request, items []T, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// incomplete reports ErrIncomplete (written as 422).
func incomplete(err error) bool { return errors.Is(err, usecase.ErrIncomplete) }
