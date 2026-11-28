package taxpayerportal

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Submission sources.
const (
	SourceReceipt         = "receipt"
	SourceDocumentRequest = "document_request"
)

// Submission states as the mükellef sees them (#780 Senaryo B).
const (
	SubmissionReceived   = "received"   // ofise ulaştı, kararı bekliyor
	SubmissionDuplicate  = "duplicate"  // daha önce gönderilmiş; yeni kayıt açılmadı
	SubmissionUnreadable = "unreadable" // fotoğraf okunamadı; ofis bakacak
	SubmissionApproved   = "approved"
	SubmissionRejected   = "rejected"
	SubmissionCancelled  = "cancelled" // belge isteği ofisçe kapatıldı
)

// Submission is one thing the mükellef sent from the app: a receipt / Z
// report photo or a file for a document request.
type Submission struct {
	UUID   uuid.UUID `json:"uuid"`
	Source string    `json:"source"`
	// Kind is the receipt kind (expense_receipt, fuel_receipt, z_report);
	// empty for a document request.
	Kind string `json:"kind"`
	// Title is the document request's title; empty for a receipt.
	Title  string `json:"title"`
	Status string `json:"status"`
	// Reason is the office's own words: the note of a request asked again,
	// or why it was closed. Empty when the office gave none.
	Reason      string    `json:"reason"`
	MediaType   string    `json:"media_type"`
	VendorName  string    `json:"vendor_name"`
	TotalAmount string    `json:"total_amount"`
	OfficeName  string    `json:"office_name"`
	TaskUUID    *string   `json:"task_uuid"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SubmissionList is GET /v1/taxpayer/submissions.
type SubmissionList struct {
	// OfficeName is the company's current office; empty without one.
	OfficeName string       `json:"office_name"`
	Items      []Submission `json:"items"`
}

// receiptStatus maps a capture's status to what the mükellef sees.
func receiptStatus(status string) string {
	switch status {
	case "approved":
		return SubmissionApproved
	case "rejected":
		return SubmissionRejected
	case "duplicate":
		return SubmissionDuplicate
	case "failed":
		return SubmissionUnreadable
	}
	return SubmissionReceived
}

// taskUploadStatus maps a document request upload to what the mükellef
// sees. A request the office asked again (back to open after the upload)
// rejected that upload; its note says why.
func taskUploadStatus(r db.ListTaxpayerSubmittedTaskUploadsRow) (status, reason string) {
	switch r.TaskStatus {
	case "matched":
		return SubmissionApproved, ""
	case "cancelled":
		return SubmissionCancelled, r.CancelReason
	case "open":
		if r.TaskUpdatedAt.Valid && r.CreatedAt.Valid && r.TaskUpdatedAt.Time.After(r.CreatedAt.Time) {
			return SubmissionRejected, r.Note
		}
	}
	return SubmissionReceived, ""
}

func requestTitle(title, kind string) string {
	if title != "" {
		return title
	}
	switch kind {
	case "undocumented_payment":
		return "Ödeme için belge"
	case "uninvoiced_collection":
		return "Tahsilat için fatura"
	}
	return "Eksik belge"
}

// listSubmissions: GET /v1/taxpayer/submissions?limit= (#780 Senaryo B).
// Receipts and document request uploads of the company, newest first, with
// the office each one reached and its state there.
func (h *Handler) listSubmissions(w http.ResponseWriter, r *http.Request) {
	s := taxpayerctx.Must(r.Context())
	limit := int32(50)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounded
	}
	ctx := r.Context()
	office, err := h.q.GetTaxpayerOfficeName(ctx, s.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		response.InternalErr(w, r, err, "could not resolve office")
		return
	}
	receipts, err := h.q.ListTaxpayerSubmittedReceipts(ctx, db.ListTaxpayerSubmittedReceiptsParams{TaxpayerID: s.ID, Lim: limit})
	if err != nil {
		response.InternalErr(w, r, err, "could not list receipts")
		return
	}
	uploads, err := h.q.ListTaxpayerSubmittedTaskUploads(ctx, db.ListTaxpayerSubmittedTaskUploadsParams{TaxpayerID: s.ID, Lim: limit})
	if err != nil {
		response.InternalErr(w, r, err, "could not list uploads")
		return
	}
	out := SubmissionList{OfficeName: office, Items: make([]Submission, 0, len(receipts)+len(uploads))}
	for _, c := range receipts {
		out.Items = append(out.Items, Submission{
			UUID: c.Uuid, Source: SourceReceipt, Kind: c.Kind.String, Status: receiptStatus(c.Status),
			MediaType: c.MediaType, VendorName: c.VendorName, TotalAmount: c.TotalAmount, OfficeName: c.OfficeName,
			CreatedAt: c.CreatedAt.Time, UpdatedAt: c.UpdatedAt.Time,
		})
	}
	for _, u := range uploads {
		status, reason := taskUploadStatus(u)
		task := u.TaskUuid.String()
		updated := u.CreatedAt.Time
		if status != SubmissionReceived && u.TaskUpdatedAt.Valid {
			updated = u.TaskUpdatedAt.Time
		}
		out.Items = append(out.Items, Submission{
			UUID: u.Uuid, Source: SourceDocumentRequest, Title: requestTitle(u.Title, u.TaskKind), Status: status, Reason: reason,
			MediaType: u.ContentType, OfficeName: u.OfficeName, TaskUUID: &task, CreatedAt: u.CreatedAt.Time, UpdatedAt: updated,
		})
	}
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].CreatedAt.After(out.Items[j].CreatedAt) })
	if len(out.Items) > int(limit) {
		out.Items = out.Items[:limit]
	}
	response.JSON(w, r, http.StatusOK, out)
}
