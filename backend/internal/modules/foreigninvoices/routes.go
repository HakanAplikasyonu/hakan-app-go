package foreigninvoices

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/journals"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ledger"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

const maxBodyBytes = 16 << 10

// Handler serves the foreign invoice routes.
type Handler struct{ svc *Service }

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Module mounts the routes. Scope runs requests in one row level security
// transaction (rls.Middleware).
type Module struct {
	H           *Handler
	Engagements engagementctx.Resolver
	Scope       func(http.Handler) http.Handler
}

// Name implements module.Module.
func (Module) Name() string { return "foreigninvoices" }

// Routes implements module.Module. KDV2 is a tax return item: reading needs
// tax_returns.read, writing and confirming tax_returns.write; the office
// setting needs settings.write.
func (m Module) Routes(r *module.Router) {
	h := m.H
	if h == nil {
		h = &Handler{}
	}
	read, write := rbac.PermTenantTaxReturnsRead, rbac.PermTenantTaxReturnsWrite
	var office []func(http.Handler) http.Handler
	if m.Scope != nil {
		office = append(office, m.Scope)
	}
	r.Tenant("GET /v1/tenant/foreign-service-kinds", read, h.GetSettings, office...)
	r.Tenant("PUT /v1/tenant/foreign-service-kinds", rbac.PermTenantSettingsWrite, h.PutSettings, office...)
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
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/foreign-invoices"
	one := base + "/{foreignInvoiceUuid}"
	r.Tenant("GET "+base, read, h.List, eng...)
	r.Tenant("POST "+base, write, h.Create, eng...)
	r.Tenant("GET "+base+"/export", read, h.Export, eng...)
	r.Tenant("PATCH "+one, write, h.Update, eng...)
	r.Tenant("DELETE "+one, write, h.Delete, eng...)
	r.Tenant("POST "+one+"/confirm", write, h.Confirm, eng...)
	r.Tenant("POST "+one+"/unconfirm", write, h.Unconfirm, eng...)
	// Fiş taslağı (#780 KDV2 B): yevmiye yazma yetkisi.
	jw := rbac.PermTenantJournalsWrite
	r.Tenant("POST "+base+"/journal", jw, h.CreatePeriodJournals, eng...)
	r.Tenant("POST "+one+"/journal", jw, h.CreateJournal, eng...)
	r.Tenant("DELETE "+one+"/journal", jw, h.RemoveJournal, eng...)
}

func subject(r *http.Request) Subject {
	e := engagementctx.Must(r.Context())
	return Subject{OrganizationID: e.OrganizationID, EngagementID: e.ID}
}

func userID(r *http.Request) int64 {
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		return p.UserInternal
	}
	return 0
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func invoiceID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("foreignInvoiceUuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "foreignInvoiceUuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

// List: GET .../foreign-invoices?period=YYYY-MM.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.List(r.Context(), subject(r), r.URL.Query().Get("period"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Create: POST .../foreign-invoices.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.Create(r.Context(), subject(r), in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Update: PATCH .../foreign-invoices/{foreignInvoiceUuid}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	var p Patch
	if !decode(w, r, &p) {
		return
	}
	out, err := h.svc.Update(r.Context(), subject(r), id, p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Delete: DELETE .../foreign-invoices/{foreignInvoiceUuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), subject(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Confirm: POST .../foreign-invoices/{foreignInvoiceUuid}/confirm.
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Confirm(r.Context(), subject(r), id, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Unconfirm: POST .../foreign-invoices/{foreignInvoiceUuid}/unconfirm.
func (h *Handler) Unconfirm(w http.ResponseWriter, r *http.Request) {
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Unconfirm(r.Context(), subject(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Export: GET .../foreign-invoices/export?period=&format=xlsx|pdf.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, name, ctype, err := h.svc.Export(r.Context(), subject(r), q.Get("period"), q.Get("format"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// CreateJournal: POST .../foreign-invoices/{foreignInvoiceUuid}/journal.
func (h *Handler) CreateJournal(w http.ResponseWriter, r *http.Request) {
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.CreateJournal(r.Context(), engagementctx.Must(r.Context()), id, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// CreatePeriodJournals: POST .../foreign-invoices/journal?period=YYYY-MM.
func (h *Handler) CreatePeriodJournals(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.CreatePeriodJournals(r.Context(), engagementctx.Must(r.Context()), r.URL.Query().Get("period"), userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RemoveJournal: DELETE .../foreign-invoices/{foreignInvoiceUuid}/journal.
func (h *Handler) RemoveJournal(w http.ResponseWriter, r *http.Request) {
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	if err := h.svc.RemoveJournal(r.Context(), subject(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetSettings: GET /v1/tenant/foreign-service-kinds.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Settings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutSettings: PUT /v1/tenant/foreign-service-kinds.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in SettingsInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.SaveSettings(r.Context(), orgctx.Must(r.Context()).ID, in, userID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, ErrNotFound):
		response.NotFound(w, r, "Foreign invoice not found")
	case errors.Is(err, ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, ErrJournalExists):
		response.Conflict(w, r, "FOREIGN_INVOICE_JOURNAL_EXISTS", "Bu faturanın fişi zaten oluşturuldu")
	case errors.Is(err, ErrJournalLocked):
		msg := "Fiş onaylandı ya da muhasebe programına aktarıldı; burada kaldırılamaz (önce Yevmiye ekranında onayı ya da aktarımı geri alın)"
		if m := err.Error(); m != ErrJournalLocked.Error() {
			msg = strings.TrimPrefix(m, ErrJournalLocked.Error()+": ")
		}
		response.Conflict(w, r, "FOREIGN_INVOICE_JOURNAL_LOCKED", msg)
	case errors.Is(err, ErrNoJournal):
		response.NotFound(w, r, "Bu faturanın fişi yok")
	case errors.Is(err, ledger.ErrNotBalanceSheet):
		response.Conflict(w, r, "LEDGER_NOT_BALANCE_SHEET", "Bu mükellef işletme defteri / serbest meslek kazanç defteri tutuyor; yevmiye fişi oluşturulmaz")
	case errors.Is(err, journals.ErrInvalid):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, ErrJournalsOff):
		response.InternalErr(w, r, err, "Journal drafts are not configured")
	case errors.Is(err, ErrDisabled):
		response.Conflict(w, r, response.CodeConflict, "The office turned foreign invoices off (Ayarlar → Yabancı hizmet faturaları)")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
