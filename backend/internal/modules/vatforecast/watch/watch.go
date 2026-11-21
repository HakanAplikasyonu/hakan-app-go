// Package watch is the daily VAT watch ("KDV nöbetçisi", #780 B).
//
// Every day each active engagement's in-period VAT is computed. When the
// taxpayer was told "ödeme çıkmayacak" (or "çıkacak") and a new invoice
// flips that, the office gets "KDV durumu değişti" with a suggestion to send
// the taxpayer new information. When the estimated payable reaches the
// office's threshold, "KDV eşiği aşıldı". Each event is written once
// (vat_watch_events.dedupe_key) and also enqueued to the outbox, so the
// communication centre's automation rules (#780 A) can trigger on it.
package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/notice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// Queries is the db subset used here.
type Queries interface {
	ListVATWatchTargets(ctx context.Context) ([]db.ListVATWatchTargetsRow, error)
	LatestVATNotice(ctx context.Context, arg db.LatestVATNoticeParams) (db.LatestVATNoticeRow, error)
	InsertVATWatchEvent(ctx context.Context, arg db.InsertVATWatchEventParams) (db.InsertVATWatchEventRow, error)
	ListEngagementAlertRecipients(ctx context.Context, arg db.ListEngagementAlertRecipientsParams) ([]int64, error)
	ListVATWatchEvents(ctx context.Context, arg db.ListVATWatchEventsParams) ([]db.ListVATWatchEventsRow, error)
	ListOfficeVATWatchEvents(ctx context.Context, arg db.ListOfficeVATWatchEventsParams) ([]db.ListOfficeVATWatchEventsRow, error)
	GetDocumentTaskNotifyNames(ctx context.Context, arg db.GetDocumentTaskNotifyNamesParams) (db.GetDocumentTaskNotifyNamesRow, error)
}

// VAT is the vatforecast usecase subset used here.
type VAT interface {
	CurrentPeriod() string
	DefaultEvaluationPeriods() []string
	Evaluate(ctx context.Context, sub usecase.Subject, periods []string) ([]usecase.Evaluation, error)
	Forecast(ctx context.Context, sub usecase.Subject, period string) (usecase.Forecast, error)
	Settings(ctx context.Context, sub usecase.Subject) (usecase.Settings, error)
	Tracking(ctx context.Context, orgID int64) (usecase.TrackingSettings, error)
}

// ErrDisabled: the office switched the watch off (KDV takibi ayarı).
var ErrDisabled = errors.New("vat watch is switched off for this office")

// Event is one raised event in the API.
type Event struct {
	UUID      uuid.UUID      `json:"uuid"`
	Period    string         `json:"period"`
	Event     string         `json:"event"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

// Outbox enqueues an event in the ambient transaction (outbox.Store).
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// Notifier enqueues notifications (notifications usecase).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// TxFunc runs fn in one system (cross-office) transaction; queries made
// through ctx join it. Each engagement gets its own, so one failure does
// not stop the sweep.
type TxFunc func(ctx context.Context, fn func(ctx context.Context) error) error

// Watcher runs the sweep.
type Watcher struct {
	Q           Queries
	VAT         VAT
	Config      engine.ConfigResolver
	Outbox      Outbox
	Notifier    Notifier
	Tx          TxFunc
	FrontendURL string
	Log         *slog.Logger
}

// LastNotice is what the taxpayer was last told for the period.
type LastNotice struct {
	UUID       uuid.UUID
	Outcome    string
	Payable    decimal.Decimal
	NotifiedAt time.Time
}

// Finding is an event the watch raises.
type Finding struct {
	Event  string
	Dedupe string
	Extra  map[string]any
}

// Detect decides the events for a forecast. limit is the office's payable
// threshold (nil: off); last is the latest notice of the period (nil: the
// taxpayer was not told anything, so nothing can "change").
func Detect(engagement uuid.UUID, f usecase.Forecast, limit *decimal.Decimal, last *LastNotice) []Finding {
	var out []Finding
	payable := f.Totals.Payable.Dec()
	if limit != nil && payable.IsPositive() && payable.GreaterThanOrEqual(*limit) {
		out = append(out, Finding{
			Event:  events.VATThresholdExceeded,
			Dedupe: fmt.Sprintf("%s:%s:%s", events.VATThresholdExceeded, engagement, f.Period),
			Extra:  map[string]any{"limit": limit.StringFixed(2)},
		})
	}
	if last != nil && notice.Outcome(f.Totals) != last.Outcome {
		out = append(out, Finding{
			Event:  events.VATStatusChanged,
			Dedupe: fmt.Sprintf("%s:%s:%s:%s", events.VATStatusChanged, engagement, f.Period, last.UUID),
			Extra: map[string]any{
				"previous_outcome": last.Outcome,
				"previous_payable": last.Payable.StringFixed(2),
				"notice_uuid":      last.UUID.String(),
				"notified_at":      last.NotifiedAt.UTC().Format(time.RFC3339),
			},
		})
	}
	return out
}

// Payload is the common event payload (#780 comment: KDV olayları).
func Payload(t db.ListVATWatchTargetsRow, f usecase.Forecast, dedupe string) map[string]any {
	p := map[string]any{
		"taxpayer_uuid":   t.TaxpayerUuid.String(),
		"taxpayer_title":  t.TaxpayerTitle,
		"engagement_uuid": t.EngagementUuid.String(),
		"period":          f.Period,
		"outcome":         notice.Outcome(f.Totals),
		"payable":         f.Totals.Payable.Dec().StringFixed(2),
		"carry_out":       f.Totals.CarryOut.Dec().StringFixed(2),
		"output_vat":      f.Totals.OutputVAT.Dec().StringFixed(2),
		"input_vat":       f.Totals.InputVAT.Dec().StringFixed(2),
		"carried_in":      f.Totals.CarriedIn.Dec().StringFixed(2),
		"due_date":        f.Calendar.DueDate,
		"dedupe_key":      dedupe,
	}
	return p
}

// Run sweeps every active engagement of offices that keep the watch on and
// returns how many events it raised.
func (w *Watcher) Run(ctx context.Context) (int, error) {
	var targets []db.ListVATWatchTargetsRow
	if err := w.Tx(ctx, func(ctx context.Context) error {
		var err error
		targets, err = w.Q.ListVATWatchTargets(ctx)
		return err
	}); err != nil {
		return 0, fmt.Errorf("vat watch targets: %w", err)
	}
	total := 0
	for _, t := range targets {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		var n int
		err := w.Tx(ctx, func(ctx context.Context) error {
			var err error
			n, err = w.Check(ctx, t)
			return err
		})
		if err != nil {
			w.log().Error("vat_watch_engagement_failed", "organization_id", t.OrganizationID, "engagement_id", t.EngagementID, "error", err)
			continue
		}
		total += n
	}
	return total, nil
}

// Check evaluates one engagement and raises its new events.
func (w *Watcher) Check(ctx context.Context, t db.ListVATWatchTargetsRow) (int, error) {
	sub := usecase.Subject{
		OrganizationID: t.OrganizationID, EngagementID: t.EngagementID, EngagementUUID: t.EngagementUuid,
		TaxpayerID: t.TaxpayerID, TaxpayerUUID: t.TaxpayerUuid,
	}
	// Keep the warning centre current (VAT threshold / deadline warnings).
	if _, err := w.VAT.Evaluate(ctx, sub, w.VAT.DefaultEvaluationPeriods()); err != nil {
		return 0, err
	}
	period := w.VAT.CurrentPeriod()
	f, err := w.VAT.Forecast(ctx, sub, period)
	if err != nil {
		return 0, err
	}
	limit, err := w.limit(ctx, sub)
	if err != nil {
		return 0, err
	}
	last, err := w.lastNotice(ctx, t, period)
	if err != nil {
		return 0, err
	}
	raised := 0
	for _, fd := range Detect(t.EngagementUuid, f, limit, last) {
		ok, err := w.raise(ctx, t, f, fd)
		if err != nil {
			return raised, err
		}
		if ok {
			raised++
		}
	}
	return raised, nil
}

// CheckNow runs the watch for one engagement on request ("Şimdi kontrol
// et"); the office's switch applies. Returns how many new events it raised.
func (w *Watcher) CheckNow(ctx context.Context, sub usecase.Subject) (int, error) {
	st, err := w.VAT.Tracking(ctx, sub.OrganizationID)
	if err != nil {
		return 0, err
	}
	if !st.WatchEnabled {
		return 0, ErrDisabled
	}
	names, err := w.Q.GetDocumentTaskNotifyNames(ctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: sub.OrganizationID, TaxpayerID: sub.TaxpayerID})
	if err != nil {
		return 0, fmt.Errorf("vat watch names: %w", err)
	}
	return w.Check(ctx, db.ListVATWatchTargetsRow{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, EngagementUuid: sub.EngagementUUID,
		TaxpayerID: sub.TaxpayerID, TaxpayerUuid: sub.TaxpayerUUID, TaxpayerTitle: names.TaxpayerTitle,
		OrganizationSlug: names.OrganizationSlug, WatchEmail: st.WatchEmail,
	})
}

// OfficeEvent is one row of the office-wide "Son KDV uyarıları" list.
type OfficeEvent struct {
	Event
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title"`
}

// OfficeFilter narrows ListOffice: without AllEngagements only UserID's
// assigned taxpayers.
type OfficeFilter struct {
	OrganizationID int64
	AllEngagements bool
	UserID         int64
	Limit          int32
}

// ListOffice returns the office's latest watch events, newest first
// (at most 50).
func (w *Watcher) ListOffice(ctx context.Context, f OfficeFilter) ([]OfficeEvent, error) {
	if f.Limit <= 0 || f.Limit > 50 {
		f.Limit = 20
	}
	rows, err := w.Q.ListOfficeVATWatchEvents(ctx, db.ListOfficeVATWatchEventsParams{
		OrganizationID: f.OrganizationID, AllEngagements: f.AllEngagements, UserID: f.UserID, LimitCount: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list office vat watch events: %w", err)
	}
	out := make([]OfficeEvent, 0, len(rows))
	for _, r := range rows {
		e := OfficeEvent{
			Event:          Event{UUID: r.Uuid, Period: r.Period, Event: r.Event, Payload: map[string]any{}, CreatedAt: r.CreatedAt.Time},
			EngagementUUID: r.EngagementUuid, TaxpayerUUID: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle,
		}
		if len(r.Payload) > 0 {
			if err := json.Unmarshal(r.Payload, &e.Payload); err != nil {
				return nil, fmt.Errorf("vat watch payload: %w", err)
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// Channels is how the office hears about a finding: in-app always, e-mail
// too when the office chose "sorumluya e-posta da gönder".
func Channels(email bool) []string {
	if email {
		return []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail}
	}
	return []string{notifmodel.ChannelInapp}
}

// List returns the engagement's raised events, newest first.
func (w *Watcher) List(ctx context.Context, sub usecase.Subject, limit int32) ([]Event, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := w.Q.ListVATWatchEvents(ctx, db.ListVATWatchEventsParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, LimitCount: limit})
	if err != nil {
		return nil, fmt.Errorf("list vat watch events: %w", err)
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		e := Event{UUID: r.Uuid, Period: r.Period, Event: r.Event, Payload: map[string]any{}, CreatedAt: r.CreatedAt.Time}
		if len(r.Payload) > 0 {
			if err := json.Unmarshal(r.Payload, &e.Payload); err != nil {
				return nil, fmt.Errorf("vat watch payload: %w", err)
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func (w *Watcher) limit(ctx context.Context, sub usecase.Subject) (*decimal.Decimal, error) {
	s, err := w.VAT.Settings(ctx, sub)
	if err != nil {
		return nil, err
	}
	if s.PayableLimit == nil || w.Config == nil {
		return s.PayableLimit, nil
	}
	cfg, err := w.Config.Resolve(ctx, engine.Subject{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID},
		[]string{usecase.CodePayableThreshold})
	if err != nil {
		return nil, fmt.Errorf("resolve vat threshold: %w", err)
	}
	if eff, ok := cfg[usecase.CodePayableThreshold]; ok && !eff.Enabled {
		return nil, nil
	}
	return s.PayableLimit, nil
}

func (w *Watcher) lastNotice(ctx context.Context, t db.ListVATWatchTargetsRow, period string) (*LastNotice, error) {
	row, err := w.Q.LatestVATNotice(ctx, db.LatestVATNoticeParams{OrganizationID: t.OrganizationID, EngagementID: t.EngagementID, Period: period})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest vat notice: %w", err)
	}
	return &LastNotice{UUID: row.Uuid, Outcome: row.Outcome, Payable: dec(row.Payable), NotifiedAt: row.CreatedAt.Time}, nil
}

// raise writes the event once, enqueues it to the outbox and tells the
// office in-app. false: it was raised before.
func (w *Watcher) raise(ctx context.Context, t db.ListVATWatchTargetsRow, f usecase.Forecast, fd Finding) (bool, error) {
	payload := Payload(t, f, fd.Dedupe)
	for k, v := range fd.Extra {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	row, err := w.Q.InsertVATWatchEvent(ctx, db.InsertVATWatchEventParams{
		OrganizationID: t.OrganizationID, EngagementID: t.EngagementID, Period: f.Period,
		Event: fd.Event, DedupeKey: fd.Dedupe, Payload: raw,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert vat watch event: %w", err)
	}
	if w.Outbox != nil {
		eng := t.EngagementUuid
		ev := events.New(fd.Event).WithOrganization(t.OrganizationID).
			WithEntity("engagement", &t.EngagementID, &eng).WithPayload(payload)
		ev.EventID = row.Uuid
		if err := w.Outbox.Enqueue(ctx, nil, ev); err != nil {
			return false, fmt.Errorf("enqueue %s: %w", fd.Event, err)
		}
	}
	if err := w.notifyOffice(ctx, t, f, fd, payload); err != nil {
		return false, err
	}
	return true, nil
}

func (w *Watcher) notifyOffice(ctx context.Context, t db.ListVATWatchTargetsRow, f usecase.Forecast, fd Finding, payload map[string]any) error {
	if w.Notifier == nil {
		return nil
	}
	users, err := w.Q.ListEngagementAlertRecipients(ctx, db.ListEngagementAlertRecipientsParams{OrganizationID: t.OrganizationID, EngagementID: t.EngagementID})
	if err != nil {
		return fmt.Errorf("vat watch recipients: %w", err)
	}
	title, body := Text(t.TaxpayerTitle, f, fd)
	link := fmt.Sprintf("%s/t/%s/engagements/%s/report", strings.TrimRight(w.FrontendURL, "/"), t.OrganizationSlug, t.EngagementUuid)
	org := t.OrganizationID
	for _, u := range users {
		user := u
		if _, err := w.Notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			OrganizationID: &org, UserID: &user, Channels: Channels(t.WatchEmail),
			Title: title, Body: body, ActionURL: &link, SourceEvent: fd.Event, Payload: payload,
		}); err != nil {
			return fmt.Errorf("notify vat watch: %w", err)
		}
	}
	return nil
}

// Text is the office notification for a finding.
func Text(taxpayer string, f usecase.Forecast, fd Finding) (string, string) {
	month := notice.MonthName(f.Period)
	payable := notice.TL(f.Totals.Payable)
	switch fd.Event {
	case events.VATStatusChanged:
		title := "KDV durumu değişti: " + taxpayer
		told := "ödeme çıkmayacak"
		if fd.Extra["previous_outcome"] == notice.OutcomePayment {
			told = "ödeme çıkacak"
		}
		var now string
		if notice.Outcome(f.Totals) == notice.OutcomePayment {
			now = fmt.Sprintf("yeni faturalarla şu an %s TL KDV ödemesi çıkıyor", payable)
		} else {
			now = "yeni faturalarla şu an KDV ödemesi çıkmıyor"
		}
		when := ""
		if at, ok := fd.Extra["notified_at"].(string); ok {
			if ts, err := time.Parse(time.RFC3339, at); err == nil {
				when = " " + notice.DateTR(ts.In(usecase.Istanbul).Format(time.DateOnly)) + " günü"
			}
		}
		return title, fmt.Sprintf("%s için mükellefe%s \"%s\" denmişti; %s. Mükellefe yeni bilgi göndermeniz önerilir.", month, when, told, now)
	default:
		limit, _ := fd.Extra["limit"].(string)
		l, _ := decimal.NewFromString(limit)
		return "KDV eşiği aşıldı: " + taxpayer,
			fmt.Sprintf("%s için tahmini ödenecek KDV %s TL; ofis eşiği %s TL.", month, payable, notice.TL(usecase.M(l)))
	}
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func (w *Watcher) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}
