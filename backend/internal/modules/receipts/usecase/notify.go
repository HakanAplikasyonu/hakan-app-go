package usecase

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Notification kinds (payload "kind"; lock-screen push texts in
// notifications/pushtext).
const (
	NoticeReceiptSubmitted = "receipt_submitted" // to the office
	NoticeReceiptApproved  = "receipt_approved"  // to the mükellef
	NoticeReceiptRejected  = "receipt_rejected"  // to the mükellef
)

// Notifier enqueues an in-app notification (mirrored to mobile push).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// WithNotifier turns on the receipt notifications of #780 Senaryo B: the
// office hears when the mükellef sends a receipt from the app, and the
// mükellef hears the office's decision. n may be nil (no notifications).
func (s *Service) WithNotifier(n Notifier, frontendURL string) *Service {
	s.notifier, s.frontendURL = n, strings.TrimRight(frontendURL, "/")
	return s
}

// kindPhrase is the receipt kind in a sentence ("masraf fişi").
func kindPhrase(kind string) string {
	switch kind {
	case "fuel_receipt":
		return "akaryakıt fişi"
	case "z_report":
		return "Z raporu"
	case "expense_receipt":
		return "masraf fişi"
	}
	return "fiş"
}

// kindAccusative is the kind as an object ("masraf fişini").
func kindAccusative(kind string) string {
	switch kind {
	case "fuel_receipt":
		return "akaryakıt fişini"
	case "z_report":
		return "Z raporunu"
	case "expense_receipt":
		return "masraf fişini"
	}
	return "fişi"
}

// SubmittedNotice is the office's in-app text for a receipt the mükellef
// sent (#780 Senaryo B). Pure: tested without a database.
func SubmittedNotice(taxpayer, kind, status string) (title, body string) {
	title = "Mükelleften " + kindPhrase(kind) + " geldi"
	if status == StatusFailed {
		return title, taxpayer + " bir " + kindPhrase(kind) + " gönderdi ama fotoğraf okunamadı. Masraflar ekranındaki okunamayan fişlere bakın; gerekirse yenisini isteyin."
	}
	return title, taxpayer + " bir " + kindPhrase(kind) + " gönderdi; onayınızı bekliyor."
}

// DecisionNotice is the mükellef's in-app text for the office's decision.
func DecisionNotice(office, kind string, approve bool) (title, body string) {
	if office == "" {
		office = "Müşaviriniz"
	}
	if approve {
		return "Fişiniz onaylandı", office + ", gönderdiğiniz " + kindAccusative(kind) + " onayladı."
	}
	return "Fişiniz kabul edilmedi", office + ", gönderdiğiniz " + kindAccusative(kind) +
		" kabul etmedi. Ayrıntıyı Gönderdiklerim ekranında görebilir, müşavirinize Mesajlarım'dan yazabilirsiniz."
}

// NotifyTaxpayerUpload tells the engagement's assigned members (else the
// office owners) that the mükellef sent a receipt from the app. A replay or
// a duplicate is not announced. Best effort: logged, never returned.
func (s *Service) NotifyTaxpayerUpload(ctx context.Context, sc engagementctx.Scope, userID int64, v View) {
	if s.notifier == nil || (v.Status != StatusDraft && v.Status != StatusFailed) {
		return
	}
	// The upload runs in the mükellef's scope, where office membership is
	// not visible: who to tell is read on purpose outside it, on a separate
	// connection (like the document request upload, issue #241).
	var (
		users  []int64
		names  db.GetEngagementAlertNamesRow
		period string
	)
	err := rls.System(detached(ctx), s.pool, "receipts: tell the office about a mükellef receipt", func(sctx context.Context, tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var err error
		if users, err = q.ListEngagementAlertRecipients(sctx, db.ListEngagementAlertRecipientsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID}); err != nil || len(users) == 0 {
			return err
		}
		if names, err = q.GetEngagementAlertNames(sctx, db.GetEngagementAlertNamesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID}); err != nil {
			return err
		}
		if v.Draft != nil && v.Draft.Type == "expense" {
			if e, err := q.GetExpense(sctx, db.GetExpenseParams{TaxpayerID: sc.TaxpayerID, ID: v.Draft.ID}); err == nil && e.Period.Valid {
				period = e.Period.String
			}
		}
		return nil
	})
	if err != nil {
		s.log.Warn("receipt_submitted_recipients_failed", "error", err, "engagement_id", sc.ID)
		return
	}
	if len(users) == 0 {
		return
	}
	kind := ""
	if v.Kind != nil {
		kind = *v.Kind
	}
	title, body := SubmittedNotice(names.TaxpayerTitle, kind, v.Status)
	// Draft: the office's inbox (Gelen kutusu) of this taxpayer, where it is
	// approved; unreadable: the expenses page with the unreadable photos.
	base := s.frontendURL + "/t/" + names.OrganizationSlug + "/engagements/" + names.EngagementUuid.String()
	link := base + "/review?view=inbox"
	if period != "" {
		link += "&period=" + period
	}
	if v.Status == StatusFailed {
		link = base + "/expenses"
	}
	err = rls.ForOffice(ctx, sc.OrganizationID, sc.TaxpayerID, func(ctx context.Context) error {
		for _, uid := range users {
			if uid == userID {
				continue
			}
			org, user := sc.OrganizationID, uid
			if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
				OrganizationID: &org, UserID: &user,
				Channels: []string{notifmodel.ChannelInapp}, Priority: notifmodel.PriorityNormal,
				Title: title, Body: body, ActionURL: &link,
				Payload: map[string]any{
					"kind": NoticeReceiptSubmitted, "receipt_uuid": v.UUID.String(), "status": v.Status,
					"engagement_uuid": names.EngagementUuid.String(), "organization_slug": names.OrganizationSlug,
				},
			}); err != nil {
				s.log.Warn("receipt_submitted_notify_failed", "error", err, "user_id", uid)
			}
		}
		return nil
	})
	if err != nil {
		s.log.Warn("receipt_submitted_notify_failed", "error", err, "engagement_id", sc.ID)
	}
}

// notifyDecision tells the mükellef user who sent the receipt (still a
// member of the company) that the office approved or rejected it. Runs in
// the office's scope after the decision is committed; best effort.
func (s *Service) notifyDecision(ctx context.Context, sc engagementctx.Scope, by db.GetCaptureDecisionNoticeParams, approve bool) {
	if s.notifier == nil {
		return
	}
	by.OrganizationID, by.EngagementID = sc.OrganizationID, sc.ID
	n, err := s.q.GetCaptureDecisionNotice(ctx, by)
	if err != nil {
		if err != pgx.ErrNoRows {
			s.log.Warn("receipt_decision_notice_failed", "error", err, "engagement_id", sc.ID)
		}
		return
	}
	title, body := DecisionNotice(n.OfficeName, n.Kind, approve)
	kind := NoticeReceiptRejected
	if approve {
		kind = NoticeReceiptApproved
	}
	// The mükellef app maps /m/{taxpayer}/gonderdiklerim to Gönderdiklerim.
	link := "/m/" + n.TaxpayerUuid.String() + "/gonderdiklerim"
	org, user := sc.OrganizationID, n.UserID
	if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		OrganizationID: &org, UserID: &user,
		Channels: []string{notifmodel.ChannelInapp}, Priority: notifmodel.PriorityNormal,
		Title: title, Body: body, ActionURL: &link,
		Payload: map[string]any{"kind": kind, "receipt_uuid": n.Uuid.String(), "taxpayer_uuid": n.TaxpayerUuid.String()},
	}); err != nil {
		s.log.Warn("receipt_decision_notify_failed", "error", err, "user_id", n.UserID)
	}
}

func byCapture(id int64) db.GetCaptureDecisionNoticeParams {
	return db.GetCaptureDecisionNoticeParams{CaptureID: pgtype.Int8{Int64: id, Valid: true}}
}

func byExpense(id int64) db.GetCaptureDecisionNoticeParams {
	return db.GetCaptureDecisionNoticeParams{ExpenseID: pgtype.Int8{Int64: id, Valid: true}}
}

func byRetailSale(id int64) db.GetCaptureDecisionNoticeParams {
	return db.GetCaptureDecisionNoticeParams{RetailSaleID: pgtype.Int8{Int64: id, Valid: true}}
}

// detached keeps the request's values (logging, tracing) but drops its
// database transaction, so rls.System opens its own connection.
func detached(ctx context.Context) context.Context { return noTx{ctx} }

type noTx struct{ context.Context }

func (c noTx) Value(key any) any {
	if v := c.Context.Value(key); v != nil {
		if _, isTx := v.(pgx.Tx); isTx {
			return nil
		}
		return v
	}
	return nil
}
