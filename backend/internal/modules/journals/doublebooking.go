package journals

// Double booking guard for exports (#466).
//
// Luca fetches e-invoices from the integrator itself (İşNet NetteFatura,
// TÜRMOB e-Belge, GİB e-Belge). An entry the portal exports for the same
// invoice would be booked twice. Before an export every entry's source
// documents (the invoices behind its lines, keyed by ETTN) are checked:
//
//   - DOCUMENT_ALREADY_EXPORTED: the document is already in a live (not
//     reversed) export file of the same program, or in an earlier entry of
//     this export.
//   - PROGRAM_PULLS_DOCUMENT: the program is Luca and the office / taxpayer
//     setting says Luca fetches this invoice's integrator itself.
//
// An entry with such a document stays out of the files until the accountant
// confirms the document for this export (ExportInput.ConfirmDocuments). The
// confirmation (codes, who, when) and the file every document went into are
// kept in export_batch_documents.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/entegrator"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// LucaIntegrators are the integrators Luca can fetch e-invoices from itself.
// The value is the integrator's source system name without "entegrator_".
var LucaIntegrators = []string{"isnet", "turmob", "gib"}

// ErrInvalidIntegrator is returned for an unknown integrator in a setting.
var ErrInvalidIntegrator = errors.New("journals: unknown integrator")

// docRef is one source document behind an entry's lines.
type docRef struct {
	// Kind is export_batch_documents.source_kind: invoice, expense (#780,
	// a receipt line "expense:<uuid>", key "exp:<uuid>"), retail_sale (a Z
	// report's receipt line "retail_sale:<uuid>", key "z:<uuid>") or
	// bank_transaction (a bank line, key "bank:<uuid>"). The keys are the
	// ones the Luca list files record (lucalists), so a Z report or a bank
	// movement that went out with a list file is not exported again in a
	// fiş file, and the other way round.
	Kind        string
	ExpenseUUID uuid.UUID
	Key         string // ETTN, or "inv:<invoice uuid>" when the invoice has none
	ETTN        string
	InvoiceUUID uuid.UUID
	No          string
	Date        *time.Time
	Integrator  string // isnet, turmob, gib, izibiz … ("" unknown)
	TaxID       string // counterparty VKN / TCKN ("" unknown)
}

// guardResult is what the double booking check decided for a batch.
type guardResult struct {
	Issues []accountingexport.Issue
	// Excluded entries (by entry ID) stay out of the files.
	Excluded map[string]bool
	// Confirmed codes per entry ID and document key, written to
	// export_batch_documents.confirmed_codes.
	Confirmed map[string]map[string][]string
}

// resolveDocuments fills the lines' source document (DocumentKey, ETTN, and
// DocNo / DocDate when the line has none) from the invoice behind
// invoice / invoice_item lines, and returns each entry's documents in line
// order.
func (s *Service) resolveDocuments(ctx context.Context, scope engagementctx.Scope, b *accountingexport.Batch) (map[string][]docRef, error) {
	type srcKey struct{ kind, id string }
	docs := map[srcKey]docRef{}
	var invIDs, itemIDs, bankIDs []int64
	var expenseIDs []uuid.UUID
	for _, e := range b.Entries {
		for _, l := range e.Lines {
			if id, ok := ExpenseSourceUUID(l.Source.Kind, l.Source.ID); ok {
				expenseIDs = append(expenseIDs, id)
				continue
			}
			if id, ok := RetailSaleSourceUUID(l.Source.Kind, l.Source.ID); ok {
				docs[srcKey{l.Source.Kind, l.Source.ID}] = docRef{Kind: "retail_sale", Key: ZReportDocumentKey(id)}
				continue
			}
			id, err := strconv.ParseInt(l.Source.ID, 10, 64)
			if err != nil {
				continue
			}
			switch l.Source.Kind {
			case "invoice":
				invIDs = append(invIDs, id)
			case "invoice_item":
				itemIDs = append(itemIDs, id)
			case BankSourceKind:
				bankIDs = append(bankIDs, id)
			}
		}
	}
	out := map[string][]docRef{}
	if len(invIDs) == 0 && len(itemIDs) == 0 && len(expenseIDs) == 0 && len(bankIDs) == 0 && len(docs) == 0 {
		return out, nil
	}
	if len(bankIDs) > 0 {
		banks, err := s.q.ResolveBankLineSources(ctx, db.ResolveBankLineSourcesParams{TaxpayerID: scope.TaxpayerID, Ids: bankIDs})
		if err != nil {
			return nil, err
		}
		for _, r := range banks {
			d := docRef{Kind: BankSourceKind, Key: BankDocumentKey(r.Uuid)}
			if r.TransactionDate.Valid {
				t := r.TransactionDate.Time
				d.Date = &t
			}
			docs[srcKey{BankSourceKind, strconv.FormatInt(r.ID, 10)}] = d
		}
	}
	if len(expenseIDs) > 0 {
		exps, err := s.q.ResolveExpenseLineSources(ctx, db.ResolveExpenseLineSourcesParams{TaxpayerID: scope.TaxpayerID, Uuids: expenseIDs})
		if err != nil {
			return nil, err
		}
		for _, r := range exps {
			d := docRef{Kind: "expense", ExpenseUUID: r.Uuid, Key: ExpenseDocumentKey(r.Uuid), No: r.ReceiptNumber, TaxID: r.VendorTaxNumber}
			if r.ExpenseDate.Valid {
				t := r.ExpenseDate.Time
				d.Date = &t
			}
			docs[srcKey{"receipt", "expense:" + r.Uuid.String()}] = d
		}
	}
	rows, err := s.q.ResolveInvoiceLineSources(ctx, db.ResolveInvoiceLineSourcesParams{TaxpayerID: scope.TaxpayerID, InvoiceIds: nonNilIDs(invIDs), ItemIds: nonNilIDs(itemIDs)})
	if err != nil {
		return nil, err
	}
	// An invoice that did not come from an integrator (Excel, UBL upload)
	// is taken to be from the taxpayer's integrator when it has exactly one.
	systems, err := s.q.TaxpayerEntegratorSystems(ctx, scope.TaxpayerID)
	if err != nil {
		return nil, err
	}
	fallback := ""
	if len(systems) == 1 {
		fallback, _ = entegrator.ProviderName(systems[0])
	}
	for _, r := range rows {
		d := docRef{Kind: "invoice", InvoiceUUID: r.Uuid, No: r.InvoiceNumber, Key: "inv:" + r.Uuid.String(), TaxID: r.CounterpartyTaxID}
		if r.Ettn.Valid {
			d.ETTN = strings.ToLower(uuid.UUID(r.Ettn.Bytes).String())
			d.Key = d.ETTN
		}
		if r.IssueDate.Valid {
			t := r.IssueDate.Time
			d.Date = &t
		}
		if name, ok := entegrator.ProviderName(r.SourceSystem); ok {
			d.Integrator = name
		} else {
			d.Integrator = fallback
		}
		docs[srcKey{r.SourceKind, r.SourceID}] = d
	}
	for ei := range b.Entries {
		e := &b.Entries[ei]
		seen := map[string]bool{}
		for li := range e.Lines {
			l := &e.Lines[li]
			d, ok := docs[srcKey{l.Source.Kind, l.Source.ID}]
			if !ok {
				continue
			}
			l.Source.DocumentKey, l.Source.ETTN = d.Key, d.ETTN
			if l.DocNo == "" {
				l.DocNo = d.No
			}
			if l.TaxID == "" {
				l.TaxID = d.TaxID
			}
			if l.DocDate == nil && d.Date != nil {
				t := *d.Date
				l.DocDate = &t
			}
			if !seen[d.Key] {
				seen[d.Key] = true
				out[e.ID] = append(out[e.ID], d)
			}
		}
	}
	return out, nil
}

// guard runs the double booking check over the entries that would go to
// the program. confirm lists the document keys (ETTN) the accountant
// confirmed for this export.
func (s *Service) guard(ctx context.Context, scope engagementctx.Scope, driver string, entries []accountingexport.Entry, docs map[string][]docRef, pull []string, confirm []string) (guardResult, error) {
	res := guardResult{Excluded: map[string]bool{}, Confirmed: map[string]map[string][]string{}}
	var keys []string
	for _, e := range entries {
		for _, d := range docs[e.ID] {
			keys = append(keys, d.Key)
		}
	}
	if len(keys) == 0 {
		return res, nil
	}
	prior, err := s.q.ListLiveDocumentExports(ctx, db.ListLiveDocumentExportsParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Driver: driver, DocumentKeys: keys,
	})
	if err != nil {
		return res, err
	}
	previous := map[string]db.ListLiveDocumentExportsRow{}
	for _, p := range prior {
		if _, ok := previous[p.DocumentKey]; !ok {
			previous[p.DocumentKey] = p
		}
	}
	confirmed := map[string]bool{}
	for _, c := range confirm {
		confirmed[strings.ToLower(strings.TrimSpace(c))] = true
	}
	inBatch := map[string]string{} // document key → entry no of its first entry here
	for _, e := range entries {
		for _, d := range docs[e.ID] {
			var codes []string
			params := map[string]any{"document_key": d.Key, "document_no": d.No}
			if d.ETTN != "" {
				params["ettn"] = d.ETTN
			}
			if p, ok := previous[d.Key]; ok {
				codes = append(codes, accountingexport.IssueDocumentAlreadyExported)
				params["previous_batch"] = p.BatchUuid.String()
				params["previous_file"] = p.FileName
				params["previous_entry"] = fmt.Sprintf("%s/%d", p.Period, p.EntryNo)
				params["previous_exported_at"] = p.CreatedAt.Time.UTC().Format(time.RFC3339)
			} else if first, ok := inBatch[d.Key]; ok && first != e.No {
				codes = append(codes, accountingexport.IssueDocumentAlreadyExported)
				params["same_export_entry"] = first
			}
			if _, ok := inBatch[d.Key]; !ok {
				inBatch[d.Key] = e.No
			}
			if driver == lucaDriver && d.ETTN != "" && d.Integrator != "" && slices.Contains(pull, d.Integrator) {
				codes = append(codes, accountingexport.IssueProgramPullsDocument)
				params["integrator"] = d.Integrator
			}
			if len(codes) == 0 {
				continue
			}
			ok := confirmed[d.Key]
			for _, code := range codes {
				p := map[string]any{"confirmed": ok, "excluded": !ok}
				for k, v := range params {
					p[k] = v
				}
				sev := accountingexport.SeverityWarning
				if ok {
					sev = accountingexport.SeverityInfo
				}
				res.Issues = append(res.Issues, accountingexport.Issue{Code: code, Severity: sev, EntryNo: e.No, Params: p})
			}
			if ok {
				if res.Confirmed[e.ID] == nil {
					res.Confirmed[e.ID] = map[string][]string{}
				}
				res.Confirmed[e.ID][d.Key] = codes
			} else {
				res.Excluded[e.ID] = true
			}
		}
	}
	return res, nil
}

const lucaDriver = "luca"

// ExpenseDocumentKey is an expense's document key in the double export
// guard (#780): "exp:<uuid>".
func ExpenseDocumentKey(id uuid.UUID) string { return "exp:" + strings.ToLower(id.String()) }

// BankSourceKind is journal_draft_lines.source_kind of a bank line (the
// same value as banking/posting.DraftSourceKind; source_id is the bank
// movement's id).
const BankSourceKind = "bank_transaction"

// BankDocumentKey is a bank movement's document key in the double export
// guard: "bank:<uuid>" (the Luca list file records the same key).
func BankDocumentKey(id uuid.UUID) string { return "bank:" + strings.ToLower(id.String()) }

// ZReportDocumentKey is a Z report's document key in the double export
// guard: "z:<uuid>" (the Luca list file records the same key).
func ZReportDocumentKey(id uuid.UUID) string { return "z:" + strings.ToLower(id.String()) }

// RetailSaleSourceUUID reads the Z report behind a receipt journal line
// (source_kind receipt, source_id "retail_sale:<uuid>").
func RetailSaleSourceUUID(kind, sourceID string) (uuid.UUID, bool) {
	if kind != "receipt" || !strings.HasPrefix(sourceID, "retail_sale:") {
		return uuid.UUID{}, false
	}
	id, err := uuid.Parse(strings.TrimPrefix(sourceID, "retail_sale:"))
	return id, err == nil
}

// ExpenseSourceUUID reads the expense behind a receipt journal line
// (source_kind receipt, source_id "expense:<uuid>").
func ExpenseSourceUUID(kind, sourceID string) (uuid.UUID, bool) {
	if kind != "receipt" || !strings.HasPrefix(sourceID, "expense:") {
		return uuid.UUID{}, false
	}
	id, err := uuid.Parse(strings.TrimPrefix(sourceID, "expense:"))
	return id, err == nil
}

// lucaPull returns the integrators Luca fetches itself for the engagement:
// the taxpayer's own setting, or the office setting when it has none.
func (s *Service) lucaPull(ctx context.Context, scope engagementctx.Scope, engagement *db.ExportSetting) ([]string, error) {
	if engagement != nil && engagement.LucaPullIntegrators != nil {
		return engagement.LucaPullIntegrators, nil
	}
	return s.officeLucaPull(ctx, scope.OrganizationID)
}

func (s *Service) officeLucaPull(ctx context.Context, orgID int64) ([]string, error) {
	row, err := s.q.GetOfficeExportSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return nonNilStrings(row.LucaPullIntegrators), nil
}

// OfficeSettings are the office-wide export settings (#466).
type OfficeSettings struct {
	// LucaPullIntegrators: integrators Luca fetches e-invoices from itself
	// for the office's taxpayers (isnet, turmob, gib).
	LucaPullIntegrators []string `json:"luca_pull_integrators"`
}

// GetOfficeSettings returns the office settings (empty when none saved).
func (s *Service) GetOfficeSettings(ctx context.Context, orgID int64) (OfficeSettings, error) {
	pull, err := s.officeLucaPull(ctx, orgID)
	if err != nil {
		return OfficeSettings{}, err
	}
	return OfficeSettings{LucaPullIntegrators: pull}, nil
}

// SaveOfficeSettings stores the office settings.
func (s *Service) SaveOfficeSettings(ctx context.Context, orgID, actorID int64, in OfficeSettings) (OfficeSettings, error) {
	pull, err := normalizeIntegrators(in.LucaPullIntegrators)
	if err != nil {
		return OfficeSettings{}, err
	}
	row, err := s.q.UpsertOfficeExportSettings(ctx, db.UpsertOfficeExportSettingsParams{OrganizationID: orgID, LucaPullIntegrators: pull, UpdatedBy: actor(actorID)})
	if err != nil {
		return OfficeSettings{}, err
	}
	return OfficeSettings{LucaPullIntegrators: nonNilStrings(row.LucaPullIntegrators)}, nil
}

// normalizeIntegrators checks, de-duplicates and sorts an integrator list.
func normalizeIntegrators(in []string) ([]string, error) {
	out := []string{}
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if !slices.Contains(LucaIntegrators, v) {
			return nil, fmt.Errorf("%w: %q (allowed: %s)", ErrInvalidIntegrator, v, strings.Join(LucaIntegrators, ", "))
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out, nil
}

// ExportDocument is one document → file trail row of the export history.
type ExportDocument struct {
	DocumentKey    string     `json:"document_key"`
	ETTN           string     `json:"ettn,omitempty"`
	InvoiceUUID    *uuid.UUID `json:"invoice_uuid,omitempty"`
	DocumentNo     string     `json:"document_no,omitempty"`
	DocumentDate   *time.Time `json:"document_date,omitempty"`
	FileName       string     `json:"file_name"`
	Batch          uuid.UUID  `json:"batch_uuid"`
	Driver         string     `json:"driver"`
	Draft          uuid.UUID  `json:"journal_draft_uuid"`
	Period         string     `json:"period"`
	EntryNo        int32      `json:"entry_no"`
	ConfirmedCodes []string   `json:"confirmed_codes"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
	ExportedAt     time.Time  `json:"exported_at"`
	ReversedAt     *time.Time `json:"reversed_at,omitempty"`
}

// DocumentFilter narrows ListDocuments.
type DocumentFilter struct {
	Batch  *uuid.UUID
	Q      string
	Limit  int32
	Offset int32
}

// ListDocuments lists the document → file trail, newest first.
func (s *Service) ListDocuments(ctx context.Context, scope engagementctx.Scope, f DocumentFilter) ([]ExportDocument, error) {
	p := db.ListExportDocumentsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Lim: f.Limit, Off: f.Offset}
	if f.Batch != nil {
		b, err := s.q.GetExportBatch(ctx, db.GetExportBatchParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: *f.Batch})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		p.BatchID = pgtype.Int8{Int64: b.ID, Valid: true}
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListExportDocuments(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]ExportDocument, 0, len(rows))
	for _, r := range rows {
		d := ExportDocument{
			DocumentKey: r.DocumentKey, DocumentNo: r.DocumentNo.String, FileName: r.FileName, Batch: r.BatchUuid, Driver: r.Driver,
			Draft: r.JournalDraftUuid, Period: r.Period, EntryNo: r.EntryNo, ConfirmedCodes: nonNilStrings(r.ConfirmedCodes), ExportedAt: r.CreatedAt.Time,
		}
		// An ETTN is a bare uuid; other keys carry a prefix (inv:, exp:,
		// bank:, z:).
		if !strings.Contains(r.DocumentKey, ":") {
			d.ETTN = r.DocumentKey
		}
		if r.InvoiceUuid.Valid {
			id := uuid.UUID(r.InvoiceUuid.Bytes)
			d.InvoiceUUID = &id
		}
		if r.DocumentDate.Valid {
			t := r.DocumentDate.Time
			d.DocumentDate = &t
		}
		if r.ConfirmedAt.Valid {
			t := r.ConfirmedAt.Time
			d.ConfirmedAt = &t
		}
		if r.ReversedAt.Valid {
			t := r.ReversedAt.Time
			d.ReversedAt = &t
		}
		out = append(out, d)
	}
	return out, nil
}

func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
