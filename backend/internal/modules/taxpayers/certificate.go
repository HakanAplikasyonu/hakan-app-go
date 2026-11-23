package taxpayers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/jwt"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ratelimit"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// SettingTaxCertificateAI is the office setting (organizations.settings)
// that turns reading the vergi levhası with AI off. Missing or true: on
// (when an AI provider is configured).
const SettingTaxCertificateAI = "tax_certificate_ai"

// TaxCertificateAIEnabled reads SettingTaxCertificateAI from an
// organization's settings object.
func TaxCertificateAIEnabled(settings []byte) bool {
	var s map[string]any
	if len(settings) == 0 || json.Unmarshal(settings, &s) != nil {
		return true
	}
	on, ok := s[SettingTaxCertificateAI].(bool)
	return !ok || on
}

// Error codes of POST /v1/tenant/tax-certificates/read.
const (
	CodeTaxCertificateOff = "TAX_CERTIFICATE_READING_OFF"
	CodeAIUnavailable     = "AI_UNAVAILABLE"
	CodeUnsupportedFile   = "UNSUPPORTED_FILE"
	CodeScannedPDF        = "SCANNED_PDF"
	CodeHEICUnsupported   = "HEIC_NOT_SUPPORTED"
	CodeUnreadable        = "TAX_CERTIFICATE_UNREADABLE"
)

// maxCertificateBytes: same limit as a receipt photo.
const maxCertificateBytes = 10 << 20

// limitCertificateReadsPerOrg bounds AI calls of one office (per window).
const limitCertificateReadsPerOrg = 60

// CertificateReader reads a vergi levhası; *extract.Extractor implements it
// (the receipts' AI pipeline: masking, image redaction, usage record).
type CertificateReader interface {
	ReadTaxCertificate(ctx context.Context, req extract.CertRequest) (extract.TaxCertificate, error)
}

// OrgSettings reads an office's settings. *db.Queries implements it.
type OrgSettings interface {
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
}

// CertificateHandler serves POST /v1/tenant/tax-certificates/read. It reads
// the file and answers the fields; nothing is stored and no taxpayer is
// looked up (D1b): the office saves the add-mükellef form itself.
type CertificateHandler struct {
	reader  CertificateReader
	orgs    OrgSettings
	limiter *ratelimit.Limiter
}

// NewCertificateHandler builds the handler. limiter may be nil.
func NewCertificateHandler(reader CertificateReader, orgs OrgSettings, limiter *ratelimit.Limiter) *CertificateHandler {
	return &CertificateHandler{reader: reader, orgs: orgs, limiter: limiter}
}

// RegisterCertificateRoutes mounts POST /v1/tenant/tax-certificates/read
// (web add-mükellef form and the office mobile app).
func RegisterCertificateRoutes(mux *http.ServeMux, tokens *jwt.Manager, loader middleware.IdentityLoader, orgResolver orgctx.Resolver, h *CertificateHandler) {
	if orgResolver == nil || h == nil {
		return
	}
	mux.Handle("POST /v1/tenant/tax-certificates/read", middleware.Chain(http.HandlerFunc(h.read),
		middleware.Authenticate(tokens, loader), middleware.RequireOrganization(orgResolver),
		middleware.RequireTenantPermission(rbac.PermTenantEngagementsWrite)))
}

// CertificateFile is a sniffed upload: an image the AI reads, or a PDF.
type CertificateFile struct {
	MediaType string
	PDF       bool
	HEIC      bool
}

// SniffCertificate tells what the uploaded file is; ok=false for anything
// but a JPEG, PNG, WebP, GIF, HEIC photo or a PDF.
func SniffCertificate(data []byte) (CertificateFile, bool) {
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1":
			return CertificateFile{MediaType: "image/heic", HEIC: true}, true
		}
	}
	switch ct := http.DetectContentType(data); ct {
	case "application/pdf":
		return CertificateFile{MediaType: ct, PDF: true}, true
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return CertificateFile{MediaType: ct}, true
	}
	return CertificateFile{}, false
}

func (h *CertificateHandler) read(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	p, _ := authctx.PrincipalFrom(r.Context())
	org, err := h.orgs.GetOrganizationByID(r.Context(), s.ID)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	if !TaxCertificateAIEnabled(org.Settings) {
		response.Error(w, r, http.StatusForbidden, CodeTaxCertificateOff, "the office turned reading the tax certificate off")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCertificateBytes+1<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "multipart form with a file is required (max 10 MB)")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxCertificateBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxCertificateBytes {
		response.BadRequest(w, r, response.CodeValidationError, "file must be 1 byte to 10 MB")
		return
	}
	kind, ok := SniffCertificate(data)
	if !ok {
		response.BadRequest(w, r, CodeUnsupportedFile, "upload a photo (JPEG, PNG, WebP) or a PDF")
		return
	}
	if kind.HEIC {
		// No AI channel reads HEIC and the repo has no converter.
		response.BadRequest(w, r, CodeHEICUnsupported, "HEIC photos cannot be read; upload JPEG or PNG")
		return
	}
	req := extract.CertRequest{OrganizationID: s.ID, UserID: p.UserInternal, MediaType: kind.MediaType, Image: data}
	if kind.PDF {
		text, err := extract.PDFText(data)
		if err != nil {
			response.RecordFailure(w, err)
			response.BadRequest(w, r, CodeScannedPDF, "the PDF has no text (a scan); upload a photo of the certificate")
			return
		}
		req.Image, req.MediaType, req.Text = nil, "", text
	}
	if !h.allow(w, r, strconv.FormatInt(s.ID, 10)) {
		return
	}
	out, err := h.reader.ReadTaxCertificate(r.Context(), req)
	if err != nil {
		response.RecordFailure(w, err)
		if errors.Is(err, extract.ErrAIUnavailable) || isAIUnavailable(err) {
			response.Error(w, r, http.StatusConflict, CodeAIUnavailable, "AI reading is not available")
			return
		}
		response.Error(w, r, http.StatusUnprocessableEntity, CodeUnreadable, "the certificate could not be read; fill the form by hand")
		return
	}
	response.JSON(w, r, http.StatusOK, certificateRead{TaxCertificate: out, CheckFields: nonNil(out.CheckFields())})
}

// certificateRead is the answer: the fields and the list to check.
type certificateRead struct {
	extract.TaxCertificate
	CheckFields []string `json:"check_fields"`
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (h *CertificateHandler) allow(w http.ResponseWriter, r *http.Request, org string) bool {
	return (&MemberHandler{limiter: h.limiter}).allow(w, r, "tax_certificate_read_org", org, limitCertificateReadsPerOrg)
}

// isAIUnavailable: no provider, or image redaction is required but has no
// OCR engine (AI_IMAGE_REDACTION): the office fills the form by hand.
func isAIUnavailable(err error) bool {
	return errors.Is(err, aiusecase.ErrDisabled) || errors.Is(err, aiusecase.ErrImageRedactionUnavailable)
}
