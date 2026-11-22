package communication

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Status codes of a taxpayer message (the screen translates them).
const (
	MessagePending   = "pending"
	MessageSent      = "sent"
	MessageDelivered = "delivered"
	MessageFailed    = "failed"
)

// TaxpayerMessage is one SMS or e-mail the office sent to the company, as
// the company sees it (#780 "Mesajlarım"). Office-internal fields (who sent
// it, provider job number and cost, error code, source rule, open tracking)
// are left out; the address is masked.
type TaxpayerMessage struct {
	UUID    uuid.UUID `json:"uuid"`
	Channel string    `json:"channel"`
	// Subject is the e-mail subject ("" for SMS).
	Subject string `json:"subject"`
	Body    string `json:"body"`
	// To is the masked phone number or e-mail address.
	To          string     `json:"to"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	SentAt      *time.Time `json:"sent_at"`
	DeliveredAt *time.Time `json:"delivered_at"`
}

// TaxpayerMessages pages the messages of engagement (the company's current
// office) newest first; dry run messages are not listed.
func (s *Service) TaxpayerMessages(ctx context.Context, org, engagementID int64, limit, offset int32) ([]TaxpayerMessage, int64, error) {
	eng := pgtype.Int8{Int64: engagementID, Valid: true}
	total, err := s.q.CountTaxpayerMessages(ctx, db.CountTaxpayerMessagesParams{OrganizationID: org, EngagementID: eng})
	if err != nil {
		return nil, 0, fmt.Errorf("count taxpayer messages: %w", err)
	}
	rows, err := s.q.ListTaxpayerMessages(ctx, db.ListTaxpayerMessagesParams{OrganizationID: org, EngagementID: eng, Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("list taxpayer messages: %w", err)
	}
	out := make([]TaxpayerMessage, 0, len(rows))
	for _, r := range rows {
		to := MaskPhone(r.Address)
		if r.Channel == ChannelEmail {
			to = MaskEmail(r.Address)
		}
		out = append(out, TaxpayerMessage{UUID: r.Uuid, Channel: r.Channel, Subject: r.Subject, Body: r.Body, To: to,
			Status: r.Status, CreatedAt: r.CreatedAt.Time, SentAt: tsPtr(r.SentAt), DeliveredAt: tsPtr(r.DeliveredAt)})
	}
	return out, total, nil
}

// MaskPhone keeps the last two digits: "5321234567" -> "******** 67".
func MaskPhone(p string) string {
	var d []rune
	for _, c := range p {
		if c >= '0' && c <= '9' {
			d = append(d, c)
		}
	}
	if len(d) <= 2 {
		return strings.Repeat("*", len(d))
	}
	return strings.Repeat("*", len(d)-2) + " " + string(d[len(d)-2:])
}

// MaskEmail keeps the first letter and the domain: "muhasebe@x.com" ->
// "m***@x.com".
func MaskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at <= 0 {
		return "***"
	}
	r := []rune(e[:at])
	return string(r[0]) + "***" + e[at:]
}

// ActiveEngagement resolves the company's current office.
type ActiveEngagement interface {
	GetActiveEngagementForTaxpayer(ctx context.Context, taxpayerID int64) (db.Engagement, error)
}

// WithPortal enables GET /v1/taxpayer/messages (it needs the company's
// current office).
func (h *Handler) WithPortal(p ActiveEngagement) *Handler {
	h.portal = p
	return h
}

// TaxpayerMessages: GET /v1/taxpayer/messages?limit=&offset=. The SMS and
// e-mails the company's current office sent it; a company without an
// office gets an empty list.
func (h *Handler) TaxpayerMessages(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil || h.portal == nil {
		response.NotFound(w, r, "")
		return
	}
	q := r.URL.Query()
	limit, offset := apiquery.DefaultLimit, int32(0)
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= int(apiquery.MaxLimit) {
		limit = int32(v) //nolint:gosec // bounded above
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v >= 0 && v <= 1_000_000 {
		offset = int32(v) //nolint:gosec // bounded above
	}
	s := taxpayerctx.Must(r.Context())
	eng, err := h.portal.GetActiveEngagementForTaxpayer(r.Context(), s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		response.JSON(w, r, http.StatusOK, apiquery.NewPage([]TaxpayerMessage{}, 0, limit, offset))
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "could not resolve office")
		return
	}
	var items []TaxpayerMessage
	var total int64
	err = rls.ForOffice(r.Context(), eng.OrganizationID, s.ID, func(ctx context.Context) error {
		var err error
		items, total, err = h.svc.TaxpayerMessages(ctx, eng.OrganizationID, eng.ID, limit, offset)
		return err
	})
	if err != nil {
		response.InternalErr(w, r, err, "could not list messages")
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}
