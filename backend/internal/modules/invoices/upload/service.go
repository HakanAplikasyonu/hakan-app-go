// Package upload is the UBL / ZIP upload channel (issue #11): the office
// uploads XML or ZIP files downloaded from an e-invoice portal; a background
// job validates (XSD + GİB Schematron), maps (`31`) and stores every invoice,
// then writes a report of added / duplicate / failed files.
package upload

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
	"strings"
	"time"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/queue"
)

// MaxUploadBytes caps one uploaded file (a ZIP of many invoices).
const MaxUploadBytes = 200 << 20

// maxCandidates caps the invoice ids listed in a POSSIBLE_DUPLICATE warning.
const maxCandidates = 10

// Producer is the data_warning producer name for this channel (`05` §1).
const Producer = "ubl_parser"

var (
	ErrUnsupported = errors.New("upload: file is not XML, ZIP or base64")
	ErrTooLarge    = errors.New("upload: file too large")
	ErrEmpty       = errors.New("upload: empty file")
	ErrNotFound    = errors.New("upload: job not found")
	ErrNoOwnTaxID  = errors.New("upload: taxpayer has no VKN/TCKN on record")
)

// DB is what the service needs from Postgres.
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Enqueuer is the Asynq client subset.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Validator runs GİB validation; ubltrValidator is the real one.
type Validator interface {
	XSD(ctx context.Context, xml []byte) error
	Schematron(ctx context.Context, xml []byte, docType string) error
}

type ubltrValidator struct{}

func (ubltrValidator) XSD(ctx context.Context, b []byte) error { return ubltr.ValidateXSD(ctx, b) }
func (ubltrValidator) Schematron(ctx context.Context, b []byte, t string) error {
	return ubltr.ValidateSchematronType(ctx, b, t)
}

// ObjectStore is the storage.Driver subset the service uses.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Exists(ctx context.Context, path string) (bool, error)
}

// Service runs uploads.
type Service struct {
	db        DB
	q         *db.Queries
	store     ObjectStore
	queue     Enqueuer // nil: process inline (tests, no Redis)
	validator Validator
	limits    ublmap.Limits
	log       *slog.Logger
	ownTaxID  func(ctx context.Context, taxpayerID int64) (string, error)
}

// New builds the service. enq may be nil.
func New(pool DB, store ObjectStore, enq Enqueuer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{db: pool, q: db.New(pool), store: store, queue: enq, validator: ubltrValidator{}, limits: ublmap.DefaultLimits, log: log}
	s.ownTaxID = s.q.GetTaxpayerOwnTaxID
	return s
}

// WithOwnTaxIDResolver replaces how the taxpayer's own VKN/TCKN is read
// (tests).
func (s *Service) WithOwnTaxIDResolver(fn func(ctx context.Context, taxpayerID int64) (string, error)) *Service {
	s.ownTaxID = fn
	return s
}

// WithValidator swaps the validator (tests).
func (s *Service) WithValidator(v Validator) *Service { s.validator = v; return s }

// Job is the API view of an upload job.
type Job struct {
	UUID       uuid.UUID  `json:"uuid"`
	FileName   string     `json:"file_name"`
	SizeBytes  int64      `json:"size_bytes"`
	Status     string     `json:"status"`
	Report     *Report    `json:"report,omitempty"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Item statuses in a report.
const (
	ItemAdded     = "added"
	ItemDuplicate = "duplicate"
	ItemFailed    = "failed"
)

// Report is the upload result (issue #11: eklenen / mükerrer / hatalı).
type Report struct {
	Total     int          `json:"total"`
	Added     int          `json:"added"`
	Duplicate int          `json:"duplicate"`
	Failed    int          `json:"failed"`
	Items     []ItemResult `json:"items"`
}

// ItemResult is one document of the upload.
type ItemResult struct {
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	ETTN          string    `json:"ettn,omitempty"`
	InvoiceNumber string    `json:"invoice_number,omitempty"`
	InvoiceUUID   uuid.UUID `json:"invoice_uuid,omitempty"`
	// Outcome / MatchRule: how the invoice was matched (`01` §6, #12).
	Outcome   string   `json:"outcome,omitempty"`
	MatchRule string   `json:"match_rule,omitempty"`
	ErrorCode string   `json:"error_code,omitempty"`
	Error     string   `json:"error,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

// Error codes in a report.
const (
	CodeUnreadable    = "unreadable"
	CodeUnsupported   = "unsupported_document"
	CodeXSD           = "xsd_invalid"
	CodePartyMismatch = model.WarnPartyMismatch
	CodeMapping       = "mapping_failed"
	CodeStore         = "store_failed"
)

// Upload stores the file and queues the job.
func (s *Service) Upload(ctx context.Context, scope engagementctx.Scope, actorID int64, fileName string, data []byte) (Job, error) {
	if len(data) == 0 {
		return Job{}, ErrEmpty
	}
	if len(data) > MaxUploadBytes {
		return Job{}, ErrTooLarge
	}
	switch ublmap.Sniff(data) {
	case ublmap.PayloadXML, ublmap.PayloadZIP, ublmap.PayloadBase64:
	default:
		return Job{}, ErrUnsupported
	}
	sum := sha256.Sum256(data)
	name := cleanName(fileName)
	key := fmt.Sprintf("ubl-uploads/%d/%s/%s", scope.TaxpayerID, uuid.NewString(), name)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), Filename: name}, key); err != nil {
		return Job{}, fmt.Errorf("upload: store file: %w", err)
	}
	row, err := s.q.CreateUBLUploadJob(ctx, db.CreateUBLUploadJobParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaxpayerID: scope.TaxpayerID, ActorID: actorID,
		FileName: name, FileKey: key, Sha256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(data)),
	})
	if err != nil {
		return Job{}, err
	}
	if s.queue == nil {
		if err := s.Process(ctx, row.ID); err != nil {
			s.log.Warn("ubl_upload_failed", "job", row.Uuid, "error", err)
		}
		row, err = s.q.GetUBLUploadJob(ctx, db.GetUBLUploadJobParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: row.Uuid})
		if err != nil {
			return Job{}, err
		}
		return view(row), nil
	}
	task, err := queue.NewUBLUploadProcessTask(row.ID)
	if err != nil {
		return Job{}, err
	}
	if _, err := s.queue.Enqueue(task, asynq.Queue(queue.QueueImports), asynq.MaxRetry(3)); err != nil {
		return Job{}, err
	}
	return view(row), nil
}

// Get returns one job of the engagement.
func (s *Service) Get(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) (Job, error) {
	row, err := s.q.GetUBLUploadJob(ctx, db.GetUBLUploadJobParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	return view(row), nil
}

// List returns the engagement's jobs, newest first (reports omitted).
func (s *Service) List(ctx context.Context, scope engagementctx.Scope, limit, offset int32) ([]Job, error) {
	rows, err := s.q.ListUBLUploadJobs(ctx, db.ListUBLUploadJobsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Lim: limit, Off: offset})
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(rows))
	for _, r := range rows {
		j := view(r)
		if j.Report != nil {
			j.Report.Items = nil
		}
		out = append(out, j)
	}
	return out, nil
}

// Process runs one job (worker entry point). It is safe to retry: invoices
// already stored are reported as duplicates.
func (s *Service) Process(ctx context.Context, jobID int64) error {
	job, err := s.q.ClaimUBLUploadJob(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already done or being processed
	}
	if err != nil {
		return err
	}
	rep, perr := s.run(ctx, job)
	status, msg := "done", pgtype.Text{}
	if perr != nil {
		status, msg = "failed", pgtype.Text{String: perr.Error(), Valid: true}
	}
	raw, _ := json.Marshal(rep)
	if err := s.q.FinishUBLUploadJob(ctx, db.FinishUBLUploadJobParams{ID: job.ID, Status: status, Report: raw, Error: msg}); err != nil {
		return err
	}
	return perr
}

func (s *Service) run(ctx context.Context, job db.UblUploadJob) (Report, error) {
	rep := Report{Items: []ItemResult{}}
	own, err := s.ownTaxID(ctx, job.TaxpayerID)
	if err != nil {
		return rep, err
	}
	if model.DigitsOnly(own) == "" {
		return rep, ErrNoOwnTaxID
	}
	rc, _, err := s.store.Download(ctx, job.FileKey)
	if err != nil {
		return rep, fmt.Errorf("upload: download: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(rc, MaxUploadBytes+1))
	_ = rc.Close()
	if err != nil {
		return rep, err
	}
	items, err := ublmap.Extract(job.FileName, data, s.limits)
	if err != nil {
		rep.Total, rep.Failed = 1, 1
		rep.Items = append(rep.Items, ItemResult{Name: job.FileName, Status: ItemFailed, ErrorCode: CodeUnreadable, Error: err.Error()})
		return rep, nil
	}
	for _, it := range items {
		res := s.processItem(ctx, job, own, it)
		rep.Total++
		switch res.Status {
		case ItemAdded:
			rep.Added++
		case ItemDuplicate:
			rep.Duplicate++
		default:
			rep.Failed++
		}
		rep.Items = append(rep.Items, res)
	}
	return rep, nil
}

func (s *Service) processItem(ctx context.Context, job db.UblUploadJob, own string, it ublmap.Item) ItemResult {
	res := ItemResult{Name: it.Name, Status: ItemFailed}
	fail := func(code string, err error) ItemResult {
		res.ErrorCode, res.Error = code, err.Error()
		return res
	}
	if it.Err != nil {
		return fail(CodeUnreadable, it.Err)
	}
	if it.Kind != ubltr.KindInvoice {
		return fail(CodeUnsupported, fmt.Errorf("document kind %s is not imported", it.Kind))
	}
	if err := s.validator.XSD(ctx, it.XML); err != nil {
		return fail(CodeXSD, summarize(err))
	}
	mapped, err := ublmap.MapXML(it.XML, ublmap.Options{OwnTaxID: own, SourceSystem: "manual_upload", CaptureMethod: model.CaptureUBLUpload})
	var pm *ublmap.PartyMismatchError
	if errors.As(err, &pm) {
		return fail(CodePartyMismatch, errors.New("own VKN/TCKN is not on the invoice"))
	}
	if err != nil {
		return fail(CodeMapping, err)
	}
	inv := mapped.Invoice
	res.ETTN, res.InvoiceNumber = inv.UUID, inv.InvoiceNumber

	// GİB Schematron: a failure is recorded as a warning, the invoice is kept
	// (docs/01 §1.1). e-Arşiv invoices use the earchive rule set.
	docType := ubltr.TypeEFatura
	if inv.ProfileRaw == model.ProfileEArsiv {
		docType = ubltr.TypeEArchive
	}
	warnings := mapped.Warnings
	if err := s.validator.Schematron(ctx, it.XML, docType); err != nil {
		w := model.Warning{Code: model.WarnUBLSchematronFail, EntityType: model.EntityInvoice, Params: map[string]any{"type": docType}}
		if vs := ubltr.Violations(err); len(vs) > 0 {
			w.Params["count"] = len(vs)
			w.Params["messages"] = violationMessages(vs, 5)
		} else {
			w.Params["messages"] = []string{err.Error()}
		}
		warnings = append(warnings, w)
	}

	saved, warnings, err := s.store1(ctx, job, inv, it.XML, warnings)
	if err != nil {
		return fail(CodeStore, err)
	}
	res.InvoiceUUID, res.Outcome, res.MatchRule = saved.InvoiceUUID, saved.Outcome, saved.MatchRule
	res.Status = ItemAdded
	if saved.Duplicate() {
		res.Status = ItemDuplicate
	}
	if saved.Written {
		for _, w := range warnings {
			res.Warnings = append(res.Warnings, w.Code)
		}
	}
	return res
}

// store1 keeps the XML as an attachment and writes the invoice and its
// warnings in one transaction. It returns the warnings actually recorded.
func (s *Service) store1(ctx context.Context, job db.UblUploadJob, inv model.Invoice, xml []byte, warnings []model.Warning) (repository.SaveResult, []model.Warning, error) {
	sum := sha256.Sum256(xml)
	sha := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("taxpayers/%d/ubl/%s.xml", job.TaxpayerID, sha)
	exists, err := s.store.Exists(ctx, key)
	if err != nil {
		return repository.SaveResult{}, nil, err
	}
	if !exists {
		if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(xml), Size: int64(len(xml)), Filename: inv.UUID + ".xml", ContentType: "application/xml"}, key); err != nil {
			return repository.SaveResult{}, nil, err
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repository.SaveResult{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	att, err := q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: job.TaxpayerID, Kind: "ubl_xml", StorageKey: key, Sha256: sha, SizeBytes: int64(len(xml)),
		ContentType: repository.Text("application/xml"), FileName: repository.Text(inv.UUID + ".xml"),
	})
	if err != nil {
		return repository.SaveResult{}, nil, err
	}
	saved, err := repository.SaveInvoice(ctx, tx, job.TaxpayerID, inv, att.ID)
	if err != nil {
		return repository.SaveResult{}, nil, err
	}
	// The XML belongs to the invoice even when it only adds a source.
	if _, err := q.SetAttachmentOwner(ctx, db.SetAttachmentOwnerParams{TaxpayerID: job.TaxpayerID, ID: att.ID, OwnerType: repository.Text("invoice"), OwnerID: pgtype.Int8{Int64: saved.InvoiceID, Valid: true}}); err != nil {
		return repository.SaveResult{}, nil, err
	}
	if !saved.Written {
		return saved, nil, tx.Commit(ctx)
	}
	if len(saved.PossibleDuplicates) > 0 {
		ids := make([]string, 0, maxCandidates)
		for i, id := range saved.PossibleDuplicates {
			if i == maxCandidates {
				break
			}
			ids = append(ids, id.String())
		}
		warnings = append(warnings, model.Warning{Code: model.WarnPossibleDuplicate, EntityType: model.EntityInvoice,
			Params: map[string]any{"candidates": ids, "count": len(saved.PossibleDuplicates)}})
	}
	recorded := make([]model.Warning, 0, len(warnings))
	for _, w := range warnings {
		setting, err := q.ResolveWarningSetting(ctx, db.ResolveWarningSettingParams{
			Fallback: model.Severity(w.Code), Code: w.Code, OrganizationID: job.OrganizationID,
			EngagementID: pgtype.Int8{Int64: job.EngagementID, Valid: true},
		})
		if err != nil {
			return repository.SaveResult{}, nil, fmt.Errorf("warning %s: %w", w.Code, err)
		}
		if !setting.Enabled {
			continue // the office switched this check off (#13)
		}
		entityID := saved.InvoiceID
		if w.EntityType == model.EntityInvoiceItem {
			if id, ok := saved.ItemIDs[w.LineNo]; ok {
				entityID = id
			}
		}
		params := w.Params
		if params == nil {
			params = map[string]any{}
		}
		if w.LineNo > 0 {
			params["line_no"] = w.LineNo
		}
		raw, _ := json.Marshal(params)
		if _, err := q.UpsertOpenDataWarning(ctx, db.UpsertOpenDataWarningParams{
			OrganizationID: job.OrganizationID, EngagementID: job.EngagementID, TaxpayerID: job.TaxpayerID,
			Period: repository.Text(inv.Period), EntityType: w.EntityType, EntityID: pgtype.Int8{Int64: entityID, Valid: true},
			Code: w.Code, Severity: setting.Severity, Params: raw, Producer: Producer,
		}); err != nil {
			return repository.SaveResult{}, nil, fmt.Errorf("warning %s: %w", w.Code, err)
		}
		recorded = append(recorded, w)
	}
	return saved, recorded, tx.Commit(ctx)
}

func summarize(err error) error {
	if vs := ubltr.Violations(err); len(vs) > 0 {
		return fmt.Errorf("%d violation(s): %s", len(vs), strings.Join(violationMessages(vs, 3), "; "))
	}
	return err
}

func violationMessages(vs []ubltr.Violation, n int) []string {
	out := make([]string, 0, n)
	for i, v := range vs {
		if i == n {
			break
		}
		out = append(out, v.Message)
	}
	return out
}

func cleanName(name string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "" || name == "." || name == "/" {
		return "upload"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20, r == '/', r == '\x7f':
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) > 200 {
		out = out[len(out)-200:]
	}
	return out
}

func view(r db.UblUploadJob) Job {
	j := Job{UUID: r.Uuid, FileName: r.FileName, SizeBytes: r.SizeBytes, Status: r.Status, CreatedAt: r.CreatedAt.Time}
	if r.Error.Valid {
		j.Error = r.Error.String
	}
	if r.StartedAt.Valid {
		t := r.StartedAt.Time
		j.StartedAt = &t
	}
	if r.FinishedAt.Valid {
		t := r.FinishedAt.Time
		j.FinishedAt = &t
	}
	if len(r.Report) > 2 {
		var rep Report
		if json.Unmarshal(r.Report, &rep) == nil {
			j.Report = &rep
		}
	}
	return j
}
