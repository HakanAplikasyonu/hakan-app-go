package doctasks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// ListMine returns the tasks of the mükellef's active engagement (soonest
// due first). Tasks of an ended engagement (an earlier office) are not
// listed (D1, D2).
func (s *Service) ListMine(ctx context.Context, taxpayerID int64, f Filter) ([]Task, int64, error) {
	status := optText(f.Status)
	rows, err := s.q.ListTaxpayerDocumentTasks(ctx, db.ListTaxpayerDocumentTasksParams{
		TaxpayerID: taxpayerID, Status: status, Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountTaxpayerDocumentTasks(ctx, db.CountTaxpayerDocumentTasksParams{TaxpayerID: taxpayerID, Status: status})
	if err != nil {
		return nil, 0, err
	}
	now := s.now()
	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, taxpayerTask(db.GetTaxpayerDocumentTaskRow(r), now))
	}
	return out, total, nil
}

// GetMine returns one task of the mükellef with its uploads.
func (s *Service) GetMine(ctx context.Context, taxpayerID int64, id uuid.UUID) (Task, error) {
	r, err := s.q.GetTaxpayerDocumentTask(ctx, db.GetTaxpayerDocumentTaskParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	t := taxpayerTask(r, s.now())
	if t.Uploads, err = s.uploads(ctx, t); err != nil {
		return Task{}, err
	}
	return t, nil
}

// UploadResult is what an upload did.
type UploadResult struct {
	Task   Task   `json:"task"`
	Upload Upload `json:"upload"`
	// Matched: the document was linked to the bank transaction.
	Matched bool `json:"matched"`
	// WarningResolved: the BNK_* warning was auto-resolved by the re-run.
	WarningResolved bool `json:"warning_resolved"`
}

// Upload stores a photo / PDF for the mükellef's task, links it to the bank
// transaction and re-runs the control for that transaction, so the warning
// becomes auto_resolved.
func (s *Service) Upload(ctx context.Context, taxpayerID int64, taskID uuid.UUID, userID int64, fileName string, data []byte) (UploadResult, error) {
	var res UploadResult
	if s.store == nil {
		return res, errors.New("doctasks: storage is not configured")
	}
	if len(data) == 0 {
		return res, ErrEmpty
	}
	if len(data) > MaxUploadBytes {
		return res, ErrTooLarge
	}
	contentType, kind, ext, ok := sniff(data)
	if !ok {
		return res, ErrUnsupported
	}
	// The task must exist before anything is written.
	if _, err := s.q.GetTaxpayerDocumentTask(ctx, db.GetTaxpayerDocumentTaskParams{TaxpayerID: taxpayerID, Uuid: taskID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return res, ErrNotFound
		}
		return res, err
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	name := cleanName(fileName, ext)
	// Content-addressed: the same file uploaded twice is one object and one
	// attachment row (uq_attachments_taxpayer_sha256).
	key := fmt.Sprintf("attachments/%d/%s%s", taxpayerID, sha, ext)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), Filename: name, ContentType: contentType}, key); err != nil {
		return res, fmt.Errorf("doctasks: store file: %w", err)
	}

	tx, err := database.Begin(ctx, s.db)
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	task, err := q.LockTaxpayerDocumentTask(ctx, db.LockTaxpayerDocumentTaskParams{TaxpayerID: taxpayerID, Uuid: taskID})
	if errors.Is(err, pgx.ErrNoRows) {
		return res, ErrNotFound
	}
	if err != nil {
		return res, err
	}
	if task.Status != StatusOpen && task.Status != StatusSubmitted {
		return res, ErrClosed
	}
	att, err := q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: taxpayerID, Kind: kind, StorageKey: key, Sha256: sha, SizeBytes: int64(len(data)),
		ContentType: pgtype.Text{String: contentType, Valid: true},
		FileName:    pgtype.Text{String: name, Valid: true},
	})
	if err != nil {
		return res, fmt.Errorf("doctasks: attachment: %w", err)
	}
	user := pgtype.Int8{Int64: userID, Valid: userID > 0}
	up, err := q.InsertDocumentTaskUpload(ctx, db.InsertDocumentTaskUploadParams{
		OrganizationID: task.OrganizationID, EngagementID: task.EngagementID, TaxpayerID: taxpayerID,
		TaskID: task.ID, AttachmentID: att.ID, UploadedBy: user,
	})
	if err != nil {
		return res, fmt.Errorf("doctasks: upload row: %w", err)
	}
	// A document request (issue #240) waits for the office; a bank task is
	// matched to its transaction automatically unless that was cancelled.
	status := StatusSubmitted
	if task.BankTransactionID.Valid {
		cancelled, err := q.IsBankTransactionCancelled(ctx, db.IsBankTransactionCancelledParams{TaxpayerID: taxpayerID, ID: task.BankTransactionID.Int64})
		if err != nil {
			return res, err
		}
		if !cancelled {
			if err := q.InsertBankTransactionDocument(ctx, db.InsertBankTransactionDocumentParams{
				TaxpayerID: taxpayerID, BankTransactionID: task.BankTransactionID.Int64, AttachmentID: att.ID,
				Method: "taxpayer_upload", MatchedBy: user,
			}); err != nil {
				return res, fmt.Errorf("doctasks: match: %w", err)
			}
			status = StatusMatched
		}
	}
	if err := q.SetDocumentTaskStatus(ctx, db.SetDocumentTaskStatusParams{
		Status: status, OrganizationID: task.OrganizationID, EngagementID: task.EngagementID, ID: task.ID,
	}); err != nil {
		return res, err
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	res.Matched = status == StatusMatched
	res.Upload = Upload{
		UUID: up.Uuid, AttachmentUUID: att.Uuid, Kind: att.Kind, ContentType: textPtr(att.ContentType),
		FileName: textPtr(att.FileName), SizeBytes: att.SizeBytes, SHA256: att.Sha256, CreatedAt: up.CreatedAt.Time,
	}

	// Re-run the control for this transaction. A failure here leaves the
	// match in place; the next detection run resolves the warning.
	if task.BankTransactionID.Valid {
		sub := engine.Subject{OrganizationID: task.OrganizationID, EngagementID: task.EngagementID, TaxpayerID: taxpayerID}
		if rep, err := s.DetectTransaction(ctx, sub, task.BankTransactionID.Int64); err != nil {
			s.log.Warn("doctasks_redetect_failed", "error", err, "task", taskID)
		} else {
			res.WarningResolved = rep.AutoResolved > 0
		}
	}
	if res.Task, err = s.GetMine(ctx, taxpayerID, taskID); err != nil {
		return res, err
	}
	// The office hears about it (issue #240): who opened the task and the
	// engagement's assignees.
	s.notifyOffice(ctx, task.OrganizationID, task.EngagementID, taxpayerID, task.CreatedBy, res.Task)
	return res, nil
}

// File is a stored upload to stream back.
type File struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Name        string
}

// OfficeFile opens an upload of an engagement task.
func (s *Service) OfficeFile(ctx context.Context, scope engagementctx.Scope, taskID, uploadID uuid.UUID) (File, error) {
	r, err := s.q.GetDocumentTaskUploadFile(ctx, db.GetDocumentTaskUploadFileParams{
		OrganizationID: scope.OrganizationID, EngagementID: scope.ID, TaskUuid: taskID, UploadUuid: uploadID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	return s.open(ctx, r.StorageKey, r.ContentType, r.FileName)
}

// MyFile opens an upload of the mükellef's task.
func (s *Service) MyFile(ctx context.Context, taxpayerID int64, taskID, uploadID uuid.UUID) (File, error) {
	r, err := s.q.GetTaxpayerDocumentTaskUploadFile(ctx, db.GetTaxpayerDocumentTaskUploadFileParams{
		TaxpayerID: taxpayerID, TaskUuid: taskID, UploadUuid: uploadID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	return s.open(ctx, r.StorageKey, r.ContentType, r.FileName)
}

func (s *Service) open(ctx context.Context, key string, ct, name pgtype.Text) (File, error) {
	if s.store == nil {
		return File{}, errors.New("doctasks: storage is not configured")
	}
	rc, size, err := s.store.Download(ctx, key)
	if err != nil {
		return File{}, fmt.Errorf("doctasks: download: %w", err)
	}
	f := File{Body: rc, Size: size, ContentType: "application/octet-stream", Name: "document"}
	if ct.Valid && ct.String != "" {
		f.ContentType = ct.String
	}
	if name.Valid && name.String != "" {
		f.Name = name.String
	}
	return f, nil
}

func taxpayerTask(r db.GetTaxpayerDocumentTaskRow, now time.Time) Task {
	t := Task{
		UUID: r.Uuid, Code: r.Code, Kind: r.Kind, Period: r.Period, Status: r.Status, Title: r.Title, Note: r.Note,
		DueAt: r.DueAt.Time, OrganizationName: r.OrganizationName,
		SubmittedAt: tsPtr(r.SubmittedAt), MatchedAt: tsPtr(r.MatchedAt),
		UploadCount: r.UploadCount, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		id: r.ID, organizationID: r.OrganizationID, engagementID: r.EngagementID,
		taxpayerID: r.TaxpayerID, bankTransactionID: r.BankTransactionID,
	}
	t.BankTransaction = bankTransaction(t, r.BankTransactionUuid, r.TransactionDate, r.DebitCredit, r.Amount, r.Currency, r.CounterpartyName, r.Description)
	t.Overdue = t.Status == StatusOpen && !now.Before(t.DueAt)
	return t
}

// sniff accepts photos (JPEG, PNG, WebP, HEIC/HEIF) and PDF by content, not
// by the file name or the client's Content-Type.
func sniff(data []byte) (contentType, kind, ext string, ok bool) {
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1":
			return "image/heic", "image", ".heic", true
		}
	}
	switch ct := http.DetectContentType(data); ct {
	case "application/pdf":
		return ct, "pdf", ".pdf", true
	case "image/jpeg":
		return ct, "image", ".jpg", true
	case "image/png":
		return ct, "image", ".png", true
	case "image/webp":
		return ct, "image", ".webp", true
	}
	return "", "", "", false
}

func cleanName(name, ext string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "document" + ext
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	return name
}
