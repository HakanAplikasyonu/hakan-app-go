package journals

// Transfers to an accounting program (issue #391,
// docs/arastirma/luca-zirve-aktarim.md §4): the office's desktop agent puts
// an export's files into the taxpayer's import folder and reports the
// outcome; the user imports them in the program and closes the transfer as
// imported or failed. No Luca / Zirve credential is needed or stored.

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

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// Transfer methods.
const (
	// MethodAgentFolder: the desktop agent writes the files to a folder.
	MethodAgentFolder = "agent_folder"
	// MethodManual: the office downloaded the files and uploaded them to the
	// program by hand ("Luca'ya yükledim", #780). The transfer is imported
	// at once, with who and when.
	MethodManual = "manual"
)

// Transfer statuses.
const (
	TransferQueued    = "queued"
	TransferClaimed   = "claimed"
	TransferDelivered = "delivered"
	TransferImported  = "imported"
	TransferFailed    = "failed"
	TransferCancelled = "cancelled"
)

// transferDrivers are the export drivers whose files go to a program. The
// formula workbook is for people, not for an import screen.
var transferDrivers = map[string]bool{"luca": true, "zirve": true}

// TransferDriver reports whether exports of driver can be sent to a program.
func TransferDriver(driver string) bool { return transferDrivers[driver] }

var (
	ErrTransferExists   = errors.New("journals: the export already has an open or imported transfer")
	ErrTransferDriver   = errors.New("journals: exports of this program cannot be transferred")
	ErrTransferState    = errors.New("journals: the transfer is not in a state that allows this")
	ErrTransferReversed = errors.New("journals: every entry of the export was reversed")
	ErrAgentUnknown     = errors.New("journals: desktop agent not found or revoked")
	ErrTransferInvalid  = errors.New("journals: invalid transfer request")
)

// errorCodeRe bounds the codes the agent reports (FOLDER_MISSING …).
var errorCodeRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

// TransferFile is one file the agent wrote to the folder.
type TransferFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// TransferAgent names the desktop agent of a transfer.
type TransferAgent struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Transfer is the API view of a transfer.
type Transfer struct {
	UUID      uuid.UUID `json:"uuid"`
	BatchUUID uuid.UUID `json:"batch_uuid"`
	Period    string    `json:"period,omitempty"`
	Driver    string    `json:"driver"`
	Method    string    `json:"method"`
	Status    string    `json:"status"`
	// CompletedByName: who closed the transfer (imported / failed /
	// cancelled), or marked a manual upload.
	CompletedByName string                  `json:"completed_by_name,omitempty"`
	Agent           *TransferAgent          `json:"agent,omitempty"`
	Files           []accountingexport.File `json:"files"`
	DeliveredFiles  []TransferFile          `json:"delivered_files"`
	FolderLabel     string                  `json:"folder_label,omitempty"`
	ErrorCode       string                  `json:"error_code,omitempty"`
	ErrorMessage    string                  `json:"error_message,omitempty"`
	ClaimedAt       *time.Time              `json:"claimed_at,omitempty"`
	DeliveredAt     *time.Time              `json:"delivered_at,omitempty"`
	CompletedAt     *time.Time              `json:"completed_at,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
}

// TransferPage is a page of transfers.
type TransferPage struct {
	Items  []Transfer `json:"items"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

// TransferFilter narrows ListTransfers.
type TransferFilter struct {
	Status string
	Limit  int32
	Offset int32
}

// RequestTransfer queues the export batch for the program. A batch has at
// most one open or imported transfer: the same entries never reach the
// program twice. MethodManual records a hand upload of the downloaded files
// as imported (who and when), for offices without the desktop agent.
func (s *Service) RequestTransfer(ctx context.Context, scope engagementctx.Scope, actorID int64, batch uuid.UUID, method string) (Transfer, error) {
	if method == "" {
		method = MethodAgentFolder
	}
	if method != MethodAgentFolder && method != MethodManual {
		return Transfer{}, fmt.Errorf("%w: method %q", ErrTransferInvalid, method)
	}
	b, err := s.q.GetExportBatch(ctx, db.GetExportBatchParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: batch})
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrNotFound
	}
	if err != nil {
		return Transfer{}, err
	}
	if !TransferDriver(b.Driver) {
		return Transfer{}, ErrTransferDriver
	}
	items, err := s.q.ListExportBatchItems(ctx, db.ListExportBatchItemsParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ExportBatchID: b.ID})
	if err != nil {
		return Transfer{}, err
	}
	live := 0
	for _, it := range items {
		if !it.ReversedAt.Valid {
			live++
		}
	}
	if live == 0 {
		return Transfer{}, ErrTransferReversed
	}
	var row db.AccountingTransfer
	if method == MethodManual {
		row, err = s.q.InsertManualAccountingTransfer(ctx, db.InsertManualAccountingTransferParams{
			OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ExportBatchID: b.ID,
			Driver: b.Driver, RequestedBy: actorID,
		})
	} else {
		row, err = s.q.InsertAccountingTransfer(ctx, db.InsertAccountingTransferParams{
			OrganizationID: scope.OrganizationID, EngagementID: scope.ID, ExportBatchID: b.ID,
			Driver: b.Driver, Method: method, RequestedBy: actorID,
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrTransferExists
	}
	if err != nil {
		return Transfer{}, err
	}
	return s.GetTransfer(ctx, scope, row.Uuid)
}

// GetTransfer returns one transfer.
func (s *Service) GetTransfer(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) (Transfer, error) {
	r, err := s.q.GetAccountingTransfer(ctx, db.GetAccountingTransferParams{OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrNotFound
	}
	if err != nil {
		return Transfer{}, err
	}
	return transferView(db.ListAccountingTransfersRow(r)), nil
}

// ListTransfers lists the engagement's transfers, newest first.
func (s *Service) ListTransfers(ctx context.Context, scope engagementctx.Scope, f TransferFilter) (TransferPage, error) {
	status := pgtype.Text{String: f.Status, Valid: f.Status != ""}
	rows, err := s.q.ListAccountingTransfers(ctx, db.ListAccountingTransfersParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Status: status, Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return TransferPage{}, err
	}
	total, err := s.q.CountAccountingTransfers(ctx, db.CountAccountingTransfersParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Status: status,
	})
	if err != nil {
		return TransferPage{}, err
	}
	out := TransferPage{Items: make([]Transfer, 0, len(rows)), Total: total, Limit: f.Limit, Offset: f.Offset}
	for _, r := range rows {
		out.Items = append(out.Items, transferView(r))
	}
	return out, nil
}

// ClaimTransfer lets a desktop agent of the office take a queued transfer.
// A transfer claimed by an agent that went silent for 15 minutes can be
// taken again.
func (s *Service) ClaimTransfer(ctx context.Context, scope engagementctx.Scope, id, agent uuid.UUID) (Transfer, error) {
	agentID, err := s.agentID(ctx, scope, agent)
	if err != nil {
		return Transfer{}, err
	}
	_, err = s.q.ClaimAccountingTransfer(ctx, db.ClaimAccountingTransferParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id, AgentID: pgtype.Int8{Int64: agentID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, s.stateOrMissing(ctx, scope, id)
	}
	if err != nil {
		return Transfer{}, err
	}
	return s.GetTransfer(ctx, scope, id)
}

// DeliveryInput is the agent's report.
type DeliveryInput struct {
	AgentUUID    uuid.UUID      `json:"agent_uuid"`
	Outcome      string         `json:"outcome"` // delivered | failed
	Files        []TransferFile `json:"files"`
	FolderLabel  string         `json:"folder_label"`
	ErrorCode    string         `json:"error_code"`
	ErrorMessage string         `json:"error_message"`
}

// ReportDelivery records the agent's outcome for a transfer it claimed.
func (s *Service) ReportDelivery(ctx context.Context, scope engagementctx.Scope, id uuid.UUID, in DeliveryInput) (Transfer, error) {
	agentID, err := s.agentID(ctx, scope, in.AgentUUID)
	if err != nil {
		return Transfer{}, err
	}
	p := db.DeliverAccountingTransferParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id,
		AgentID: pgtype.Int8{Int64: agentID, Valid: true}, FolderLabel: optText(in.FolderLabel, 200),
		DeliveredFiles: []byte("[]"),
	}
	switch in.Outcome {
	case TransferDelivered:
		if len(in.Files) == 0 || len(in.Files) > 100 {
			return Transfer{}, fmt.Errorf("%w: delivered needs the files written", ErrTransferInvalid)
		}
		for _, f := range in.Files {
			if strings.TrimSpace(f.Name) == "" || len(f.Name) > 255 || len(f.SHA256) != 64 {
				return Transfer{}, fmt.Errorf("%w: file name and sha256 are required", ErrTransferInvalid)
			}
		}
		p.Status = TransferDelivered
		p.DeliveredFiles, _ = json.Marshal(in.Files)
	case TransferFailed:
		code, msg, err := errorFields(in.ErrorCode, in.ErrorMessage)
		if err != nil {
			return Transfer{}, err
		}
		p.Status, p.ErrorCode, p.ErrorMessage = TransferFailed, code, msg
	default:
		return Transfer{}, fmt.Errorf("%w: outcome must be delivered or failed", ErrTransferInvalid)
	}
	if _, err := s.q.DeliverAccountingTransfer(ctx, p); errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, s.stateOrMissing(ctx, scope, id)
	} else if err != nil {
		return Transfer{}, err
	}
	return s.GetTransfer(ctx, scope, id)
}

// CompleteInput is the user's closing of a delivered transfer.
type CompleteInput struct {
	Outcome string `json:"outcome"` // imported | failed
	Message string `json:"message"`
}

// CompleteTransfer closes a delivered transfer: the user imported the files
// in the program, or the program rejected them (with its message).
func (s *Service) CompleteTransfer(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID, in CompleteInput) (Transfer, error) {
	p := db.CompleteAccountingTransferParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id, CompletedBy: actor(actorID),
	}
	switch in.Outcome {
	case TransferImported:
		p.Status = TransferImported
	case TransferFailed:
		if strings.TrimSpace(in.Message) == "" {
			return Transfer{}, fmt.Errorf("%w: write the program's error message", ErrTransferInvalid)
		}
		_, msg, err := errorFields("PROGRAM_REJECTED", in.Message)
		if err != nil {
			return Transfer{}, err
		}
		p.Status, p.ErrorCode, p.ErrorMessage = TransferFailed, pgtype.Text{String: "PROGRAM_REJECTED", Valid: true}, msg
	default:
		return Transfer{}, fmt.Errorf("%w: outcome must be imported or failed", ErrTransferInvalid)
	}
	if _, err := s.q.CompleteAccountingTransfer(ctx, p); errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, s.stateOrMissing(ctx, scope, id)
	} else if err != nil {
		return Transfer{}, err
	}
	return s.GetTransfer(ctx, scope, id)
}

// CancelTransfer withdraws an open transfer.
func (s *Service) CancelTransfer(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID) (Transfer, error) {
	_, err := s.q.CancelAccountingTransfer(ctx, db.CancelAccountingTransferParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, Uuid: id, CompletedBy: actor(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, s.stateOrMissing(ctx, scope, id)
	}
	if err != nil {
		return Transfer{}, err
	}
	return s.GetTransfer(ctx, scope, id)
}

// RetryTransfer queues a failed or cancelled transfer's export again as a
// new transfer; the old one stays in the history with its error.
func (s *Service) RetryTransfer(ctx context.Context, scope engagementctx.Scope, actorID int64, id uuid.UUID) (Transfer, error) {
	t, err := s.GetTransfer(ctx, scope, id)
	if err != nil {
		return Transfer{}, err
	}
	if t.Status != TransferFailed && t.Status != TransferCancelled {
		return Transfer{}, ErrTransferState
	}
	return s.RequestTransfer(ctx, scope, actorID, t.BatchUUID, t.Method)
}

func (s *Service) agentID(ctx context.Context, scope engagementctx.Scope, agent uuid.UUID) (int64, error) {
	if agent == uuid.Nil {
		return 0, ErrAgentUnknown
	}
	a, err := s.q.GetActiveDesktopAgent(ctx, db.GetActiveDesktopAgentParams{OrganizationID: scope.OrganizationID, Uuid: agent})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrAgentUnknown
	}
	if err != nil {
		return 0, err
	}
	return a.ID, nil
}

// stateOrMissing explains a guarded update that touched no row.
func (s *Service) stateOrMissing(ctx context.Context, scope engagementctx.Scope, id uuid.UUID) error {
	if _, err := s.GetTransfer(ctx, scope, id); err != nil {
		return err
	}
	return ErrTransferState
}

func errorFields(code, msg string) (pgtype.Text, pgtype.Text, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "AGENT_ERROR"
	}
	if !errorCodeRe.MatchString(code) {
		return pgtype.Text{}, pgtype.Text{}, fmt.Errorf("%w: error_code", ErrTransferInvalid)
	}
	return pgtype.Text{String: code, Valid: true}, optText(msg, 2000), nil
}

// optText trims s and cuts it to max runes; empty is NULL.
func optText(s string, max int) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return pgtype.Text{String: s, Valid: true}
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func transferView(r db.ListAccountingTransfersRow) Transfer {
	out := Transfer{
		UUID: r.Uuid, BatchUUID: r.BatchUuid, Period: r.BatchPeriod.String, Driver: r.Driver, Method: r.Method, Status: r.Status,
		CompletedByName: r.CompletedByName,
		Files:           []accountingexport.File{}, DeliveredFiles: []TransferFile{},
		FolderLabel: r.FolderLabel.String, ErrorCode: r.ErrorCode.String, ErrorMessage: r.ErrorMessage.String,
		ClaimedAt: tsPtr(r.ClaimedAt), DeliveredAt: tsPtr(r.DeliveredAt), CompletedAt: tsPtr(r.CompletedAt), CreatedAt: r.CreatedAt.Time,
	}
	if r.AgentUuid.Valid {
		out.Agent = &TransferAgent{UUID: r.AgentUuid.Bytes, Name: r.AgentName.String}
	}
	var stored []BatchFile
	if json.Unmarshal(r.BatchFiles, &stored) == nil {
		for _, f := range stored {
			out.Files = append(out.Files, f.File)
		}
	}
	_ = json.Unmarshal(r.DeliveredFiles, &out.DeliveredFiles)
	if out.DeliveredFiles == nil {
		out.DeliveredFiles = []TransferFile{}
	}
	return out
}
