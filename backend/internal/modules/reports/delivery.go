package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	netmail "net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
)

// Delivery errors.
var (
	// ErrNoRecipient: no address given and the taxpayer has no contact email.
	ErrNoRecipient = errors.New("reports: the taxpayer has no contact email")
	// ErrBadRecipient: the given address is not an email address.
	ErrBadRecipient = errors.New("reports: recipient is not a valid email address")
)

// Origins of a delivery.
const (
	OriginManual    = "manual"
	OriginBulk      = "bulk"
	OriginScheduled = "scheduled"
)

// MaxAttempts is how many times the sweep tries one delivery.
const MaxAttempts = 3

// DeliveryQueries is the db subset of deliveries.
type DeliveryQueries interface {
	Queries
	InsertReportDelivery(ctx context.Context, arg db.InsertReportDeliveryParams) (db.ReportDelivery, error)
	ListEngagementReportDeliveries(ctx context.Context, arg db.ListEngagementReportDeliveriesParams) ([]db.ReportDelivery, error)
	CountEngagementReportDeliveries(ctx context.Context, arg db.CountEngagementReportDeliveriesParams) (int64, error)
	ListOfficeReportDeliveries(ctx context.Context, arg db.ListOfficeReportDeliveriesParams) ([]db.ListOfficeReportDeliveriesRow, error)
	CountOfficeReportDeliveries(ctx context.Context, arg db.CountOfficeReportDeliveriesParams) (int64, error)
	ClaimReportDeliveries(ctx context.Context, batchLimit int32) ([]db.ReportDelivery, error)
	MarkReportDeliverySent(ctx context.Context, id int64) error
	MarkReportDeliveryRetry(ctx context.Context, arg db.MarkReportDeliveryRetryParams) error
	MarkReportDeliveryFailed(ctx context.Context, arg db.MarkReportDeliveryFailedParams) error
}

// Delivery is the API view of one report email.
type Delivery struct {
	UUID           uuid.UUID  `json:"uuid"`
	EngagementUUID *uuid.UUID `json:"engagement_uuid,omitempty"`
	TaxpayerTitle  string     `json:"taxpayer_title"`
	Kind           string     `json:"kind"`
	Period         string     `json:"period"`
	Label          string     `json:"label"`
	Recipient      string     `json:"recipient"`
	Origin         string     `json:"origin"`
	// Status: queued | sending | sent | failed.
	Status    string     `json:"status"`
	Attempts  int32      `json:"attempts"`
	LastError *string    `json:"last_error"`
	SentAt    *time.Time `json:"sent_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func deliveryView(d db.ReportDelivery) Delivery {
	v := Delivery{UUID: d.Uuid, Kind: d.Kind, Period: d.Period, Recipient: d.Recipient, Origin: d.Origin,
		Status: d.Status, Attempts: d.Attempts, CreatedAt: d.CreatedAt.Time}
	var snap struct {
		Label    string   `json:"label"`
		Taxpayer Taxpayer `json:"taxpayer"`
	}
	_ = json.Unmarshal(d.Report, &snap)
	v.Label, v.TaxpayerTitle = snap.Label, snap.Taxpayer.Title
	if d.LastError.Valid {
		e := d.LastError.String
		v.LastError = &e
	}
	if d.SentAt.Valid {
		t := d.SentAt.Time
		v.SentAt = &t
	}
	return v
}

// DeliveryService queues report emails and lists their history.
type DeliveryService struct {
	q   DeliveryQueries
	now func() time.Time
}

// NewDeliveryService builds the service.
func NewDeliveryService(q DeliveryQueries) *DeliveryService {
	return &DeliveryService{q: q, now: time.Now}
}

// QueueInput is one report email to queue.
type QueueInput struct {
	Range      Range
	Recipient  string // empty: the taxpayer's contact email
	OfficeName string
	UserID     int64
	Origin     string
}

// Queue builds the report (approved comment only), snapshots it and queues
// the email; the worker sweep renders the PDF and sends it.
func (s *DeliveryService) Queue(ctx context.Context, sc engagementctx.Scope, in QueueInput) (Delivery, error) {
	to, err := s.recipient(ctx, sc.TaxpayerID, in.Recipient)
	if err != nil {
		return Delivery{}, err
	}
	rep, err := Build(ctx, s.q, sc.TaxpayerID, in.Range,
		Options{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ApprovedOnly: true}, s.now())
	if err != nil {
		return Delivery{}, err
	}
	snap, err := json.Marshal(rep)
	if err != nil {
		return Delivery{}, err
	}
	origin := in.Origin
	if origin == "" {
		origin = OriginManual
	}
	var by pgtype.Int8
	if in.UserID > 0 {
		by = pgtype.Int8{Int64: in.UserID, Valid: true}
	}
	row, err := s.q.InsertReportDelivery(ctx, db.InsertReportDeliveryParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID,
		Kind: in.Range.Kind, Period: in.Range.Key, Recipient: to, Origin: origin,
		Report: snap, OfficeName: in.OfficeName, RequestedBy: by,
	})
	if err != nil {
		return Delivery{}, fmt.Errorf("reports: queue delivery: %w", err)
	}
	v := deliveryView(row)
	id := sc.UUID
	v.EngagementUUID = &id
	return v, nil
}

func (s *DeliveryService) recipient(ctx context.Context, taxpayerID int64, given string) (string, error) {
	given = strings.TrimSpace(given)
	if given == "" {
		head, err := s.q.ReportTaxpayer(ctx, taxpayerID)
		if err != nil {
			return "", fmt.Errorf("reports: taxpayer: %w", err)
		}
		if !head.ContactEmail.Valid || strings.TrimSpace(head.ContactEmail.String) == "" {
			return "", ErrNoRecipient
		}
		given = strings.TrimSpace(head.ContactEmail.String)
	}
	return ValidEmail(given)
}

// ValidEmail returns the bare address of s or ErrBadRecipient.
func ValidEmail(s string) (string, error) {
	if len(s) > 254 || strings.ContainsAny(s, "\r\n,;") {
		return "", ErrBadRecipient
	}
	a, err := netmail.ParseAddress(s)
	if err != nil || !strings.Contains(a.Address, "@") {
		return "", ErrBadRecipient
	}
	return a.Address, nil
}

// Page is a page of deliveries.
type Page struct {
	Items  []Delivery `json:"items"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

// ListEngagement is the history of one engagement.
func (s *DeliveryService) ListEngagement(ctx context.Context, sc engagementctx.Scope, limit, offset int32) (Page, error) {
	rows, err := s.q.ListEngagementReportDeliveries(ctx, db.ListEngagementReportDeliveriesParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Lim: limit, Off: offset})
	if err != nil {
		return Page{}, err
	}
	total, err := s.q.CountEngagementReportDeliveries(ctx, db.CountEngagementReportDeliveriesParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID})
	if err != nil {
		return Page{}, err
	}
	out := Page{Items: make([]Delivery, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, r := range rows {
		v := deliveryView(r)
		id := sc.UUID
		v.EngagementUUID = &id
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// ListOffice is the office-wide history; without read_all only the
// caller's assigned engagements.
func (s *DeliveryService) ListOffice(ctx context.Context, orgID, userID int64, readAll bool, limit, offset int32) (Page, error) {
	rows, err := s.q.ListOfficeReportDeliveries(ctx, db.ListOfficeReportDeliveriesParams{
		OrganizationID: orgID, AllEngagements: readAll, UserID: userID, Lim: limit, Off: offset})
	if err != nil {
		return Page{}, err
	}
	total, err := s.q.CountOfficeReportDeliveries(ctx, db.CountOfficeReportDeliveriesParams{OrganizationID: orgID, AllEngagements: readAll, UserID: userID})
	if err != nil {
		return Page{}, err
	}
	out := Page{Items: make([]Delivery, 0, len(rows)), Total: total, Limit: limit, Offset: offset}
	for _, r := range rows {
		v := deliveryView(db.ReportDelivery{ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, EngagementID: r.EngagementID,
			TaxpayerID: r.TaxpayerID, Kind: r.Kind, Period: r.Period, Recipient: r.Recipient, Origin: r.Origin, Status: r.Status,
			Attempts: r.Attempts, LastError: r.LastError, Report: r.Report, SentAt: r.SentAt, CreatedAt: r.CreatedAt})
		eu := r.EngagementUuid
		v.EngagementUUID = &eu
		v.TaxpayerTitle = r.TaxpayerTitle
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// Sender sends queued deliveries (worker sweep, every office: run it inside
// rls.System).
type Sender struct {
	Q    DeliveryQueries
	Mail mail.Sender
	Log  *slog.Logger
	// Now is the clock (tests); nil = time.Now.
	Now func() time.Time
	// Batch is how many deliveries one sweep claims (default 20).
	Batch int32
}

// SendDue sends the deliveries whose turn has come and returns how many
// were sent.
func (s *Sender) SendDue(ctx context.Context) (int, error) {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	batch := s.Batch
	if batch <= 0 {
		batch = 20
	}
	rows, err := s.Q.ClaimReportDeliveries(ctx, batch)
	if err != nil {
		return 0, fmt.Errorf("reports: claim deliveries: %w", err)
	}
	sent := 0
	for _, d := range rows {
		err := s.send(ctx, d, now())
		if err == nil {
			if err := s.Q.MarkReportDeliverySent(ctx, d.ID); err != nil {
				return sent, err
			}
			sent++
			continue
		}
		msg := truncate(err.Error(), 500)
		log.Warn("report_delivery_failed", "delivery", d.Uuid.String(), "attempt", d.Attempts, "error", msg)
		if d.Attempts >= MaxAttempts {
			if err := s.Q.MarkReportDeliveryFailed(ctx, db.MarkReportDeliveryFailedParams{ID: d.ID, LastError: pgtype.Text{String: msg, Valid: true}}); err != nil {
				return sent, err
			}
			continue
		}
		next := now().Add(time.Duration(d.Attempts) * 5 * time.Minute)
		if err := s.Q.MarkReportDeliveryRetry(ctx, db.MarkReportDeliveryRetryParams{ID: d.ID,
			LastError: pgtype.Text{String: msg, Valid: true}, NextAttemptAt: pgtype.Timestamptz{Time: next, Valid: true}}); err != nil {
			return sent, err
		}
	}
	return sent, nil
}

func (s *Sender) send(ctx context.Context, d db.ReportDelivery, now time.Time) error {
	if s.Mail == nil {
		return errors.New("mail sender is not configured")
	}
	var rep Report
	if err := json.Unmarshal(d.Report, &rep); err != nil {
		return fmt.Errorf("report snapshot: %w", err)
	}
	body, err := RenderPDF(rep, PDFOptions{OfficeName: d.OfficeName, Now: now})
	if err != nil {
		return fmt.Errorf("pdf: %w", err)
	}
	msg := Email(rep, d.OfficeName)
	msg.To = []string{d.Recipient}
	msg.Attachments = []mail.Attachment{{Name: FileName(rep), ContentType: "application/pdf", Data: body}}
	return s.Mail.Send(ctx, msg)
}

// FileName is the PDF's name: rapor-2026-10-ornek-gida.pdf.
func FileName(r Report) string {
	return "rapor-" + r.Period + "-" + slug(r.Taxpayer.Title) + ".pdf"
}

func slug(s string) string {
	repl := strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
		"Ç", "c", "Ğ", "g", "İ", "i", "I", "i", "Ö", "o", "Ş", "s", "Ü", "u")
	s = strings.ToLower(repl.Replace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "mukellef"
	}
	return out
}

// Email is the report email (without recipient and attachment): a short
// Turkish summary; the PDF carries the charts.
func Email(r Report, office string) mail.Message {
	office = strings.TrimSpace(office)
	if office == "" {
		office = "Mali müşaviriniz"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sayın %s,\n\n", r.Taxpayer.Title)
	fmt.Fprintf(&b, "%s dönemine ait raporunuz ektedir (PDF).\n\n", r.Label)
	b.WriteString("Özet (KDV hariç):\n")
	fmt.Fprintf(&b, "  Ciro:   %s\n", TRMoney(r.Totals.Revenue))
	fmt.Fprintf(&b, "  Gider:  %s\n", TRMoney(r.Totals.Expenses))
	fmt.Fprintf(&b, "  Net:    %s\n", TRMoney(r.Totals.Net))
	vat := dec0(r.Totals.VATPayable)
	if vat.IsNegative() {
		fmt.Fprintf(&b, "  Devreden KDV (tahmini): %s\n", TRMoney(vat.Abs().StringFixed(2)))
	} else {
		fmt.Fprintf(&b, "  Ödenecek KDV (tahmini): %s\n", TRMoney(vat.StringFixed(2)))
	}
	if r.Commentary != nil && r.Commentary.Status == "approved" && strings.TrimSpace(r.Commentary.Body) != "" {
		b.WriteString("\nMüşavir yorumu:\n")
		b.WriteString(strings.TrimSpace(r.Commentary.Body))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\nSaygılarımızla,\n%s\n\n", office)
	b.WriteString("Bu e-posta HAKAN üzerinden gönderildi. Rakamlar portala kayıtlı verilerden hesaplanmıştır; KDV tutarı tahminidir ve önceki dönem devrini içermez.\n")
	return mail.Message{Subject: fmt.Sprintf("%s — %s raporu", r.Taxpayer.Title, r.Label), Body: b.String()}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
