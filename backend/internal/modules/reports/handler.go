package reports

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Handler serves the report routes.
type Handler struct {
	q   Queries
	now func() time.Time
}

// NewHandler builds the handler.
func NewHandler(q Queries) *Handler { return &Handler{q: q, now: time.Now} }

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

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	default:
		response.InternalErr(w, r, err, "")
	}
}
