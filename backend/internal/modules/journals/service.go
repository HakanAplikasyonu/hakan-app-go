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
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ledger"
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
	// ErrLineFromSource: the line's amount comes from its source (bank,
	// invoice, payroll); it is corrected there, not on the draft.
	ErrLineFromSource = errors.New("journals: the line's amount comes from its source document")
	// ErrInvoicePosted: the engagement already booked this invoice (#669).
	ErrInvoicePosted = errors.New("journals: the invoice already has an entry")
	// ErrForeignInvoicePosted: the engagement already booked this foreign
	// service invoice (#780 KDV2 B).
	ErrForeignInvoicePosted = errors.New("journals: the foreign invoice already has an entry")
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

// PlanGate checks the accounts of the entries going to driver against the
// engagement's target plan (#30, #319) and returns ACCOUNT_NOT_IN_TARGET_PLAN
// issues (blockers when block). It may rewrite a line's account to the
// plan's normalized form. No plan loaded: no issues.
type PlanGate func(ctx context.Context, scope engagementctx.Scope, driver string, entries []accountingexport.Entry, block bool) ([]accountingexport.Issue, error)

// Service manages drafts and exports.
type Service struct {
	db    DB
	q     *db.Queries
	store ObjectStore
	plan  PlanLoader
	gate  PlanGate
	now   func() time.Time
}

// New builds the service.
func New(pool DB, store ObjectStore) *Service {
	// Ambient: statements and transactions join the request's row level
	// security scope when there is one (issue #166).
	amb := database.Ambient{Pool: pool}
	return &Service{db: amb, q: db.New(amb), store: store, now: time.Now}
}

// WithPlanLoader sets how the target account plan is found.
func (s *Service) WithPlanLoader(fn PlanLoader) *Service { s.plan = fn; return s }

// WithPlanGate sets the pre-export account check (#319). It replaces the
// plan loader's exact-code check: codes are normalized, group accounts are
// rejected and the result is recorded as a warning.
func (s *Service) WithPlanGate(fn PlanGate) *Service { s.gate = fn; return s }

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
	// InvoiceID links the draft to the invoice it books (#669), in the same
	// transaction: an invoice already booked by this engagement gives
	// ErrInvoicePosted and no draft. Not part of the API body.
	InvoiceID int64 `json:"-"`
	// ForeignInvoiceID links the draft to the foreign service invoice it
	// books (#780 KDV2 B), in the same transaction: an invoice already
	// booked by this engagement gives ErrForeignInvoicePosted and no draft.
	ForeignInvoiceID int64 `json:"-"`
}

// Draft is the API view of a draft.
type Draft struct {
	UUID    uuid.UUID `json:"uuid"`
	Period  string    `json:"period"`
	EntryNo int32     `json:"entry_no"`
	Type    string    `json:"entry_type"`
	Date    time.Time `json:"entry_date"`
	// Description: the entry's açıklama, shown in the list (#573).
	Description string          `json:"description"`
	Status      string          `json:"status"`
	Debit       decimal.Decimal `json:"debit"`
	Credit      decimal.Decimal `json:"credit"`
	Lines       int32           `json:"lines"`
	// Issues: why a draft cannot be approved yet (#566); only on drafts.
	Issues []DraftIssue `json:"issues,omitempty"`
	// Export: the live export of an exported draft and its latest
	// transfer to the program (#574).
	Export *DraftExport `json:"export,omitempty"`
	// ApprovalReverted: the last time the entry's approval was taken back
	// and why (#566); absent when it never was.
	ApprovalReverted *ApprovalRevert `json:"approval_reverted,omitempty"`
}

// ApprovalRevert is when and why an approval was taken back.
type ApprovalRevert struct {
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
}

func approvalRevert(at pgtype.Timestamptz, reason pgtype.Text) *ApprovalRevert {
	if !at.Valid {
		return nil
	}
	return &ApprovalRevert{At: at.Time, Reason: reason.String}
}

// DraftExport is where an exported draft went.
type DraftExport struct {
	BatchUUID  uuid.UUID `json:"batch_uuid"`
	Driver     string    `json:"driver"`
	ExportedAt time.Time `json:"exported_at"`
	// TransferStatus: queued / claimed / delivered / imported / failed /
	// cancelled; empty when the files were only downloaded.
	TransferStatus string `json:"transfer_status,omitempty"`
	TransferError  string `json:"transfer_error,omitempty"`
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
	// İşletme defteri / serbest meslek mükellefinde yevmiye yok (#780).
	if err := ledger.RequireBalance(ctx, s.q, scope.OrganizationID, scope.ID); err != nil {
		return Draft{}, err
	}
	period := in.Date.Format("2006-01")
	origin, err := json.Marshal(nonNil(in.Origin))
	if err != nil {
		return Draft{}, err
	}
	tx, err := database.Begin(ctx, s.db)
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
	if in.InvoiceID > 0 {
		n, err := q.InsertInvoiceJournalLink(ctx, db.InsertInvoiceJournalLinkParams{
			OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID,
			InvoiceID: in.InvoiceID, JournalDraftID: row.ID, CreatedBy: actor(actorID),
		})
		if err != nil {
			return Draft{}, fmt.Errorf("journals: invoice link: %w", err)
		}
		if n == 0 {
			return Draft{}, ErrInvoicePosted
		}
	}
	if in.ForeignInvoiceID > 0 {
		n, err := q.InsertForeignInvoiceJournalLink(ctx, db.InsertForeignInvoiceJournalLinkParams{
			OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
			ForeignInvoiceID: in.ForeignInvoiceID, JournalDraftID: row.ID, CreatedBy: actor(actorID),
		})
		if err != nil {
			return Draft{}, fmt.Errorf("journals: foreign invoice link: %w", err)
		}
		if n == 0 {
			return Draft{}, ErrForeignInvoicePosted
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Draft{}, err
	}
	return Draft{UUID: row.Uuid, Period: period, EntryNo: row.EntryNo, Type: row.EntryType, Date: in.Date, Description: in.Description, Status: row.Status,
		Debit: debit, Credit: credit, Lines: int32(len(in.Lines))}, nil //nolint:gosec // small
}

// Approve moves a balanced draft to approved. An unbalanced draft is refused
// (docs/02 §2: blocker before any driver sees it).
func (s *Service) Approve(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID) error {
	return s.ApproveWith(ctx, scope, actorID, id, Confirmation{Confirmed: true})
}

// ApproveWith approves a draft. A possible duplicate is refused unless
// c.SendDuplicate with a reason (#710); a missing document needs
// c.Confirmed. The accepted warnings and the note / reason stay on the entry.
func (s *Service) ApproveWith(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID, c Confirmation) error {
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
	// Engelli fiş onaylanmaz (#566): kilitli dönem. Hesap kodu sorunları
	// uyarıdır; kontrol ekranında görünür.
	issues, err := s.draftIssues(ctx, scope, []int64{row.ID})
	if err != nil {
		return err
	}
	if b := blocking(issues[row.ID]); len(b) > 0 {
		return &DraftBlockedError{Issues: b}
	}
	warn := confirmable(issues[row.ID])
	// Mükerrer fiş engellenir (#710); yalnız ayrı "yine de gönder" ve
	// gerekçeyle geçer. Eksik belge açık onayla geçer (#705).
	dup, other := splitDuplicates(warn)
	reason := strings.TrimSpace(c.Reason)
	if len(dup) > 0 {
		if !c.SendDuplicate {
			return &DraftDuplicateError{Issues: dup}
		}
		if reason == "" {
			return ErrReasonRequired
		}
	}
	if len(other) > 0 && !c.Confirmed {
		return &DraftNeedsConfirmationError{Issues: other}
	}
	codes := []string{}
	for _, w := range warn {
		codes = append(codes, w.Code)
	}
	note := pgtype.Text{}
	text := strings.TrimSpace(c.Note)
	if len(dup) > 0 {
		text = reason
	}
	if text != "" && len(warn) > 0 {
		note = pgtype.Text{String: text, Valid: true}
	}
	n, err := s.q.ApproveJournalDraft(ctx, db.ApproveJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: row.ID, ApprovedBy: actor(actorID),
		ApprovalWarnings: codes, ApprovalNote: note})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: status is %s", ErrInvalid, row.Status)
	}
	return nil
}

// Unapprove takes back the approval of an entry that has not been exported
// yet (#566): the accountant approved the wrong entry, or it must be
// corrected before it goes to the program. The entry is a draft again (its
// lines can be edited, it must be approved again); who, when and why stay on
// the entry. An exported entry is reversed first (Reverse); an entry of a
// locked period is not touched.
func (s *Service) Unapprove(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrReasonRequired
	}
	row, err := s.q.GetJournalDraft(ctx, db.GetJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	switch row.Status {
	case "exported":
		return fmt.Errorf("%w: entry %s/%d", ErrAlreadyExported, row.Period, row.EntryNo)
	case "draft":
		return fmt.Errorf("%w: entry %s/%d", ErrNotApproved, row.Period, row.EntryNo)
	}
	issues, err := s.draftIssues(ctx, scope, []int64{row.ID})
	if err != nil {
		return err
	}
	for _, is := range issues[row.ID] {
		if is.Code == IssuePeriodLocked {
			return &DraftBlockedError{Issues: []DraftIssue{is}}
		}
	}
	n, err := s.q.RevertJournalDraftApproval(ctx, db.RevertJournalDraftApprovalParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ID: row.ID,
		RevertedBy: actor(actorID), Reason: repository.Text(reason),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		// Decided meanwhile (exported or already reverted by someone else).
		return fmt.Errorf("%w: entry %s/%d", ErrNotApproved, row.Period, row.EntryNo)
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
	// ConfirmDocuments are document keys (invoice ETTN) the accountant
	// confirmed for this export although they were already exported or the
	// program fetches them itself (#466). Their entries go into the files.
	ConfirmDocuments []string `json:"confirm_documents,omitempty"`
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
	// Excluded are the entries the double booking guard left out (#466).
	Excluded []ExcludedEntry `json:"excluded"`
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

// CheckView is the pre-export check (#466): what an export with the same
// input would report and leave out, without writing anything.
type CheckView struct {
	Driver           string                   `json:"driver"`
	TemplateVersion  string                   `json:"template_version"`
	TemplateVerified bool                     `json:"template_verified"`
	Issues           []accountingexport.Issue `json:"issues"`
	// Entries is the number of entries that would go into the files.
	Entries int `json:"entries"`
	// Excluded lists the entries left out by the double booking guard
	// until their documents are confirmed.
	Excluded []ExcludedEntry `json:"excluded"`
	Blocked  bool            `json:"blocked"`
}

// ExcludedEntry is an entry the double booking guard leaves out.
type ExcludedEntry struct {
	Draft   uuid.UUID `json:"journal_draft_uuid"`
	EntryNo string    `json:"entry_no"`
	Period  string    `json:"period"`
	// Documents are the document keys (ETTN) to confirm to include it.
	Documents []string `json:"documents"`
}

// prepared is an export worked out up to the driver run.
type prepared struct {
	drv      accountingexport.Driver
	rows     []db.JournalDraft
	batch    accountingexport.Batch
	docs     map[string][]docRef
	guard    guardResult
	pre      []accountingexport.Issue // guard + plan gate
	issues   []accountingexport.Issue // pre + driver validation
	excluded []ExcludedEntry
}

func (s *Service) prepare(ctx context.Context, scope engagementctx.Scope, in ExportInput) (*prepared, error) {
	drv, err := accountingexport.Open(in.Driver, accountingexport.Config{Template: in.Template})
	if err != nil {
		return nil, err
	}
	rows, err := s.selectDrafts(ctx, scope, in)
	if err != nil {
		return nil, err
	}
	batch, settings, err := s.buildBatch(ctx, scope, rows, in)
	if err != nil {
		return nil, err
	}
	p := &prepared{drv: drv, rows: rows}
	if p.docs, err = s.resolveDocuments(ctx, scope, &batch); err != nil {
		return nil, err
	}
	pull := []string{}
	if in.Driver == lucaDriver {
		if pull, err = s.lucaPull(ctx, scope, settings); err != nil {
			return nil, err
		}
	}
	selected, _ := accountingexport.SelectEntries(batch)
	if p.guard, err = s.guard(ctx, scope, in.Driver, selected, p.docs, pull, in.ConfirmDocuments); err != nil {
		return nil, err
	}
	if len(p.guard.Excluded) > 0 {
		byID := map[string]db.JournalDraft{}
		for _, r := range rows {
			byID[r.Uuid.String()] = r
		}
		kept := batch.Entries[:0:0]
		for _, e := range batch.Entries {
			if !p.guard.Excluded[e.ID] {
				kept = append(kept, e)
				continue
			}
			x := ExcludedEntry{EntryNo: e.No, Documents: []string{}}
			if r, ok := byID[e.ID]; ok {
				x.Draft, x.Period = r.Uuid, r.Period
			}
			for _, d := range p.docs[e.ID] {
				x.Documents = append(x.Documents, d.Key)
			}
			p.excluded = append(p.excluded, x)
		}
		batch.Entries = kept
	}
	var gateIssues []accountingexport.Issue
	switch {
	case s.gate != nil:
		entries, _ := accountingexport.SelectEntries(batch)
		if gateIssues, err = s.gate(ctx, scope, in.Driver, entries, batch.Settings.BlockOnUnknownAccount); err != nil {
			return nil, err
		}
	case s.plan != nil:
		if batch.TargetPlan, err = s.plan(ctx, scope, in.Driver); err != nil {
			return nil, err
		}
	}
	p.batch = batch
	p.pre = append(append([]accountingexport.Issue{}, p.guard.Issues...), gateIssues...)
	p.issues = append(append([]accountingexport.Issue{}, p.pre...), drv.Validate(ctx, batch)...)
	return p, nil
}

// Check runs the pre-export check (#466) for an export input: the same
// validation and double booking guard as Export, nothing is written.
func (s *Service) Check(ctx context.Context, scope engagementctx.Scope, in ExportInput) (CheckView, error) {
	p, err := s.prepare(ctx, scope, in)
	if err != nil {
		return CheckView{}, err
	}
	info := p.drv.Info()
	entries, _ := accountingexport.SelectEntries(p.batch)
	return CheckView{
		Driver: info.Name, TemplateVersion: info.TemplateVer, TemplateVerified: info.Verified,
		Issues: nonNilIssues(p.issues), Entries: len(entries), Excluded: nonNilExcluded(p.excluded),
		Blocked: accountingexport.HasBlocker(p.issues) || len(entries) == 0,
	}, nil
}

// Export runs a driver over approved drafts, stores the files and records the
// batch. Each draft can be in one live batch only: a second export of the same
// draft fails with ErrAlreadyExported until it is reversed (docs/02 §4).
// Entries whose source documents were already exported or are fetched by
// the program itself stay out unless confirmed (#466); every document and
// the file it went into is recorded.
func (s *Service) Export(ctx context.Context, scope engagementctx.Scope, actorID int64, in ExportInput) (BatchView, error) {
	p, err := s.prepare(ctx, scope, in)
	if err != nil {
		return BatchView{}, err
	}
	drv, rows, batch := p.drv, p.rows, p.batch
	if accountingexport.HasBlocker(p.issues) {
		return BatchView{}, &BlockedError{Issues: p.issues}
	}
	if len(batch.Entries) == 0 && len(p.excluded) > 0 {
		// Everything was left out by the double booking guard: show why.
		return BatchView{}, &BlockedError{Issues: p.issues}
	}
	gateIssues := p.pre
	mem := accountingexport.NewMemoryWriter()
	res, err := drv.Export(ctx, batch, mem)
	if err != nil {
		return BatchView{}, err
	}
	if accountingexport.HasBlocker(res.Issues) {
		return BatchView{}, &BlockedError{Issues: res.Issues}
	}
	if len(res.EntryIDs) == 0 {
		if len(p.excluded) > 0 {
			return BatchView{}, &BlockedError{Issues: append(gateIssues, res.Issues...)}
		}
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
	view := BatchView{Driver: info.Name, TemplateVersion: info.TemplateVer, TemplateVerified: info.Verified, Files: files,
		Issues: nonNilIssues(append(gateIssues, res.Issues...)), Entries: len(res.EntryIDs), Excluded: nonNilExcluded(p.excluded)}
	filesJSON, _ := json.Marshal(files)
	issuesJSON, _ := json.Marshal(view.Issues)

	tx, err := database.Begin(ctx, s.db)
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
		for _, d := range p.docs[id] {
			codes := p.guard.Confirmed[id][d.Key]
			doc := db.InsertExportBatchDocumentParams{
				OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ExportBatchID: b.ID, JournalDraftID: draftID,
				SourceKind: d.Kind, DocumentKey: d.Key, InvoiceUuid: pgtype.UUID{Bytes: d.InvoiceUUID, Valid: d.Kind == "invoice"},
				ExpenseUuid: pgtype.UUID{Bytes: d.ExpenseUUID, Valid: d.Kind == "expense"},
				DocumentNo:  repository.Text(d.No), DocumentDate: repository.Date(d.Date), FileName: entryFile(res, id),
				ConfirmedCodes: nonNilStrings(codes),
			}
			if len(codes) > 0 {
				doc.ConfirmedBy = actor(actorID)
			}
			if err := q.InsertExportBatchDocument(ctx, doc); err != nil {
				return BatchView{}, fmt.Errorf("journals: record document %s: %w", d.Key, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BatchView{}, err
	}
	view.UUID = batchUUID
	return view, nil
}

// entryFile is the file an entry went into; a driver that does not report
// it (one file) gives its first file.
func entryFile(res accountingexport.Result, id string) string {
	if n := res.EntryFiles[id]; n != "" {
		return n
	}
	if len(res.Files) > 0 {
		return res.Files[0].Name
	}
	return ""
}

func nonNilExcluded(v []ExcludedEntry) []ExcludedEntry {
	if v == nil {
		return []ExcludedEntry{}
	}
	return v
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

// buildBatch reads the drafts into a driver batch. It also returns the
// engagement's saved export settings (nil when none).
func (s *Service) buildBatch(ctx context.Context, scope engagementctx.Scope, rows []db.JournalDraft, in ExportInput) (accountingexport.Batch, *db.ExportSetting, error) {
	b := accountingexport.Batch{Period: in.Period, GeneratedAt: s.now()}
	var saved *db.ExportSetting
	settings, err := s.q.GetExportSettings(ctx, db.GetExportSettingsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		b.Settings.BlockOnUnknownAccount = true // column default
	case err != nil:
		return b, nil, err
	default:
		saved = &settings
		b.Settings.BlockOnUnknownAccount = settings.BlockOnUnknownAccount
		b.Settings.InvoicesReachLucaDirectly = settings.InvoicesReachLucaDirectly
	}
	b.Settings.IncludeInvoiceEntries = in.IncludeInvoiceEntries
	// The portal's own KDV summary for the period (excel_formula shows it
	// next to what the entries post to 391 / 191).
	if in.Period != "" {
		vat, err := s.q.PeriodVATSummary(ctx, db.PeriodVATSummaryParams{TaxpayerID: scope.TaxpayerID, Period: in.Period})
		if err != nil {
			return b, nil, err
		}
		for _, v := range vat {
			b.VATSummary = append(b.VATSummary, accountingexport.VATRow{
				Direction: v.Direction, Rate: repository.Dec(v.TaxRate), Base: repository.Dec(v.Base), VAT: repository.Dec(v.Vat),
			})
		}
	}
	for _, r := range rows {
		lines, err := s.q.ListJournalDraftLines(ctx, db.ListJournalDraftLinesParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, JournalDraftID: r.ID})
		if err != nil {
			return b, nil, err
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
	return b, saved, nil
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
