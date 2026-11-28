package whatsapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Rozet durumları (#780): uzantı WhatsApp Web'de kaydedilmiş medyanın
// üstünde gösterir.
const (
	BadgePending   = "pending"   // fiş okundu, taslak onay bekliyor
	BadgeApproved  = "approved"  // ofis onayladı
	BadgeRejected  = "rejected"  // ofis reddetti
	BadgeFailed    = "failed"    // fotoğraf okunamadı
	BadgeDuplicate = "duplicate" // aynı fiş daha önce kayıtlı
	BadgeSaved     = "saved"     // belge / yabancı fatura saklandı
)

// MaxStatusFiles bounds one status request.
const MaxStatusFiles = 50

// CaptureStatus is one saved file's badge (GET /v1/tenant/whatsapp/captures/status).
type CaptureStatus struct {
	Sha256         string    `json:"sha256"`
	UUID           uuid.UUID `json:"uuid"`
	Target         string    `json:"target"`
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"created_at"`
}

// BadgeState maps a capture's receipt status and its draft's approval to
// the badge. Documents and foreign invoices are only "saved".
func BadgeState(target, receiptStatus, approval string) string {
	if target != TargetExpense && target != TargetIncome {
		return BadgeSaved
	}
	switch {
	case receiptStatus == "failed":
		return BadgeFailed
	case receiptStatus == "duplicate":
		return BadgeDuplicate
	case receiptStatus == "rejected" || approval == "rejected":
		return BadgeRejected
	case receiptStatus == "approved" || approval == "approved":
		return BadgeApproved
	default:
		return BadgePending
	}
}

// ParseShaList reads "a,b,c" (hex SHA-256 of the files): lowercased,
// deduplicated, at most MaxStatusFiles.
func ParseShaList(raw string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		if len(p) != 64 || strings.Trim(p, "0123456789abcdef") != "" {
			return nil, fmt.Errorf("%w: sha256 must be 64 hex characters", ErrInvalid)
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 || len(out) > MaxStatusFiles {
		return nil, fmt.Errorf("%w: give 1 to %d sha256 values", ErrInvalid, MaxStatusFiles)
	}
	return out, nil
}

// CaptureStatuses answers which of these files the office saved from
// WhatsApp and where each one stands. A file saved to several targets
// appears once per target, newest first.
func (s *Service) CaptureStatuses(ctx context.Context, organizationID int64, shas []string) ([]CaptureStatus, error) {
	rows, err := s.q.ListWhatsAppCaptureStatuses(ctx, db.ListWhatsAppCaptureStatusesParams{OrganizationID: organizationID, Sha256s: shas})
	if err != nil {
		return nil, fmt.Errorf("whatsapp: capture statuses: %w", err)
	}
	out := make([]CaptureStatus, 0, len(rows))
	for _, r := range rows {
		out = append(out, CaptureStatus{
			Sha256: r.Sha256, UUID: r.Uuid, Target: r.Target, EngagementUUID: r.EngagementUuid,
			TaxpayerTitle: r.TaxpayerTitle, State: BadgeState(r.Target, r.ReceiptStatus, r.ApprovalStatus),
			CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}
