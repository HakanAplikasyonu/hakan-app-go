// Package officedash is the office home dashboard (issue #237): what the
// müşavir mobil app and the office panel show first. One office-wide read
// with counts of the review queue, document tasks, open warnings and today's
// uploads, plus the latest uploads of the day.
//
// The route runs in the request's row level security scope (issue #166,
// rls.Middleware opened by RequireOrganization); the queries also filter on
// organization_id and, without tenant.engagements.read_all, on the caller's
// engagement assignments. Each section needs its own read permission; a
// section the caller may not read is null.
package officedash

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// RecentLimit caps the uploads listed on the dashboard.
const RecentLimit = 10

// Istanbul is the day boundary of "today" (offices work in Turkish time).
var Istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*60*60)
	}
	return loc
}()

// Queries is what the handler reads (db.Queries on database.QueriesFor).
type Queries interface {
	OfficeDashboardCounts(ctx context.Context, arg db.OfficeDashboardCountsParams) (db.OfficeDashboardCountsRow, error)
	OfficeDashboardRecentUploads(ctx context.Context, arg db.OfficeDashboardRecentUploadsParams) ([]db.OfficeDashboardRecentUploadsRow, error)
}

// Handler serves GET /v1/tenant/dashboard.
type Handler struct {
	Q   Queries
	Now func() time.Time
}

// Dashboard is the answer.
type Dashboard struct {
	GeneratedAt time.Time `json:"generated_at"`
	Since       time.Time `json:"since"`
	Review      *Review   `json:"review"`
	DocTasks    *DocTasks `json:"doc_tasks"`
	Warnings    *Warnings `json:"warnings"`
	Uploads     *Uploads  `json:"uploads"`
}

// Review counts the pending suggestions of the review queue (issue #14).
type Review struct {
	Pending int64 `json:"pending"`
}

// DocTasks counts the document tasks waiting on the mükellef or the office.
type DocTasks struct {
	Open      int64 `json:"open"`
	Submitted int64 `json:"submitted"`
	Overdue   int64 `json:"overdue"`
}

// Warnings counts open data warnings by severity (issue #13).
type Warnings struct {
	Open    int64 `json:"open"`
	Blocker int64 `json:"blocker"`
	Warning int64 `json:"warning"`
	Info    int64 `json:"info"`
}

// Uploads counts what came in since the start of the day (Europe/Istanbul).
type Uploads struct {
	Total     int64    `json:"total"`
	Receipts  int64    `json:"receipts"`
	Invoices  int64    `json:"invoices"`
	Documents int64    `json:"documents"`
	Recent    []Upload `json:"recent"`
}

// Upload is one of today's uploads.
type Upload struct {
	Kind           string     `json:"kind"`
	UUID           uuid.UUID  `json:"uuid"`
	EngagementUUID uuid.UUID  `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID  `json:"taxpayer_uuid"`
	TaxpayerTitle  string     `json:"taxpayer_title"`
	Status         string     `json:"status"`
	TaskUUID       *uuid.UUID `json:"task_uuid"`
	CreatedAt      time.Time  `json:"created_at"`
}

// StartOfDay is midnight of t's day in Europe/Istanbul.
func StartOfDay(t time.Time) time.Time {
	l := t.In(Istanbul)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, Istanbul)
}

// Get: GET /v1/tenant/dashboard.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := orgctx.Must(ctx)
	var user int64
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		user = p.UserInternal
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	since := StartOfDay(now)
	all := s.Has(rbac.PermTenantEngagementsReadAll)
	c, err := h.Q.OfficeDashboardCounts(ctx, db.OfficeDashboardCountsParams{
		OrganizationID: s.ID, AllEngagements: all, UserID: user,
		Now: ts(now), Since: ts(since),
	})
	if err != nil {
		response.InternalErr(w, r, err, "Could not load the dashboard")
		return
	}
	out := Dashboard{GeneratedAt: now.UTC(), Since: since}
	if s.Has(rbac.PermTenantReviewRead) {
		out.Review = &Review{Pending: c.ReviewPending}
	}
	if s.Has(rbac.PermTenantDocTasksRead) {
		out.DocTasks = &DocTasks{Open: c.DocTasksOpen, Submitted: c.DocTasksSubmitted, Overdue: c.DocTasksOverdue}
	}
	if s.Has(rbac.PermTenantWarningsRead) {
		out.Warnings = &Warnings{
			Open:    c.WarningsBlocker + c.WarningsWarning + c.WarningsInfo,
			Blocker: c.WarningsBlocker, Warning: c.WarningsWarning, Info: c.WarningsInfo,
		}
	}
	// Uploads are the engagement's documents: any engagement reader sees
	// them (the route already requires tenant.engagements.read).
	rows, err := h.Q.OfficeDashboardRecentUploads(ctx, db.OfficeDashboardRecentUploadsParams{
		OrganizationID: s.ID, AllEngagements: all, UserID: user, Since: ts(since), Lim: RecentLimit,
	})
	if err != nil {
		response.InternalErr(w, r, err, "Could not load the dashboard")
		return
	}
	up := &Uploads{
		Receipts: c.UploadsReceipts, Invoices: c.UploadsInvoices, Documents: c.UploadsDocuments,
		Total:  c.UploadsReceipts + c.UploadsInvoices + c.UploadsDocuments,
		Recent: make([]Upload, 0, len(rows)),
	}
	for _, row := range rows {
		u := Upload{
			Kind: row.Kind, UUID: row.Uuid, EngagementUUID: row.EngagementUuid,
			TaxpayerUUID: row.TaxpayerUuid, TaxpayerTitle: row.TaxpayerTitle,
			Status: row.Status, CreatedAt: row.CreatedAt.Time,
		}
		if row.TaskUuid.Valid {
			id := uuid.UUID(row.TaskUuid.Bytes)
			u.TaskUUID = &id
		}
		up.Recent = append(up.Recent, u)
	}
	out.Uploads = up
	response.JSON(w, r, http.StatusOK, out)
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// Module mounts the dashboard and client routes. Engagement routes need
// Engagements (the engagement resolver); without it they are not mounted.
type Module struct {
	H           *Handler
	Clients     *ClientsHandler
	Review      *ReviewHandler
	Engagements engagementctx.Resolver
}

// Name implements module.Module.
func (Module) Name() string { return "officedash" }

// Routes implements module.Module.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	r.Tenant("GET /v1/tenant/dashboard", rbac.PermTenantEngagementsRead, h.Get)

	c := m.Clients
	if c == nil {
		c = &ClientsHandler{}
	}
	r.Tenant("GET /v1/tenant/clients", rbac.PermTenantEngagementsRead, c.List)
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}"
	r.Tenant("GET "+base+"/contacts", rbac.PermTenantEngagementsRead, c.Contacts, eng...)
	// Mükellefin telefonu ve e-postası (#780): yazma yetkisi olan düzenler.
	r.Tenant("PUT "+base+"/contacts", rbac.PermTenantEngagementsWrite, c.UpdateContacts, middleware.RequireEngagement(m.Engagements))
	r.Tenant("GET "+base+"/receipts/{receiptUuid}/image", rbac.PermTenantReceiptsRead, c.ReceiptImage, eng...)

	rv := m.Review
	if rv == nil {
		rv = &ReviewHandler{}
	}
	r.Tenant("GET "+base+"/suggestions/{suggestionUuid}/context", rbac.PermTenantReviewRead, rv.Context, eng...)
}
