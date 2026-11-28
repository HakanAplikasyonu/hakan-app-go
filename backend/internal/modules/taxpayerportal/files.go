package taxpayerportal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/office"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/view"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Downloader reads a stored object (storage.Driver satisfies it).
type Downloader interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// WithStore enables the receipt photo route (#780, mobile document viewer).
// Without it the route answers 500.
func (h *Handler) WithStore(d Downloader) *Handler {
	h.store = d
	return h
}

// invoicePDF: GET /v1/taxpayer/invoices/{uuid}/pdf (#780). The same PDF the
// office downloads (drawn from the stored record), for the company's own
// invoice only.
func (h *Handler) invoicePDF(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "invoice not found")
		return
	}
	ctx := r.Context()
	inv, err := h.q.GetInvoice(ctx, db.GetInvoiceParams{TaxpayerID: s.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "invoice not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice")
		return
	}
	d, err := view.LoadDetail(ctx, h.q, inv)
	if err != nil {
		response.InternalErr(w, r, err, "could not load invoice details")
		return
	}
	out, err := office.InvoicePDF(d)
	if err != nil {
		response.InternalErr(w, r, err, "could not render invoice PDF")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, office.PDFFilename(d.InvoiceNumber)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	_, _ = w.Write(out)
}

// receiptImages are the photo types served; a PDF capture is 404 like the
// office preview (GET .../receipts/{receiptUuid}/image).
var receiptImages = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/heic": true}

// receiptImage: GET /v1/taxpayer/receipts/{uuid}/image (#780). The photo of
// one of the company's own receipts (any office it was sent to).
func (h *Handler) receiptImage(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "receipt not found")
		return
	}
	ctx := r.Context()
	row, err := h.q.GetTaxpayerReceiptImage(ctx, db.GetTaxpayerReceiptImageParams{TaxpayerID: s.ID, ReceiptUuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(w, r, "receipt not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not load the receipt")
		return
	}
	ct := strings.ToLower(row.ContentType.String)
	if !receiptImages[ct] {
		response.NotFound(w, r, "receipt has no image")
		return
	}
	if h.store == nil {
		response.InternalErr(w, r, errors.New("taxpayerportal: storage is not configured"), "could not load the image")
		return
	}
	rc, size, err := h.store.Download(ctx, row.StorageKey)
	if err != nil {
		response.InternalErr(w, r, err, "could not load the image")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	_, _ = io.Copy(w, rc)
}
