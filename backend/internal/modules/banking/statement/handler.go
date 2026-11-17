package statement

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// MaxFileBytes: bank statements are small; 20 MB like other attachments.
const MaxFileBytes = 20 << 20

// Handler serves /v1/tenant/engagements/{engagementUuid}/bank-statements.
type Handler struct {
	im   *Importer
	bank *banking.Service
	q    *db.Queries
}

// NewHandler builds the handler.
func NewHandler(im *Importer, bank *banking.Service, q *db.Queries) *Handler {
	return &Handler{im: im, bank: bank, q: q}
}

// RegisterRoutes mounts the statement routes behind the active-engagement
// chain. A nil resolver (tenancy off) or nil handler registers nothing.
func RegisterRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader,
	orgResolver orgctx.Resolver, engResolver engagementctx.Resolver, h *Handler,
) {
	if orgResolver == nil || engResolver == nil || h == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(orgResolver)
	eng := middleware.RequireEngagement(engResolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantBankRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantBankWrite)
	base := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-statements"
	mux.Handle("POST "+base, middleware.Chain(http.HandlerFunc(h.upload), authn, org, write, eng))
	mux.Handle("DELETE "+base+"/{importId}", middleware.Chain(http.HandlerFunc(h.undo), authn, org, write, eng))
	boxes := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/cash-boxes"
	mux.Handle("GET "+boxes, middleware.Chain(http.HandlerFunc(h.listCashBoxes), authn, org, read, eng))
	mux.Handle("POST "+boxes, middleware.Chain(http.HandlerFunc(h.createCashBox), authn, org, write, eng))
	mux.Handle("PATCH "+boxes+"/{cashBoxUuid}", middleware.Chain(http.HandlerFunc(h.renameCashBox), authn, org, write, eng))
	txs := "/v1/tenant/engagements/{" + engagementctx.PathParam + "}/bank-transactions"
	mux.Handle("GET "+txs, middleware.Chain(http.HandlerFunc(h.listTransactions), authn, org, read, eng))
	mux.Handle("GET "+txs+"/export", middleware.Chain(http.HandlerFunc(h.exportTransactions), authn, org, read, eng))
	// Office statement templates (#383): no engagement, the office's layouts.
	tpl := "/v1/tenant/bank-statement-templates"
	mux.Handle("GET "+tpl, middleware.Chain(http.HandlerFunc(h.listTemplates), authn, org, read))
	mux.Handle("POST "+tpl, middleware.Chain(http.HandlerFunc(h.saveTemplate), authn, org, write))
	mux.Handle("POST "+tpl+"/analyze", middleware.Chain(http.HandlerFunc(h.analyze), authn, org, write))
	mux.Handle("DELETE "+tpl+"/{templateUuid}", middleware.Chain(http.HandlerFunc(h.deleteTemplate), authn, org, write))
}

// upload: multipart "file", optional "cash_box_uuid" (when the file names no
// account) and ?preview=true for the dry run (23 §2 step 4).
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large or the form is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field \"file\" is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil || len(data) == 0 {
		response.Error(w, r, http.StatusBadRequest, "FILE_UNREADABLE", "could not read the file")
		return
	}
	if len(data) > MaxFileBytes {
		response.Error(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "file is too large")
		return
	}
	req := Request{
		TaxpayerID: scope.TaxpayerID, OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
		FileName: header.Filename, Data: data,
	}
	req.Preview, _ = strconv.ParseBool(r.URL.Query().Get("preview"))
	if v := r.FormValue("cash_box_uuid"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			response.BadRequest(w, r, "INVALID_CASH_BOX", "cash_box_uuid is not a uuid")
			return
		}
		box, err := h.q.GetCashBoxByUUID(r.Context(), db.GetCashBoxByUUIDParams{TaxpayerID: scope.TaxpayerID, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			response.NotFound(w, r, "cash box not found")
			return
		} else if err != nil {
			response.InternalErr(w, r, err, "upload failed")
			return
		}
		req.CashBoxID = box.ID
	}
	rep, err := h.im.Import(r.Context(), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if req.Preview {
		status = http.StatusOK
	}
	response.JSON(w, r, status, rep)
}

func (h *Handler) undo(w http.ResponseWriter, r *http.Request) {
	scope := engagementctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("importId"))
	if err != nil {
		response.NotFound(w, r, "import not found")
		return
	}
	n, err := h.bank.UndoImport(r.Context(), scope.TaxpayerID, id)
	if err != nil {
		response.InternalErr(w, r, err, "undo failed")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"import_id": id, "deleted": n})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ue *UnknownAccountError
	switch {
	case errors.As(err, &ue):
		details := make([]response.Detail, len(ue.Accounts))
		for i, a := range ue.Accounts {
			details[i] = response.Detail{Field: "account", Message: maskAccount(a)}
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "UNKNOWN_BANK_ACCOUNT",
			"the file names a bank account that is not defined for this taxpayer; add it first with POST .../cash-boxes", details)
	case errors.Is(err, ErrAccountRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, "BANK_ACCOUNT_REQUIRED", "the file names no account; send cash_box_uuid")
	case errors.Is(err, ErrUnsupported):
		response.Error(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE", "upload MT940, .xlsx, CSV or a text PDF statement")
	case errors.Is(err, ErrScannedPDF):
		response.Error(w, r, http.StatusUnprocessableEntity, "SCANNED_PDF", "the PDF has no text layer; scanned statements are not supported yet")
	case errors.Is(err, ErrLayoutUnknown):
		response.Error(w, r, http.StatusUnprocessableEntity, "STATEMENT_LAYOUT_UNKNOWN",
			"the columns of this file are not known; analyse it with POST /v1/tenant/bank-statement-templates/analyze and save a template")
	case errors.Is(err, ErrPDFBalance):
		response.Error(w, r, http.StatusUnprocessableEntity, "STATEMENT_BALANCE_MISMATCH", err.Error())
	case errors.Is(err, banking.ErrInvalidTransaction):
		response.Error(w, r, http.StatusUnprocessableEntity, "STATEMENT_INVALID_ROW", err.Error())
	default:
		var pe parseError
		if errors.As(err, &pe) {
			response.Error(w, r, http.StatusUnprocessableEntity, "STATEMENT_UNREADABLE", err.Error())
			return
		}
		response.InternalErr(w, r, err, "statement import failed")
	}
}
