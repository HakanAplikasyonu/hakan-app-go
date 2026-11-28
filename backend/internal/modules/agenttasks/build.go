package agenttasks

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/lucalists"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// ListExporter writes a Luca list file (lucalists.Service).
type ListExporter interface {
	Export(ctx context.Context, sc engagementctx.Scope, userID int64, list lucalists.List, in lucalists.Input) (lucalists.Result, error)
}

// ExpenseExporter writes the expenses' Luca fiş file (receipts usecase).
type ExpenseExporter interface {
	ExportExpensesLuca(ctx context.Context, sc engagementctx.Scope, userID int64, in receiptsusecase.LucaExportInput) (receiptsusecase.LucaFile, error)
}

// Uploader stores a built file.
type Uploader interface {
	Upload(ctx context.Context, file storage.File, path string) error
}

// Builder makes the files of a bulk export with the existing writers: the
// Luca list files (bank, Z, invoices), the expenses' fiş file and the
// journal exports not yet sent. Every writer applies the double export
// guard and records what went (a document already sent stays out) and
// splits by Luca's limits (50 vouchers per upload, 400 lines per mahsup):
// a zip of several files becomes several task files.
type Builder struct {
	Lists    ListExporter
	Expenses ExpenseExporter
	Store    Uploader
}

// BuildInput names the task.
type BuildInput struct {
	OrganizationID      int64
	EngagementID        int64
	EngagementUUID      uuid.UUID
	EngagementStatus    string
	EngagementStartedAt time.Time
	TaxpayerID          int64
	TaxpayerUUID        uuid.UUID
	TaskID              int64
	TaskUUID            uuid.UUID
	Period              string
	Kinds               []string
	UserID              int64
	AgentID             int64
}

// Built is what Build made.
type Built struct {
	Files  int
	Report ServerReport
}

// listOf maps a file kind to its Luca list.
var listOf = map[string]lucalists.List{
	FileBank:     lucalists.BankTransactions,
	FileZ:        lucalists.ZReports,
	FileIncoming: lucalists.IncomingInvoices,
	FileOutgoing: lucalists.OutgoingInvoices,
}

// part is one file to store.
type part struct {
	name, contentType string
	data              []byte
}

// Build writes the task's files in upload order.
func (b *Builder) Build(ctx context.Context, q *db.Queries, in BuildInput) (Built, error) {
	out := Built{Report: ServerReport{Check: []ReportRow{}, Skipped: []ReportRow{}, AlreadyExported: []ReportRow{}}}
	sc := engagementctx.Scope{
		ID: in.EngagementID, UUID: in.EngagementUUID, OrganizationID: in.OrganizationID,
		TaxpayerID: in.TaxpayerID, TaxpayerUUID: in.TaxpayerUUID, Status: in.EngagementStatus, StartedAt: in.EngagementStartedAt,
	}
	want := map[string]bool{}
	for _, k := range in.Kinds {
		want[k] = true
	}
	seq := int32(0)
	next := func() int32 {
		seq++
		return seq
	}
	store := func(kind string, p part, docs []FileDocument, exportUUID *uuid.UUID, transferID int64, entries int) error {
		n := next()
		sum := sha256.Sum256(p.data)
		key := fmt.Sprintf("exports/agent-tasks/%d/%s/%02d-%s", in.EngagementID, in.TaskUUID, n, path.Base(p.name))
		if err := b.Store.Upload(ctx, storage.File{Body: bytes.NewReader(p.data), Size: int64(len(p.data)), Filename: path.Base(p.name), ContentType: p.contentType}, key); err != nil {
			return fmt.Errorf("agenttasks: store %s: %w", p.name, err)
		}
		if docs == nil {
			docs = []FileDocument{}
		}
		raw, _ := json.Marshal(docs)
		if entries == 0 {
			entries = len(docs)
		}
		params := db.InsertAgentTaskFileParams{
			OrganizationID: in.OrganizationID, EngagementID: in.EngagementID, TaskID: in.TaskID, Sequence: n,
			Kind: kind, Name: path.Base(p.name), ContentType: p.contentType, Size: int64(len(p.data)),
			Sha256: hex.EncodeToString(sum[:]), StorageKey: key, Entries: int32(entries), Documents: raw,
		}
		if exportUUID != nil {
			params.ExportUuid = pgtype.UUID{Bytes: *exportUUID, Valid: true}
		}
		if transferID > 0 {
			params.TransferID = pgtype.Int8{Int64: transferID, Valid: true}
		}
		if _, err := q.InsertAgentTaskFile(ctx, params); err != nil {
			return err
		}
		out.Files++
		return nil
	}
	for _, kind := range FileKinds {
		if !want[kind] {
			continue
		}
		switch kind {
		case FileExpenses:
			if b.Expenses == nil {
				continue
			}
			f, err := b.Expenses.ExportExpensesLuca(ctx, sc, in.UserID, receiptsusecase.LucaExportInput{Period: in.Period})
			if errors.Is(err, receiptsusecase.ErrNothingToExport) {
				continue
			}
			if errors.Is(err, receiptsusecase.ErrInvalid) {
				out.Report.Skipped = append(out.Report.Skipped, ReportRow{Kind: kind, Reason: "NOT_EXPORTABLE"})
				continue
			}
			if err != nil {
				return Built{}, err
			}
			if err := storeExport(ctx, q, in, kind, f.Name, f.ContentType, f.Data, f.Export, store); err != nil {
				return Built{}, err
			}
		case FileJournals:
			n, err := b.journals(ctx, q, in, next)
			if err != nil {
				return Built{}, err
			}
			out.Files += n
		default:
			if b.Lists == nil {
				continue
			}
			res, err := b.Lists.Export(ctx, sc, in.UserID, listOf[kind], lucalists.Input{Period: in.Period})
			addRows(&out.Report.Check, kind, res.Report.Check)
			addRows(&out.Report.Skipped, kind, res.Report.Skipped)
			addRows(&out.Report.AlreadyExported, kind, res.Report.AlreadyExported)
			if errors.Is(err, lucalists.ErrNothingToExport) {
				continue
			}
			if errors.Is(err, lucalists.ErrInvalid) {
				out.Report.Skipped = append(out.Report.Skipped, ReportRow{Kind: kind, Reason: "NOT_EXPORTABLE"})
				continue
			}
			if err != nil {
				return Built{}, err
			}
			if err := storeExport(ctx, q, in, kind, res.Name, res.ContentType, res.Data, res.Export, store); err != nil {
				return Built{}, err
			}
		}
	}
	return out, nil
}

type storeFn func(kind string, p part, docs []FileDocument, exportUUID *uuid.UUID, transferID int64, entries int) error

// storeExport splits a list / expense export into its files and stores
// each with the documents that went into it.
func storeExport(ctx context.Context, q *db.Queries, in BuildInput, kind, name, contentType string, data []byte, export uuid.UUID, store storeFn) error {
	parts, err := split(name, contentType, data)
	if err != nil {
		return err
	}
	rows, err := q.ListLucaListExportFileDocuments(ctx, db.ListLucaListExportFileDocumentsParams{
		OrganizationID: in.OrganizationID, EngagementID: in.EngagementID, ExportUuid: export,
	})
	if err != nil {
		return err
	}
	byFile := map[string][]FileDocument{}
	for _, r := range rows {
		d := FileDocument{DocumentKey: r.DocumentKey, DocumentNo: r.DocumentNo}
		if r.DocumentDate.Valid {
			d.DocumentDate = r.DocumentDate.Time.Format("2006-01-02")
		}
		byFile[path.Base(r.FileName)] = append(byFile[path.Base(r.FileName)], d)
	}
	for _, p := range parts {
		docs := byFile[path.Base(p.name)]
		if len(parts) == 1 && docs == nil {
			// One file: every document of the export is in it.
			for _, ds := range byFile {
				docs = append(docs, ds...)
			}
		}
		e := export
		if err := store(kind, p, docs, &e, 0, 0); err != nil {
			return err
		}
	}
	return nil
}

// journalFile is a stored journal export file (journals.BatchFile).
type journalFile struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Entries     int    `json:"entries"`
	StorageKey  string `json:"storage_key"`
}

// journals adds the period's Luca journal exports not yet sent: each gets a
// luca_web transfer (one open or imported transfer per export) and its
// stored files are referenced under their key, not copied.
func (b *Builder) journals(ctx context.Context, q *db.Queries, in BuildInput, next func() int32) (int, error) {
	batches, err := q.ListLucaBatchesWithoutTransfer(ctx, db.ListLucaBatchesWithoutTransferParams{
		OrganizationID: in.OrganizationID, EngagementID: in.EngagementID, Period: pgtype.Text{String: in.Period, Valid: true},
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, bt := range batches {
		var files []journalFile
		if err := json.Unmarshal(bt.Files, &files); err != nil || len(files) == 0 {
			continue
		}
		tr, err := q.InsertLucaWebTransfer(ctx, db.InsertLucaWebTransferParams{
			OrganizationID: in.OrganizationID, EngagementID: in.EngagementID, ExportBatchID: bt.ID,
			AgentID: optInt8(in.AgentID), RequestedBy: in.UserID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, err
		}
		for _, f := range files {
			if _, err := q.InsertAgentTaskFile(ctx, db.InsertAgentTaskFileParams{
				OrganizationID: in.OrganizationID, EngagementID: in.EngagementID, TaskID: in.TaskID,
				Sequence: next(), Kind: FileJournals, Name: path.Base(f.Name), ContentType: f.ContentType,
				Size: f.Size, Sha256: f.SHA256, StorageKey: f.StorageKey, Entries: int32(f.Entries), Documents: []byte("[]"),
				TransferID: pgtype.Int8{Int64: tr.ID, Valid: true},
			}); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, nil
}

func addRows(dst *[]ReportRow, kind string, rows []lucalists.Row) {
	for _, r := range rows {
		*dst = append(*dst, ReportRow{Kind: kind, DocumentNo: r.DocumentNo, Date: r.Date, Reason: r.Reason, Account: r.Account})
	}
}

// maxSplitFiles bounds the files of one zip.
const maxSplitFiles = 500

// split returns the files of an export: a zip (Luca limit hit) is opened
// into its files; anything else is one file.
func split(name, contentType string, data []byte) ([]part, error) {
	if contentType != "application/zip" {
		return []part{{name: name, contentType: contentType, data: data}}, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("agenttasks: read zip: %w", err)
	}
	if len(zr.File) > maxSplitFiles {
		return nil, fmt.Errorf("agenttasks: zip has %d files", len(zr.File))
	}
	out := make([]part, 0, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, part{name: f.Name, contentType: xlsxType, data: body})
	}
	return out, nil
}

// xlsxType is the content type of a Luca file inside a zip.
const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
