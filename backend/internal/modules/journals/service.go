// Package journals holds JournalDrafts (yevmiye taslağı) and their export to
// accounting programs (issue #26, docs/02 §2, §4). Drafts are the office's
// work product, scoped by organization + engagement.
package journals

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

var (
	ErrNotFound        = errors.New("journals: not found")
	ErrInvalid         = errors.New("journals: invalid draft")
	ErrUnbalanced      = errors.New("journals: entry is not balanced")
	ErrNotApproved     = errors.New("journals: entry is not approved")
	ErrAlreadyExported = errors.New("journals: entry already exported; reverse it first")
	ErrBlocked         = errors.New("journals: export blocked by validation issues")
	ErrNothingToExport = errors.New("journals: no approved entries to export")
	ErrReasonRequired  = errors.New("journals: a reason is required to reverse an export")
)

// DB is what the service needs from Postgres.
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ObjectStore is the storage subset used for export files.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
}

// PlanLoader returns the office's target account plan for a driver (#30).
// Nil plan means none loaded.
type PlanLoader func(ctx context.Context, scope engagementctx.Scope, driver string) (accountingexport.AccountPlan, error)

// Service manages drafts and exports.
type Service struct {
	db    DB
	q     *db.Queries
	store ObjectStore
	plan  PlanLoader
	now   func() time.Time
}

// New builds the service.
func New(pool DB, store ObjectStore) *Service {
	return &Service{db: pool, q: db.New(pool), store: store, now: time.Now}
}

// WithPlanLoader sets how the target account plan is found.
func (s *Service) WithPlanLoader(fn PlanLoader) *Service { s.plan = fn; return s }

// LineInput is one line of a new draft.
type LineInput struct {
	Account       string           `json:"account"`
	Description   string           `json:"description"`
	Debit         decimal.Decimal  `json:"debit"`
	Credit        decimal.Decimal  `json:"credit"`
	Quantity      *decimal.Decimal `json:"quantity,omitempty"`
	Currency      string           `json:"currency,omitempty"`
	ExchangeRate  *decimal.Decimal `json:"exchange_rate,omitempty"`
	ForeignAmount *decimal.Decimal `json:"foreign_amount,omitempty"`
	DocumentNo    string           `json:"document_no,omitempty"`
	DocumentDate  *time.Time       `json:"document_date,omitempty"`
	SourceKind    string           `json:"source_kind,omitempty"`
	SourceID      string           `json:"source_id,omitempty"`
	Confidence    *decimal.Decimal `json:"confidence,omitempty"`
}

// DraftInput creates a draft.
type DraftInput struct {
	Type         accountingexport.EntryType `json:"entry_type"`
	Date         time.Time                  `json:"entry_date"`
	Description  string                     `json:"description"`
	DocumentType string                     `json:"document_type,omitempty"`
	Origin       map[string]any             `json:"origin,omitempty"`
	Lines        []LineInput                `json:"lines"`
}

// Draft is the API view of a draft.
type Draft struct {
	UUID    uuid.UUID       `json:"uuid"`
	Period  string          `json:"period"`
	EntryNo int32           `json:"entry_no"`
	Type    string          `json:"entry_type"`
	Date    time.Time       `json:"entry_date"`
	Status  string          `json:"status"`
	Debit   decimal.Decimal `json:"debit"`
	Credit  decimal.Decimal `json:"credit"`
	Lines   int32           `json:"lines"`
}

// CreateDraft stores a new draft (status draft). The entry number is the next
// one in the engagement and period. Balance is enforced at approval.
func (s *Service) CreateDraft(ctx context.Context, scope engagementctx.Scope, actorID int64, in DraftInput) (Draft, error) {
	if in.Type == "" {
		in.Type = accountingexport.EntryMahsup
	}
	switch in.Type {
	case accountingexport.EntryMahsup, accountingexport.EntryAcilis, accountingexport.EntryKapanis:
	default:
		return Draft{}, fmt.Errorf("%w: entry type %q", ErrInvalid, in.Type)
	}
	if in.Date.IsZero() || len(in.Lines) == 0 {
		return Draft{}, fmt.Errorf("%w: date and lines are required", ErrInvalid)
	}
	for i, l := range in.Lines {
		if strings.TrimSpace(l.Account) == "" || l.Debit.IsNegative() || l.Credit.IsNegative() ||
			(l.Debit.IsZero() == l.Credit.IsZero()) || (l.SourceKind == "") != (l.SourceID == "") {
			return Draft{}, fmt.Errorf("%w: line %d needs an account and exactly one of debit / credit", ErrInvalid, i+1)
		}
	}
	period := in.Date.Format("2006-01")
	origin, err := json.Marshal(nonNil(in.Origin))
	if err != nil {
		return Draft{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Draft{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	// Serialise numbering per engagement + period.
	lockKey := fmt.Sprintf("journal_drafts:%d:%s", scope.ID, period)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", lockKey); err != nil {
		return Draft{}, err
	}
	next, err := q.NextJournalEntryNo(ctx, db.NextJournalEntryNoParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Period: period})
	if err != nil {
		return Draft{}, err
	}
	row, err := q.InsertJournalDraft(ctx, db.InsertJournalDraftParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, Period: period,
		EntryNo: next, EntryType: string(in.Type), EntryDate: repository.Date(&in.Date), Description: in.Description,
		DocumentType: repository.Text(in.DocumentType), Origin: origin, CreatedBy: actor(actorID),
	})
	if err != nil {
		return Draft{}, err
	}
	var debit, credit decimal.Decimal
	for i, l := range in.Lines {
		debit, credit = debit.Add(l.Debit), credit.Add(l.Credit)
		if err := q.InsertJournalDraftLine(ctx, db.InsertJournalDraftLineParams{
			OrganizationID: scope.OrganizationID, EngagementID: scope.ID, JournalDraftID: row.ID, LineNo: int32(i + 1), //nolint:gosec // line count is small
			AccountCode: strings.TrimSpace(l.Account), Description: l.Description,
			Debit: repository.Num(l.Debit), Credit: repository.Num(l.Credit),
			Quantity: repository.NumPtr(l.Quantity), Currency: repository.Text(l.Currency),
			ExchangeRate: repository.NumPtr(l.ExchangeRate), ForeignAmount: repository.NumPtr(l.ForeignAmount),
			DocumentNo: repository.Text(l.DocumentNo), DocumentDate: repository.Date(l.DocumentDate),
			SourceKind: repository.Text(l.SourceKind), SourceID: repository.Text(l.SourceID), Confidence: repository.NumPtr(l.Confidence),
		}); err != nil {
			return Draft{}, fmt.Errorf("journals: line %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Draft{}, err
	}
	return Draft{UUID: row.Uuid, Period: period, EntryNo: row.EntryNo, Type: row.EntryType, Date: in.Date, Status: row.Status,
		Debit: debit, Credit: credit, Lines: int32(len(in.Lines))}, nil //nolint:gosec // small
}

// Approve moves a balanced draft to approved. An unbalanced draft is refused
// (docs/02 §2: blocker before any driver sees it).
func (s *Service) Approve(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID) error {
	row, err := s.q.GetJournalDraft(ctx, db.GetJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	tot, err := s.q.JournalDraftTotals(ctx, db.JournalDraftTotalsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, JournalDraftID: row.ID})
	if err != nil {
		return err
	}
	if tot.Lines == 0 || !repository.Dec(tot.Debit).Equal(repository.Dec(tot.Credit)) {
		return fmt.Errorf("%w: debit %s, credit %s", ErrUnbalanced, repository.Dec(tot.Debit).StringFixed(2), repository.Dec(tot.Credit).StringFixed(2))
	}
	n, err := s.q.ApproveJournalDraft(ctx, db.ApproveJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: row.ID, ApprovedBy: actor(actorID)})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: status is %s", ErrInvalid, row.Status)
	}
	return nil
}

// ExportInput selects what to export: explicit drafts, or every approved
// draft of a period.
type ExportInput struct {
	Driver   string      `json:"driver"`
	Template string      `json:"template,omitempty"`
	Period   string      `json:"period,omitempty"`
	Drafts   []uuid.UUID `json:"drafts,omitempty"`
	// IncludeInvoiceEntries overrides invoices_reach_luca_directly for this run.
	IncludeInvoiceEntries bool `json:"include_invoice_entries,omitempty"`
}

// BatchView is the result of an export.
type BatchView struct {
	UUID             uuid.UUID                `json:"uuid"`
	Driver           string                   `json:"driver"`
	TemplateVersion  string                   `json:"template_version"`
	TemplateVerified bool                     `json:"template_verified"`
	Files            []BatchFile              `json:"files"`
	Issues           []accountingexport.Issue `json:"issues"`
	Entries          int                      `json:"entries"`
}

// BatchFile is a stored export file.
type BatchFile struct {
	accountingexport.File
	StorageKey string `json:"storage_key"`
}

// BlockedError carries the issues that stopped an export.
type BlockedError struct{ Issues []accountingexport.Issue }

func (e *BlockedError) Error() string { return ErrBlocked.Error() }
func (e *BlockedError) Unwrap() error { return ErrBlocked }

// Export runs a driver over approved drafts, stores the files and records the
// batch. Each draft can be in one live batch only: a second export of the same
// draft fails with ErrAlreadyExported until it is reversed (docs/02 §4).
func (s *Service) Export(ctx context.Context, scope engagementctx.Scope, actorID int64, in ExportInput) (BatchView, error) {
	drv, err := accountingexport.Open(in.Driver, accountingexport.Config{Template: in.Template})
	if err != nil {
		return BatchView{}, err
	}
	rows, err := s.selectDrafts(ctx, scope, in)
	if err != nil {
		return BatchView{}, err
	}
	batch, err := s.buildBatch(ctx, scope, rows, in)
	if err != nil {
		return BatchView{}, err
	}
	if s.plan != nil {
		if batch.TargetPlan, err = s.plan(ctx, scope, in.Driver); err != nil {
			return BatchView{}, err
		}
	}
	issues := drv.Validate(ctx, batch)
	if accountingexport.HasBlocker(issues) {
		return BatchView{}, &BlockedError{Issues: issues}
	}
	mem := accountingexport.NewMemoryWriter()
	res, err := drv.Export(ctx, batch, mem)
	if err != nil {
		return BatchView{}, err
	}
	if accountingexport.HasBlocker(res.Issues) {
		return BatchView{}, &BlockedError{Issues: res.Issues}
	}
	if len(res.EntryIDs) == 0 {
		return BatchView{}, ErrNothingToExport
	}
	batchUUID := uuid.New()
	files := make([]BatchFile, 0, len(res.Files))
	for _, f := range res.Files {
		data, _ := mem.File(f.Name)
		key := fmt.Sprintf("exports/journals/%d/%s/%s", scope.ID, batchUUID, f.Name)
		if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), Filename: f.Name, ContentType: f.ContentType}, key); err != nil {
			return BatchView{}, fmt.Errorf("journals: store %s: %w", f.Name, err)
		}
		files = append(files, BatchFile{File: f, StorageKey: key})
	}
	byUUID := map[string]int64{}
	for _, r := range rows {
		byUUID[r.Uuid.String()] = r.ID
	}
	info := drv.Info()
	view := BatchView{Driver: info.Name, TemplateVersion: info.TemplateVer, TemplateVerified: info.Verified, Files: files, Issues: nonNilIssues(res.Issues), Entries: len(res.EntryIDs)}
	filesJSON, _ := json.Marshal(files)
	issuesJSON, _ := json.Marshal(view.Issues)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	b, err := q.InsertExportBatch(ctx, db.InsertExportBatchParams{
		Uuid: batchUUID, OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID,
		Driver: info.Name, TemplateVersion: info.TemplateVer, TemplateVerified: info.Verified,
		Period: repository.Text(in.Period), Files: filesJSON, Issues: issuesJSON, CreatedBy: actorID,
	})
	if err != nil {
		return BatchView{}, err
	}
	for _, id := range res.EntryIDs {
		draftID, ok := byUUID[id]
		if !ok {
			return BatchView{}, fmt.Errorf("journals: driver returned unknown entry %s", id)
		}
		if err := q.InsertExportBatchItem(ctx, db.InsertExportBatchItemParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ExportBatchID: b.ID, JournalDraftID: draftID}); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return BatchView{}, ErrAlreadyExported
			}
			return BatchView{}, err
		}
		if n, err := q.MarkJournalDraftExported(ctx, db.MarkJournalDraftExportedParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: draftID}); err != nil {
			return BatchView{}, err
		} else if n == 0 {
			return BatchView{}, ErrAlreadyExported
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BatchView{}, err
	}
	view.UUID = batchUUID
	return view, nil
}

// Reverse takes a draft out of its live batch so it can be exported again.
// Who, when and why are recorded (docs/02 §4).
func (s *Service) Reverse(ctx context.Context, scope engagementctx.Scope, actorID int64, draft uuid.UUID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	row, err := s.q.GetJournalDraft(ctx, db.GetJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: draft})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	n, err := s.q.ReverseExportBatchItem(ctx, db.ReverseExportBatchItemParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, JournalDraftID: row.ID,
		ReversedBy: actor(actorID), Reason: repository.Text(strings.TrimSpace(reason)),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: entry is not in a live export", ErrInvalid)
	}
	return nil
}

func (s *Service) selectDrafts(ctx context.Context, scope engagementctx.Scope, in ExportInput) ([]db.JournalDraft, error) {
	if len(in.Drafts) > 0 {
		rows, err := s.q.ListJournalDraftsByUUID(ctx, db.ListJournalDraftsByUUIDParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuids: in.Drafts})
		if err != nil {
			return nil, err
		}
		if len(rows) != len(in.Drafts) {
			return nil, ErrNotFound
		}
		for _, r := range rows {
			switch r.Status {
			case "exported":
				return nil, fmt.Errorf("%w: entry %s/%d", ErrAlreadyExported, r.Period, r.EntryNo)
			case "draft":
				return nil, fmt.Errorf("%w: entry %s/%d", ErrNotApproved, r.Period, r.EntryNo)
			}
		}
		return rows, nil
	}
	rows, err := s.q.ListExportableJournalDrafts(ctx, db.ListExportableJournalDraftsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Period: repository.Text(in.Period)})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNothingToExport
	}
	return rows, nil
}

func (s *Service) buildBatch(ctx context.Context, scope engagementctx.Scope, rows []db.JournalDraft, in ExportInput) (accountingexport.Batch, error) {
	b := accountingexport.Batch{Period: in.Period, GeneratedAt: s.now()}
	settings, err := s.q.GetExportSettings(ctx, db.GetExportSettingsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		b.Settings.BlockOnUnknownAccount = true // column default
	case err != nil:
		return b, err
	default:
		b.Settings.BlockOnUnknownAccount = settings.BlockOnUnknownAccount
		b.Settings.InvoicesReachLucaDirectly = settings.InvoicesReachLucaDirectly
	}
	b.Settings.IncludeInvoiceEntries = in.IncludeInvoiceEntries
	for _, r := range rows {
		lines, err := s.q.ListJournalDraftLines(ctx, db.ListJournalDraftLinesParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, JournalDraftID: r.ID})
		if err != nil {
			return b, err
		}
		e := accountingexport.Entry{
			ID: r.Uuid.String(), No: strconv.Itoa(int(r.EntryNo)), Date: r.EntryDate.Time, Type: accountingexport.EntryType(r.EntryType),
			Description: r.Description, DocumentType: r.DocumentType.String,
		}
		for _, l := range lines {
			line := accountingexport.Line{
				Account: l.AccountCode, Description: l.Description, Debit: repository.Dec(l.Debit), Credit: repository.Dec(l.Credit),
				Quantity: decPtr(l.Quantity), Currency: l.Currency.String, ExchangeRate: decPtr(l.ExchangeRate),
				ForeignAmount: decPtr(l.ForeignAmount), DocNo: l.DocumentNo.String, Confidence: decPtr(l.Confidence),
				Source: accountingexport.SourceRef{Kind: l.SourceKind.String, ID: l.SourceID.String},
			}
			if l.DocumentDate.Valid {
				t := l.DocumentDate.Time
				line.DocDate = &t
			}
			e.Lines = append(e.Lines, line)
		}
		b.Entries = append(b.Entries, e)
	}
	return b, nil
}

func decPtr(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid {
		return nil
	}
	d := repository.Dec(n)
	return &d
}

func actor(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nonNilIssues(is []accountingexport.Issue) []accountingexport.Issue {
	if is == nil {
		return []accountingexport.Issue{}
	}
	return is
}
