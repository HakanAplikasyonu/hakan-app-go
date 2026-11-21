// Package notice sends the taxpayer the in-period VAT status ("bu ay şu an
// KDV ödemeniz çıkıyor / çıkmıyor") and keeps the proof of sending (#780 B,
// 21.11 ofis görüşmesi).
//
// The forecast at send time is frozen in vat_notices, so the VAT watch can
// later tell when new invoices changed what the taxpayer was told. Every
// channel row is an ordinary notification (in-app and e-mail today); its
// sent / read times are the proof. Channels added later (SMS, WhatsApp,
// #780 A) plug in through Channel.
package notice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
)

// Outcomes of a notice.
const (
	OutcomePayment   = "payment"
	OutcomeNoPayment = "no_payment"
)

// Errors.
var (
	ErrInvalid      = errors.New("vat notice: invalid input")
	ErrNoRecipients = errors.New("vat notice: the taxpayer has no panel user")
)

const maxMessage = 1000

// Queries is the db subset used here.
type Queries interface {
	InsertVATNotice(ctx context.Context, arg db.InsertVATNoticeParams) (db.VatNotice, error)
	SetVATNoticeNotifications(ctx context.Context, arg db.SetVATNoticeNotificationsParams) error
	ListVATNotices(ctx context.Context, arg db.ListVATNoticesParams) ([]db.ListVATNoticesRow, error)
	ListVATNoticeRecipients(ctx context.Context, taxpayerID int64) ([]int64, error)
	GetDocumentTaskNotifyNames(ctx context.Context, arg db.GetDocumentTaskNotifyNamesParams) (db.GetDocumentTaskNotifyNamesRow, error)
}

// Notifier enqueues notifications (notifications usecase).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Forecaster computes the in-period forecast (vatforecast usecase).
type Forecaster interface {
	Forecast(ctx context.Context, sub usecase.Subject, period string) (usecase.Forecast, error)
	CurrentPeriod() string
}

// Channels sent today. SMS and WhatsApp are added with #780 A.
var Channels = []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail}

// Service sends notices and lists what the taxpayer was told.
type Service struct {
	q           Queries
	forecast    Forecaster
	notifier    Notifier
	frontendURL string
}

// New builds the service; notifier may be nil (nothing is sent, Send fails).
func New(q Queries, f Forecaster, n Notifier, frontendURL string) *Service {
	return &Service{q: q, forecast: f, notifier: n, frontendURL: strings.TrimRight(frontendURL, "/")}
}

// Notice is the API view.
type Notice struct {
	UUID       uuid.UUID     `json:"uuid"`
	Period     string        `json:"period"`
	Outcome    string        `json:"outcome"`
	OutputVAT  usecase.Money `json:"output_vat"`
	InputVAT   usecase.Money `json:"input_vat"`
	CarriedIn  usecase.Money `json:"carried_in"`
	Payable    usecase.Money `json:"payable"`
	CarryOut   usecase.Money `json:"carry_out"`
	DueDate    *string       `json:"due_date"`
	Message    *string       `json:"message"`
	SentByName string        `json:"sent_by_name"`
	CreatedAt  time.Time     `json:"created_at"`
	// MessageRef links the communication centre's message record (#780 A).
	MessageRef *string `json:"message_ref"`
}

// SendInput is the POST body.
type SendInput struct {
	Period  string `json:"period"`
	Message string `json:"message"`
}

// Outcome is what the taxpayer is told for totals.
func Outcome(t usecase.Totals) string {
	if t.Payable.Dec().IsPositive() {
		return OutcomePayment
	}
	return OutcomeNoPayment
}

// Text is the notification title and body for a forecast.
func Text(f usecase.Forecast, office, message string) (string, string) {
	t := f.Totals
	month := MonthName(f.Period)
	var b strings.Builder
	var title string
	if Outcome(t) == OutcomePayment {
		title = month + ": KDV ödemeniz çıkıyor"
		fmt.Fprintf(&b, "%s için şu ana kadarki faturalara göre tahmini %s TL KDV ödemeniz çıkıyor.", month, tl(t.Payable))
		if f.Calendar.DueDate != "" {
			fmt.Fprintf(&b, " Son ödeme günü %s.", dateTR(f.Calendar.DueDate))
		}
	} else {
		title = month + ": KDV ödemeniz çıkmıyor"
		fmt.Fprintf(&b, "%s için şu ana kadarki faturalara göre KDV ödemeniz çıkmıyor.", month)
		if t.CarryOut.Dec().IsPositive() {
			fmt.Fprintf(&b, " Sonraki aya devreden tahmini %s TL.", tl(t.CarryOut))
		}
	}
	fmt.Fprintf(&b, " Satış KDV'si %s TL, alış KDV'si %s TL, önceki aydan devreden %s TL.", tl(t.OutputVAT), tl(t.InputVAT), tl(t.CarriedIn))
	b.WriteString(" Ay içinde yeni fatura gelirse durum değişebilir.")
	if m := strings.TrimSpace(message); m != "" {
		b.WriteString("\n\n" + m)
	}
	if office != "" {
		b.WriteString("\n\n" + office)
	}
	return title, b.String()
}

// Send freezes the current forecast and notifies the taxpayer's panel users
// in-app and by e-mail.
func (s *Service) Send(ctx context.Context, sub usecase.Subject, userID int64, in SendInput) (Notice, error) {
	period := strings.TrimSpace(in.Period)
	if period == "" {
		period = s.forecast.CurrentPeriod()
	}
	if _, err := usecase.ParsePeriod(period); err != nil {
		return Notice{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	msg := strings.TrimSpace(in.Message)
	if utf8.RuneCountInString(msg) > maxMessage {
		return Notice{}, fmt.Errorf("%w: message is too long", ErrInvalid)
	}
	if s.notifier == nil {
		return Notice{}, ErrNoRecipients
	}
	users, err := s.q.ListVATNoticeRecipients(ctx, sub.TaxpayerID)
	if err != nil {
		return Notice{}, fmt.Errorf("vat notice recipients: %w", err)
	}
	if len(users) == 0 {
		return Notice{}, ErrNoRecipients
	}
	f, err := s.forecast.Forecast(ctx, sub, period)
	if err != nil {
		return Notice{}, err
	}
	names, err := s.q.GetDocumentTaskNotifyNames(ctx, db.GetDocumentTaskNotifyNamesParams{OrganizationID: sub.OrganizationID, TaxpayerID: sub.TaxpayerID})
	if err != nil {
		return Notice{}, fmt.Errorf("vat notice names: %w", err)
	}
	t := f.Totals
	var due pgtype.Date
	if d, err := time.Parse(time.DateOnly, f.Calendar.DueDate); err == nil {
		due = pgtype.Date{Time: d, Valid: true}
	}
	var by pgtype.Int8
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	row, err := s.q.InsertVATNotice(ctx, db.InsertVATNoticeParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID,
		Period: period, Outcome: Outcome(t),
		OutputVat: num(t.OutputVAT), InputVat: num(t.InputVAT), CarriedIn: num(t.CarriedIn),
		Payable: num(t.Payable), CarryOut: num(t.CarryOut), DueDate: due,
		Message: pgtype.Text{String: msg, Valid: msg != ""}, SentBy: by,
	})
	if err != nil {
		return Notice{}, fmt.Errorf("insert vat notice: %w", err)
	}
	title, body := Text(f, names.OrganizationName, msg)
	link := s.frontendURL + "/m/" + names.TaxpayerUuid.String() + "/report"
	var ids []uuid.UUID
	for _, uid := range users {
		org, user := sub.OrganizationID, uid
		out, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			OrganizationID: &org, UserID: &user, Channels: Channels, Priority: notifmodel.PriorityNormal,
			Title: title, Body: body, ActionURL: &link, SourceEvent: events.VATNoticeSent,
			Payload: map[string]any{
				"kind": "vat_notice", "notice_uuid": row.Uuid.String(), "taxpayer_uuid": names.TaxpayerUuid.String(),
				"period": period, "outcome": row.Outcome, "payable": t.Payable.Dec().StringFixed(2),
			},
		})
		if err != nil {
			return Notice{}, fmt.Errorf("enqueue vat notice: %w", err)
		}
		for _, n := range out {
			ids = append(ids, n.UUID)
		}
	}
	if err := s.q.SetVATNoticeNotifications(ctx, db.SetVATNoticeNotificationsParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, ID: row.ID, NotificationUuids: ids,
	}); err != nil {
		return Notice{}, fmt.Errorf("vat notice notifications: %w", err)
	}
	list, err := s.List(ctx, sub, 1)
	if err != nil || len(list) == 0 {
		return Notice{}, err
	}
	return list[0], nil
}

// List returns the engagement's notices, newest first.
func (s *Service) List(ctx context.Context, sub usecase.Subject, limit int32) ([]Notice, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.q.ListVATNotices(ctx, db.ListVATNoticesParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, LimitCount: limit})
	if err != nil {
		return nil, fmt.Errorf("list vat notices: %w", err)
	}
	out := make([]Notice, 0, len(rows))
	for _, r := range rows {
		n := Notice{
			UUID: r.Uuid, Period: r.Period, Outcome: r.Outcome,
			OutputVAT: money(r.OutputVat), InputVAT: money(r.InputVat), CarriedIn: money(r.CarriedIn),
			Payable: money(r.Payable), CarryOut: money(r.CarryOut), SentByName: r.SentByName,
			CreatedAt: r.CreatedAt.Time,
		}
		if r.DueDate.Valid {
			d := r.DueDate.Time.Format(time.DateOnly)
			n.DueDate = &d
		}
		if r.Message.Valid {
			m := r.Message.String
			n.Message = &m
		}
		if r.MessageRef.Valid {
			ref := r.MessageRef.String
			n.MessageRef = &ref
		}
		out = append(out, n)
	}
	return out, nil
}

func num(m usecase.Money) pgtype.Numeric {
	d := m.Dec().Round(2)
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func money(n pgtype.Numeric) usecase.Money {
	if !n.Valid || n.Int == nil {
		return usecase.M(decimal.Zero)
	}
	return usecase.M(decimal.NewFromBigInt(n.Int, n.Exp))
}

var months = []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// MonthName: 2026-11 → "Kasım 2026".
func MonthName(p string) string {
	t, err := usecase.ParsePeriod(p)
	if err != nil {
		return p
	}
	return months[t.Month()-1] + " " + p[:4]
}

func dateTR(iso string) string {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

// tl formats a TL amount the Turkish way: 12.500,00.
func tl(m usecase.Money) string {
	s := m.Dec().StringFixed(2)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := b.String() + "," + frac
	if neg {
		return "-" + out
	}
	return out
}
