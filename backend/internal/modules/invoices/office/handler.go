// Package office serves the office panel's invoice views (issue #142):
// /v1/tenant/engagements/{engagementUuid}/invoices. An office sees the
// engagement's taxpayer invoices of its own period plus whatever the owner
// shared from before it (engagement_document_visible, issue #8).
package office

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/view"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/pagecursor"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Downloader reads an attachment from the object store.
type Downloader func(r *http.Request, path string) (io.ReadCloser, int64, error)

// Handler serves the routes.
type Handler struct {
	q        *db.Queries
	download Downloader
}

// NewHandler builds the handler. download may be nil (no UBL preview).
func NewHandler(q *db.Queries, download Downloader) *Handler {
	return &Handler{q: q, download: download}
}

// RegisterRoutes mounts the routes behind the active-engagement chain. A nil
// resolver (tenancy off) or handler registers nothing.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	chain := func(fn http.HandlerFunc) http.Handler {
		return middleware.Chain(fn,
			middleware.Authenticate(tokens, loader),
			middleware.RequireOrganization(orgResolver),
			middleware.RequireTenantPermission(rbac.PermTenantInvoicesRead),
			middleware.RequireEngagement(engResolver),
		)
	}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/invoices"
	mux.Handle("GET "+base, chain(h.list))
	mux.Handle("GET "+base+"/{invoiceUuid}", chain(h.get))
	mux.Handle("GET "+base+"/{invoiceUuid}/ubl", chain(h.ubl))
	mux.Handle("GET "+base+"/{invoiceUuid}/pdf", chain(h.pdf))
}

// Row is a list row: the invoice plus its open warning count.
type Row struct {
	view.Invoice
	OpenWarnings int64 `json:"open_warnings"`
}

type page struct {
	Items  []Row `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
	// NextCursor fetches the next page with ?cursor= (keyset pagination,
	// #182); null on the last page.
	NextCursor *string `json:"next_cursor"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	q := r.URL.Query()
	limit, offset := paging(q.Get("limit"), q.Get("offset"))
	period := strings.TrimSpace(q.Get("period"))
	direction := strings.TrimSpace(q.Get("direction"))
	if direction != "" && direction != "outgoing" && direction != "incoming" {
		response.Error(w, r, http.StatusBadRequest, "INVALID_DIRECTION", "direction must be outgoing or incoming")
		return
	}
	var rows []db.ListEngagementInvoicesRow
	if c := strings.TrimSpace(q.Get("cursor")); c != "" {
		// Keyset page: offset is ignored; deep pages cost as little as the first.
		date, id, err := pagecursor.Decode(c)
		if err != nil {
			response.Error(w, r, http.StatusBadRequest, "INVALID_CURSOR", "cursor is not valid")
			return
		}
		offset = 0
		after, err := h.q.ListEngagementInvoicesAfter(r.Context(), db.ListEngagementInvoicesAfterParams{
			OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID,
			CursorDate: pgtype.Date{Time: date, Valid: true}, CursorID: id,
			Period: repository.Text(period), Direction: repository.Text(direction), Lim: limit,
		})
		if err != nil {
			response.InternalErr(w, r, err, "could not list invoices")
			return
		}
		for _, a := range after {
			rows = append(rows, db.ListEngagementInvoicesRow(a))
		}
	} else {
		var err error
		rows, err = h.q.ListEngagementInvoices(r.Context(), db.ListEngagementInvoicesParams{
			OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID,
			Period: repository.Text(period), Direction: repository.Text(direction), Lim: limit, Off: offset,
		})
		if err != nil {
			response.InternalErr(w, r, err, "could not list invoices")
			return
		}
	}
	total, err := h.q.CountEngagementInvoices(r.Context(), db.CountEngagementInvoicesParams{
		TaxpayerID: s.TaxpayerID, EngagementID: s.ID, Period: repository.Text(period), Direction: repository.Text(direction),
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not count invoices")
		return
	}
	out := page{Items: make([]Row, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, row := range rows {
		out.Items = append(out.Items, Row{Invoice: view.FromInvoice(row.Invoice), OpenWarnings: row.OpenWarnings})
	}
	if n := len(rows); n > 0 && n == int(limit) {
		next := pagecursor.Encode(rows[n-1].Invoice.IssueDate.Time, rows[n-1].Invoice.ID)
		out.NextCursor = &next
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Warning is a data warning on the invoice or one of its lines.
type Warning struct {
	UUID       uuid.UUID      `json:"uuid"`
	EntityType string         `json:"entity_type"`
	Code       string         `json:"code"`
	Severity   string         `json:"severity"`
	Status     string         `json:"status"`
	Params     map[string]any `json:"params"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Detail is the office invoice detail.
type Detail struct {
	view.Detail
	HasUBL   bool      `json:"has_ubl"`
	Warnings []Warning `json:"warnings"`
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request) (db.Invoice, bool) {
	s := engagementctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("invoiceUuid"))
	if err != nil {
		response.NotFound(w, r, "invoice not found")
		return db.Invoice{}, false
	}
	inv, err := h.q.GetEngagementInvoice(r.Context(), db.GetEngagementInvoiceParams{TaxpayerID: s.TaxpayerID, Uuid: id, EngagementID: s.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "invoice not found")
		return db.Invoice{}, false
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice")
		return db.Invoice{}, false
	}
	return inv, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	d, err := view.LoadDetail(r.Context(), h.q, inv)
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice details")
		return
	}
	ws, err := h.q.ListInvoiceWarnings(r.Context(), db.ListInvoiceWarningsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.ID, TaxpayerID: s.TaxpayerID, InvoiceID: pgtype.Int8{Int64: inv.ID, Valid: true},
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not load warnings")
		return
	}
	out := Detail{Detail: d, HasUBL: inv.UblAttachmentID.Valid && h.download != nil, Warnings: make([]Warning, 0, len(ws))}
	for _, x := range ws {
		out.Warnings = append(out.Warnings, Warning{
			UUID: x.Uuid, EntityType: x.EntityType, Code: x.Code, Severity: x.Severity, Status: x.Status,
			Params: params(x.Params), CreatedAt: x.CreatedAt.Time,
		})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ubl streams the stored UBL XML for the invoice preview.
func (h *Handler) ubl(w http.ResponseWriter, r *http.Request) {
	s := engagementctx.Must(r.Context())
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if !inv.UblAttachmentID.Valid || h.download == nil {
		response.NotFound(w, r, "no UBL stored for this invoice")
		return
	}
	att, err := h.q.GetInvoiceAttachment(r.Context(), db.GetInvoiceAttachmentParams{TaxpayerID: s.TaxpayerID, ID: inv.UblAttachmentID.Int64})
	if err != nil {
		response.InternalErr(w, r, err, "could not load attachment")
		return
	}
	body, size, err := h.download(r, att.StorageKey)
	if err != nil {
		response.InternalErr(w, r, err, "could not read UBL")
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = io.Copy(w, body)
}

func paging(l, o string) (limit, offset int32) {
	limit = 50
	if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded
	}
	if v, err := strconv.Atoi(o); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded
	}
	return limit, offset
}

func params(raw []byte) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}
