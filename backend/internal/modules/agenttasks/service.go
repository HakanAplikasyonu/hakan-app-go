// Package agenttasks is the task queue of the desktop helper (docs/tasarim/
// masaustu-luca-sgk.md §2): the office's desktop app works in the user's own
// Luca session — the user signs in and types the verification code; no Luca
// password, cookie or page content reaches the server. The server keeps the
// tasks, builds the files (Luca limits, double export guard) and records the
// outcome per file and per row.
//
// Two kinds:
//   - luca_cari_sync: look the taxpayer's new counterparties up in Luca, open
//     the missing ones, write the codes back to the counterparty memory.
//   - luca_bulk_export: "Luca'ya aktar" for one taxpayer and period: bank,
//     Z reports, invoices, expenses and journal exports, built when the agent
//     claims the task (after the taxpayer's cari task of the same run).
package agenttasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
)

// Task kinds.
const (
	KindCariSync   = "luca_cari_sync"
	KindBulkExport = "luca_bulk_export"
)

// Task statuses.
const (
	StatusQueued      = "queued"
	StatusClaimed     = "claimed"
	StatusWaitingUser = "waiting_user"
	StatusRunning     = "running"
	StatusDone        = "done"
	StatusPartial     = "partial"
	StatusFailed      = "failed"
	StatusNeedsCheck  = "needs_check"
	StatusCancelled   = "cancelled"
)

// File kinds of a bulk export, in upload order.
const (
	FileBank     = "bank"
	FileZ        = "z"
	FileIncoming = "incoming_invoices"
	FileOutgoing = "outgoing_invoices"
	FileExpenses = "expenses"
	FileJournals = "journals"
)

// FileKinds is the upload order of a bulk export (cari first, as its own
// task; then bank, Z, invoices, expenses, journal exports).
var FileKinds = []string{FileBank, FileZ, FileIncoming, FileOutgoing, FileExpenses, FileJournals}

// File and row statuses reported by the agent.
const (
	FilePending = "pending"
	FileOK      = "ok"
	FileFailed  = "failed"
	FileUnknown = "unknown"
)

// Counterparty row statuses.
const (
	CariPending  = "pending"
	CariFound    = "found"
	CariCreated  = "created"
	CariFailed   = "failed"
	CariConflict = "conflict"
)

var (
	ErrNotFound      = errors.New("agenttasks: not found")
	ErrDisabled      = errors.New("agenttasks: the desktop helper is turned off for this office")
	ErrInvalid       = errors.New("agenttasks: invalid request")
	ErrState         = errors.New("agenttasks: the task is not in a state that allows this")
	ErrNotApproved   = errors.New("agenttasks: the task is not approved")
	ErrNotReady      = errors.New("agenttasks: an earlier task of the run is not finished")
	ErrAgentBusy     = errors.New("agenttasks: the agent already works on another task")
	ErrAgentUnknown  = errors.New("agenttasks: desktop agent not found or revoked")
	ErrNothingToSend = errors.New("agenttasks: nothing to export")
)

var (
	errorCodeRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
	taxIDRe     = regexp.MustCompile(`^[0-9]{10,11}$`)
	mainCodeRe  = regexp.MustCompile(`^[0-9]{3}$`)
	accountRe   = regexp.MustCompile(`^[0-9]{3}(\.[0-9A-Za-z]+)*$`)
)

// Terminal reports whether status ends a task.
func Terminal(status string) bool {
	switch status {
	case StatusDone, StatusPartial, StatusFailed, StatusNeedsCheck, StatusCancelled:
		return true
	}
	return false
}

// Scope is the office and the caller.
type Scope struct {
	OrganizationID int64
	UserID         int64
	// ReadAll: the caller sees every engagement of the office.
	ReadAll bool
}

// Settings is the office's helper switch (default off).
type Settings struct {
	LucaEnabled bool       `json:"luca_enabled"`
	SGKEnabled  bool       `json:"sgk_enabled"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

// TaskTaxpayer names the taxpayer for the Luca firm check.
type TaskTaxpayer struct {
	Title string `json:"title"`
	TaxID string `json:"tax_id"`
}

// TaskAgent names the desktop agent of a task.
type TaskAgent struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Progress is the agent's counter ("Dosya 2/4").
type Progress struct {
	Step    int    `json:"step"`
	Steps   int    `json:"steps"`
	Message string `json:"message,omitempty"`
}

// ReportRow is a document the file builder left out or wrote with a
// fallback account.
type ReportRow struct {
	Kind       string `json:"kind"`
	DocumentNo string `json:"document_no"`
	Date       string `json:"date,omitempty"`
	Reason     string `json:"reason"`
	Account    string `json:"account,omitempty"`
}

// ServerReport is what the file builder left out.
type ServerReport struct {
	Check           []ReportRow `json:"check"`
	Skipped         []ReportRow `json:"skipped"`
	AlreadyExported []ReportRow `json:"already_exported"`
}

// FileDocument is a document in a file.
type FileDocument struct {
	DocumentKey  string `json:"document_key"`
	DocumentNo   string `json:"document_no,omitempty"`
	DocumentDate string `json:"document_date,omitempty"`
}

// EntryResult is one row of the error report.
type EntryResult struct {
	EntryNo     string `json:"entry_no,omitempty"`
	DocumentNo  string `json:"document_no,omitempty"`
	DocumentKey string `json:"document_key,omitempty"`
	Row         int    `json:"row,omitempty"`
	Status      string `json:"status"`
	LucaEntryNo string `json:"luca_entry_no,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// FileView is one file of a bulk export.
type FileView struct {
	UUID         uuid.UUID      `json:"uuid"`
	Sequence     int32          `json:"sequence"`
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	ContentType  string         `json:"content_type"`
	Size         int64          `json:"size"`
	SHA256       string         `json:"sha256"`
	Entries      int32          `json:"entries"`
	Documents    []FileDocument `json:"documents"`
	Status       string         `json:"status"`
	LucaMessage  string         `json:"luca_message,omitempty"`
	EntryResults []EntryResult  `json:"entry_results"`
	ReportedAt   *time.Time     `json:"reported_at,omitempty"`
}

// CounterpartyView is one counterparty of a cari task.
type CounterpartyView struct {
	TaxID            string `json:"tax_id"`
	Title            string `json:"title"`
	MainCode         string `json:"main_code"`
	Status           string `json:"status"`
	AccountCode      string `json:"account_code,omitempty"`
	HakanAccountCode string `json:"hakan_account_code,omitempty"`
	Message          string `json:"message,omitempty"`
}

// Task is the API view of a task.
type Task struct {
	UUID            uuid.UUID          `json:"uuid"`
	RunUUID         *uuid.UUID         `json:"run_uuid"`
	Sequence        int32              `json:"sequence"`
	Kind            string             `json:"kind"`
	EngagementUUID  uuid.UUID          `json:"engagement_uuid"`
	Taxpayer        TaskTaxpayer       `json:"taxpayer"`
	Period          *string            `json:"period"`
	Kinds           []string           `json:"kinds"`
	Status          string             `json:"status"`
	Approved        bool               `json:"approved"`
	ApprovedByName  string             `json:"approved_by_name,omitempty"`
	ApprovedAt      *time.Time         `json:"approved_at,omitempty"`
	RequestedByName string             `json:"requested_by_name,omitempty"`
	Agent           *TaskAgent         `json:"agent"`
	Progress        *Progress          `json:"progress"`
	Files           []FileView         `json:"files"`
	Counterparties  []CounterpartyView `json:"counterparties"`
	ServerReport    *ServerReport      `json:"server_report"`
	ErrorCode       string             `json:"error_code,omitempty"`
	ErrorMessage    string             `json:"error_message,omitempty"`
	Resolution      *string            `json:"resolution"`
	ClaimedAt       *time.Time         `json:"claimed_at,omitempty"`
	FinishedAt      *time.Time         `json:"finished_at,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// TaskPage is a page of tasks.
type TaskPage struct {
	Items  []Task `json:"items"`
	Total  int64  `json:"total"`
	Limit  int32  `json:"limit"`
	Offset int32  `json:"offset"`
}

// Run is the answer to "Luca'ya aktar".
type Run struct {
	RunUUID uuid.UUID `json:"run_uuid"`
	Items   []Task    `json:"items"`
}

// Filter narrows List.
type Filter struct {
	Status         string
	Kind           string
	OpenOnly       bool
	RunUUID        *uuid.UUID
	EngagementUUID *uuid.UUID
	Limit          int32
	Offset         int32
}

// Service runs the task queue.
type Service struct {
	pool    database.Pool
	q       *db.Queries
	builder *Builder
}

// New builds the service. builder makes the files of a bulk export; nil
// means bulk exports cannot be claimed (no storage).
func New(pool database.Pool, builder *Builder) *Service {
	return &Service{pool: pool, q: database.QueriesFor(pool), builder: builder}
}

// GetSettings returns the office's helper switch (off when never set).
func (s *Service) GetSettings(ctx context.Context, sc Scope) (Settings, error) {
	r, err := s.q.GetDesktopHelperSettings(ctx, sc.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return Settings{LucaEnabled: r.LucaEnabled, SGKEnabled: r.SgkEnabled, UpdatedAt: tsPtr(r.UpdatedAt)}, nil
}

// PutSettings stores the office's helper switch.
func (s *Service) PutSettings(ctx context.Context, sc Scope, in Settings) (Settings, error) {
	r, err := s.q.UpsertDesktopHelperSettings(ctx, db.UpsertDesktopHelperSettingsParams{
		OrganizationID: sc.OrganizationID, LucaEnabled: in.LucaEnabled, SgkEnabled: in.SGKEnabled,
		UpdatedBy: optInt8(sc.UserID),
	})
	if err != nil {
		return Settings{}, err
	}
	return Settings{LucaEnabled: r.LucaEnabled, SGKEnabled: r.SgkEnabled, UpdatedAt: tsPtr(r.UpdatedAt)}, nil
}

func (s *Service) lucaEnabled(ctx context.Context, sc Scope) error {
	st, err := s.GetSettings(ctx, sc)
	if err != nil {
		return err
	}
	if !st.LucaEnabled {
		return ErrDisabled
	}
	return nil
}

// ExportInput is one "Luca'ya aktar" click.
type ExportInput struct {
	EngagementUUIDs []uuid.UUID `json:"engagement_uuids"`
	Period          string      `json:"period"`
	Kinds           []string    `json:"kinds"`
}

// maxRunEngagements bounds one click.
const maxRunEngagements = 200

// NormalizeKinds keeps the known kinds in upload order; empty means all.
func NormalizeKinds(in []string) ([]string, error) {
	if len(in) == 0 {
		return append([]string(nil), FileKinds...), nil
	}
	want := map[string]bool{}
	for _, k := range in {
		k = strings.TrimSpace(k)
		if !validFileKind(k) {
			return nil, fmt.Errorf("%w: unknown kind %q", ErrInvalid, k)
		}
		want[k] = true
	}
	out := make([]string, 0, len(want))
	for _, k := range FileKinds {
		if want[k] {
			out = append(out, k)
		}
	}
	return out, nil
}

func validFileKind(k string) bool {
	for _, f := range FileKinds {
		if f == k {
			return true
		}
	}
	return false
}

// StartExport opens a run: per taxpayer (in title order) its pending cari
// task — with the period's invoice counterparties missing from the
// counterparty memory — and one bulk export. The request is the approval
// (one confirmation for the whole list, recorded with who and when). A
// taxpayer whose period already has an open export is skipped.
func (s *Service) StartExport(ctx context.Context, sc Scope, in ExportInput) (Run, error) {
	if err := s.lucaEnabled(ctx, sc); err != nil {
		return Run{}, err
	}
	period := strings.TrimSpace(in.Period)
	if !periodq.Valid(period) {
		return Run{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if len(in.EngagementUUIDs) == 0 || len(in.EngagementUUIDs) > maxRunEngagements {
		return Run{}, fmt.Errorf("%w: choose 1–%d taxpayers", ErrInvalid, maxRunEngagements)
	}
	kinds, err := NormalizeKinds(in.Kinds)
	if err != nil {
		return Run{}, err
	}
	engs, err := s.q.ListHelperEngagements(ctx, db.ListHelperEngagementsParams{
		OrganizationID: sc.OrganizationID, Uuids: in.EngagementUUIDs, AllEngagements: sc.ReadAll, UserID: sc.UserID,
	})
	if err != nil {
		return Run{}, err
	}
	if len(engs) == 0 {
		return Run{}, ErrNotFound
	}
	run := uuid.New()
	runID := pgtype.UUID{Bytes: run, Valid: true}
	var ids []uuid.UUID
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		seq := int32(0)
		for _, e := range engs {
			// Pending cari task + the period's counterparties not in memory.
			cari, err := q.EnsureCariAgentTask(ctx, db.EnsureCariAgentTaskParams{
				OrganizationID: sc.OrganizationID, EngagementID: e.ID, RequestedBy: optInt8(sc.UserID),
			})
			if err != nil {
				return err
			}
			missing, err := q.ListMissingCounterparties(ctx, db.ListMissingCounterpartiesParams{
				TaxpayerID: e.TaxpayerID, Period: period, OrganizationID: sc.OrganizationID, EngagementID: e.ID,
			})
			if err != nil {
				return err
			}
			for _, m := range missing {
				if _, err := q.AddAgentTaskCounterparty(ctx, db.AddAgentTaskCounterpartyParams{
					OrganizationID: sc.OrganizationID, EngagementID: e.ID, TaskID: cari.ID,
					TaxID: m.TaxID, MainCode: m.MainCode, Title: clip(m.Title, 300),
				}); err != nil {
					return err
				}
			}
			n, err := q.CountAgentTaskCounterparties(ctx, db.CountAgentTaskCounterpartiesParams{
				OrganizationID: sc.OrganizationID, EngagementID: e.ID, TaskID: cari.ID,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				if err := q.DeleteEmptyCariAgentTask(ctx, db.DeleteEmptyCariAgentTaskParams{
					OrganizationID: sc.OrganizationID, EngagementID: e.ID, ID: cari.ID,
				}); err != nil {
					return err
				}
			}
			bulk, err := q.InsertBulkAgentTask(ctx, db.InsertBulkAgentTaskParams{
				OrganizationID: sc.OrganizationID, EngagementID: e.ID, RunUuid: runID, Sequence: seq + 2,
				Period: pgtype.Text{String: period, Valid: true}, Kinds: kinds, RequestedBy: optInt8(sc.UserID),
			})
			if errors.Is(err, pgx.ErrNoRows) {
				// The period already has an open export: skip the taxpayer,
				// its cari task stays pending.
				continue
			}
			if err != nil {
				return err
			}
			if n > 0 {
				c, err := q.AttachCariAgentTaskToRun(ctx, db.AttachCariAgentTaskToRunParams{
					RunUuid: runID, Sequence: seq + 1, ApprovedBy: optInt8(sc.UserID),
					OrganizationID: sc.OrganizationID, EngagementID: e.ID, ID: cari.ID,
				})
				if err != nil {
					return err
				}
				ids = append(ids, c.Uuid)
			}
			ids = append(ids, bulk.Uuid)
			seq += 2
		}
		return nil
	})
	if err != nil {
		return Run{}, err
	}
	out := Run{RunUUID: run, Items: []Task{}}
	if len(ids) == 0 {
		return out, ErrState
	}
	page, err := s.List(ctx, sc, Filter{RunUUID: &run, Limit: 500})
	if err != nil {
		return Run{}, err
	}
	out.Items = page.Items
	return out, nil
}

// List lists the office's tasks the caller can see, newest first; a run's
// tasks in their order.
func (s *Service) List(ctx context.Context, sc Scope, f Filter) (TaskPage, error) {
	p := db.ListAgentTasksParams{
		OrganizationID: sc.OrganizationID, AllEngagements: sc.ReadAll, UserID: sc.UserID,
		Status: pgtype.Text{String: f.Status, Valid: f.Status != ""}, OpenOnly: f.OpenOnly,
		Kind: pgtype.Text{String: f.Kind, Valid: f.Kind != ""}, Lim: f.Limit, Off: f.Offset,
	}
	if f.RunUUID != nil {
		p.RunUuid = pgtype.UUID{Bytes: *f.RunUUID, Valid: true}
	}
	if f.EngagementUUID != nil {
		p.EngagementUuid = pgtype.UUID{Bytes: *f.EngagementUUID, Valid: true}
	}
	rows, err := s.q.ListAgentTasks(ctx, p)
	if err != nil {
		return TaskPage{}, err
	}
	out := TaskPage{Items: make([]Task, 0, len(rows)), Limit: f.Limit, Offset: f.Offset}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		out.Total = r.Total
		ids = append(ids, r.ID)
	}
	files, cps, err := s.children(ctx, sc, ids)
	if err != nil {
		return TaskPage{}, err
	}
	for _, r := range rows {
		out.Items = append(out.Items, taskView(db.GetAgentTaskRow{
			ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, Kind: r.Kind,
			RunUuid: r.RunUuid, Sequence: r.Sequence, Period: r.Period, Kinds: r.Kinds, Status: r.Status,
			Progress: r.Progress, ServerReport: r.ServerReport, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage,
			Resolution: r.Resolution, RequestedBy: r.RequestedBy, ApprovedBy: r.ApprovedBy, ApprovedAt: r.ApprovedAt,
			ResolvedBy: r.ResolvedBy, AgentID: r.AgentID, ClaimedAt: r.ClaimedAt, FinishedAt: r.FinishedAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, EngagementUuid: r.EngagementUuid, TaxpayerID: r.TaxpayerID,
			EngagementStatus: r.EngagementStatus, EngagementStartedAt: r.EngagementStartedAt, TaxpayerUuid: r.TaxpayerUuid,
			TaxpayerTitle: r.TaxpayerTitle, TaxpayerTaxID: r.TaxpayerTaxID, AgentUuid: r.AgentUuid, AgentName: r.AgentName,
			RequestedByName: r.RequestedByName, ApprovedByName: r.ApprovedByName,
		}, files[r.ID], cps[r.ID]))
	}
	return out, nil
}

func (s *Service) children(ctx context.Context, sc Scope, ids []int64) (map[int64][]db.AgentTaskFile, map[int64][]db.AgentTaskCounterparty, error) {
	files := map[int64][]db.AgentTaskFile{}
	cps := map[int64][]db.AgentTaskCounterparty{}
	if len(ids) == 0 {
		return files, cps, nil
	}
	fr, err := s.q.ListAgentTaskFiles(ctx, db.ListAgentTaskFilesParams{OrganizationID: sc.OrganizationID, TaskIds: ids})
	if err != nil {
		return nil, nil, err
	}
	for _, f := range fr {
		files[f.TaskID] = append(files[f.TaskID], f)
	}
	cr, err := s.q.ListAgentTaskCounterparties(ctx, db.ListAgentTaskCounterpartiesParams{OrganizationID: sc.OrganizationID, TaskIds: ids})
	if err != nil {
		return nil, nil, err
	}
	for _, c := range cr {
		cps[c.TaskID] = append(cps[c.TaskID], c)
	}
	return files, cps, nil
}

// row loads a task the caller can see.
func (s *Service) row(ctx context.Context, sc Scope, id uuid.UUID) (db.GetAgentTaskRow, error) {
	r, err := s.q.GetAgentTask(ctx, db.GetAgentTaskParams{
		OrganizationID: sc.OrganizationID, Uuid: id, AllEngagements: sc.ReadAll, UserID: sc.UserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetAgentTaskRow{}, ErrNotFound
	}
	return r, err
}

// Get returns one task with its files and counterparties.
func (s *Service) Get(ctx context.Context, sc Scope, id uuid.UUID) (Task, error) {
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	files, cps, err := s.children(ctx, sc, []int64{r.ID})
	if err != nil {
		return Task{}, err
	}
	return taskView(r, files[r.ID], cps[r.ID]), nil
}

// Approve is the single confirmation of a cari task opened by the new
// counterparty trigger: the agent may run it from now on.
func (s *Service) Approve(ctx context.Context, sc Scope, id uuid.UUID) (Task, error) {
	if err := s.lucaEnabled(ctx, sc); err != nil {
		return Task{}, err
	}
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	if _, err := s.q.ApproveAgentTask(ctx, db.ApproveAgentTaskParams{
		ApprovedBy: optInt8(sc.UserID), OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrState
	} else if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, sc, id)
}

// Cancel stops a task that has not started uploading. A running task (the
// user may be pressing "Yükle" in Luca) cannot be cancelled from the panel.
func (s *Service) Cancel(ctx context.Context, sc Scope, id uuid.UUID) (Task, error) {
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.CancelAgentTask(ctx, db.CancelAgentTaskParams{
			ResolvedBy: optInt8(sc.UserID), OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID,
		}); errors.Is(err, pgx.ErrNoRows) {
			return ErrState
		} else if err != nil {
			return err
		}
		return s.closeTransfers(ctx, q, sc.OrganizationID, r.EngagementID, r.ID, sc.UserID, "TASK_CANCELLED")
	})
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, sc, id)
}

// ResolveInput closes a needs_check task.
type ResolveInput struct {
	Imported *bool `json:"imported"`
}

// Resolve: the user looked in Luca and closes a needs_check task as
// imported or not. It is never retried by itself.
func (s *Service) Resolve(ctx context.Context, sc Scope, id uuid.UUID, in ResolveInput) (Task, error) {
	if in.Imported == nil {
		return Task{}, fmt.Errorf("%w: imported is required", ErrInvalid)
	}
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	res := "not_imported"
	if *in.Imported {
		res = "imported"
	}
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.ResolveAgentTask(ctx, db.ResolveAgentTaskParams{
			Resolution: pgtype.Text{String: res, Valid: true}, ResolvedBy: optInt8(sc.UserID),
			OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID,
		}); errors.Is(err, pgx.ErrNoRows) {
			return ErrState
		} else if err != nil {
			return err
		}
		files, err := q.ListTaskFilesOfTask(ctx, db.ListTaskFilesOfTaskParams{OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, TaskID: r.ID})
		if err != nil {
			return err
		}
		status, code := "failed", "NOT_IMPORTED"
		if *in.Imported {
			status, code = "imported", ""
		}
		for _, f := range files {
			if !f.TransferID.Valid || f.Status == FileOK || f.Status == FileFailed {
				continue
			}
			if _, err := q.FinishLucaWebTransfer(ctx, db.FinishLucaWebTransferParams{
				Status: status, CompletedBy: optInt8(sc.UserID), ErrorCode: optCode(code),
				OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: f.TransferID.Int64,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, sc, id)
}

// agentID resolves the office's active desktop agent.
func (s *Service) agentID(ctx context.Context, sc Scope, agent uuid.UUID) (int64, error) {
	if agent == uuid.Nil {
		return 0, ErrAgentUnknown
	}
	a, err := s.q.GetActiveDesktopAgent(ctx, db.GetActiveDesktopAgentParams{OrganizationID: sc.OrganizationID, Uuid: agent})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrAgentUnknown
	}
	if err != nil {
		return 0, err
	}
	return a.ID, nil
}

// Claim lets an office agent take an approved queued task. The agent works
// on one task at a time and a run in order. A bulk export's files are built
// now — after the taxpayer's cari task wrote the Luca codes — inside the
// double export guard; when nothing can go, the task ends as done with
// NOTHING_TO_EXPORT. The same agent claiming again gets the same task.
func (s *Service) Claim(ctx context.Context, sc Scope, id, agent uuid.UUID) (Task, error) {
	if err := s.lucaEnabled(ctx, sc); err != nil {
		return Task{}, err
	}
	agentID, err := s.agentID(ctx, sc, agent)
	if err != nil {
		return Task{}, err
	}
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	if !r.ApprovedAt.Valid {
		return Task{}, ErrNotApproved
	}
	// Aynı ajanın bitmemiş görevi yeniden alınabilir (idempotent claim).
	ownOpen := r.AgentID.Valid && r.AgentID.Int64 == agentID && !Terminal(r.Status)
	if r.Status != StatusQueued && !ownOpen {
		return Task{}, ErrState
	}
	busy, err := s.q.CountAgentBusyTasks(ctx, db.CountAgentBusyTasksParams{OrganizationID: sc.OrganizationID, AgentID: optInt8(agentID), ID: r.ID})
	if err != nil {
		return Task{}, err
	}
	if busy > 0 {
		return Task{}, ErrAgentBusy
	}
	if r.RunUuid.Valid {
		n, err := s.q.CountRunBlockers(ctx, db.CountRunBlockersParams{OrganizationID: sc.OrganizationID, RunUuid: r.RunUuid, Sequence: r.Sequence})
		if err != nil {
			return Task{}, err
		}
		if n > 0 {
			return Task{}, ErrNotReady
		}
	}
	first := r.Status == StatusQueued
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.ClaimAgentTask(ctx, db.ClaimAgentTaskParams{
			AgentID: optInt8(agentID), OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID,
		}); errors.Is(err, pgx.ErrNoRows) {
			return ErrState
		} else if err != nil {
			return err
		}
		if !first || r.Kind != KindBulkExport {
			return nil
		}
		if s.builder == nil {
			return fmt.Errorf("agenttasks: no file builder")
		}
		built, err := s.builder.Build(ctx, q, BuildInput{
			OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, EngagementUUID: r.EngagementUuid,
			TaxpayerID: r.TaxpayerID, TaxpayerUUID: r.TaxpayerUuid, EngagementStatus: r.EngagementStatus,
			EngagementStartedAt: r.EngagementStartedAt.Time,
			TaskID:              r.ID, TaskUUID: r.Uuid, Period: r.Period.String, Kinds: r.Kinds,
			UserID: sc.UserID, AgentID: agentID,
		})
		if err != nil {
			return err
		}
		rep, _ := json.Marshal(built.Report)
		if err := q.SetAgentTaskServerReport(ctx, db.SetAgentTaskServerReportParams{
			ServerReport: rep, OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID,
		}); err != nil {
			return err
		}
		if built.Files == 0 {
			_, err := q.FinishAgentTaskBySystem(ctx, db.FinishAgentTaskBySystemParams{
				Status: StatusDone, ErrorCode: optCode("NOTHING_TO_EXPORT"),
				OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID,
			})
			return err
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, sc, id)
}

// ProgressInput is the agent's counter.
type ProgressInput struct {
	AgentUUID uuid.UUID `json:"agent_uuid"`
	Status    string    `json:"status"`
	Step      int       `json:"step"`
	Steps     int       `json:"steps"`
	Message   string    `json:"message"`
}

// Progress records waiting_user / running and "Dosya 2/4".
func (s *Service) Progress(ctx context.Context, sc Scope, id uuid.UUID, in ProgressInput) (Task, error) {
	if in.Status != StatusWaitingUser && in.Status != StatusRunning {
		return Task{}, fmt.Errorf("%w: status must be waiting_user or running", ErrInvalid)
	}
	if in.Step < 0 || in.Steps < 0 || in.Step > 10000 || in.Steps > 10000 {
		return Task{}, fmt.Errorf("%w: step out of range", ErrInvalid)
	}
	agentID, err := s.agentID(ctx, sc, in.AgentUUID)
	if err != nil {
		return Task{}, err
	}
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	raw, _ := json.Marshal(Progress{Step: in.Step, Steps: in.Steps, Message: clip(in.Message, 200)})
	if _, err := s.q.SetAgentTaskProgress(ctx, db.SetAgentTaskProgressParams{
		Status: in.Status, Progress: raw, OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID, AgentID: optInt8(agentID),
	}); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrState
	} else if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, sc, id)
}

// FileRef is a stored task file for download.
type FileRef struct {
	Name        string
	ContentType string
	SHA256      string
	StorageKey  string
	Size        int64
}

// File returns a file of a task the caller can see (agent download).
func (s *Service) File(ctx context.Context, sc Scope, id, file uuid.UUID) (FileRef, error) {
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return FileRef{}, err
	}
	f, err := s.q.GetAgentTaskFile(ctx, db.GetAgentTaskFileParams{OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, TaskID: r.ID, Uuid: file})
	if errors.Is(err, pgx.ErrNoRows) {
		return FileRef{}, ErrNotFound
	}
	if err != nil {
		return FileRef{}, err
	}
	return FileRef{Name: f.Name, ContentType: f.ContentType, SHA256: f.Sha256, StorageKey: f.StorageKey, Size: f.Size}, nil
}

// FileResultInput is the agent's outcome of one uploaded file.
type FileResultInput struct {
	AgentUUID   uuid.UUID     `json:"agent_uuid"`
	Status      string        `json:"status"`
	LucaMessage string        `json:"luca_message"`
	Entries     []EntryResult `json:"entries"`
}

// maxEntryResults bounds the error report of one file.
const maxEntryResults = 2000

// ReportFile records the outcome of one file right after its upload, so a
// crash later loses nothing. Error rows are matched to the file's
// documents (entry number or document number → document key). A journal
// export's transfer becomes imported / failed with its file.
func (s *Service) ReportFile(ctx context.Context, sc Scope, id, file uuid.UUID, in FileResultInput) (Task, error) {
	switch in.Status {
	case FileOK, FileFailed, FileUnknown:
	default:
		return Task{}, fmt.Errorf("%w: status must be ok, failed or unknown", ErrInvalid)
	}
	if len(in.Entries) > maxEntryResults {
		return Task{}, fmt.Errorf("%w: at most %d entry results", ErrInvalid, maxEntryResults)
	}
	agentID, err := s.agentID(ctx, sc, in.AgentUUID)
	if err != nil {
		return Task{}, err
	}
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	if !r.AgentID.Valid || r.AgentID.Int64 != agentID || Terminal(r.Status) || r.Status == StatusQueued {
		return Task{}, ErrState
	}
	f, err := s.q.GetAgentTaskFile(ctx, db.GetAgentTaskFileParams{OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, TaskID: r.ID, Uuid: file})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	var docs []FileDocument
	_ = json.Unmarshal(f.Documents, &docs)
	entries, err := MatchEntries(in.Entries, docs)
	if err != nil {
		return Task{}, err
	}
	raw, _ := json.Marshal(entries)
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.ReportAgentTaskFile(ctx, db.ReportAgentTaskFileParams{
			Status: in.Status, LucaMessage: optText(in.LucaMessage, 500), EntryResults: raw,
			OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, TaskID: r.ID, Uuid: file,
		}); errors.Is(err, pgx.ErrNoRows) {
			return ErrState
		} else if err != nil {
			return err
		}
		if !f.TransferID.Valid {
			return nil
		}
		return settleTransfer(ctx, q, r, f.TransferID.Int64, in.LucaMessage)
	})
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, sc, id)
}

// TransferOutcome is a journal export's transfer status once each of its
// files has an outcome: all ok → imported, one failed → failed; a file
// without an outcome (or unknown) leaves it open ("" ).
func TransferOutcome(files []string) string {
	failed := false
	for _, st := range files {
		switch st {
		case FilePending, FileUnknown:
			return ""
		case FileFailed:
			failed = true
		}
	}
	if failed {
		return "failed"
	}
	return "imported"
}

// settleTransfer closes a luca_web transfer when every file of its export
// in the task has an outcome.
func settleTransfer(ctx context.Context, q *db.Queries, r db.GetAgentTaskRow, transferID int64, message string) error {
	files, err := q.ListTaskFilesOfTask(ctx, db.ListTaskFilesOfTaskParams{OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, TaskID: r.ID})
	if err != nil {
		return err
	}
	var sts []string
	for _, f := range files {
		if f.TransferID.Valid && f.TransferID.Int64 == transferID {
			sts = append(sts, f.Status)
		}
	}
	st := TransferOutcome(sts)
	if st == "" {
		return nil
	}
	code := ""
	if st == "failed" {
		code = "LUCA_REJECTED"
	}
	_, err = q.FinishLucaWebTransfer(ctx, db.FinishLucaWebTransferParams{
		Status: st, CompletedBy: r.ApprovedBy, ErrorCode: optCode(code), ErrorMessage: optText(message, 2000),
		OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, ID: transferID,
	})
	return err
}

// MatchEntries checks the agent's error rows and fills document_key from
// the file's documents: by document number, else by entry number (the
// 1-based position of the document in the file: Fiş No runs 1…n).
func MatchEntries(in []EntryResult, docs []FileDocument) ([]EntryResult, error) {
	byNo := map[string]string{}
	for _, d := range docs {
		if d.DocumentNo != "" {
			byNo[strings.TrimSpace(d.DocumentNo)] = d.DocumentKey
		}
	}
	out := make([]EntryResult, 0, len(in))
	for _, e := range in {
		if e.Status != FileOK && e.Status != FileFailed {
			return nil, fmt.Errorf("%w: entry status must be ok or failed", ErrInvalid)
		}
		if e.Row < 0 {
			return nil, fmt.Errorf("%w: row out of range", ErrInvalid)
		}
		e.EntryNo = clip(strings.TrimSpace(e.EntryNo), 40)
		e.DocumentNo = clip(strings.TrimSpace(e.DocumentNo), 100)
		e.LucaEntryNo = clip(strings.TrimSpace(e.LucaEntryNo), 40)
		e.Reason = clip(strings.TrimSpace(e.Reason), 300)
		e.DocumentKey = ""
		if k, ok := byNo[e.DocumentNo]; ok && e.DocumentNo != "" {
			e.DocumentKey = k
		} else if n := atoi(e.EntryNo); n >= 1 && n <= len(docs) {
			e.DocumentKey = docs[n-1].DocumentKey
			if e.DocumentNo == "" {
				e.DocumentNo = docs[n-1].DocumentNo
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// CounterpartyResult is the agent's outcome for one counterparty.
type CounterpartyResult struct {
	TaxID       string `json:"tax_id"`
	MainCode    string `json:"main_code"`
	Status      string `json:"status"`
	AccountCode string `json:"account_code"`
	Title       string `json:"title"`
	Message     string `json:"message"`
}

// ResultInput is the agent's final report.
type ResultInput struct {
	AgentUUID      uuid.UUID            `json:"agent_uuid"`
	Outcome        string               `json:"outcome"`
	ErrorCode      string               `json:"error_code"`
	ErrorMessage   string               `json:"error_message"`
	Counterparties []CounterpartyResult `json:"counterparties"`
}

// FinalStatus derives a task's end from the agent's outcome and its files:
// failed / needs_check as reported; otherwise every file ok → done, a
// failed file → partial, a file without an outcome → needs_check (the
// upload may have happened: never retried by itself).
func FinalStatus(outcome string, files []string) string {
	if outcome == StatusFailed || outcome == StatusNeedsCheck {
		return outcome
	}
	failed := false
	for _, st := range files {
		switch st {
		case FilePending, FileUnknown:
			return StatusNeedsCheck
		case FileFailed:
			failed = true
		}
	}
	if failed {
		return StatusPartial
	}
	return StatusDone
}

// Finish records the agent's final report. Cari results are written to the
// counterparty memory (source Luca, the task's approver as decider); a
// memory entry with another code is kept and the row becomes conflict for
// the office to check.
func (s *Service) Finish(ctx context.Context, sc Scope, id uuid.UUID, in ResultInput) (Task, error) {
	switch in.Outcome {
	case StatusDone, StatusFailed, StatusNeedsCheck:
	default:
		return Task{}, fmt.Errorf("%w: outcome must be done, failed or needs_check", ErrInvalid)
	}
	code := strings.TrimSpace(in.ErrorCode)
	if code != "" && !errorCodeRe.MatchString(code) {
		return Task{}, fmt.Errorf("%w: error_code", ErrInvalid)
	}
	if len(in.Counterparties) > 1000 {
		return Task{}, fmt.Errorf("%w: at most 1000 counterparties", ErrInvalid)
	}
	for _, c := range in.Counterparties {
		if err := checkCounterparty(c); err != nil {
			return Task{}, err
		}
	}
	agentID, err := s.agentID(ctx, sc, in.AgentUUID)
	if err != nil {
		return Task{}, err
	}
	r, err := s.row(ctx, sc, id)
	if err != nil {
		return Task{}, err
	}
	if !r.AgentID.Valid || r.AgentID.Int64 != agentID || Terminal(r.Status) {
		return Task{}, ErrState
	}
	err = database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, c := range in.Counterparties {
			if err := s.writeCounterparty(ctx, q, r, c); err != nil {
				return err
			}
		}
		files, err := q.ListTaskFilesOfTask(ctx, db.ListTaskFilesOfTaskParams{OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, TaskID: r.ID})
		if err != nil {
			return err
		}
		sts := make([]string, 0, len(files))
		for _, f := range files {
			sts = append(sts, f.Status)
		}
		status := FinalStatus(in.Outcome, sts)
		if r.Kind == KindCariSync && in.Outcome == StatusDone {
			status = cariStatus(in.Counterparties)
		}
		if _, err := q.FinishAgentTask(ctx, db.FinishAgentTaskParams{
			Status: status, ErrorCode: optCode(code), ErrorMessage: optText(in.ErrorMessage, 2000),
			OrganizationID: sc.OrganizationID, EngagementID: r.EngagementID, ID: r.ID, AgentID: optInt8(agentID),
		}); errors.Is(err, pgx.ErrNoRows) {
			return ErrState
		} else if err != nil {
			return err
		}
		if status == StatusFailed {
			// Nothing was uploaded for the files without an outcome.
			return s.closeTransfers(ctx, q, sc.OrganizationID, r.EngagementID, r.ID, 0, nonEmpty(code, "TASK_FAILED"))
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, sc, id)
}

// cariStatus: every counterparty found or created → done; some failed →
// partial.
func cariStatus(rs []CounterpartyResult) string {
	for _, c := range rs {
		if c.Status == CariFailed {
			return StatusPartial
		}
	}
	return StatusDone
}

func checkCounterparty(c CounterpartyResult) error {
	if !taxIDRe.MatchString(c.TaxID) || !mainCodeRe.MatchString(c.MainCode) {
		return fmt.Errorf("%w: counterparty tax_id / main_code", ErrInvalid)
	}
	switch c.Status {
	case CariFound, CariCreated:
		if !accountRe.MatchString(c.AccountCode) || !strings.HasPrefix(c.AccountCode, c.MainCode) {
			return fmt.Errorf("%w: counterparty account_code %q", ErrInvalid, c.AccountCode)
		}
	case CariFailed:
	default:
		return fmt.Errorf("%w: counterparty status must be found, created or failed", ErrInvalid)
	}
	return nil
}

// writeCounterparty stores one cari result and, when found / created,
// writes the code to the counterparty memory without overwriting another
// decision.
func (s *Service) writeCounterparty(ctx context.Context, q *db.Queries, r db.GetAgentTaskRow, c CounterpartyResult) error {
	p := db.SetAgentTaskCounterpartyResultParams{
		Status: c.Status, Message: optText(c.Message, 500), Title: clip(c.Title, 300),
		OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, TaskID: r.ID, TaxID: c.TaxID, MainCode: c.MainCode,
	}
	if c.Status == CariFound || c.Status == CariCreated {
		p.AccountCode = pgtype.Text{String: c.AccountCode, Valid: true}
		existing, err := q.GetCounterpartyAccountCode(ctx, db.GetCounterpartyAccountCodeParams{
			OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, TaxID: c.TaxID, MainCode: c.MainCode,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if _, err := q.InsertCounterpartyAccountFromLuca(ctx, db.InsertCounterpartyAccountFromLucaParams{
				OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, TaxpayerID: r.TaxpayerID,
				TaxID: c.TaxID, MainCode: c.MainCode, AccountCode: c.AccountCode, AccountName: clip(c.Title, 300),
				DecidedBy: r.ApprovedBy,
			}); err != nil {
				return err
			}
		case err != nil:
			return err
		case existing != c.AccountCode:
			p.Status = CariConflict
			p.HakanAccountCode = pgtype.Text{String: existing, Valid: true}
		}
	}
	// A counterparty the task did not ask for is ignored.
	_, err := q.SetAgentTaskCounterpartyResult(ctx, p)
	return err
}

// closeTransfers fails the luca_web transfers of a task's files without an
// outcome, so the journal export can be sent again.
func (s *Service) closeTransfers(ctx context.Context, q *db.Queries, orgID, engID, taskID, userID int64, code string) error {
	files, err := q.ListTaskFilesOfTask(ctx, db.ListTaskFilesOfTaskParams{OrganizationID: orgID, EngagementID: engID, TaskID: taskID})
	if err != nil {
		return err
	}
	for _, f := range files {
		if !f.TransferID.Valid || f.Status != FilePending {
			continue
		}
		if _, err := q.FinishLucaWebTransfer(ctx, db.FinishLucaWebTransferParams{
			Status: "failed", CompletedBy: optInt8(userID), ErrorCode: optCode(code),
			OrganizationID: orgID, EngagementID: engID, ID: f.TransferID.Int64,
		}); err != nil {
			return err
		}
	}
	return nil
}

func taskView(r db.GetAgentTaskRow, files []db.AgentTaskFile, cps []db.AgentTaskCounterparty) Task {
	t := Task{
		UUID: r.Uuid, Sequence: r.Sequence, Kind: r.Kind, EngagementUUID: r.EngagementUuid,
		Taxpayer: TaskTaxpayer{Title: r.TaxpayerTitle, TaxID: r.TaxpayerTaxID},
		Kinds:    nonNilStrings(r.Kinds), Status: r.Status, Approved: r.ApprovedAt.Valid,
		ApprovedByName: r.ApprovedByName, ApprovedAt: tsPtr(r.ApprovedAt), RequestedByName: r.RequestedByName,
		ErrorCode: r.ErrorCode.String, ErrorMessage: r.ErrorMessage.String,
		ClaimedAt: tsPtr(r.ClaimedAt), FinishedAt: tsPtr(r.FinishedAt),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		Files: make([]FileView, 0, len(files)), Counterparties: make([]CounterpartyView, 0, len(cps)),
	}
	if !r.ApprovedAt.Valid {
		t.ApprovedByName = ""
	}
	if r.RunUuid.Valid {
		u := uuid.UUID(r.RunUuid.Bytes)
		t.RunUUID = &u
	}
	if r.Period.Valid {
		p := r.Period.String
		t.Period = &p
	}
	if r.Resolution.Valid {
		res := r.Resolution.String
		t.Resolution = &res
	}
	if r.AgentUuid.Valid {
		t.Agent = &TaskAgent{UUID: uuid.UUID(r.AgentUuid.Bytes), Name: r.AgentName.String}
	}
	if len(r.Progress) > 0 {
		var p Progress
		if json.Unmarshal(r.Progress, &p) == nil {
			t.Progress = &p
		}
	}
	if len(r.ServerReport) > 0 {
		var rep ServerReport
		if json.Unmarshal(r.ServerReport, &rep) == nil {
			rep.Check, rep.Skipped, rep.AlreadyExported = nonNilRows(rep.Check), nonNilRows(rep.Skipped), nonNilRows(rep.AlreadyExported)
			t.ServerReport = &rep
		}
	}
	for _, f := range files {
		v := FileView{
			UUID: f.Uuid, Sequence: f.Sequence, Kind: f.Kind, Name: f.Name, ContentType: f.ContentType,
			Size: f.Size, SHA256: f.Sha256, Entries: f.Entries, Status: f.Status, LucaMessage: f.LucaMessage.String,
			Documents: []FileDocument{}, EntryResults: []EntryResult{}, ReportedAt: tsPtr(f.ReportedAt),
		}
		_ = json.Unmarshal(f.Documents, &v.Documents)
		_ = json.Unmarshal(f.EntryResults, &v.EntryResults)
		if v.Documents == nil {
			v.Documents = []FileDocument{}
		}
		if v.EntryResults == nil {
			v.EntryResults = []EntryResult{}
		}
		t.Files = append(t.Files, v)
	}
	for _, c := range cps {
		t.Counterparties = append(t.Counterparties, CounterpartyView{
			TaxID: c.TaxID, Title: c.Title, MainCode: c.MainCode, Status: c.Status,
			AccountCode: c.AccountCode.String, HakanAccountCode: c.HakanAccountCode.String, Message: c.Message.String,
		})
	}
	return t
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func optInt8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v > 0} }

func optText(s string, max int) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: clip(s, max), Valid: true}
}

func optCode(code string) pgtype.Text { return pgtype.Text{String: code, Valid: code != ""} }

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// clip cuts s to max runes.
func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

func atoi(s string) int {
	n := 0
	if s == "" || len(s) > 6 {
		return 0
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func nonNilRows(v []ReportRow) []ReportRow {
	if v == nil {
		return []ReportRow{}
	}
	return v
}
