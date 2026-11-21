package office

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
)

// Notifier enqueues an in-app notification (notifications usecase).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// WithNotifier tells the mali müşavir when a late purchase invoice is taken
// to a later month's KDV return (#669 A.1); n may be nil. frontendURL builds
// the link to the invoice.
func (h *Handler) WithNotifier(n Notifier, frontendURL string) *Handler {
	h.notifier, h.frontendURL = n, strings.TrimRight(frontendURL, "/")
	return h
}

// notifyLatePurchase: a purchase invoice's KDV was taken to a later month
// (geç gelen alış faturası, ofis kararı #669 A.1). No approval is needed,
// but the engagement's müşavir is told who did it, which invoice and to
// which month. Recipients are the engagement's assigned members (else the
// office owners), never the person who made the change. Fail-soft: the
// change stands even when nobody could be told.
func (h *Handler) notifyLatePurchase(ctx context.Context, s engagementctx.Scope, inv db.Invoice, actorID int64, to string) {
	if h.notifier == nil {
		return
	}
	log := slog.Default()
	users, err := h.q.ListEngagementAlertRecipients(ctx, db.ListEngagementAlertRecipientsParams{OrganizationID: s.OrganizationID, EngagementID: s.ID})
	if err != nil {
		log.Warn("late_purchase_notice_recipients_failed", "error", err)
		return
	}
	names, err := h.q.GetEngagementAlertNames(ctx, db.GetEngagementAlertNamesParams{OrganizationID: s.OrganizationID, EngagementID: s.ID})
	if err != nil {
		log.Warn("late_purchase_notice_names_failed", "error", err)
		return
	}
	actor := "Ofisten biri"
	if e, err := h.q.InvoiceVATDeductionEntry(ctx, db.InvoiceVATDeductionEntryParams{TaxpayerID: s.TaxpayerID, ID: inv.ID}); err == nil && e.SetByName != "" {
		actor = e.SetByName
	}
	number := strings.TrimSpace(inv.InvoiceNumber)
	if number == "" {
		number = "numarasız"
	}
	link := h.frontendURL + "/t/" + names.OrganizationSlug + "/engagements/" + names.EngagementUuid.String() + "/documents?belge=" + inv.Uuid.String()
	vars := map[string]string{
		"actor": actor, "taxpayer_name": names.TaxpayerTitle, "invoice_number": number,
		"document_month": monthName(inv.Period), "vat_month": monthName(to), "link": link,
	}
	body := actor + ", " + names.TaxpayerTitle + " için " + number + " numaralı alış faturasını (" + vars["document_month"] +
		") " + vars["vat_month"] + " KDV beyanına aldı; faturanın fişi o ayın ilk gününe tarihlenir."
	for _, uid := range users {
		if uid == actorID {
			continue
		}
		org, user := s.OrganizationID, uid
		if _, err := h.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			OrganizationID: &org, UserID: &user,
			Channels: []string{notifmodel.ChannelInapp}, Priority: notifmodel.PriorityNormal,
			Title: "Geç gelen alış faturası işlendi", Body: body, ActionURL: &link,
			TemplateCode: events.InvoicesLatePurchase, TemplateVars: vars, SourceEvent: events.InvoicesLatePurchase,
			Payload: map[string]any{
				"kind": "late_purchase_invoice", "engagement_uuid": names.EngagementUuid.String(),
				"organization_slug": names.OrganizationSlug, "invoice_uuid": inv.Uuid.String(),
				"invoice_number": number, "document_period": inv.Period, "vat_period": to,
			},
		}); err != nil {
			log.Warn("late_purchase_notice_failed", "error", err, "user_id", uid)
		}
	}
}

var months = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// monthName: 2026-11 → "Kasım 2026".
func monthName(p string) string {
	if len(p) == 7 && p[4] == '-' {
		if m, err := strconv.Atoi(p[5:]); err == nil && m >= 1 && m <= 12 {
			return months[m-1] + " " + p[:4]
		}
	}
	return p
}
