package documents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	accountplan "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/payroll"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// MaxFileBytes is the upload limit (the payroll import reads 20 MB).
const MaxFileBytes = 20 << 20

// Statuses of an agent document.
const (
	StatusNeedsMatch = "needs_match"
	StatusImported   = "imported"
	StatusFailed     = "failed"
	StatusDismissed  = "dismissed"
)

// Errors.
var (
	ErrNotFound       = errors.New("agentdocs: not found")
	ErrInvalid        = errors.New("agentdocs: invalid input")
	ErrConflict       = errors.New("agentdocs: already imported")
	ErrStorageMissing = errors.New("agentdocs: storage is not configured")
	// ErrFileMissing: the stored file cannot be read back (deleted, or the
	// row came from another store); the agent must upload it again.
	ErrFileMissing = errors.New("agentdocs: the stored file is missing")
)

// Queries is the db subset the service uses.
type Queries interface {
	GetDesktopAgent(ctx context.Context, arg db.GetDesktopAgentParams) (db.DesktopAgent, error)
	GetAgentDocumentBySha(ctx context.Context, arg db.GetAgentDocumentByShaParams) (db.AgentDocument, error)
	CreateAgentDocument(ctx context.Context, arg db.CreateAgentDocumentParams) (db.AgentDocument, error)
	SetAgentDocumentOutcome(ctx context.Context, arg db.SetAgentDocumentOutcomeParams) (db.AgentDocument, error)
	GetAgentDocument(ctx context.Context, arg db.GetAgentDocumentParams) (db.AgentDocument, error)
	GetAgentDocumentView(ctx context.Context, arg db.GetAgentDocumentViewParams) (db.GetAgentDocumentViewRow, error)
	ListAgentDocuments(ctx context.Context, arg db.ListAgentDocumentsParams) ([]db.ListAgentDocumentsRow, error)
	CountAgentDocuments(ctx context.Context, arg db.CountAgentDocumentsParams) (int64, error)
	ListAgentDocumentCandidates(ctx context.Context, organizationID int64) ([]db.ListAgentDocumentCandidatesRow, error)
}

// ObjectStore keeps the file until it is imported (the office may resolve
// it later).
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// PayrollImporter is payroll.Service.
type PayrollImporter interface {
	Import(ctx context.Context, scope engagementctx.Scope, actorID int64, fileName string, data []byte, opt payroll.ImportOptions) (payroll.ImportResult, error)
}

// PlanImporter is the account plan / trial balance import.
type PlanImporter interface {
	Import(ctx context.Context, sub accountplan.Subject, userID int64, tgt, fileName, period string, data []byte) (accountplan.ImportResult, error)
}

// ForeignIntake opens a KDV2 draft from a file.
type ForeignIntake interface {
	Submit(ctx context.Context, sub foreigninvoices.Subject, taxpayerID int64, f foreigninvoices.IntakeInput) (foreigninvoices.IntakeResult, error)
}

// Actor is who uploads or resolves: the office user and their permissions.
type Actor struct {
	OrganizationID int64
	UserID         int64
	Has            func(perm string) bool
}

// Service routes agent documents.
type Service struct {
	pool    database.Pool
	q       Queries
	store   ObjectStore
	eng     engagementctx.Resolver
	payroll PayrollImporter
	plan    PlanImporter
	foreign ForeignIntake
	log     *slog.Logger
}

// New builds the service. pool opens savepoints around each import (an
// import that fails leaves the document row and the request intact). Any
// importer may be nil: that kind then waits for the office.
func New(pool database.Pool, q Queries, store ObjectStore, eng engagementctx.Resolver,
	pay PayrollImporter, plan PlanImporter, foreign ForeignIntake, log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: q, store: store, eng: eng, payroll: pay, plan: plan, foreign: foreign, log: log}
}

// Document is the API view.
type Document struct {
	UUID           uuid.UUID  `json:"uuid"`
	AgentName      string     `json:"agent_name"`
	Folder         string     `json:"folder"`
	FileName       string     `json:"file_name"`
	SizeBytes      int64      `json:"size_bytes"`
	Kind           string     `json:"kind"`
	Period         *string    `json:"period"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason"`
	MatchSource    string     `json:"match_source"`
	EngagementUUID *uuid.UUID `json:"engagement_uuid"`
	TaxpayerTitle  string     `json:"taxpayer_title"`
	ResultUUID     *uuid.UUID `json:"result_uuid"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	// Replayed: the same content came before; the first row is answered.
	Replayed bool `json:"replayed"`
}

// UploadInput is one file from the agent.
type UploadInput struct {
	AgentUUID uuid.UUID
	// Folder is the watched folder path (taxpayer title / VKN may be in it).
	Folder   string
	FileName string
	// EngagementUUID is the watched folder's engagement, when it has one.
	EngagementUUID *uuid.UUID
	Data           []byte
}

// Upload stores the file, recognises it and imports it when the kind, the
// taxpayer and the period are clear; otherwise it waits as "eşleştir".
func (s *Service) Upload(ctx context.Context, a Actor, in UploadInput) (Document, error) {
	if len(in.Data) == 0 || len(in.Data) > MaxFileBytes {
		return Document{}, fmt.Errorf("%w: the file is empty or larger than 20 MB", ErrInvalid)
	}
	name := cleanName(in.FileName)
	folder := strings.TrimSpace(in.Folder)
	if r := []rune(folder); len(r) > 1024 {
		folder = string(r[len(r)-1024:])
	}
	agent, err := s.q.GetDesktopAgent(ctx, db.GetDesktopAgentParams{OrganizationID: a.OrganizationID, Uuid: in.AgentUUID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && agent.RevokedAt.Valid) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("agentdocs: agent: %w", err)
	}
	sum := sha256.Sum256(in.Data)
	sha := hex.EncodeToString(sum[:])
	if prev, err := s.q.GetAgentDocumentBySha(ctx, db.GetAgentDocumentByShaParams{OrganizationID: a.OrganizationID, Sha256: sha}); err == nil {
		return s.view(ctx, a.OrganizationID, prev, true)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Document{}, fmt.Errorf("agentdocs: find: %w", err)
	}
	if s.store == nil {
		return Document{}, ErrStorageMissing
	}
	cls := Classify(name, in.Data)
	ct := contentType(name, in.Data)
	key := fmt.Sprintf("agent-documents/%d/%s%s", a.OrganizationID, sha, strings.ToLower(path.Ext(name)))
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(in.Data), Size: int64(len(in.Data)), ContentType: ct, Filename: name}, key); err != nil {
		return Document{}, fmt.Errorf("agentdocs: store: %w", err)
	}
	row, err := s.q.CreateAgentDocument(ctx, db.CreateAgentDocumentParams{
		OrganizationID: a.OrganizationID, AgentID: pgtype.Int8{Int64: agent.ID, Valid: true}, Folder: folder,
		FileName: name, Sha256: sha, SizeBytes: int64(len(in.Data)), ContentType: ct, StorageKey: key,
		Kind: cls.Kind, Period: optText(cls.Period), CreatedBy: optID(a.UserID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent upload of the same content came first.
		prev, err := s.q.GetAgentDocumentBySha(ctx, db.GetAgentDocumentByShaParams{OrganizationID: a.OrganizationID, Sha256: sha})
		if err != nil {
			return Document{}, fmt.Errorf("agentdocs: find: %w", err)
		}
		return s.view(ctx, a.OrganizationID, prev, true)
	}
	if err != nil {
		return Document{}, fmt.Errorf("agentdocs: create: %w", err)
	}
	target, src, err := s.match(ctx, a.OrganizationID, cls, folder, in.EngagementUUID)
	if err != nil {
		return Document{}, err
	}
	row, err = s.process(ctx, a, row, cls.Kind, cls.Period, target, src, in.Data)
	if err != nil {
		return Document{}, err
	}
	return s.view(ctx, a.OrganizationID, row, false)
}

// match finds the engagement: the VKN in the file, the watched folder's
// engagement, then the folder name.
func (s *Service) match(ctx context.Context, orgID int64, cls Classified, folder string, watch *uuid.UUID) (*uuid.UUID, string, error) {
	rows, err := s.q.ListAgentDocumentCandidates(ctx, orgID)
	if err != nil {
		return nil, "", fmt.Errorf("agentdocs: candidates: %w", err)
	}
	cands := make([]Candidate, 0, len(rows))
	for _, r := range rows {
		cands = append(cands, Candidate{EngagementUUID: r.EngagementUuid, TaxID: r.TaxID, Title: r.Title})
	}
	if c, ok := ByTaxID(cls.TaxIDs, cands); ok {
		return &c.EngagementUUID, MatchContent, nil
	}
	if watch != nil {
		for _, c := range cands {
			if c.EngagementUUID == *watch {
				return &c.EngagementUUID, MatchWatch, nil
			}
		}
	}
	if c, ok := ByFolder(folder, cands); ok {
		return &c.EngagementUUID, MatchFolder, nil
	}
	return nil, "", nil
}

// ResolveInput is the office's choice for an "eşleştir" item.
type ResolveInput struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	Kind           string    `json:"kind"`
	// Period "YYYY-MM"; empty keeps the one read from the file.
	Period string `json:"period"`
}

// Resolve imports a waiting (or failed / dismissed) document with the
// office's engagement, kind and period.
func (s *Service) Resolve(ctx context.Context, a Actor, id uuid.UUID, in ResolveInput) (Document, error) {
	if !ValidKind(in.Kind) {
		return Document{}, fmt.Errorf("%w: kind must be payroll, trial_balance, account_plan or foreign_invoice", ErrInvalid)
	}
	in.Period = strings.TrimSpace(in.Period)
	if in.Period != "" && !accountplan.ValidPeriod(in.Period) {
		return Document{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if in.EngagementUUID == uuid.Nil {
		return Document{}, fmt.Errorf("%w: engagement_uuid is required", ErrInvalid)
	}
	row, err := s.get(ctx, a.OrganizationID, id)
	if err != nil {
		return Document{}, err
	}
	if row.Status == StatusImported {
		return Document{}, ErrConflict
	}
	if s.store == nil {
		return Document{}, ErrStorageMissing
	}
	period := in.Period
	if period == "" && row.Period.Valid {
		period = row.Period.String
	}
	rc, _, err := s.store.Download(ctx, row.StorageKey)
	if err != nil {
		s.log.Warn("agent_document_download_failed", "organization_id", a.OrganizationID, "error", err)
		return Document{}, fmt.Errorf("%w: %v", ErrFileMissing, err)
	}
	data, err := io.ReadAll(io.LimitReader(rc, MaxFileBytes+1))
	_ = rc.Close()
	if err != nil {
		return Document{}, fmt.Errorf("agentdocs: read: %w", err)
	}
	eng := in.EngagementUUID
	row, err = s.process(ctx, a, row, in.Kind, period, &eng, MatchOffice, data)
	if err != nil {
		return Document{}, err
	}
	return s.view(ctx, a.OrganizationID, row, false)
}

// Dismiss drops a waiting document from the office's list.
func (s *Service) Dismiss(ctx context.Context, a Actor, id uuid.UUID) (Document, error) {
	row, err := s.get(ctx, a.OrganizationID, id)
	if err != nil {
		return Document{}, err
	}
	if row.Status == StatusImported {
		return Document{}, ErrConflict
	}
	row, err = s.q.SetAgentDocumentOutcome(ctx, db.SetAgentDocumentOutcomeParams{
		OrganizationID: a.OrganizationID, ID: row.ID, EngagementID: row.EngagementID, Kind: row.Kind,
		Period: row.Period, MatchSource: row.MatchSource, Status: StatusDismissed, Reason: row.Reason,
		ResolvedBy: optID(a.UserID),
	})
	if err != nil {
		return Document{}, fmt.Errorf("agentdocs: dismiss: %w", err)
	}
	return s.view(ctx, a.OrganizationID, row, false)
}

// Page is a page of documents.
type Page struct {
	Items []Document `json:"items"`
	Total int64      `json:"total"`
}

// List returns the office's documents, newest first. status: "open"
// (needs_match + failed, the default), "all" or one status.
func (s *Service) List(ctx context.Context, orgID int64, status string, limit, offset int32) (Page, error) {
	var statuses []string
	switch status {
	case "", "open":
		statuses = []string{StatusNeedsMatch, StatusFailed}
	case "all":
		statuses = []string{}
	case StatusNeedsMatch, StatusImported, StatusFailed, StatusDismissed:
		statuses = []string{status}
	default:
		return Page{}, fmt.Errorf("%w: status must be open, all, needs_match, imported, failed or dismissed", ErrInvalid)
	}
	rows, err := s.q.ListAgentDocuments(ctx, db.ListAgentDocumentsParams{
		OrganizationID: orgID, Statuses: statuses, LimitRows: limit, OffsetRows: offset,
	})
	if err != nil {
		return Page{}, fmt.Errorf("agentdocs: list: %w", err)
	}
	total, err := s.q.CountAgentDocuments(ctx, db.CountAgentDocumentsParams{OrganizationID: orgID, Statuses: statuses})
	if err != nil {
		return Page{}, fmt.Errorf("agentdocs: count: %w", err)
	}
	out := Page{Items: make([]Document, 0, len(rows)), Total: total}
	for _, r := range rows {
		out.Items = append(out.Items, fromView(r))
	}
	return out, nil
}

func (s *Service) get(ctx context.Context, orgID int64, id uuid.UUID) (db.AgentDocument, error) {
	row, err := s.q.GetAgentDocument(ctx, db.GetAgentDocumentParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("agentdocs: get: %w", err)
	}
	return row, nil
}

// outcome is what process decided.
type outcome struct {
	engagementID pgtype.Int8
	status       string
	reason       string
	result       pgtype.UUID
	period       string
}

// Reasons shown to the office (Turkish UI text).
const (
	reasonUnknownKind = "Dosyanın türü anlaşılamadı; türü ve mükellefi seçin."
	reasonNoTaxpayer  = "Mükellef dosyadan ya da klasör adından bulunamadı; mükellefi seçin."
	reasonNoAccess    = "Bu mükellefe erişiminiz yok ya da hizmet ilişkisi sona ermiş."
	reasonNoPerm      = "Bu dosya türünü içe aktarma yetkiniz yok."
	reasonNoPeriod    = "Dönem dosyadan okunamadı; dönemi seçin."
	reasonNoImporter  = "Bu dosya türü otomatik içe aktarılamıyor; ofisten elle yükleyin."
)

// process imports the file when it can and records the outcome.
func (s *Service) process(ctx context.Context, a Actor, row db.AgentDocument, kind, period string, eng *uuid.UUID, src string, data []byte) (db.AgentDocument, error) {
	o := s.decide(ctx, a, row.FileName, kind, period, eng, data)
	if !o.engagementID.Valid {
		src = ""
	}
	var by pgtype.Int8
	if src == MatchOffice || o.status == StatusImported {
		by = optID(a.UserID)
	}
	out, err := s.q.SetAgentDocumentOutcome(ctx, db.SetAgentDocumentOutcomeParams{
		OrganizationID: a.OrganizationID, ID: row.ID, EngagementID: o.engagementID, Kind: kind,
		Period: optText(o.period), MatchSource: src, Status: o.status, Reason: truncate(o.reason, 500),
		ResultUuid: o.result, ResolvedBy: by,
	})
	if err != nil {
		return row, fmt.Errorf("agentdocs: outcome: %w", err)
	}
	return out, nil
}

func (s *Service) decide(ctx context.Context, a Actor, name, kind, period string, eng *uuid.UUID, data []byte) outcome {
	o := outcome{status: StatusNeedsMatch, period: period}
	if !ValidKind(kind) {
		o.reason = reasonUnknownKind
		return o
	}
	if eng == nil {
		o.reason = reasonNoTaxpayer
		return o
	}
	acc, err := s.eng.ResolveEngagement(ctx, a.OrganizationID, *eng, a.UserID)
	if err != nil || acc.OrganizationID != a.OrganizationID || acc.Status != engagementctx.StatusActive ||
		(!acc.Assigned && !a.has(rbac.PermTenantEngagementsReadAll)) {
		o.reason = reasonNoAccess
		return o
	}
	o.engagementID = pgtype.Int8{Int64: acc.ID, Valid: true}
	if !a.has(kindPermission(kind)) {
		o.reason = reasonNoPerm
		return o
	}
	if (kind == KindPayroll || kind == KindTrialBalance) && period == "" {
		o.reason = reasonNoPeriod
		return o
	}
	scope := engagementctx.Scope{
		ID: acc.ID, UUID: acc.UUID, OrganizationID: acc.OrganizationID, TaxpayerID: acc.TaxpayerID,
		TaxpayerUUID: acc.TaxpayerUUID, Status: acc.Status, StartedAt: acc.StartedAt, ArchiveUntil: acc.ArchiveUntil,
		ViaReadAll: !acc.Assigned,
	}
	result, gotPeriod, err := s.runImport(ctx, a, scope, name, kind, period, data)
	if errors.Is(err, errNoImporter) {
		o.reason = reasonNoImporter
		return o
	}
	if err != nil {
		o.status, o.reason = StatusFailed, failReason(err)
		if !isInputError(err) {
			s.log.Warn("agent_document_import_failed", "organization_id", a.OrganizationID, "kind", kind, "error", err)
		}
		return o
	}
	o.status, o.reason = StatusImported, ""
	o.result = pgtype.UUID{Bytes: result, Valid: result != uuid.Nil}
	if gotPeriod != "" {
		o.period = gotPeriod
	}
	return o
}

var errNoImporter = errors.New("agentdocs: no importer for the kind")

// runImport calls the import of the kind in a savepoint: a refused import
// rolls back its own writes only.
func (s *Service) runImport(ctx context.Context, a Actor, scope engagementctx.Scope, name, kind, period string, data []byte) (uuid.UUID, string, error) {
	var (
		result    uuid.UUID
		gotPeriod string
	)
	run := func(ctx context.Context) error {
		switch kind {
		case KindPayroll:
			if s.payroll == nil {
				return errNoImporter
			}
			res, err := s.payroll.Import(ctx, scope, a.UserID, name, data, payroll.ImportOptions{Period: period})
			if err != nil {
				return err
			}
			gotPeriod = res.Period
			if res.Run != nil {
				result = res.Run.UUID
			}
		case KindTrialBalance, KindAccountPlan:
			if s.plan == nil {
				return errNoImporter
			}
			p := period
			if kind == KindAccountPlan {
				p = ""
			}
			res, err := s.plan.Import(ctx, accountplan.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID}, a.UserID, "", name, p, data)
			if err != nil {
				return err
			}
			result = res.Import.UUID
			if res.Import.Period != nil {
				gotPeriod = *res.Import.Period
			}
		case KindForeignInvoice:
			if s.foreign == nil {
				return errNoImporter
			}
			res, err := s.foreign.Submit(ctx, foreigninvoices.Subject{OrganizationID: scope.OrganizationID, EngagementID: scope.ID},
				scope.TaxpayerID, foreigninvoices.IntakeInput{Source: foreigninvoices.SourceAgent, FileName: name, Data: data, UserID: a.UserID})
			if err != nil {
				return err
			}
			result, gotPeriod = res.Item.UUID, res.Item.Period
		default:
			return errNoImporter
		}
		return nil
	}
	if s.pool == nil {
		return result, gotPeriod, run(ctx)
	}
	tx, err := database.Ambient{Pool: s.pool}.Begin(ctx)
	if err != nil {
		return result, gotPeriod, err
	}
	if err := run(dbtx.WithTx(ctx, tx)); err != nil {
		_ = tx.Rollback(ctx)
		return result, gotPeriod, err
	}
	// A best-effort side effect (activity, notification) that failed inside
	// the import can leave the savepoint aborted while the import itself
	// reports success; releasing it would then fail and leave the request
	// transaction unusable. Probe first and roll the savepoint back instead.
	if _, err := tx.Exec(ctx, "SELECT 1"); err != nil {
		_ = tx.Rollback(ctx)
		return result, gotPeriod, fmt.Errorf("agentdocs: import left the transaction aborted: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return result, gotPeriod, err
	}
	return result, gotPeriod, nil
}

func kindPermission(kind string) string {
	switch kind {
	case KindPayroll:
		return rbac.PermTenantPayrollWrite
	case KindTrialBalance, KindAccountPlan:
		return rbac.PermTenantAccountPlanWrite
	default:
		return rbac.PermTenantTaxReturnsWrite
	}
}

func (a Actor) has(perm string) bool { return a.Has != nil && a.Has(perm) }

// isInputError: the import refused the file (not a server fault).
func isInputError(err error) bool {
	for _, e := range []error{
		payroll.ErrNoPayrollSheet, payroll.ErrMappingMissing, payroll.ErrPeriodRequired, payroll.ErrInvalid,
		accountplan.ErrEmpty, accountplan.ErrInvalid, accountplan.ErrTooLarge, accountplan.ErrUnsupported,
		foreigninvoices.ErrUnsupported, foreigninvoices.ErrDisabled, foreigninvoices.ErrInvalid,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// failReason is the office's text for a refused import.
func failReason(err error) string {
	switch {
	case errors.Is(err, payroll.ErrNoPayrollSheet), errors.Is(err, payroll.ErrMappingMissing):
		return "Bordro sütunları tanınmadı; dosyayı Bordro sayfasından elle yükleyip sütunları eşleyin."
	case errors.Is(err, payroll.ErrPeriodRequired):
		return reasonNoPeriod
	case errors.Is(err, payroll.ErrInvalid):
		return "Bordro dökümünde çalışan satırı bulunamadı."
	case errors.Is(err, accountplan.ErrEmpty):
		return "Dosyada hesap kodu bulunamadı."
	case errors.Is(err, accountplan.ErrUnsupported):
		return "Dosya biçimi desteklenmiyor; .xlsx ya da .csv olarak kaydedin."
	case errors.Is(err, accountplan.ErrTooLarge):
		return "Dosya çok büyük."
	case errors.Is(err, foreigninvoices.ErrDisabled):
		return "Yabancı hizmet faturaları ofiste kapalı (Ayarlar → Yabancı hizmet faturaları)."
	case errors.Is(err, foreigninvoices.ErrUnsupported):
		return "Yabancı fatura PDF ya da fotoğraf olmalı."
	case errors.Is(err, foreigninvoices.ErrStorageMissing):
		return "Dosya deposu yapılandırılmamış."
	}
	return "İçe aktarma tamamlanamadı; dosyayı ilgili sayfadan elle yükleyin."
}

func (s *Service) view(ctx context.Context, orgID int64, row db.AgentDocument, replayed bool) (Document, error) {
	r, err := s.q.GetAgentDocumentView(ctx, db.GetAgentDocumentViewParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return Document{}, fmt.Errorf("agentdocs: view: %w", err)
	}
	d := fromView(db.ListAgentDocumentsRow(r))
	d.Replayed = replayed
	return d, nil
}

// fromView maps a row with the agent and taxpayer names.
func fromView(r db.ListAgentDocumentsRow) Document {
	d := fromRow(db.AgentDocument{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, AgentID: r.AgentID, EngagementID: r.EngagementID,
		Folder: r.Folder, FileName: r.FileName, Sha256: r.Sha256, SizeBytes: r.SizeBytes, ContentType: r.ContentType,
		StorageKey: r.StorageKey, Kind: r.Kind, Period: r.Period, MatchSource: r.MatchSource, Status: r.Status,
		Reason: r.Reason, ResultUuid: r.ResultUuid, CreatedBy: r.CreatedBy, ResolvedBy: r.ResolvedBy,
		ResolvedAt: r.ResolvedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
	d.AgentName, d.TaxpayerTitle = r.AgentName, r.TaxpayerTitle
	if r.EngagementUuid.Valid {
		u := uuid.UUID(r.EngagementUuid.Bytes)
		d.EngagementUUID = &u
	}
	return d
}

func fromRow(r db.AgentDocument) Document {
	d := Document{
		UUID: r.Uuid, Folder: r.Folder, FileName: r.FileName, SizeBytes: r.SizeBytes, Kind: r.Kind,
		Status: r.Status, Reason: r.Reason, MatchSource: r.MatchSource, CreatedAt: r.CreatedAt.Time,
	}
	if r.Period.Valid {
		p := r.Period.String
		d.Period = &p
	}
	if r.ResultUuid.Valid {
		u := uuid.UUID(r.ResultUuid.Bytes)
		d.ResultUUID = &u
	}
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time
		d.ResolvedAt = &t
	}
	return d
}

func cleanName(name string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "dosya"
	}
	return truncate(name, 255)
}

func contentType(name string, data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return "application/pdf"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case strings.EqualFold(path.Ext(name), ".xls"):
		return "application/vnd.ms-excel"
	case strings.EqualFold(path.Ext(name), ".csv"):
		return "text/csv"
	}
	return "application/octet-stream"
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func optText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func optID(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }
