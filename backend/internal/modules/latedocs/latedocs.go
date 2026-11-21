// Package latedocs is "Geçmiş dönem takibi" (#780 C): a document has two
// separate dates, its arrival (geliş tarihi: when it entered the system, the
// document row's created_at) and its period (ait olduğu dönem: the month of
// the document date). When a document arrives for a period that was already
// closed (the period was locked, or a tax return of that period was filed),
// the office is warned and decides what to do with it: take it into a
// correction return (düzeltme beyannamesi, #575), book it in the current
// period, or ignore it. The decision is kept on the row.
//
// Rows are written by a scan (queries/late_documents.sql ScanLateDocuments)
// that runs before every read, so every way a document comes in (source sync,
// upload, e-mail, Excel, a change request applied to a locked period) is
// covered without hooks in the write paths. The KDV side of a late purchase
// invoice (KDV indirim dönemi, #640) stays in the invoices module; this
// package only shows it.
package latedocs

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Statuses of a late document (000152 chk_late_documents_status).
const (
	StatusPending       = "pending"
	StatusCorrection    = "correction"
	StatusCurrentPeriod = "current_period"
	StatusIgnored       = "ignored"
)

// Statuses lists every status in display order.
var Statuses = []string{StatusPending, StatusCorrection, StatusCurrentPeriod, StatusIgnored}

// Errors.
var (
	ErrNotFound = errors.New("latedocs: late document not found")
	ErrInvalid  = errors.New("latedocs: invalid decision")
)

// Istanbul is the month boundary of "the current period".
var Istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*60*60)
	}
	return loc
}()

var periodRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ValidPeriod reports whether p is YYYY-MM.
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

// ValidStatus reports whether s is a known status.
func ValidStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// CurrentPeriod is the month of now in Europe/Istanbul (YYYY-MM).
func CurrentPeriod(now time.Time) string { return now.In(Istanbul).Format("2006-01") }

// Decision is the office's answer for one late document.
type Decision struct {
	Status       string `json:"status"`
	TargetPeriod string `json:"target_period"`
	Note         string `json:"note"`
}

// Normalize checks a decision against the document's period and fills the
// target month of "cari döneme alındı" (the current month when empty). It
// returns the decision to store, or a short reason when it is not allowed.
func Normalize(d Decision, docPeriod string, now time.Time) (Decision, string) {
	d.Status = strings.TrimSpace(d.Status)
	d.TargetPeriod = strings.TrimSpace(d.TargetPeriod)
	d.Note = strings.TrimSpace(d.Note)
	if !ValidStatus(d.Status) {
		return d, "status must be one of pending, correction, current_period, ignored"
	}
	if len(d.Note) > 1000 {
		return d, "note is too long"
	}
	if d.Status != StatusCurrentPeriod {
		d.TargetPeriod = ""
		if d.Status == StatusPending {
			d.Note = ""
		}
		return d, ""
	}
	if d.TargetPeriod == "" {
		d.TargetPeriod = CurrentPeriod(now)
	}
	if !ValidPeriod(d.TargetPeriod) {
		return d, "target_period must be YYYY-MM"
	}
	if d.TargetPeriod <= docPeriod {
		return d, "target_period must be after the document period"
	}
	return d, ""
}

// Subject is one engagement (office × taxpayer).
type Subject struct {
	OrganizationID, EngagementID, TaxpayerID int64
}

// Item is one late document as the API shows it. Empty strings are null.
type Item struct {
	UUID         uuid.UUID `json:"uuid"`
	DocumentType string    `json:"document_type"`
	DocumentUUID uuid.UUID `json:"document_uuid"`
	Label        string    `json:"label"`
	Counterparty *string   `json:"counterparty"`
	DocumentDate *string   `json:"document_date"`
	Amount       string    `json:"amount"`
	Currency     string    `json:"currency"`
	// Period is the month the document belongs to; ArrivedAt is when it
	// entered the system; ClosedAt is when the period was closed.
	Period      string    `json:"period"`
	ArrivedAt   time.Time `json:"arrived_at"`
	ClosedAt    time.Time `json:"closed_at"`
	CloseReason string    `json:"close_reason"`
	Status      string    `json:"status"`
	// TargetPeriod: the month it was booked in ("cari döneme alındı").
	TargetPeriod  *string    `json:"target_period"`
	Note          *string    `json:"note"`
	DecidedByName *string    `json:"decided_by_name"`
	DecidedAt     *time.Time `json:"decided_at"`
	// InvoiceDirection: incoming / outgoing for invoices, else null.
	InvoiceDirection *string `json:"invoice_direction"`
	// VATDeductionPeriod: a late purchase invoice's KDV indirim dönemi
	// (#640) when it was moved forward; null otherwise.
	VATDeductionPeriod *string `json:"vat_deduction_period"`
}

// Counts are the engagement's late documents by status.
type Counts struct {
	Pending       int64 `json:"pending"`
	Correction    int64 `json:"correction"`
	CurrentPeriod int64 `json:"current_period"`
	Ignored       int64 `json:"ignored"`
}

// List is the engagement list answer.
type List struct {
	Items  []Item `json:"items"`
	Counts Counts `json:"counts"`
}

// Filter narrows the engagement list.
type Filter struct {
	Status       string
	Period       string
	DocumentUUID *uuid.UUID
}

// SummaryRow is one taxpayer in the office-wide count.
type SummaryRow struct {
	EngagementUUID      uuid.UUID `json:"engagement_uuid"`
	TaxpayerTitle       string    `json:"taxpayer_title"`
	Pending             int64     `json:"pending"`
	Total               int64     `json:"total"`
	OldestPendingPeriod *string   `json:"oldest_pending_period"`
}

// Summary is the office-wide answer.
type Summary struct {
	Pending int64        `json:"pending"`
	Total   int64        `json:"total"`
	Items   []SummaryRow `json:"items"`
}

// Queries is the subset of db.Querier used here.
type Queries interface {
	ScanLateDocuments(ctx context.Context, arg db.ScanLateDocumentsParams) (int64, error)
	ListLateDocuments(ctx context.Context, arg db.ListLateDocumentsParams) ([]db.ListLateDocumentsRow, error)
	CountLateDocumentsByStatus(ctx context.Context, arg db.CountLateDocumentsByStatusParams) ([]db.CountLateDocumentsByStatusRow, error)
	GetLateDocument(ctx context.Context, arg db.GetLateDocumentParams) (db.LateDocument, error)
	DecideLateDocument(ctx context.Context, arg db.DecideLateDocumentParams) (db.LateDocument, error)
	SummarizeLateDocuments(ctx context.Context, arg db.SummarizeLateDocumentsParams) ([]db.SummarizeLateDocumentsRow, error)
}

// Service reads and decides late documents.
type Service struct {
	q   Queries
	now func() time.Time
}

// New builds the service.
func New(q Queries) *Service { return &Service{q: q, now: time.Now} }

// WithClock sets the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// Scan records the documents of the engagement that arrived after their
// period was closed; it returns how many new rows were written.
func (s *Service) Scan(ctx context.Context, sub Subject) (int64, error) {
	n, err := s.q.ScanLateDocuments(ctx, db.ScanLateDocumentsParams{
		OrganizationID: sub.OrganizationID, EngagementID: pgtype.Int8{Int64: sub.EngagementID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("scan late documents: %w", err)
	}
	return n, nil
}

// List scans the engagement and lists its late documents.
func (s *Service) List(ctx context.Context, sub Subject, f Filter) (List, error) {
	if _, err := s.Scan(ctx, sub); err != nil {
		return List{}, err
	}
	p := db.ListLateDocumentsParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID}
	if f.Status != "" {
		if !ValidStatus(f.Status) {
			return List{}, fmt.Errorf("%w: unknown status", ErrInvalid)
		}
		p.Status = pgtype.Text{String: f.Status, Valid: true}
	}
	if f.Period != "" {
		if !ValidPeriod(f.Period) {
			return List{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
		}
		p.Period = pgtype.Text{String: f.Period, Valid: true}
	}
	if f.DocumentUUID != nil {
		p.DocumentUuid = pgtype.UUID{Bytes: *f.DocumentUUID, Valid: true}
	}
	rows, err := s.q.ListLateDocuments(ctx, p)
	if err != nil {
		return List{}, fmt.Errorf("list late documents: %w", err)
	}
	out := List{Items: make([]Item, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, itemFromRow(r))
	}
	counts, err := s.q.CountLateDocumentsByStatus(ctx, db.CountLateDocumentsByStatusParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
	})
	if err != nil {
		return List{}, fmt.Errorf("count late documents: %w", err)
	}
	for _, c := range counts {
		switch c.Status {
		case StatusPending:
			out.Counts.Pending = c.N
		case StatusCorrection:
			out.Counts.Correction = c.N
		case StatusCurrentPeriod:
			out.Counts.CurrentPeriod = c.N
		case StatusIgnored:
			out.Counts.Ignored = c.N
		}
	}
	return out, nil
}

// Decide records the office's decision and returns the updated row.
func (s *Service) Decide(ctx context.Context, sub Subject, id uuid.UUID, d Decision, userID int64) (Item, error) {
	cur, err := s.q.GetLateDocument(ctx, db.GetLateDocumentParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get late document: %w", err)
	}
	d, reason := Normalize(d, cur.Period, s.now())
	if reason != "" {
		return Item{}, fmt.Errorf("%w: %s", ErrInvalid, reason)
	}
	p := db.DecideLateDocumentParams{
		Status: d.Status, OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
		TargetPeriod: text(d.TargetPeriod), Note: text(d.Note),
	}
	if userID > 0 {
		p.DecidedBy = pgtype.Int8{Int64: userID, Valid: true}
	}
	if _, err := s.q.DecideLateDocument(ctx, p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("decide late document: %w", err)
	}
	rows, err := s.q.ListLateDocuments(ctx, db.ListLateDocumentsParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID,
		DocumentUuid: pgtype.UUID{Bytes: cur.DocumentUuid, Valid: true},
	})
	if err != nil {
		return Item{}, fmt.Errorf("reload late document: %w", err)
	}
	for _, r := range rows {
		if r.Uuid == id {
			return itemFromRow(r), nil
		}
	}
	return Item{}, ErrNotFound
}

// Summary scans the office and counts late documents per taxpayer. Without
// all, only the user's assigned engagements are counted.
func (s *Service) Summary(ctx context.Context, orgID int64, all bool, userID int64) (Summary, error) {
	if _, err := s.q.ScanLateDocuments(ctx, db.ScanLateDocumentsParams{OrganizationID: orgID}); err != nil {
		return Summary{}, fmt.Errorf("scan late documents: %w", err)
	}
	rows, err := s.q.SummarizeLateDocuments(ctx, db.SummarizeLateDocumentsParams{
		OrganizationID: orgID, AllEngagements: all, UserID: userID,
	})
	if err != nil {
		return Summary{}, fmt.Errorf("summarize late documents: %w", err)
	}
	out := Summary{Items: make([]SummaryRow, 0, len(rows))}
	for _, r := range rows {
		out.Pending += r.Pending
		out.Total += r.Total
		out.Items = append(out.Items, SummaryRow{
			EngagementUUID: r.EngagementUuid, TaxpayerTitle: r.TaxpayerTitle,
			Pending: r.Pending, Total: r.Total, OldestPendingPeriod: opt(r.OldestPendingPeriod),
		})
	}
	return out, nil
}

func itemFromRow(r db.ListLateDocumentsRow) Item {
	it := Item{
		UUID: r.Uuid, DocumentType: r.DocumentType, DocumentUUID: r.DocumentUuid,
		Label: r.Label, Counterparty: opt(r.Counterparty), DocumentDate: opt(r.DocumentDate),
		Amount: r.Amount, Currency: r.Currency, Period: r.Period,
		ArrivedAt: r.ArrivedAt.Time, ClosedAt: r.ClosedAt.Time, CloseReason: r.CloseReason,
		Status: r.Status, TargetPeriod: opt(r.TargetPeriod), DecidedByName: opt(r.DecidedByName),
		InvoiceDirection: opt(r.InvoiceDirection), VATDeductionPeriod: opt(r.VatDeductionPeriod),
	}
	if r.Note.Valid {
		it.Note = opt(r.Note.String)
	}
	if r.DecidedAt.Valid {
		t := r.DecidedAt.Time
		it.DecidedAt = &t
	}
	return it
}

func opt(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
