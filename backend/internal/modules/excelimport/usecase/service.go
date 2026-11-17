// Package usecase runs the Excel import flow (04 §3): upload → analyze
// (template or AI mapping) → mapping_review → validate → preview → commit
// (accountant only, idempotent) → rollback.
package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/build"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/target"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Statuses (04 §3).
const (
	StatusUploaded      = "uploaded"
	StatusAnalyzing     = "analyzing"
	StatusMappingReview = "mapping_review"
	StatusValidating    = "validating"
	StatusHasErrors     = "has_errors"
	StatusPreview       = "preview"
	StatusCommitted     = "committed"
	StatusFailed        = "failed"
	StatusRolledBack    = "rolled_back"
)

// Template scopes (04 §6).
const (
	ScopeCompany = "company"
	ScopeTenant  = "tenant"
	ScopeGlobal  = "global"
)

// Errors.
var (
	ErrNotFound    = errors.New("excelimport: not found")
	ErrInvalid     = errors.New("excelimport: invalid input")
	ErrState       = errors.New("excelimport: not allowed in the import's status")
	ErrIdempotency = errors.New("excelimport: import already committed with another Idempotency-Key")
	// ErrNoRollback: only invoice imports are rolled back here; the other
	// data types are corrected in their own screens (#395).
	ErrNoRollback = errors.New("excelimport: bu veri türünün içe aktarması geri alınamaz; kayıtları kendi ekranından düzeltin")
)

// DB is the Postgres pool subset.
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ObjectStore is the storage.Driver subset.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// Service runs Excel imports.
type Service struct {
	pool  DB
	q     *db.Queries
	store ObjectStore
	ai    *mapping.AIMapper
	log   *slog.Logger
	// targets write the non-invoice data types (#395).
	targets map[string]target.Target
}

// New builds the service; ai may be nil (mapping is then manual).
func New(pool DB, store ObjectStore, ai mapping.Completer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	var m *mapping.AIMapper
	if ai != nil {
		m = mapping.NewAIMapper(ai)
	}
	return &Service{pool: pool, q: database.QueriesFor(pool), store: store, ai: m, log: log, targets: map[string]target.Target{}}
}

// WithTarget registers the module that writes dataType (payroll, expenses,
// retail_sales, bank_transactions).
func (s *Service) WithTarget(dataType string, t target.Target) *Service {
	if mapping.Supported(dataType) && !mapping.IsInvoice(dataType) && t != nil {
		s.targets[dataType] = t
	}
	return s
}

// Available reports whether dataType can be committed by this server.
func (s *Service) Available(dataType string) bool {
	if mapping.IsInvoice(dataType) {
		return true
	}
	_, ok := s.targets[dataType]
	return ok
}

// DataType is one data type of the catalog endpoint.
type DataType struct {
	mapping.Catalog
	Available bool `json:"available"`
}

// DataTypes lists the importable data types with their fields.
func (s *Service) DataTypes() []DataType {
	cats := mapping.Catalogs()
	out := make([]DataType, 0, len(cats))
	for _, c := range cats {
		out = append(out, DataType{Catalog: c, Available: s.Available(c.DataType)})
	}
	return out
}

// UploadOptions are the optional upload fields (04 §7).
type UploadOptions struct {
	DataType     string
	SourceSystem string
	Period       string
	Sheet        string
}

var (
	periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	sourceRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)
)

// Upload stores the file and analyzes it at once: template, else AI,
// else manual mapping.
func (s *Service) Upload(ctx context.Context, sc engagementctx.Scope, userID int64, fileName string, data []byte, opt UploadOptions) (View, error) {
	if opt.DataType != "" && !mapping.Supported(opt.DataType) {
		return View{}, fmt.Errorf("%w: data_type must be one of %v", ErrInvalid, mapping.SupportedDataTypes)
	}
	if opt.DataType != "" && !s.Available(opt.DataType) {
		return View{}, fmt.Errorf("%w: data_type %s is not enabled on this server", ErrInvalid, opt.DataType)
	}
	if opt.Period != "" && !periodRe.MatchString(opt.Period) {
		return View{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if opt.SourceSystem != "" && !sourceRe.MatchString(opt.SourceSystem) {
		return View{}, fmt.Errorf("%w: source_system must be snake_case", ErrInvalid)
	}
	format, sheets, perr := parse.Read(fileName, data)
	if perr != nil && (errors.Is(perr, parse.ErrTooLarge) || errors.Is(perr, parse.ErrUnsupported) || errors.Is(perr, parse.ErrEmpty)) {
		return View{}, fmt.Errorf("%w: %v", ErrInvalid, perr)
	}
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	name := cleanName(fileName)
	key := fmt.Sprintf("excel-imports/%d/%s/%s", sc.TaxpayerID, uuid.NewString(), name)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), Filename: name}, key); err != nil {
		return View{}, fmt.Errorf("excelimport: store file: %w", err)
	}
	att, err := s.q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: sc.TaxpayerID, Kind: "source_file", StorageKey: key, Sha256: hexSum,
		SizeBytes: int64(len(data)), FileName: text(name),
	})
	if err != nil {
		return View{}, fmt.Errorf("excelimport: attachment: %w", err)
	}
	if format == "" {
		format = parse.FormatCSV
	}
	row, err := s.q.CreateExcelImport(ctx, db.CreateExcelImportParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, CreatedBy: optID(userID),
		FileName: name, Format: string(format), Sha256: hexSum, SizeBytes: int64(len(data)),
		StorageKey: att.StorageKey, AttachmentID: pgtype.Int8{Int64: att.ID, Valid: true},
		DataType: text(opt.DataType), SourceSystem: text(opt.SourceSystem), Period: text(opt.Period), Status: StatusAnalyzing,
	})
	if err != nil {
		return View{}, err
	}
	if perr != nil {
		return s.fail(ctx, row, perr)
	}
	return s.analyze(ctx, row, sheets, opt, userID)
}

func (s *Service) analyze(ctx context.Context, row db.ExcelImport, sheets []parse.Sheet, opt UploadOptions, userID int64) (View, error) {
	sheet, layout, ok := pickSheet(sheets, opt.Sheet)
	if !ok {
		return s.fail(ctx, row, errors.New("no header row found in the first 50 rows"))
	}
	dataType := opt.DataType
	var (
		mp       mapping.Mapping
		template *db.ExcelImportTemplate
		aiUsed   bool
	)
	tpls, err := s.q.FindExcelImportTemplates(ctx, db.FindExcelImportTemplatesParams{
		Fingerprint: layout.Fingerprint, DataType: text(dataType), OrganizationID: pgtype.Int8{Int64: row.OrganizationID, Valid: true},
		TaxpayerID: pgtype.Int8{Int64: row.TaxpayerID, Valid: true},
	})
	if err != nil {
		return View{}, err
	}
	if len(tpls) > 0 {
		t := tpls[0]
		template = &t
		mp, err = fromTemplate(t, layout)
		if err != nil {
			template = nil
		}
	}
	// The owning module's header names come before AI: its own export and
	// the common program layouts map without a model call (#395).
	var heuristic *mapping.Mapping
	if t, ok := s.targets[dataType]; ok && template == nil {
		h := t.Heuristic(layout)
		h.DataType = dataType
		heuristic = &h
	}
	switch {
	case template != nil:
	case heuristic != nil && len(mapping.MissingRequired(dataType, *heuristic)) == 0:
		mp = *heuristic
	default:
		mp, err = s.ai.Map(ctx, mapping.AIRequest{
			OrganizationID: row.OrganizationID, TaxpayerID: row.TaxpayerID, UserID: userID,
			DataType: dataType, Sheet: sheet, Layout: layout,
		})
		switch {
		case errors.Is(err, mapping.ErrAIUnavailable):
			mp = mapping.Unmapped(dataType, layout, "AI yapılandırılmamış; kolonları müşavir eşler")
		case err != nil:
			s.log.Warn("excel_import_ai_failed", "import", row.Uuid, "error", err)
			mp = mapping.Unmapped(dataType, layout, "AI çağrısı başarısız; kolonları müşavir eşler")
		default:
			aiUsed = true
		}
		if !aiUsed && heuristic != nil {
			// Keep what the headers told; the accountant maps the rest.
			w := append(mp.Warnings, "Kolonların bir kısmı başlıklardan eşlendi; eksik alanları müşavir eşler")
			mp = *heuristic
			mp.Warnings = w
			for i := range mp.Columns {
				mp.Columns[i].NeedsReview = mp.Columns[i].Field == ""
			}
		}
	}
	if opt.SourceSystem != "" {
		mp.SourceSystem = opt.SourceSystem
	}
	if dataType != "" {
		mp.DataType = dataType
	}
	status := StatusMappingReview
	if mp.Validate() == nil && !mp.NeedsReview() {
		status = StatusValidating
	}
	layoutJSON, _ := json.Marshal(layout)
	mapJSON, _ := json.Marshal(mp)
	var tplID pgtype.Int8
	if template != nil {
		tplID = pgtype.Int8{Int64: template.ID, Valid: true}
	}
	row, err = s.q.UpdateExcelImportAnalysis(ctx, db.UpdateExcelImportAnalysisParams{
		Status: status, DataType: text(mp.DataType), SourceSystem: text(mp.SourceSystem), SheetName: text(sheet.Name),
		Layout: layoutJSON, Mapping: mapJSON, TemplateID: tplID, AiUsed: aiUsed, Summary: []byte("{}"),
		OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ID: row.ID,
	})
	if err != nil {
		return View{}, err
	}
	if status == StatusValidating {
		return s.validate(ctx, row, sheet)
	}
	return s.view(row), nil
}

// MappingInput is the accountant's corrected mapping (04 §7 PUT mapping).
type MappingInput struct {
	DataType     string `json:"data_type"`
	SourceSystem string `json:"source_system"`
	Columns      []struct {
		Column string `json:"column"`
		Field  string `json:"field"`
	} `json:"columns"`
	// SaveTemplate: "", "company" or "tenant".
	SaveTemplate string `json:"save_template"`
}

// SetMapping stores the accountant's mapping, optionally saves it as a
// template, and validates.
func (s *Service) SetMapping(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, in MappingInput) (View, error) {
	row, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	switch row.Status {
	case StatusMappingReview, StatusHasErrors, StatusPreview:
	default:
		return View{}, ErrState
	}
	var layout parse.Layout
	var cur mapping.Mapping
	_ = json.Unmarshal(row.Layout, &layout)
	_ = json.Unmarshal(row.Mapping, &cur)
	fields := map[string]string{}
	for _, c := range in.Columns {
		fields[strings.ToUpper(strings.TrimSpace(c.Column))] = strings.TrimSpace(c.Field)
	}
	mp := mapping.Mapping{DataType: firstNonEmpty(in.DataType, cur.DataType), SourceSystem: firstNonEmpty(in.SourceSystem, cur.SourceSystem), HeaderRow: layout.HeaderRow}
	if mp.SourceSystem != "" && mp.SourceSystem != "unknown" && !sourceRe.MatchString(mp.SourceSystem) {
		return View{}, fmt.Errorf("%w: source_system must be snake_case", ErrInvalid)
	}
	prev := map[string]mapping.ColumnMap{}
	for _, c := range cur.Columns {
		prev[c.Column] = c
	}
	for _, col := range layout.Columns {
		cm := prev[col.Letter]
		cm.Column, cm.Header = col.Letter, col.Header
		if f, ok := fields[col.Letter]; ok {
			if f != cm.Field {
				cm.Origin, cm.Confidence = mapping.OriginUser, 1
			}
			cm.Field = f
		}
		// The accountant has looked at the mapping now.
		cm.NeedsReview = false
		mp.Columns = append(mp.Columns, cm)
	}
	if err := mp.Validate(); err != nil {
		return View{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !s.Available(mp.DataType) {
		return View{}, fmt.Errorf("%w: data_type %s is not enabled on this server", ErrInvalid, mp.DataType)
	}
	if missing := mapping.MissingRequired(mp.DataType, mp); len(missing) > 0 {
		return View{}, fmt.Errorf("%w: required fields not mapped: %v", ErrInvalid, missing)
	}
	var tplID pgtype.Int8
	if row.TemplateID.Valid {
		tplID = row.TemplateID
	}
	switch in.SaveTemplate {
	case "":
	case ScopeCompany, ScopeTenant:
		t, err := s.saveTemplate(ctx, row, layout, mp, in.SaveTemplate, userID)
		if err != nil {
			return View{}, err
		}
		tplID = pgtype.Int8{Int64: t.ID, Valid: true}
	default:
		return View{}, fmt.Errorf("%w: save_template must be company or tenant", ErrInvalid)
	}
	mapJSON, _ := json.Marshal(mp)
	row, err = s.q.UpdateExcelImportAnalysis(ctx, db.UpdateExcelImportAnalysisParams{
		Status: StatusValidating, DataType: text(mp.DataType), SourceSystem: text(mp.SourceSystem), SheetName: row.SheetName,
		Layout: row.Layout, Mapping: mapJSON, TemplateID: tplID, AiUsed: row.AiUsed, Summary: []byte("{}"),
		OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ID: row.ID,
	})
	if err != nil {
		return View{}, err
	}
	sheet, err := s.loadSheet(ctx, row)
	if err != nil {
		return s.fail(ctx, row, err)
	}
	return s.validate(ctx, row, sheet)
}

// Revalidate runs validation again (04 §7 POST validate).
func (s *Service) Revalidate(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (View, error) {
	row, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	switch row.Status {
	case StatusHasErrors, StatusPreview, StatusValidating:
	default:
		return View{}, ErrState
	}
	sheet, err := s.loadSheet(ctx, row)
	if err != nil {
		return s.fail(ctx, row, err)
	}
	return s.validate(ctx, row, sheet)
}

// SkipRow marks a row to be skipped (or not) and refreshes the summary.
func (s *Service) SkipRow(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, rowNo int32, skip bool) (View, error) {
	row, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	if row.Status != StatusHasErrors && row.Status != StatusPreview {
		return View{}, ErrState
	}
	n, err := s.q.SetExcelImportRowSkipped(ctx, db.SetExcelImportRowSkippedParams{
		Skipped: skip, OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID, RowNo: rowNo,
	})
	if err != nil {
		return View{}, err
	}
	if n == 0 {
		return View{}, ErrNotFound
	}
	sheet, err := s.loadSheet(ctx, row)
	if err != nil {
		return s.fail(ctx, row, err)
	}
	return s.validate(ctx, row, sheet)
}

// validate converts the rows, checks the database for existing records and
// stores the row results and the preview summary.
func (s *Service) validate(ctx context.Context, row db.ExcelImport, sheet parse.Sheet) (View, error) {
	var layout parse.Layout
	var mp mapping.Mapping
	_ = json.Unmarshal(row.Layout, &layout)
	_ = json.Unmarshal(row.Mapping, &mp)
	skipped := map[int32]bool{}
	if nums, err := s.q.ListSkippedExcelImportRows(ctx, db.ListSkippedExcelImportRowsParams{
		OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID,
	}); err == nil {
		for _, n := range nums {
			skipped[n] = true
		}
	}
	invoice := mapping.IsInvoice(mp.DataType)
	var rows []build.Row
	if invoice {
		rows = build.Invoices(sheet, layout, mp, build.Options{Period: row.Period.String})
	} else {
		rows = build.Records(sheet, layout, mp, build.Options{Period: row.Period.String})
	}
	existing := 0
	for i := range rows {
		if skipped[int32(rows[i].RowNo)] { //nolint:gosec // row numbers are bounded by MaxRows
			rows[i].Result = build.ResultSkipped
			continue
		}
		inv := rows[i].Invoice
		if !invoice || inv == nil || rows[i].Result == build.ResultError {
			continue
		}
		if _, err := s.q.GetInvoiceBySource(ctx, db.GetInvoiceBySourceParams{TaxpayerID: row.TaxpayerID, SourceSystem: inv.SourceSystem, SourceID: inv.SourceID}); err == nil {
			existing++
			rows[i].Messages = append(rows[i].Messages, build.Message{Code: "EXISTING", Severity: "warning", Detail: "already imported; will be skipped"})
			if rows[i].Result == build.ResultOK {
				rows[i].Result = build.ResultWarning
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return View{}, err
		}
	}
	sum := build.Summarize(rows)
	status := StatusPreview
	if sum.Errors > 0 {
		status = StatusHasErrors
	}
	summary := Summary{Summary: sum, Existing: existing}
	sumJSON, _ := json.Marshal(summary)

	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.DeleteExcelImportRows(ctx, db.DeleteExcelImportRowsParams{OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID}); err != nil {
		return View{}, err
	}
	for _, r := range rows {
		raw, _ := json.Marshal(r.Raw)
		msgs, _ := json.Marshal(r.Messages)
		var rec []byte
		switch {
		case r.Invoice != nil:
			rec, _ = json.Marshal(r.Invoice)
		case r.Record != nil:
			rec, _ = json.Marshal(r.Record)
		}
		if err := q.InsertExcelImportRow(ctx, db.InsertExcelImportRowParams{
			OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID,
			RowNo: int32(r.RowNo), Raw: raw, Record: rec, Result: r.Result, Messages: msgs, //nolint:gosec // bounded
			Skipped: r.Result == build.ResultSkipped,
		}); err != nil {
			return View{}, err
		}
	}
	row, err = q.UpdateExcelImportAnalysis(ctx, db.UpdateExcelImportAnalysisParams{
		Status: status, DataType: row.DataType, SourceSystem: row.SourceSystem, SheetName: row.SheetName,
		Layout: row.Layout, Mapping: row.Mapping, TemplateID: row.TemplateID, AiUsed: row.AiUsed, Summary: sumJSON,
		OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ID: row.ID,
	})
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return s.view(row), nil
}

// Commit writes the importable rows to the common model (accountant only;
// the route enforces tenant.excel_imports.commit). The same Idempotency-Key
// returns the committed import again.
func (s *Service) Commit(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64, key string) (View, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 {
		return View{}, fmt.Errorf("%w: Idempotency-Key header is required (max 128)", ErrInvalid)
	}
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	row, err := q.GetExcelImportForUpdate(ctx, db.GetExcelImportForUpdateParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	if row.Status == StatusCommitted {
		if row.IdempotencyKey.String == key {
			return s.view(row), nil
		}
		return View{}, ErrIdempotency
	}
	if row.Status != StatusPreview {
		return View{}, ErrState
	}
	rows, err := q.ListAllExcelImportRows(ctx, db.ListAllExcelImportRowsParams{OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID})
	if err != nil {
		return View{}, err
	}
	var summary Summary
	_ = json.Unmarshal(row.Summary, &summary)
	summary.Added, summary.Duplicates = 0, 0
	if dt := row.DataType.String; !mapping.IsInvoice(dt) {
		if err := s.commitRecords(ctx, sc, userID, row, rows, &summary); err != nil {
			return View{}, err
		}
		rows = nil
	}
	for _, r := range rows {
		if r.Skipped || (r.Result != build.ResultOK && r.Result != build.ResultWarning) || len(r.Record) == 0 {
			continue
		}
		var inv model.Invoice
		if err := json.Unmarshal(r.Record, &inv); err != nil {
			return View{}, fmt.Errorf("excelimport: row %d: %w", r.RowNo, err)
		}
		res, err := repository.SaveInvoice(ctx, tx, row.TaxpayerID, inv, 0)
		if err != nil {
			return View{}, fmt.Errorf("excelimport: row %d: %w", r.RowNo, err)
		}
		if res.Duplicate() {
			summary.Duplicates++
		} else {
			summary.Added++
		}
		if err := q.SetExcelImportRowOutcome(ctx, db.SetExcelImportRowOutcomeParams{
			InvoiceID: pgtype.Int8{Int64: res.InvoiceID, Valid: true}, Duplicate: res.Duplicate(),
			OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID, RowNo: r.RowNo,
		}); err != nil {
			return View{}, err
		}
	}
	sumJSON, _ := json.Marshal(summary)
	row, err = q.MarkExcelImportCommitted(ctx, db.MarkExcelImportCommittedParams{
		Summary: sumJSON, IdempotencyKey: text(key), CommittedBy: optID(userID),
		OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ID: row.ID,
	})
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return s.view(row), nil
}

// Rollback deletes the invoices this import created (04 §3 rolled_back).
// Invoices that existed before (duplicates) are left alone.
func (s *Service) Rollback(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, userID int64) (View, error) {
	tx, err := database.Begin(ctx, s.pool)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	row, err := q.GetExcelImportForUpdate(ctx, db.GetExcelImportForUpdateParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	if row.Status != StatusCommitted {
		return View{}, ErrState
	}
	if !mapping.IsInvoice(row.DataType.String) {
		return View{}, ErrNoRollback
	}
	ids, err := q.ListExcelImportCreatedInvoices(ctx, db.ListExcelImportCreatedInvoicesParams{OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID})
	if err != nil {
		return View{}, err
	}
	removed := 0
	for _, invID := range ids {
		n, err := q.DeleteExcelImportedInvoice(ctx, db.DeleteExcelImportedInvoiceParams{TaxpayerID: row.TaxpayerID, ID: invID})
		if err != nil {
			return View{}, err
		}
		removed += int(n)
	}
	var summary Summary
	_ = json.Unmarshal(row.Summary, &summary)
	summary.RolledBack = removed
	sumJSON, _ := json.Marshal(summary)
	row, err = q.MarkExcelImportRolledBack(ctx, db.MarkExcelImportRolledBackParams{
		Summary: sumJSON, RolledBackBy: optID(userID), OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ID: row.ID,
	})
	if err != nil {
		return View{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return s.view(row), nil
}

// Get returns one import.
func (s *Service) Get(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (View, error) {
	row, err := s.get(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	return s.view(row), nil
}

// List returns one page of imports.
func (s *Service) List(ctx context.Context, sc engagementctx.Scope, status string, limit, offset int32) ([]View, int64, error) {
	rows, err := s.q.ListExcelImports(ctx, db.ListExcelImportsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Status: text(status), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountExcelImports(ctx, db.CountExcelImportsParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Status: text(status)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(r))
	}
	return out, total, nil
}

// RowView is one row result.
type RowView struct {
	RowNo     int32           `json:"row_no"`
	Result    string          `json:"result"`
	Skipped   bool            `json:"skipped"`
	Raw       json.RawMessage `json:"raw"`
	Messages  json.RawMessage `json:"messages"`
	Duplicate bool            `json:"duplicate"`
	Written   bool            `json:"written"`
}

// Rows returns one page of row results (filter: ok, warning, error, skipped).
func (s *Service) Rows(ctx context.Context, sc engagementctx.Scope, id uuid.UUID, result string, limit, offset int32) ([]RowView, int64, error) {
	row, err := s.get(ctx, sc, id)
	if err != nil {
		return nil, 0, err
	}
	switch result {
	case "", build.ResultOK, build.ResultWarning, build.ResultError, build.ResultSkipped:
	default:
		return nil, 0, fmt.Errorf("%w: result", ErrInvalid)
	}
	rs, err := s.q.ListExcelImportRows(ctx, db.ListExcelImportRowsParams{OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID, Result: text(result), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountExcelImportRows(ctx, db.CountExcelImportRowsParams{OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ImportID: row.ID, Result: text(result)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]RowView, 0, len(rs))
	for _, r := range rs {
		out = append(out, RowView{RowNo: r.RowNo, Result: r.Result, Skipped: r.Skipped, Raw: r.Raw, Messages: r.Messages,
			Duplicate: r.Duplicate, Written: r.InvoiceID.Valid && !r.Duplicate})
	}
	return out, total, nil
}

// TemplateView is a stored template.
type TemplateView struct {
	UUID         uuid.UUID       `json:"uuid"`
	Scope        string          `json:"scope"`
	Fingerprint  string          `json:"fingerprint"`
	DataType     string          `json:"data_type"`
	SourceSystem string          `json:"source_system"`
	HeaderRow    int32           `json:"header_row"`
	Columns      json.RawMessage `json:"columns"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Templates lists the office's templates and the global ones.
func (s *Service) Templates(ctx context.Context, organizationID int64) ([]TemplateView, error) {
	rows, err := s.q.ListExcelImportTemplates(ctx, pgtype.Int8{Int64: organizationID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]TemplateView, 0, len(rows))
	for _, t := range rows {
		out = append(out, TemplateView{UUID: t.Uuid, Scope: t.Scope, Fingerprint: t.Fingerprint, DataType: t.DataType,
			SourceSystem: t.SourceSystem, HeaderRow: t.HeaderRow, Columns: t.Columns, UpdatedAt: t.UpdatedAt.Time})
	}
	return out, nil
}

// DeleteTemplate removes one of the office's templates.
func (s *Service) DeleteTemplate(ctx context.Context, organizationID int64, id uuid.UUID) error {
	n, err := s.q.DeleteExcelImportTemplate(ctx, db.DeleteExcelImportTemplateParams{OrganizationID: pgtype.Int8{Int64: organizationID, Valid: true}, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) saveTemplate(ctx context.Context, row db.ExcelImport, l parse.Layout, mp mapping.Mapping, scope string, userID int64) (db.ExcelImportTemplate, error) {
	cols := make([]mapping.ColumnMap, len(mp.Columns))
	for i, c := range mp.Columns {
		cols[i] = mapping.ColumnMap{Column: c.Column, Header: c.Header, Field: c.Field, Confidence: 1, Origin: mapping.OriginTemplate}
	}
	colsJSON, _ := json.Marshal(cols)
	p := db.UpsertExcelImportTemplateParams{
		Scope: scope, OrganizationID: pgtype.Int8{Int64: row.OrganizationID, Valid: true}, Fingerprint: l.Fingerprint,
		DataType: mp.DataType, SourceSystem: firstNonEmpty(mp.SourceSystem, "unknown"), HeaderRow: int32(l.HeaderRow), //nolint:gosec // ≤ 50
		Columns: colsJSON, CreatedBy: optID(userID),
	}
	if scope == ScopeCompany {
		p.TaxpayerID = pgtype.Int8{Int64: row.TaxpayerID, Valid: true}
	}
	return s.q.UpsertExcelImportTemplate(ctx, p)
}

// fromTemplate applies a template by header order (the fingerprint
// guarantees the same ordered headers; letters may shift).
func fromTemplate(t db.ExcelImportTemplate, l parse.Layout) (mapping.Mapping, error) {
	var cols []mapping.ColumnMap
	if err := json.Unmarshal(t.Columns, &cols); err != nil {
		return mapping.Mapping{}, err
	}
	if len(cols) != len(l.Columns) {
		return mapping.Mapping{}, errors.New("template column count differs")
	}
	mp := mapping.Mapping{DataType: t.DataType, SourceSystem: t.SourceSystem, HeaderRow: l.HeaderRow}
	for i, c := range l.Columns {
		mp.Columns = append(mp.Columns, mapping.ColumnMap{Column: c.Letter, Header: c.Header, Field: cols[i].Field, Confidence: 1, Origin: mapping.OriginTemplate})
	}
	return mp, mp.Validate()
}

func (s *Service) loadSheet(ctx context.Context, row db.ExcelImport) (parse.Sheet, error) {
	rc, _, err := s.store.Download(ctx, row.StorageKey)
	if err != nil {
		return parse.Sheet{}, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, parse.MaxBytes+1))
	if err != nil {
		return parse.Sheet{}, err
	}
	_, sheets, err := parse.Read(row.FileName, data)
	if err != nil {
		return parse.Sheet{}, err
	}
	for _, sh := range sheets {
		if sh.Name == row.SheetName.String {
			return sh, nil
		}
	}
	return parse.Sheet{}, errors.New("sheet not found")
}

func (s *Service) fail(ctx context.Context, row db.ExcelImport, cause error) (View, error) {
	row, err := s.q.UpdateExcelImportAnalysis(ctx, db.UpdateExcelImportAnalysisParams{
		Status: StatusFailed, DataType: row.DataType, SourceSystem: row.SourceSystem, SheetName: row.SheetName,
		Layout: row.Layout, Mapping: row.Mapping, TemplateID: row.TemplateID, AiUsed: row.AiUsed, Summary: row.Summary,
		Error: text(cause.Error()), OrganizationID: row.OrganizationID, EngagementID: row.EngagementID, ID: row.ID,
	})
	if err != nil {
		return View{}, err
	}
	return s.view(row), nil
}

func (s *Service) get(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (db.ExcelImport, error) {
	row, err := s.q.GetExcelImport(ctx, db.GetExcelImportParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ExcelImport{}, ErrNotFound
	}
	return row, err
}

func pickSheet(sheets []parse.Sheet, name string) (parse.Sheet, parse.Layout, bool) {
	for _, sh := range sheets {
		if name != "" && sh.Name != name {
			continue
		}
		if l, ok := parse.DetectLayout(sh); ok {
			return sh, l, true
		}
	}
	return parse.Sheet{}, parse.Layout{}, false
}

func cleanName(n string) string {
	n = path.Base(strings.ReplaceAll(n, "\\", "/"))
	if n == "." || n == "/" || n == "" {
		return "upload"
	}
	if len(n) > 200 {
		n = n[:200]
	}
	return n
}

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func optID(v int64) pgtype.Int8 {
	if v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// commitRecords hands the accepted rows of a non-invoice import to the
// module that owns the data type (#395). The module writes in its own
// transaction while the import row stays locked; its duplicate rules make a
// retry after a failure safe.
func (s *Service) commitRecords(ctx context.Context, sc engagementctx.Scope, userID int64, row db.ExcelImport, rows []db.ExcelImportRow, summary *Summary) error {
	t, ok := s.targets[row.DataType.String]
	if !ok {
		return fmt.Errorf("%w: data_type %s is not enabled on this server", ErrInvalid, row.DataType.String)
	}
	var mp mapping.Mapping
	_ = json.Unmarshal(row.Mapping, &mp)
	in := target.Input{Scope: sc, UserID: userID, FileName: row.FileName, Period: row.Period.String, SourceSystem: mp.SourceSystem}
	for _, r := range rows {
		if r.Skipped || (r.Result != build.ResultOK && r.Result != build.ResultWarning) || len(r.Record) == 0 {
			continue
		}
		var vals map[string]string
		if err := json.Unmarshal(r.Record, &vals); err != nil {
			return fmt.Errorf("excelimport: row %d: %w", r.RowNo, err)
		}
		in.Rows = append(in.Rows, target.Row{RowNo: int(r.RowNo), Values: vals})
	}
	if len(in.Rows) == 0 {
		return fmt.Errorf("%w: no importable rows", ErrInvalid)
	}
	res, err := t.Commit(ctx, in)
	if errors.Is(err, target.ErrRejected) {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err != nil {
		return err
	}
	summary.Added, summary.Duplicates, summary.Rejected = res.Added, res.Duplicates, len(res.Errors)
	summary.RowErrors, summary.Note = res.Errors, res.Note
	return nil
}
