package foreigninvoices

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Portal resolves the company's current office for the mükellef panel.
type Portal interface {
	GetActiveEngagementForTaxpayer(ctx context.Context, taxpayerID int64) (db.Engagement, error)
}

// WithIntake enables the file routes (#780 A2): the office's source file
// view and the mükellef panel's upload and list.
func (h *Handler) WithIntake(in *Intake, portal Portal) *Handler {
	h.intake, h.portal = in, portal
	return h
}

// File: GET /v1/tenant/engagements/{e}/foreign-invoices/{uuid}/file.
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	if h.intake == nil {
		response.NotFound(w, r, "Foreign invoice file not found")
		return
	}
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	f, err := h.intake.SourceFile(r.Context(), subject(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	writeFile(w, f)
}

func writeFile(w http.ResponseWriter, f File) {
	safe := strings.Map(func(c rune) rune {
		if c < 0x20 || c == 0x7f || c == '"' || c == '\\' || c > 0x7e {
			return '_'
		}
		return c
	}, f.Name)
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safe))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// taxpayerSubject is the engagement of the company's current office; ok
// false when it answered (no office: 404 for reads, 409 for uploads).
func (h *Handler) taxpayerSubject(w http.ResponseWriter, r *http.Request, write bool) (Subject, taxpayerctx.Scope, bool) {
	s := taxpayerctx.Must(r.Context())
	if h.intake == nil || h.portal == nil {
		response.NotFound(w, r, "")
		return Subject{}, s, false
	}
	eng, err := h.portal.GetActiveEngagementForTaxpayer(r.Context(), s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		if write {
			response.Error(w, r, http.StatusConflict, "NO_ACTIVE_ENGAGEMENT", "the company has no active office to process uploads")
		} else {
			response.NotFound(w, r, "The taxpayer has no active office")
		}
		return Subject{}, s, false
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not resolve office")
		return Subject{}, s, false
	}
	return Subject{OrganizationID: eng.OrganizationID, EngagementID: eng.ID}, s, true
}

// TaxpayerList: GET /v1/taxpayer/foreign-invoices?period=YYYY-MM. The
// company's own invoices sent to the office, read only.
func (h *Handler) TaxpayerList(w http.ResponseWriter, r *http.Request) {
	sub, s, ok := h.taxpayerSubject(w, r, false)
	if !ok {
		return
	}
	var out TaxpayerList
	err := rls.ForOffice(r.Context(), sub.OrganizationID, s.ID, func(ctx context.Context) error {
		var err error
		out, err = h.intake.ForTaxpayer(ctx, sub, r.URL.Query().Get("period"))
		return err
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// TaxpayerUpload: POST /v1/taxpayer/foreign-invoices (multipart: file,
// note). The file goes to the office as a draft foreign invoice.
func (h *Handler) TaxpayerUpload(w http.ResponseWriter, r *http.Request) {
	sub, s, ok := h.taxpayerSubject(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file must be a PDF or photo up to 20 MB")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file could not be read")
		return
	}
	var out IntakeResult
	err = rls.ForOffice(r.Context(), sub.OrganizationID, s.ID, func(ctx context.Context) error {
		var err error
		out, err = h.intake.Submit(ctx, sub, s.ID, IntakeInput{
			Source: SourceTaxpayerUpload, FileName: hdr.Filename, Data: data, Note: r.FormValue("note"), UserID: userID(r),
		})
		return err
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	it := out.Item
	status := http.StatusCreated
	if out.Replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, TaxpayerUploadResult{
		Item: TaxpayerItem{UUID: it.UUID, Vendor: it.Vendor, InvoiceDate: it.InvoiceDate, Status: TaxpayerStatus(it.Status),
			SourceKind: SourceTaxpayerUpload, CreatedAt: it.CreatedAt},
		Period: it.Period, Replayed: out.Replayed,
	})
}

// TaxpayerUploadResult answers the panel upload.
type TaxpayerUploadResult struct {
	Item     TaxpayerItem `json:"item"`
	Period   string       `json:"period"`
	Replayed bool         `json:"replayed"`
}

// TaxpayerFile: GET /v1/taxpayer/foreign-invoices/{foreignInvoiceUuid}/file
// (#780, mobile document viewer). The source file of one of the company's
// foreign invoices at its current office; it comes from the company's own
// attachments.
func (h *Handler) TaxpayerFile(w http.ResponseWriter, r *http.Request) {
	sub, s, ok := h.taxpayerSubject(w, r, false)
	if !ok {
		return
	}
	id, ok := invoiceID(w, r)
	if !ok {
		return
	}
	var f File
	err := rls.ForOffice(r.Context(), sub.OrganizationID, s.ID, func(ctx context.Context) error {
		var err error
		f, err = h.intake.SourceFile(ctx, sub, id)
		return err
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	writeFile(w, f)
}
