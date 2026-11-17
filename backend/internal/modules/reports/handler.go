package reports

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// MaxBulk is the most engagements one bulk request may name.
const MaxBulk = 200

// Handler serves the report routes.
type Handler struct {
	q           DeliveryQueries
	deliveries  *DeliveryService
	engagements engagementctx.Resolver
	now         func() time.Time
}

// NewHandler builds the handler; engagements resolves the bulk request's
// engagements (nil: bulk sending answers 404 for every engagement).
func NewHandler(q DeliveryQueries, engagements engagementctx.Resolver) *Handler {
	return &Handler{q: q, deliveries: NewDeliveryService(q), engagements: engagements, now: time.Now}
}

func rangeOf(r *http.Request) (Range, error) {
	q := r.URL.Query()
	return ParseRange(strings.TrimSpace(q.Get("period")), strings.TrimSpace(q.Get("year")))
}

// Office: GET /v1/tenant/engagements/{engagementUuid}/report?period=|year=.
// The office sees the AI comment also while it is a draft.
func (h *Handler) Office(w http.ResponseWriter, r *http.Request) {
	rg, err := rangeOf(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	sc := engagementctx.Must(r.Context())
	rep, err := Build(r.Context(), h.q, sc.TaxpayerID, rg, Options{OrganizationID: sc.OrganizationID, EngagementID: sc.ID}, h.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

// PDF: GET /v1/tenant/engagements/{engagementUuid}/report/pdf?period=|year=.
// The PDF is what the taxpayer receives: only an approved comment is printed.
func (h *Handler) PDF(w http.ResponseWriter, r *http.Request) {
	rg, err := rangeOf(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	sc := engagementctx.Must(r.Context())
	rep, err := Build(r.Context(), h.q, sc.TaxpayerID, rg,
		Options{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ApprovedOnly: true}, h.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	org, _ := orgctx.From(r.Context())
	body, err := RenderPDF(rep, PDFOptions{OfficeName: org.Name, Now: h.now()})
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+FileName(rep)+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(body)
}

type sendInput struct {
	Period    string `json:"period"`
	Year      string `json:"year"`
	Recipient string `json:"recipient"`
}

// Send: POST /v1/tenant/engagements/{engagementUuid}/report/deliveries —
// one click: queue the report email to the taxpayer.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	var in sendInput
	if !decode(w, r, &in) {
		return
	}
	rg, err := ParseRange(strings.TrimSpace(in.Period), strings.TrimSpace(in.Year))
	if err != nil {
		writeError(w, r, err)
		return
	}
	org, _ := orgctx.From(r.Context())
	d, err := h.deliveries.Queue(r.Context(), engagementctx.Must(r.Context()), QueueInput{
		Range: rg, Recipient: in.Recipient, OfficeName: org.Name, UserID: userID(r), Origin: OriginManual})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, d)
}

// History: GET /v1/tenant/engagements/{engagementUuid}/report/deliveries.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	limit, offset := page(r)
	p, err := h.deliveries.ListEngagement(r.Context(), engagementctx.Must(r.Context()), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// OfficeHistory: GET /v1/tenant/report-deliveries (every visible engagement).
func (h *Handler) OfficeHistory(w http.ResponseWriter, r *http.Request) {
	org, _ := orgctx.From(r.Context())
	limit, offset := page(r)
	p, err := h.deliveries.ListOffice(r.Context(), org.ID, userID(r), org.Has(rbac.PermTenantEngagementsReadAll), limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

type bulkInput struct {
	EngagementUUIDs []uuid.UUID `json:"engagement_uuids"`
	Period          string      `json:"period"`
	Year            string      `json:"year"`
}

// BulkItem is the outcome for one engagement of a bulk request.
type BulkItem struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	// Status: queued | skipped.
	Status string `json:"status"`
	// Reason of a skip: no_access | ended | no_email | invalid_email.
	Reason   *string   `json:"reason"`
	Delivery *Delivery `json:"delivery"`
}

// Bulk: POST /v1/tenant/report-deliveries — queue the report email for each
// selected engagement. An engagement the caller may not reach, an ended one
// or a taxpayer without a contact email is skipped and reported.
func (h *Handler) Bulk(w http.ResponseWriter, r *http.Request) {
	var in bulkInput
	if !decode(w, r, &in) {
		return
	}
	if len(in.EngagementUUIDs) == 0 || len(in.EngagementUUIDs) > MaxBulk {
		response.BadRequest(w, r, response.CodeValidationError, "engagement_uuids must name 1-200 engagements")
		return
	}
	rg, err := ParseRange(strings.TrimSpace(in.Period), strings.TrimSpace(in.Year))
	if err != nil {
		writeError(w, r, err)
		return
	}
	org, _ := orgctx.From(r.Context())
	uid := userID(r)
	readAll := org.Has(rbac.PermTenantEngagementsReadAll)
	seen := map[uuid.UUID]bool{}
	items := make([]BulkItem, 0, len(in.EngagementUUIDs))
	skip := func(id uuid.UUID, reason string) {
		items = append(items, BulkItem{EngagementUUID: id, Status: "skipped", Reason: &reason})
	}
	queued := 0
	for _, id := range in.EngagementUUIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if h.engagements == nil {
			skip(id, "no_access")
			continue
		}
		a, err := h.engagements.ResolveEngagement(r.Context(), org.ID, id, uid)
		if errors.Is(err, engagementctx.ErrNotFound) || (err == nil && (a.OrganizationID != org.ID || (!a.Assigned && !readAll))) {
			skip(id, "no_access")
			continue
		}
		if err != nil {
			response.InternalErr(w, r, err, "")
			return
		}
		if a.Status != engagementctx.StatusActive {
			skip(id, "ended")
			continue
		}
		sc := engagementctx.Scope{ID: a.ID, UUID: a.UUID, OrganizationID: a.OrganizationID, TaxpayerID: a.TaxpayerID,
			TaxpayerUUID: a.TaxpayerUUID, Status: a.Status, StartedAt: a.StartedAt}
		d, err := h.deliveries.Queue(r.Context(), sc, QueueInput{Range: rg, OfficeName: org.Name, UserID: uid, Origin: OriginBulk})
		switch {
		case errors.Is(err, ErrNoRecipient):
			skip(id, "no_email")
		case errors.Is(err, ErrBadRecipient):
			skip(id, "invalid_email")
		case err != nil:
			writeError(w, r, err)
			return
		default:
			queued++
			items = append(items, BulkItem{EngagementUUID: id, Status: "queued", Delivery: &d})
		}
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{"items": items, "queued": queued, "skipped": len(items) - queued})
}

// Mine: GET /v1/taxpayer/report?period=|year= (mükellef paneli). The
// numbers only; the AI comment reaches the taxpayer with the office's
// report email.
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	rg, err := rangeOf(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	sc := taxpayerctx.Must(r.Context())
	rep, err := Build(r.Context(), h.q, sc.ID, rg, Options{}, h.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rep)
}

func page(r *http.Request) (int32, int32) {
	return intconv.ParseInt32(r.URL.Query().Get("limit"), 50, 1, 100),
		intconv.ParseInt32(r.URL.Query().Get("offset"), 0, 0, math.MaxInt32)
}

func userID(r *http.Request) int64 {
	p, _ := authctx.PrincipalFrom(r.Context())
	return p.UserInternal
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

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrNoRecipient):
		response.Error(w, r, http.StatusUnprocessableEntity, "NO_RECIPIENT", "The taxpayer has no contact email; give a recipient")
	case errors.Is(err, ErrBadRecipient):
		response.BadRequest(w, r, response.CodeValidationError, "recipient is not a valid email address")
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	default:
		response.InternalErr(w, r, err, "")
	}
}
