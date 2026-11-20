package journals

// Office-wide transfer overview (#574): which taxpayers / periods still have
// approved entries that have not reached the accounting program, and the
// latest transfers of the office with the trial balance (mizan) check of
// the transferred period. Read only; every engagement the caller may see.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// OfficeScope is who asks for an office-wide list: without read_all only
// the caller's assigned engagements are listed.
type OfficeScope struct {
	OrganizationID int64
	UserID         int64
	ReadAll        bool
	// Reconciliation: the caller may read reconciliation runs; without it
	// transfers carry no trial balance check.
	Reconciliation bool
}

// PendingTransfer is one taxpayer period with entries not yet in the
// program.
type PendingTransfer struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	Period         string    `json:"period"`
	// Approved: approved entries without an export.
	Approved int64 `json:"approved"`
	// InTransfer: exported entries whose latest transfer is under way.
	InTransfer int64 `json:"in_transfer"`
	// Failed: exported entries whose latest transfer failed.
	Failed int64 `json:"failed"`
}

// PendingTransferPage is a page of PendingTransfer.
type PendingTransferPage struct {
	Items  []PendingTransfer `json:"items"`
	Total  int64             `json:"total"`
	Limit  int32             `json:"limit"`
	Offset int32             `json:"offset"`
}

// Trial balance check states of an imported transfer.
const (
	CheckOK          = "ok"
	CheckDifferences = "differences"
	CheckStale       = "stale"
	CheckNone        = "none"
)

// TransferCheck is the trial balance (mizan) check of a transferred period.
type TransferCheck struct {
	// Status: ok (matches), differences, stale (compared before the
	// transfer was imported) or none (never compared).
	Status string     `json:"status"`
	Issues int64      `json:"issues"`
	RunAt  *time.Time `json:"run_at,omitempty"`
}

// OfficeTransfer is the latest transfer of one export.
type OfficeTransfer struct {
	UUID           uuid.UUID      `json:"uuid"`
	EngagementUUID uuid.UUID      `json:"engagement_uuid"`
	TaxpayerTitle  string         `json:"taxpayer_title"`
	BatchUUID      uuid.UUID      `json:"batch_uuid"`
	Period         string         `json:"period,omitempty"`
	Driver         string         `json:"driver"`
	Status         string         `json:"status"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	Check          *TransferCheck `json:"check,omitempty"`
}

// OfficeTransferPage is a page of OfficeTransfer.
type OfficeTransferPage struct {
	Items  []OfficeTransfer `json:"items"`
	Total  int64            `json:"total"`
	Limit  int32            `json:"limit"`
	Offset int32            `json:"offset"`
}

// ListOfficePending lists the taxpayer periods with entries not yet in the
// program, oldest period first.
func (s *Service) ListOfficePending(ctx context.Context, sc OfficeScope, limit, offset int32) (PendingTransferPage, error) {
	rows, err := s.q.ListOfficePendingTransfers(ctx, db.ListOfficePendingTransfersParams{
		OrganizationID: sc.OrganizationID, AllEngagements: sc.ReadAll, UserID: sc.UserID, Lim: limit, Off: offset,
	})
	if err != nil {
		return PendingTransferPage{}, err
	}
	out := PendingTransferPage{Items: make([]PendingTransfer, 0, len(rows)), Limit: limit, Offset: offset}
	for _, r := range rows {
		out.Total = r.Total
		out.Items = append(out.Items, PendingTransfer{
			EngagementUUID: r.EngagementUuid, TaxpayerTitle: r.TaxpayerTitle, Period: r.Period,
			Approved: r.Approved, InTransfer: r.InTransfer, Failed: r.Failed,
		})
	}
	return out, nil
}

// ListOfficeTransfers lists the latest transfer of every export, newest
// first; status narrows on that latest state.
func (s *Service) ListOfficeTransfers(ctx context.Context, sc OfficeScope, status string, limit, offset int32) (OfficeTransferPage, error) {
	rows, err := s.q.ListOfficeAccountingTransfers(ctx, db.ListOfficeAccountingTransfersParams{
		OrganizationID: sc.OrganizationID, AllEngagements: sc.ReadAll, UserID: sc.UserID,
		Status: pgtype.Text{String: status, Valid: status != ""}, Lim: limit, Off: offset,
	})
	if err != nil {
		return OfficeTransferPage{}, err
	}
	out := OfficeTransferPage{Items: make([]OfficeTransfer, 0, len(rows)), Limit: limit, Offset: offset}
	for _, r := range rows {
		out.Total = r.Total
		v := OfficeTransfer{
			UUID: r.Uuid, EngagementUUID: r.EngagementUuid, TaxpayerTitle: r.TaxpayerTitle,
			BatchUUID: r.BatchUuid, Period: strings.TrimSpace(r.BatchPeriod), Driver: r.Driver, Status: r.Status,
			ErrorMessage: r.ErrorMessage.String, CompletedAt: tsPtr(r.CompletedAt), CreatedAt: r.CreatedAt.Time,
		}
		if sc.Reconciliation && r.Status == TransferImported && v.Period != "" {
			v.Check = transferCheck(r.Checked, tsPtr(r.CheckAt), r.CheckIssues, v.CompletedAt)
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// transferCheck states the trial balance check of an imported transfer: a
// comparison run before the import does not cover the transferred entries.
func transferCheck(checked bool, runAt *time.Time, issues int64, importedAt *time.Time) *TransferCheck {
	switch {
	case !checked || runAt == nil:
		return &TransferCheck{Status: CheckNone}
	case importedAt != nil && runAt.Before(*importedAt):
		return &TransferCheck{Status: CheckStale, RunAt: runAt}
	case issues > 0:
		return &TransferCheck{Status: CheckDifferences, Issues: issues, RunAt: runAt}
	default:
		return &TransferCheck{Status: CheckOK, RunAt: runAt}
	}
}

func officeScope(r *http.Request) OfficeScope {
	org := orgctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	return OfficeScope{
		OrganizationID: org.ID, UserID: p.UserInternal,
		ReadAll:        org.Has(rbac.PermTenantEngagementsReadAll),
		Reconciliation: org.Has(rbac.PermTenantReconciliationRead),
	}
}

// listOfficePending: GET /v1/tenant/journal-transfers/pending.
func (h *Handler) listOfficePending(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	page, err := h.svc.ListOfficePending(r.Context(), officeScope(r), limit, offset)
	if err != nil {
		response.InternalErr(w, r, err, "could not list pending transfers")
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

// listOfficeTransfers: GET /v1/tenant/journal-transfers.
func (h *Handler) listOfficeTransfers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	status := strings.TrimSpace(q.Get("status"))
	if status != "" && !transferStatuses[status] {
		response.BadRequest(w, r, "INVALID_STATUS", "unknown transfer status")
		return
	}
	page, err := h.svc.ListOfficeTransfers(r.Context(), officeScope(r), status, limit, offset)
	if err != nil {
		response.InternalErr(w, r, err, "could not list transfers")
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}
