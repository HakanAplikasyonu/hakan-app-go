package taxreturns

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// MaxAccrualBytes: a GİB tahakkuk fişi is a page or two; 10 MB is generous.
const MaxAccrualBytes = 10 << 20

// ObjectStore is the storage subset for the accrual slips.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// AccrualDocument is the attached tahakkuk PDF of a filed return.
type AccrualDocument struct {
	FileName   string    `json:"file_name"`
	SizeBytes  int64     `json:"size_bytes"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// AccrualFile is an opened accrual slip.
type AccrualFile struct {
	Body io.ReadCloser
	Size int64
	Name string
}

// UploadAccrual attaches the GİB tahakkuk PDF to a filed return; a new file
// replaces the old one (a wrong slip can be corrected).
func (s *Service) UploadAccrual(ctx context.Context, sc engagementctx.Scope, actorID int64, kind, period, fileName string, data []byte) (Preview, error) {
	if err := check(kind, period); err != nil {
		return Preview{}, err
	}
	if s.store == nil {
		return Preview{}, ErrStorageMissing
	}
	if len(data) == 0 || len(data) > MaxAccrualBytes {
		return Preview{}, fmt.Errorf("%w: the file is empty or larger than 10 MB", ErrInvalidInput)
	}
	if !bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 1024)], "\x00\t\r\n "), []byte("%PDF-")) {
		return Preview{}, ErrNotPDF
	}
	rec, err := s.record(ctx, sc, kind, period)
	if err != nil {
		return Preview{}, err
	}
	if rec == nil || rec.Status != StatusFiled {
		return Preview{}, ErrNotFiled
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	name := accrualFileName(fileName, kind, period)
	key := fmt.Sprintf("tax-returns/%d/%d/%s/%s/%s.pdf", sc.OrganizationID, sc.ID, kind, period, digest)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/pdf", Filename: name}, key); err != nil {
		return Preview{}, fmt.Errorf("tax return accrual upload: %w", err)
	}
	_, err = s.q.SetTaxReturnAccrualDocument(ctx, db.SetTaxReturnAccrualDocumentParams{
		StorageKey: key, FileName: name, SizeBytes: int64(len(data)), Sha256: digest, UploadedBy: actor(actorID),
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Kind: kind, Period: period,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Reopened or gone between the read and the write.
		return Preview{}, ErrNotFiled
	}
	if err != nil {
		return Preview{}, fmt.Errorf("set tax return accrual: %w", err)
	}
	if old := rec.AccrualStorageKey; old.Valid && old.String != key {
		// The row points at the new file; the old object is only storage.
		_ = s.store.Delete(ctx, old.String)
	}
	return s.Preview(ctx, sc, kind, period)
}

// OpenAccrual opens the attached tahakkuk PDF.
func (s *Service) OpenAccrual(ctx context.Context, sc engagementctx.Scope, kind, period string) (AccrualFile, error) {
	if err := check(kind, period); err != nil {
		return AccrualFile{}, err
	}
	if s.store == nil {
		return AccrualFile{}, ErrStorageMissing
	}
	rec, err := s.record(ctx, sc, kind, period)
	if err != nil {
		return AccrualFile{}, err
	}
	if rec == nil || !rec.AccrualStorageKey.Valid {
		return AccrualFile{}, ErrNotFound
	}
	rc, size, err := s.store.Download(ctx, rec.AccrualStorageKey.String)
	if err != nil {
		return AccrualFile{}, fmt.Errorf("tax return accrual download: %w", err)
	}
	return AccrualFile{Body: rc, Size: size, Name: rec.AccrualFileName.String}, nil
}

// accrualFileName keeps the uploaded name's base (a .pdf, at most 120 runes)
// or names the slip after the return.
func accrualFileName(n, kind, period string) string {
	n = strings.TrimSpace(path.Base(strings.ReplaceAll(n, "\\", "/")))
	n = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '/' {
			return -1
		}
		return r
	}, n)
	if n == "" || n == "." || n == "/" {
		n = fmt.Sprintf("tahakkuk-%s-%s.pdf", kind, period)
	}
	if r := []rune(n); len(r) > 120 {
		n = string(r[:116])
	}
	if !strings.HasSuffix(strings.ToLower(n), ".pdf") {
		n += ".pdf"
	}
	return n
}
