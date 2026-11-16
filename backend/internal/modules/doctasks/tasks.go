package doctasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// BankTransaction is the movement a task asks a document for.
type BankTransaction struct {
	UUID             uuid.UUID `json:"uuid"`
	Date             string    `json:"transaction_date"`
	Direction        string    `json:"direction"` // in | out
	Amount           string    `json:"amount"`
	Currency         string    `json:"currency"`
	CounterpartyName *string   `json:"counterparty_name"`
	Description      string    `json:"description"`
}

// Upload is one file uploaded for a task.
type Upload struct {
	UUID           uuid.UUID `json:"uuid"`
	AttachmentUUID uuid.UUID `json:"attachment_uuid"`
	Kind           string    `json:"kind"`
	ContentType    *string   `json:"content_type"`
	FileName       *string   `json:"file_name"`
	SizeBytes      int64     `json:"size_bytes"`
	SHA256         string    `json:"sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

// Task is the API view of a document task. Office-only fields are omitted
// on the mükellef side.
type Task struct {
	UUID             uuid.UUID       `json:"uuid"`
	Code             string          `json:"code"`
	Kind             string          `json:"kind"`
	Period           string          `json:"period"`
	Status           string          `json:"status"`
	Title            string          `json:"title"`
	Note             string          `json:"note"`
	DueAt            time.Time       `json:"due_at"`
	Overdue          bool            `json:"overdue"`
	OrganizationName string          `json:"organization_name,omitempty"`
	ReminderCount    *int32          `json:"reminder_count,omitempty"`
	RequestCount     *int32          `json:"request_count,omitempty"`
	LastRemindedAt   *time.Time      `json:"last_reminded_at,omitempty"`
	NextReminderAt   *time.Time      `json:"next_reminder_at,omitempty"`
	SubmittedAt      *time.Time      `json:"submitted_at"`
	MatchedAt        *time.Time      `json:"matched_at"`
	CancelledAt      *time.Time      `json:"cancelled_at,omitempty"`
	CancelReason     *string         `json:"cancel_reason,omitempty"`
	BankTransaction  BankTransaction `json:"bank_transaction"`
	UploadCount      int64           `json:"upload_count"`
	Uploads          []Upload        `json:"uploads,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`

	id, organizationID, engagementID, taxpayerID int64
	bankTransactionID                            pgtype.Int8
}

// Filter narrows a task list.
type Filter struct {
	Status        string // "" = any
	Period        string
	Limit, Offset int32
}

// GenerateInput is an office request to open tasks.
type GenerateInput struct {
	// Period limits the warnings; with Detect it is also detected first.
	Period string
	// WarningUUIDs limits the warnings to these (nil = every open one).
	WarningUUIDs []uuid.UUID
	// Detect runs the control for Period before opening tasks.
	Detect  bool
	DueDays int
	Note    string
}

// GenerateResult is what Generate did.
type GenerateResult struct {
	Detection *DetectReport `json:"detection,omitempty"`
	Created   int           `json:"created"`
	Notified  int           `json:"notified"`
}

func subjectOf(scope engagementctx.Scope) engine.Subject {
	return engine.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID}
}

// DetectEngagement runs the control for an engagement scope.
func (s *Service) DetectEngagement(ctx context.Context, scope engagementctx.Scope, period string) (DetectReport, error) {
	return s.Detect(ctx, subjectOf(scope), period)
}

// Generate opens tasks from the engagement's open BNK_* warnings and tells
// the mükellef's members.
func (s *Service) Generate(ctx context.Context, scope engagementctx.Scope, actorID int64, in GenerateInput) (GenerateResult, error) {
	var res GenerateResult
	in.Note = strings.TrimSpace(in.Note)
	if len([]rune(in.Note)) > 1000 {
		return res, fmt.Errorf("%w: note is longer than 1000 characters", ErrBadInput)
	}
	if in.Period != "" && !ValidPeriod(in.Period) {
		return res, ErrBadPeriod
	}
	if in.Detect {
		if in.Period == "" {
			return res, fmt.Errorf("%w: detect needs a period", ErrBadInput)
		}
		rep, err := s.DetectEngagement(ctx, scope, in.Period)
		if err != nil {
			return res, err
		}
		res.Detection = &rep
	}
	due := in.DueDays
	if due <= 0 {
		due = s.cfg.DueDays
	}
	if due > 90 {
		return res, fmt.Errorf("%w: due_days must be at most 90", ErrBadInput)
	}
	now := s.now()
	params := db.CreateDocumentTasksFromWarningsParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
		Note:           in.Note,
		DueAt:          ts(now.AddDate(0, 0, due)),
		NextReminderAt: ts(now.AddDate(0, 0, s.cfg.ReminderBaseDays)),
		Period:         pgtype.Text{String: in.Period, Valid: in.Period != ""},
		WarningUuids:   in.WarningUUIDs,
	}
	if actorID > 0 {
		params.CreatedBy = pgtype.Int8{Int64: actorID, Valid: true}
	}
	ids, err := s.q.CreateDocumentTasksFromWarnings(ctx, params)
	if err != nil {
		return res, fmt.Errorf("doctasks: create tasks: %w", err)
	}
	res.Created = len(ids)
	if res.Created > 0 {
		res.Notified = s.notifyMembers(ctx, TemplateCreated, scope.OrganizationID, scope.TaxpayerID, res.Created, s.singleTask(ctx, scope.OrganizationID, scope.ID, ids))
	}
	return res, nil
}

// List returns the engagement's tasks, newest first.
func (s *Service) List(ctx context.Context, scope engagementctx.Scope, f Filter) ([]Task, int64, error) {
	status, period := optText(f.Status), optText(f.Period)
	rows, err := s.q.ListDocumentTasks(ctx, db.ListDocumentTasksParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
		Status: status, Period: period, Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountDocumentTasks(ctx, db.CountDocumentTasksParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Status: status, Period: period,
	})
	if err != nil {
		return nil, 0, err
	}
	now := s.now()
	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, officeTask(db.GetDocumentTaskRow(r), now))
	}
	return out, total, nil
}

// Get returns one task of the engagement with its uploads.
func (s *Service) Get(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) (Task, error) {
	r, err := s.q.GetDocumentTask(ctx, db.GetDocumentTaskParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	t := officeTask(r, s.now())
	if t.Uploads, err = s.uploads(ctx, t); err != nil {
		return Task{}, err
	}
	return t, nil
}

// Cancel cancels an open / submitted task.
func (s *Service) Cancel(ctx context.Context, scope engagementctx.Scope, id uuid.UUID, actorID int64, reason string) (Task, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "cancelled"
	}
	if len([]rune(reason)) > 500 {
		return Task{}, fmt.Errorf("%w: reason is longer than 500 characters", ErrBadInput)
	}
	p := db.CancelDocumentTaskParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id, CancelReason: pgtype.Text{String: reason, Valid: true}}
	if actorID > 0 {
		p.CancelledBy = pgtype.Int8{Int64: actorID, Valid: true}
	}
	n, err := s.q.CancelDocumentTask(ctx, p)
	if err != nil {
		return Task{}, err
	}
	t, err := s.Get(ctx, scope, id)
	if err != nil {
		return Task{}, err
	}
	if n == 0 {
		return t, ErrClosed
	}
	return t, nil
}

func (s *Service) uploads(ctx context.Context, t Task) ([]Upload, error) {
	rows, err := s.q.ListDocumentTaskUploads(ctx, db.ListDocumentTaskUploadsParams{
		OrganizationID: t.organizationID, EngagementID: t.engagementID, TaskID: t.id,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Upload, 0, len(rows))
	for _, r := range rows {
		out = append(out, Upload{
			UUID: r.Uuid, AttachmentUUID: r.AttachmentUuid, Kind: r.Kind,
			ContentType: textPtr(r.ContentType), FileName: textPtr(r.FileName),
			SizeBytes: r.SizeBytes, SHA256: r.Sha256, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

func officeTask(r db.GetDocumentTaskRow, now time.Time) Task {
	rc, rq := r.ReminderCount, r.RequestCount
	t := Task{
		UUID: r.Uuid, Code: r.Code, Kind: r.Kind, Period: r.Period, Status: r.Status, Title: r.Title, Note: r.Note,
		DueAt: r.DueAt.Time, ReminderCount: &rc, RequestCount: &rq,
		LastRemindedAt: tsPtr(r.LastRemindedAt), NextReminderAt: tsPtr(r.NextReminderAt),
		SubmittedAt: tsPtr(r.SubmittedAt), MatchedAt: tsPtr(r.MatchedAt),
		CancelledAt: tsPtr(r.CancelledAt), CancelReason: textPtr(r.CancelReason),
		UploadCount: r.UploadCount, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		id: r.ID, organizationID: r.OrganizationID, engagementID: r.EngagementID,
		taxpayerID: r.TaxpayerID, bankTransactionID: r.BankTransactionID,
	}
	t.BankTransaction = bankTransaction(t, r.BankTransactionUuid, r.TransactionDate, r.DebitCredit, r.Amount, r.Currency, r.CounterpartyName, r.Description)
	t.Overdue = t.Status == StatusOpen && !now.Before(t.DueAt)
	return t
}

// bankTransaction is the movement of a bank task. A document request
// (issue #240) has none; it gets a stand-in so that clients built for bank
// tasks (mükellef mobil #200 reads bank_transaction unconditionally) keep
// working: the task's own uuid, the due date, amount 0 and the title as
// description.
func bankTransaction(t Task, id pgtype.UUID, date pgtype.Date, dc pgtype.Text, amount pgtype.Numeric, currency, counterparty, description pgtype.Text) BankTransaction {
	if !id.Valid {
		return BankTransaction{
			UUID: t.UUID, Date: t.DueAt.Format(time.DateOnly), Direction: "out", Amount: "0", Currency: "TRY",
			Description: t.Title,
		}
	}
	return BankTransaction{
		UUID: uuid.UUID(id.Bytes), Date: dateString(date), Direction: direction(dc.String),
		Amount: numericString(amount), Currency: currency.String,
		CounterpartyName: textPtr(counterparty), Description: description.String,
	}
}

func direction(dc string) string {
	if dc == "A" {
		return "in"
	}
	return "out"
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func optText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
