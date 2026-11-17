package easytrade

// Webhook receiver (issue #385). Easytrade has no outgoing webhooks yet;
// this is the contract the portal proposes (docs/easytrade-senkron.md §2,
// "Easytrade'den istenecekler"). Until Easytrade pushes, the scheduled
// sync (polling) is the only trigger; with a push the next sync starts
// within seconds. The push carries no data: it only says "company X
// changed", the sync then reads through the normal API, so a forged or
// replayed push can at most cause an extra (idempotent, deduplicated) read.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// Webhook headers and limits.
const (
	WebhookPath            = "/v1/webhooks/easytrade"
	HeaderWebhookTimestamp = "X-Easytrade-Timestamp" // Unix seconds
	HeaderWebhookSignature = "X-Easytrade-Signature" // "sha256=" + hex(HMAC-SHA256(secret, timestamp + "." + body))
	// WebhookTolerance bounds the clock skew and the replay window.
	WebhookTolerance = 5 * time.Minute
	webhookMaxBody   = 64 << 10
)

// WebhookEvent is the push body. Event and resource names are Easytrade's
// own (invoice, expense, retail_sale, financial_transaction …); the portal
// only needs company_id, the rest is logged for diagnosis.
type WebhookEvent struct {
	EventID    string `json:"event_id"`
	Event      string `json:"event"`
	CompanyID  string `json:"company_id"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	OccurredAt string `json:"occurred_at"`
}

// Sign returns the signature header value of body at ts (Easytrade side;
// also used by tests and the docs example).
func Sign(secret []byte, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Webhook serves POST /v1/webhooks/easytrade.
type Webhook struct {
	svc     *Service
	secret  []byte
	enqueue func(ctx context.Context, taxpayerID int64, id uuid.UUID) error
	now     func() time.Time
}

// NewWebhook builds the receiver. An empty secret turns it off (404).
// enqueue may be nil: then only next_sync_at moves and the next sweep
// (at most 5 minutes) picks the connection.
func NewWebhook(svc *Service, secret string, enqueue func(ctx context.Context, taxpayerID int64, id uuid.UUID) error) *Webhook {
	return &Webhook{svc: svc, secret: []byte(secret), enqueue: enqueue, now: time.Now}
}

// RegisterWebhook mounts the public receiver (no bearer token: the
// signature authenticates Easytrade).
func RegisterWebhook(mux *http.ServeMux, wh *Webhook) {
	if wh == nil {
		return
	}
	mux.Handle("POST "+WebhookPath, wh)
}

var (
	errStale     = errors.New("easytrade webhook: timestamp outside the tolerance")
	errSignature = errors.New("easytrade webhook: bad signature")
)

// verify checks the timestamp window and the signature.
func (wh *Webhook) verify(tsHeader, sigHeader string, body []byte) error {
	ts, err := strconv.ParseInt(strings.TrimSpace(tsHeader), 10, 64)
	if err != nil {
		return errStale
	}
	if d := wh.now().Sub(time.Unix(ts, 0)); d > WebhookTolerance || d < -WebhookTolerance {
		return errStale
	}
	want := Sign(wh.secret, ts, body)
	if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(sigHeader))) {
		return errSignature
	}
	return nil
}

func (wh *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(wh.secret) == 0 {
		response.NotFound(w, r, "not found")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookMaxBody))
	if err != nil {
		response.BadRequest(w, r, "INVALID_BODY", "body too large or unreadable")
		return
	}
	if err := wh.verify(r.Header.Get(HeaderWebhookTimestamp), r.Header.Get(HeaderWebhookSignature), body); err != nil {
		code := "WEBHOOK_SIGNATURE_INVALID"
		if errors.Is(err, errStale) {
			code = "WEBHOOK_TIMESTAMP_INVALID"
		}
		response.Error(w, r, http.StatusUnauthorized, code, "the webhook signature or timestamp is invalid")
		return
	}
	var ev WebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil || strings.TrimSpace(ev.CompanyID) == "" {
		response.BadRequest(w, r, "INVALID_BODY", "company_id is required")
		return
	}
	n, err := wh.notify(r.Context(), strings.TrimSpace(ev.CompanyID))
	if err != nil {
		response.InternalErr(w, r, err, "webhook could not be processed")
		return
	}
	// The count is not a lookup oracle: an unknown company is 0, as is a
	// known one without an active connection.
	response.JSON(w, r, http.StatusAccepted, map[string]any{"accepted": n})
}

// notify pulls every active connection of the company forward and
// enqueues its sync.
func (wh *Webhook) notify(ctx context.Context, companyID string) (int, error) {
	type target struct {
		tp int64
		id uuid.UUID
	}
	var ts []target
	now := pgtype.Timestamptz{Time: wh.now(), Valid: true}
	// The push names a company, not a taxpayer: a deliberate, narrow bypass.
	err := rls.System(ctx, wh.svc.pool, "easytrade: webhook names a company, not a taxpayer", func(ctx context.Context, _ pgx.Tx) error {
		rows, err := wh.svc.q.ListSourceConnectionsByExternalCompany(ctx, db.ListSourceConnectionsByExternalCompanyParams{
			SourceSystem: SourceSystem, ExternalCompanyID: pgtype.Text{String: companyID, Valid: true},
		})
		if err != nil {
			return err
		}
		for _, r := range rows {
			if _, err := wh.svc.q.PullSourceConnectionNow(ctx, db.PullSourceConnectionNowParams{Now: now, TaxpayerID: r.TaxpayerID, Uuid: r.Uuid}); err != nil {
				return err
			}
			ts = append(ts, target{r.TaxpayerID, r.Uuid})
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if wh.enqueue != nil {
		for _, t := range ts {
			// A failed enqueue is not the sender's problem: next_sync_at
			// already moved, the next sweep picks the connection.
			_ = wh.enqueue(ctx, t.tp, t.id)
		}
	}
	return len(ts), nil
}
