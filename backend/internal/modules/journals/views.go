package journals

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// FileStore reads stored export files back (storage.Driver satisfies it).
type FileStore interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// DraftFilter narrows ListDrafts.
type DraftFilter struct {
	Period string
	Status string
	Limit  int32
	Offset int32
}

// DraftPage is a page of drafts.
type DraftPage struct {
	Items  []Draft `json:"items"`
	Total  int64   `json:"total"`
	Limit  int32   `json:"limit"`
	Offset int32   `json:"offset"`
}

// ListDrafts lists the engagement's drafts, newest period first.
func (s *Service) ListDrafts(ctx context.Context, scope engagementctx.Scope, f DraftFilter) (DraftPage, error) {
	rows, err := s.q.ListJournalDrafts(ctx, db.ListJournalDraftsParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
		Period: repository.Text(f.Period), Status: repository.Text(f.Status), Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return DraftPage{}, err
	}
	total, err := s.q.CountJournalDrafts(ctx, db.CountJournalDraftsParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
		Period: repository.Text(f.Period), Status: repository.Text(f.Status),
	})
	if err != nil {
		return DraftPage{}, err
	}
	out := DraftPage{Items: make([]Draft, 0, len(rows)), Total: total, Limit: f.Limit, Offset: f.Offset}
	for _, r := range rows {
		out.Items = append(out.Items, Draft{
			UUID: r.Uuid, Period: r.Period, EntryNo: r.EntryNo, Type: r.EntryType, Date: r.EntryDate.Time, Status: r.Status,
			Debit: repository.Dec(r.TotalDebit), Credit: repository.Dec(r.TotalCredit), Lines: r.LineCount,
		})
	}
	return out, nil
}

// DraftLine is one stored line.
type DraftLine struct {
	LineNo        int32            `json:"line_no"`
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
}

// DraftDetail is a draft with its lines.
type DraftDetail struct {
	Draft
	Description  string         `json:"description"`
	DocumentType string         `json:"document_type,omitempty"`
	Origin       map[string]any `json:"origin"`
	ApprovedAt   *time.Time     `json:"approved_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	Items        []DraftLine    `json:"items"`
}

// GetDraft returns one draft with its lines.
func (s *Service) GetDraft(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) (DraftDetail, error) {
	row, err := s.q.GetJournalDraft(ctx, db.GetJournalDraftParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return DraftDetail{}, ErrNotFound
	}
	if err != nil {
		return DraftDetail{}, err
	}
	lines, err := s.q.ListJournalDraftLines(ctx, db.ListJournalDraftLinesParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, JournalDraftID: row.ID})
	if err != nil {
		return DraftDetail{}, err
	}
	out := DraftDetail{
		Draft:       Draft{UUID: row.Uuid, Period: row.Period, EntryNo: row.EntryNo, Type: row.EntryType, Date: row.EntryDate.Time, Status: row.Status, Lines: int32(len(lines))}, //nolint:gosec // small
		Description: row.Description, DocumentType: row.DocumentType.String, Origin: map[string]any{},
		CreatedAt: row.CreatedAt.Time, Items: make([]DraftLine, 0, len(lines)),
	}
	_ = json.Unmarshal(row.Origin, &out.Origin)
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		out.ApprovedAt = &t
	}
	for _, l := range lines {
		d := DraftLine{
			LineNo: l.LineNo, Account: l.AccountCode, Description: l.Description,
			Debit: repository.Dec(l.Debit), Credit: repository.Dec(l.Credit),
			Quantity: decPtr(l.Quantity), Currency: l.Currency.String, ExchangeRate: decPtr(l.ExchangeRate),
			ForeignAmount: decPtr(l.ForeignAmount), DocumentNo: l.DocumentNo.String,
			SourceKind: l.SourceKind.String, SourceID: l.SourceID.String,
		}
		if l.DocumentDate.Valid {
			t := l.DocumentDate.Time
			d.DocumentDate = &t
		}
		out.Debit, out.Credit = out.Debit.Add(d.Debit), out.Credit.Add(d.Credit)
		out.Items = append(out.Items, d)
	}
	return out, nil
}

// BatchSummary is one export batch in a list.
type BatchSummary struct {
	UUID             uuid.UUID                `json:"uuid"`
	Driver           string                   `json:"driver"`
	TemplateVersion  string                   `json:"template_version"`
	TemplateVerified bool                     `json:"template_verified"`
	Period           string                   `json:"period,omitempty"`
	Files            []accountingexport.File  `json:"files"`
	Issues           []accountingexport.Issue `json:"issues"`
	Entries          int32                    `json:"entries"`
	Reversed         int32                    `json:"reversed"`
	CreatedAt        time.Time                `json:"created_at"`
}

// BatchEntry is one draft of a batch.
type BatchEntry struct {
	Draft          uuid.UUID  `json:"journal_draft_uuid"`
	Period         string     `json:"period"`
	EntryNo        int32      `json:"entry_no"`
	ReversedAt     *time.Time `json:"reversed_at,omitempty"`
	ReversalReason string     `json:"reversal_reason,omitempty"`
}

// BatchDetail is a batch with its entries.
type BatchDetail struct {
	BatchSummary
	Items []BatchEntry `json:"items"`
}

// ListBatches lists the engagement's exports, newest first.
func (s *Service) ListBatches(ctx context.Context, scope engagementctx.Scope, limit, offset int32) ([]BatchSummary, error) {
	rows, err := s.q.ListExportBatches(ctx, db.ListExportBatchesParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Lim: limit, Off: offset})
	if err != nil {
		return nil, err
	}
	out := make([]BatchSummary, 0, len(rows))
	for _, r := range rows {
		b := summary(db.ExportBatch{
			Uuid: r.Uuid, Driver: r.Driver, TemplateVersion: r.TemplateVersion, TemplateVerified: r.TemplateVerified,
			Period: r.Period, Files: r.Files, Issues: r.Issues, CreatedAt: r.CreatedAt,
		})
		b.Entries, b.Reversed = r.EntryCount, r.ReversedCount
		out = append(out, b)
	}
	return out, nil
}

// GetBatch returns one export batch with its entries.
func (s *Service) GetBatch(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) (BatchDetail, error) {
	b, err := s.q.GetExportBatch(ctx, db.GetExportBatchParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return BatchDetail{}, ErrNotFound
	}
	if err != nil {
		return BatchDetail{}, err
	}
	items, err := s.q.ListExportBatchItems(ctx, db.ListExportBatchItemsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ExportBatchID: b.ID})
	if err != nil {
		return BatchDetail{}, err
	}
	out := BatchDetail{BatchSummary: summary(b), Items: make([]BatchEntry, 0, len(items))}
	for _, it := range items {
		e := BatchEntry{Draft: it.JournalDraftUuid, Period: it.Period, EntryNo: it.EntryNo, ReversalReason: it.ReversalReason.String}
		if it.ReversedAt.Valid {
			t := it.ReversedAt.Time
			e.ReversedAt = &t
			out.Reversed++
		}
		out.Items = append(out.Items, e)
	}
	out.Entries = int32(len(items)) //nolint:gosec // small
	return out, nil
}

// OpenFile opens a stored file of a batch by its name. The storage key is
// taken from the batch record, never from the request.
func (s *Service) OpenFile(ctx context.Context, scope engagementctx.Scope, batch uuid.UUID, name string, files FileStore) (accountingexport.File, io.ReadCloser, error) {
	b, err := s.q.GetExportBatch(ctx, db.GetExportBatchParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: batch})
	if errors.Is(err, pgx.ErrNoRows) {
		return accountingexport.File{}, nil, ErrNotFound
	}
	if err != nil {
		return accountingexport.File{}, nil, err
	}
	var stored []BatchFile
	if err := json.Unmarshal(b.Files, &stored); err != nil {
		return accountingexport.File{}, nil, err
	}
	for _, f := range stored {
		if f.Name == name {
			rc, _, err := files.Download(ctx, f.StorageKey)
			if err != nil {
				return accountingexport.File{}, nil, err
			}
			return f.File, rc, nil
		}
	}
	return accountingexport.File{}, nil, ErrNotFound
}

// Drivers describes the registered export drivers with their default
// template.
func Drivers() []accountingexport.DriverInfo {
	out := []accountingexport.DriverInfo{}
	for _, name := range accountingexport.Drivers() {
		d, err := accountingexport.Open(name, accountingexport.Config{})
		if err != nil {
			continue
		}
		out = append(out, d.Info())
	}
	return out
}

func summary(b db.ExportBatch) BatchSummary {
	out := BatchSummary{
		UUID: b.Uuid, Driver: b.Driver, TemplateVersion: b.TemplateVersion, TemplateVerified: b.TemplateVerified,
		Period: b.Period.String, Files: []accountingexport.File{}, Issues: []accountingexport.Issue{}, CreatedAt: b.CreatedAt.Time,
	}
	var stored []BatchFile
	if json.Unmarshal(b.Files, &stored) == nil {
		for _, f := range stored {
			out.Files = append(out.Files, f.File)
		}
	}
	_ = json.Unmarshal(b.Issues, &out.Issues)
	if out.Issues == nil {
		out.Issues = []accountingexport.Issue{}
	}
	return out
}

// Settings are the engagement's export settings (docs/02 §2).
type Settings struct {
	// InvoicesReachLucaDirectly: invoices already reach the program through
	// the integrator, so invoice entries are left out of exports.
	InvoicesReachLucaDirectly bool `json:"invoices_reach_luca_directly"`
	// BlockOnUnknownAccount: an account missing from the target plan blocks
	// the export instead of warning.
	BlockOnUnknownAccount bool `json:"block_on_unknown_account"`
}

// GetSettings returns the settings, or the column defaults when none saved.
func (s *Service) GetSettings(ctx context.Context, scope engagementctx.Scope) (Settings, error) {
	row, err := s.q.GetExportSettings(ctx, db.GetExportSettingsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{BlockOnUnknownAccount: true}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return Settings{InvoicesReachLucaDirectly: row.InvoicesReachLucaDirectly, BlockOnUnknownAccount: row.BlockOnUnknownAccount}, nil
}

// SaveSettings stores the settings.
func (s *Service) SaveSettings(ctx context.Context, scope engagementctx.Scope, actorID int64, in Settings) (Settings, error) {
	row, err := s.q.UpsertExportSettings(ctx, db.UpsertExportSettingsParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID,
		InvoicesReachLucaDirectly: in.InvoicesReachLucaDirectly, BlockOnUnknownAccount: in.BlockOnUnknownAccount,
		UpdatedBy: actor(actorID),
	})
	if err != nil {
		return Settings{}, err
	}
	return Settings{InvoicesReachLucaDirectly: row.InvoicesReachLucaDirectly, BlockOnUnknownAccount: row.BlockOnUnknownAccount}, nil
}
