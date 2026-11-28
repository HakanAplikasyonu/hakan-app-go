package lucalists

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// PathParam is the list path parameter.
const PathParam = "list"

// Handler serves the route.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the route. Scope runs requests in one row level security
// transaction (rls.Middleware).
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Scope       func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "lucalists" }

// Routes implements module.Module. The export records what went to Luca
// (double export guard), so it is a POST with journal write, like the
// invoice postings.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	if m.Engagements == nil {
		return
	}
	eng := []func(http.Handler) http.Handler{
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsRead),
		middleware.RequireEngagement(m.Engagements),
	}
	if m.Scope != nil {
		eng = append(eng, m.Scope)
	}
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/luca-lists/{" + PathParam + "}"
	r.Tenant("POST "+base, rbac.PermTenantJournalsWrite, h.Export, eng...)
}

// maxReportRows bounds each list of the X-Luca-Report header.
const maxReportRows = 50

// Export: POST .../luca-lists/{list} {period, confirm_documents,
// include_exported} — the period's documents of the list as a Luca fiş
// file: .xlsx, or .zip when Luca's 50-voucher limit needs several files.
// Documents already exported stay out unless re-included. X-Luca-* headers
// count what went in; X-Luca-Report carries the rows to check (base64url
// JSON); X-Export-UUID names the export.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "LUCA_LISTS_UNAVAILABLE", "the Luca list export is not available")
		return
	}
	list := List(r.PathValue(PathParam))
	if !list.Valid() {
		response.NotFound(w, r, "list not found")
		return
	}
	var in Input
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "body must be {\"period\": \"YYYY-MM\"}")
		return
	}
	var user int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		user = p.UserInternal
	}
	res, err := h.svc.Export(r.Context(), engagementctx.Must(r.Context()), user, list, in)
	switch {
	case errors.Is(err, ErrNothingToExport):
		w.Header().Set("X-Luca-Report", encodeReport(res.Report))
		response.Error(w, r, http.StatusUnprocessableEntity, "NOTHING_TO_EXPORT", err.Error())
		return
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	case err != nil:
		response.InternalErr(w, r, err, "could not write the Luca file")
		return
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, res.Name))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Export-UUID", res.Export.String())
	w.Header().Set("X-Luca-Exported", strconv.Itoa(res.Report.Exported))
	w.Header().Set("X-Luca-Check", strconv.Itoa(len(res.Report.Check)))
	w.Header().Set("X-Luca-Skipped", strconv.Itoa(len(res.Report.Skipped)))
	w.Header().Set("X-Luca-Already-Exported", strconv.Itoa(len(res.Report.AlreadyExported)))
	w.Header().Set("X-Luca-Report", encodeReport(res.Report))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Data)
}

// encodeReport is the report as base64url JSON, each list cut to
// maxReportRows (the counts headers keep the full numbers).
func encodeReport(rep Report) string {
	cut := func(rows []Row) []Row {
		if len(rows) > maxReportRows {
			return rows[:maxReportRows]
		}
		return nonNil(rows)
	}
	rep.Check, rep.Skipped, rep.AlreadyExported = cut(rep.Check), cut(rep.Skipped), cut(rep.AlreadyExported)
	raw, err := json.Marshal(rep)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
