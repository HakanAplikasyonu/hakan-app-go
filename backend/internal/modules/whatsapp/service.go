package whatsapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/foreigninvoices"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/extract"
	receiptsusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Capture targets (the extension's three buttons).
const (
	TargetExpense  = "expense"  // Gider olarak kaydet → receipt capture
	TargetIncome   = "income"   // Z raporu olarak kaydet → receipt capture (Z raporu)
	TargetDocument = "document" // Belge olarak kaydet → document task / attachment
	// TargetForeignInvoice: Yabancı fatura olarak kaydet → draft foreign
	// service invoice (#780 KDV2 A2).
	TargetForeignInvoice = "foreign_invoice"
)

// MaxFileBytes caps one file (documents; receipts keep their own 10 MB cap).
const MaxFileBytes = doctasks.MaxUploadBytes

var (
	// ErrInvalid: bad input.
	ErrInvalid = errors.New("whatsapp: invalid input")
	// ErrNotFound: no such link / capture in the office.
	ErrNotFound = errors.New("whatsapp: not found")
	// ErrLinkedElsewhere: the chat is linked to a taxpayer the caller cannot see.
	ErrLinkedElsewhere = errors.New("whatsapp: chat is linked to another taxpayer")
	// ErrNeedsImage: gider / gelir are read from a photo; a PDF goes as a document.
	ErrNeedsImage = errors.New("whatsapp: expense and income need an image")

	groupRe = regexp.MustCompile(`^[0-9A-Za-z-]{6,64}$`)
	digitRe = regexp.MustCompile(`^[0-9]{6,20}$`)
)

// NormalizeChatKey turns a WhatsApp chat id into the stored key: a contact
// ("905551112233@c.us", "+90 555 111 22 33") → digits, a group
// ("1203...@g.us") → "g:<id>", a hidden-number contact ("...@lid") →
// "l:<id>". ok is false for anything else.
func NormalizeChatKey(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	lower := strings.ToLower(s)
	for _, p := range []struct{ suffix, prefix string }{{"@g.us", "g:"}, {"@lid", "l:"}} {
		if strings.HasSuffix(lower, p.suffix) {
			id := s[:len(s)-len(p.suffix)]
			if !groupRe.MatchString(id) {
				return "", false
			}
			return p.prefix + id, true
		}
	}
	if strings.HasPrefix(s, "g:") || strings.HasPrefix(s, "l:") {
		if !groupRe.MatchString(s[2:]) {
			return "", false
		}
		return s, true
	}
	for _, suffix := range []string{"@c.us", "@s.whatsapp.net"} {
		if strings.HasSuffix(lower, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	digits := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')':
			return -1
		}
		return 'x'
	}, s)
	if !digitRe.MatchString(digits) {
		return "", false
	}
	return digits, true
}

// ReceiptCapturer is the receipts use case subset. The photo is stored and
// read in the background (#780): the capture answers at once with
// receipt_status "processing", which becomes draft, duplicate or failed.
type ReceiptCapturer interface {
	CaptureKeyedAsync(ctx context.Context, sc engagementctx.Scope, userID int64, fileName string, data []byte, kindHint, clientKey string) (receiptsusecase.View, error)
}

// TaskUploader is the doctasks subset.
type TaskUploader interface {
	OfficeUpload(ctx context.Context, scope engagementctx.Scope, taskID uuid.UUID, userID int64, fileName string, data []byte) (doctasks.UploadResult, error)
	// OpenRequests: the engagement's open document requests (#571).
	OpenRequests(ctx context.Context, scope engagementctx.Scope) ([]doctasks.Task, error)
}

// ForeignIntake is the foreign invoice intake subset (#780 A2).
type ForeignIntake interface {
	Submit(ctx context.Context, sub foreigninvoices.Subject, taxpayerID int64, f foreigninvoices.IntakeInput) (foreigninvoices.IntakeResult, error)
}

// ObjectStore is the storage subset.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// Service is the extension's data side.
type Service struct {
	q        *db.Queries
	receipts ReceiptCapturer
	tasks    TaskUploader
	store    ObjectStore
	foreign  ForeignIntake
	log      *slog.Logger
}

// WithForeignInvoices enables the "Yabancı hizmet faturası" target.
func (s *Service) WithForeignInvoices(f ForeignIntake) *Service {
	s.foreign = f
	return s
}

// New builds the service. receipts, tasks and store may be nil (the matching
// targets then fail).
func New(pool database.Pool, receipts ReceiptCapturer, tasks TaskUploader, store ObjectStore, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{q: database.QueriesFor(pool), receipts: receipts, tasks: tasks, store: store, log: log}
}

// Taxpayer is a taxpayer the chat can be linked to.
type Taxpayer struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	Title          string    `json:"title"`
	TaxID          string    `json:"tax_id"`
}

// ChatLink is a remembered chat → taxpayer mapping.
type ChatLink struct {
	UUID      uuid.UUID `json:"uuid"`
	ChatKey   string    `json:"chat_key"`
	Label     string    `json:"label"`
	Taxpayer  Taxpayer  `json:"taxpayer"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatLookup answers "which taxpayer is this chat?": the link, or (first
// time) taxpayers whose contact phone matches the chat number.
type ChatLookup struct {
	ChatKey     string     `json:"chat_key"`
	Link        *ChatLink  `json:"link"`
	Suggestions []Taxpayer `json:"suggestions"`
}

// LookupChat finds the link of a chat.
func (s *Service) LookupChat(ctx context.Context, organizationID int64, rawKey string) (ChatLookup, error) {
	key, ok := NormalizeChatKey(rawKey)
	if !ok {
		return ChatLookup{}, fmt.Errorf("%w: chat_key", ErrInvalid)
	}
	out := ChatLookup{ChatKey: key, Suggestions: []Taxpayer{}}
	row, err := s.q.GetWhatsAppChatLink(ctx, db.GetWhatsAppChatLinkParams{OrganizationID: organizationID, ChatKey: key})
	switch {
	case err == nil:
		out.Link = &ChatLink{
			UUID: row.Uuid, ChatKey: row.ChatKey, Label: row.Label, UpdatedAt: row.UpdatedAt.Time,
			Taxpayer: Taxpayer{EngagementUUID: row.EngagementUuid, TaxpayerUUID: row.TaxpayerUuid, Title: row.TaxpayerTitle, TaxID: row.TaxID},
		}
		return out, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return ChatLookup{}, fmt.Errorf("whatsapp: lookup: %w", err)
	}
	if !digitRe.MatchString(key) || len(key) < 10 {
		return out, nil
	}
	sugg, err := s.q.SuggestWhatsAppTaxpayers(ctx, db.SuggestWhatsAppTaxpayersParams{OrganizationID: organizationID, Phone: key})
	if err != nil {
		return ChatLookup{}, fmt.Errorf("whatsapp: suggest: %w", err)
	}
	for _, r := range sugg {
		out.Suggestions = append(out.Suggestions, Taxpayer{EngagementUUID: r.EngagementUuid, TaxpayerUUID: r.TaxpayerUuid, Title: r.TaxpayerTitle, TaxID: r.TaxID})
	}
	return out, nil
}

// LinkInput is PUT /v1/tenant/whatsapp/chat-links.
type LinkInput struct {
	ChatKey        string    `json:"chat_key"`
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	Label          string    `json:"label"`
}

// SetLink remembers (or moves) a chat's taxpayer. sc is the engagement the
// caller resolved (RequireEngagement semantics are applied by the handler).
func (s *Service) SetLink(ctx context.Context, sc engagementctx.Scope, userID int64, in LinkInput) (ChatLookup, error) {
	key, ok := NormalizeChatKey(in.ChatKey)
	if !ok {
		return ChatLookup{}, fmt.Errorf("%w: chat_key", ErrInvalid)
	}
	label := cleanLabel(in.Label)
	_, err := s.q.UpsertWhatsAppChatLink(ctx, db.UpsertWhatsAppChatLinkParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, ChatKey: key, Label: label,
		UserID: pgtype.Int8{Int64: userID, Valid: userID > 0},
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42501" {
		// The existing link points at an engagement this caller cannot see.
		return ChatLookup{}, ErrLinkedElsewhere
	}
	if err != nil {
		return ChatLookup{}, fmt.Errorf("whatsapp: link: %w", err)
	}
	return s.LookupChat(ctx, sc.OrganizationID, key)
}

// ListLinks returns the office's links, newest first.
func (s *Service) ListLinks(ctx context.Context, organizationID int64, limit, offset int32) ([]ChatLink, int64, error) {
	rows, err := s.q.ListWhatsAppChatLinks(ctx, db.ListWhatsAppChatLinksParams{OrganizationID: organizationID, Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("whatsapp: list links: %w", err)
	}
	out := make([]ChatLink, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, ChatLink{
			UUID: r.Uuid, ChatKey: r.ChatKey, Label: r.Label, UpdatedAt: r.UpdatedAt.Time,
			Taxpayer: Taxpayer{EngagementUUID: r.EngagementUuid, TaxpayerUUID: r.TaxpayerUuid, Title: r.TaxpayerTitle, TaxID: r.TaxID},
		})
	}
	return out, total, nil
}

// DeleteLink forgets a chat's taxpayer.
func (s *Service) DeleteLink(ctx context.Context, organizationID int64, id uuid.UUID) error {
	n, err := s.q.DeleteWhatsAppChatLink(ctx, db.DeleteWhatsAppChatLinkParams{OrganizationID: organizationID, Uuid: id})
	if err != nil {
		return fmt.Errorf("whatsapp: delete link: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CaptureInput is one clicked file.
type CaptureInput struct {
	Target   string
	ChatKey  string
	FileName string
	Data     []byte
	// TaskUUID (document only): the open document task the file answers.
	TaskUUID *uuid.UUID
	// AutoLink (document without TaskUUID, #571): the caller may answer
	// document tasks, so a document saved without a task is tied to the
	// engagement's open request when there is exactly one.
	AutoLink bool
}

// Capture is the API view of an extension upload.
type Capture struct {
	UUID          uuid.UUID  `json:"uuid"`
	Target        string     `json:"target"`
	FileName      string     `json:"file_name"`
	ContentType   string     `json:"content_type"`
	SizeBytes     int64      `json:"size_bytes"`
	Taxpayer      Taxpayer   `json:"taxpayer"`
	CreatedByName string     `json:"created_by_name"`
	ReceiptUUID   *uuid.UUID `json:"receipt_uuid"`
	ReceiptStatus *string    `json:"receipt_status"`
	TaskUUID      *uuid.UUID `json:"task_uuid"`
	TaskTitle     *string    `json:"task_title"`
	// HasFile: the document is stored here and can be downloaded
	// (GET .../whatsapp/captures/{captureUuid}/file).
	HasFile   bool      `json:"has_file"`
	CreatedAt time.Time `json:"created_at"`
	// Replayed: the same file was already saved to the same target.
	Replayed bool `json:"replayed"`
	// AutoLinked: the document was saved without a task and tied to the
	// engagement's only open document request (#571). Only on the answer
	// of the save.
	AutoLinked bool `json:"auto_linked"`
	// OpenRequests: how many open document requests the engagement had when
	// a document was saved without a task; with 2 or more nothing is tied
	// and the office chooses (#571). Only on the answer of the save.
	OpenRequests int `json:"open_requests"`
}

// Capture saves one file the user clicked in WhatsApp Web. The same file
// sent again to the same target answers the first capture (Replayed).
func (s *Service) Capture(ctx context.Context, sc engagementctx.Scope, userID int64, in CaptureInput) (Capture, error) {
	switch in.Target {
	case TargetExpense, TargetIncome, TargetDocument, TargetForeignInvoice:
	default:
		return Capture{}, fmt.Errorf("%w: target must be expense, income, document or foreign_invoice", ErrInvalid)
	}
	if len(in.Data) == 0 || len(in.Data) > MaxFileBytes {
		return Capture{}, fmt.Errorf("%w: file must be 1 byte to %d MB", ErrInvalid, MaxFileBytes>>20)
	}
	var chat pgtype.Text
	if strings.TrimSpace(in.ChatKey) != "" {
		key, ok := NormalizeChatKey(in.ChatKey)
		if !ok {
			return Capture{}, fmt.Errorf("%w: chat_key", ErrInvalid)
		}
		chat = pgtype.Text{String: key, Valid: true}
	}
	if in.TaskUUID != nil && in.Target != TargetDocument {
		return Capture{}, fmt.Errorf("%w: task_uuid is only for documents", ErrInvalid)
	}
	contentType, kind, ext, ok := sniff(in.Data)
	if !ok {
		return Capture{}, fmt.Errorf("%w: file must be a photo (JPEG, PNG, WebP, HEIC) or PDF", ErrInvalid)
	}
	sum := sha256.Sum256(in.Data)
	sha := hex.EncodeToString(sum[:])
	name := cleanName(in.FileName, ext)

	if id, err := s.q.GetWhatsAppCaptureByFile(ctx, db.GetWhatsAppCaptureByFileParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Target: in.Target, Sha256: sha,
	}); err == nil {
		v, err := s.get(ctx, sc, id)
		v.Replayed = true
		return v, err
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Capture{}, fmt.Errorf("whatsapp: find capture: %w", err)
	}

	ins := db.InsertWhatsAppCaptureParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, TaxpayerID: sc.TaxpayerID, Target: in.Target,
		ChatKey: chat, FileName: name, ContentType: contentType, SizeBytes: int64(len(in.Data)), Sha256: sha,
		CreatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
	}
	autoLinked, openRequests := false, 0
	switch in.Target {
	case TargetExpense, TargetIncome:
		if kind != "image" || contentType == "image/heic" {
			return Capture{}, ErrNeedsImage
		}
		if s.receipts == nil {
			return Capture{}, errors.New("whatsapp: receipt capture is not configured")
		}
		hint := ""
		if in.Target == TargetIncome {
			hint = extract.KindZReport
		}
		v, err := s.receipts.CaptureKeyedAsync(ctx, sc, userID, name, in.Data, hint, "whatsapp:"+in.Target+":"+sha)
		if err != nil {
			return Capture{}, err
		}
		ins.ReceiptUuid = pgtype.UUID{Bytes: v.UUID, Valid: true}
	case TargetDocument:
		if in.TaskUUID != nil {
			if s.tasks == nil {
				return Capture{}, errors.New("whatsapp: document tasks are not configured")
			}
			res, err := s.tasks.OfficeUpload(ctx, sc, *in.TaskUUID, userID, name, in.Data)
			if err != nil {
				return Capture{}, err
			}
			ins.TaskUuid = pgtype.UUID{Bytes: *in.TaskUUID, Valid: true}
			ins.AttachmentUuid = pgtype.UUID{Bytes: res.Upload.AttachmentUUID, Valid: true}
			break
		}
		// No task named (#571): when the engagement waits for exactly one
		// document, this is it. With several open requests nothing is
		// guessed; the document is kept and the office ties it by hand.
		if in.AutoLink && s.tasks != nil {
			open, err := s.tasks.OpenRequests(ctx, sc)
			if err != nil {
				return Capture{}, fmt.Errorf("whatsapp: open requests: %w", err)
			}
			openRequests = len(open)
			if len(open) == 1 {
				res, err := s.tasks.OfficeUpload(ctx, sc, open[0].UUID, userID, name, in.Data)
				if err != nil {
					return Capture{}, err
				}
				ins.TaskUuid = pgtype.UUID{Bytes: open[0].UUID, Valid: true}
				ins.AttachmentUuid = pgtype.UUID{Bytes: res.Upload.AttachmentUUID, Valid: true}
				autoLinked = true
				break
			}
		}
		att, err := s.storeAttachment(ctx, sc.TaxpayerID, name, contentType, kind, ext, sha, in.Data)
		if err != nil {
			return Capture{}, err
		}
		ins.AttachmentUuid = pgtype.UUID{Bytes: att, Valid: true}
	case TargetForeignInvoice:
		// The intake keeps the file in the same attachments store and opens
		// a draft the office checks and confirms.
		if s.foreign == nil {
			return Capture{}, errors.New("whatsapp: foreign invoices are not configured")
		}
		res, err := s.foreign.Submit(ctx, foreigninvoices.Subject{OrganizationID: sc.OrganizationID, EngagementID: sc.ID}, sc.TaxpayerID,
			foreigninvoices.IntakeInput{Source: foreigninvoices.SourceWhatsApp, FileName: name, Data: in.Data, UserID: userID})
		if err != nil {
			return Capture{}, err
		}
		if res.Item.SourceUUID != nil {
			ins.AttachmentUuid = pgtype.UUID{Bytes: *res.Item.SourceUUID, Valid: true}
		}
	}
	id, err := s.q.InsertWhatsAppCapture(ctx, ins)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent request saved the same file first.
		id, err = s.q.GetWhatsAppCaptureByFile(ctx, db.GetWhatsAppCaptureByFileParams{
			OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Target: in.Target, Sha256: sha,
		})
		if err != nil {
			return Capture{}, fmt.Errorf("whatsapp: capture: %w", err)
		}
		v, err := s.get(ctx, sc, id)
		v.Replayed = true
		return v, err
	}
	if err != nil {
		return Capture{}, fmt.Errorf("whatsapp: capture: %w", err)
	}
	s.log.Info("whatsapp_capture", "organization_id", sc.OrganizationID, "engagement", sc.UUID, "target", in.Target, "size", len(in.Data), "auto_linked", autoLinked)
	v, err := s.get(ctx, sc, id)
	v.AutoLinked, v.OpenRequests = autoLinked, openRequests
	return v, err
}

// ErrAlreadyLinked: the capture answers a task already, or is not a
// document kept here.
var ErrAlreadyLinked = errors.New("whatsapp: capture cannot be linked to a task")

// LinkTask ties a document that was saved without a task to an open
// document request afterwards (#571): the stored file becomes an upload of
// the task (the task is "submitted", the office sees the file on the request)
// and the capture names the task.
func (s *Service) LinkTask(ctx context.Context, sc engagementctx.Scope, userID int64, captureID, taskID uuid.UUID) (Capture, error) {
	if s.tasks == nil {
		return Capture{}, errors.New("whatsapp: document tasks are not configured")
	}
	cur, err := s.get(ctx, sc, captureID)
	if err != nil {
		return Capture{}, err
	}
	if cur.Target != TargetDocument || cur.TaskUUID != nil || !cur.HasFile {
		return Capture{}, ErrAlreadyLinked
	}
	f, err := s.CaptureFile(ctx, sc, captureID)
	if err != nil {
		return Capture{}, err
	}
	data, err := io.ReadAll(io.LimitReader(f.Body, MaxFileBytes+1))
	_ = f.Body.Close()
	if err != nil {
		return Capture{}, fmt.Errorf("whatsapp: read stored file: %w", err)
	}
	// The task must be open (or submitted): OfficeUpload refuses a closed
	// one and a task of another engagement.
	if _, err := s.tasks.OfficeUpload(ctx, sc, taskID, userID, cur.FileName, data); err != nil {
		return Capture{}, err
	}
	n, err := s.q.LinkWhatsAppCaptureTask(ctx, db.LinkWhatsAppCaptureTaskParams{
		OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: captureID, TaskUuid: taskID,
	})
	if err != nil {
		return Capture{}, fmt.Errorf("whatsapp: link task: %w", err)
	}
	if n == 0 {
		return Capture{}, ErrAlreadyLinked
	}
	return s.get(ctx, sc, captureID)
}

// storeAttachment keeps a document that answers no task: content-addressed
// like the document task uploads (attachments/{taxpayer}/{sha}{ext}).
func (s *Service) storeAttachment(ctx context.Context, taxpayerID int64, name, contentType, kind, ext, sha string, data []byte) (uuid.UUID, error) {
	if s.store == nil {
		return uuid.Nil, errors.New("whatsapp: storage is not configured")
	}
	key := fmt.Sprintf("attachments/%d/%s%s", taxpayerID, sha, ext)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), Filename: name, ContentType: contentType}, key); err != nil {
		return uuid.Nil, fmt.Errorf("whatsapp: store file: %w", err)
	}
	att, err := s.q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: taxpayerID, Kind: kind, StorageKey: key, Sha256: sha, SizeBytes: int64(len(data)),
		ContentType: pgtype.Text{String: contentType, Valid: true},
		FileName:    pgtype.Text{String: name, Valid: true},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("whatsapp: attachment: %w", err)
	}
	return att.Uuid, nil
}

func (s *Service) get(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (Capture, error) {
	r, err := s.q.GetWhatsAppCapture(ctx, db.GetWhatsAppCaptureParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Capture{}, ErrNotFound
	}
	if err != nil {
		return Capture{}, fmt.Errorf("whatsapp: get capture: %w", err)
	}
	return captureView(db.ListWhatsAppCapturesRow{
		Uuid: r.Uuid, Target: r.Target, FileName: r.FileName, ContentType: r.ContentType, SizeBytes: r.SizeBytes,
		CreatedAt: r.CreatedAt, EngagementUuid: r.EngagementUuid, TaxpayerUuid: r.TaxpayerUuid, TaxpayerTitle: r.TaxpayerTitle,
		CreatedByName: r.CreatedByName, ReceiptUuid: r.ReceiptUuid, ReceiptStatus: r.ReceiptStatus,
		TaskUuid: r.TaskUuid, TaskTitle: r.TaskTitle, AttachmentUuid: r.AttachmentUuid,
	}), nil
}

// Filter narrows the capture list.
type Filter struct {
	EngagementID *int64
	Target       string
	Limit        int32
	Offset       int32
}

// ListCaptures returns the office's extension uploads, newest first.
func (s *Service) ListCaptures(ctx context.Context, organizationID int64, f Filter) ([]Capture, int64, error) {
	switch f.Target {
	case "", TargetExpense, TargetIncome, TargetDocument, TargetForeignInvoice:
	default:
		return nil, 0, fmt.Errorf("%w: target", ErrInvalid)
	}
	p := db.ListWhatsAppCapturesParams{OrganizationID: organizationID, Lim: f.Limit, Off: f.Offset}
	if f.EngagementID != nil {
		p.EngagementID = pgtype.Int8{Int64: *f.EngagementID, Valid: true}
	}
	if f.Target != "" {
		p.Target = pgtype.Text{String: f.Target, Valid: true}
	}
	rows, err := s.q.ListWhatsAppCaptures(ctx, p)
	if err != nil {
		return nil, 0, fmt.Errorf("whatsapp: list captures: %w", err)
	}
	out := make([]Capture, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, captureView(r))
	}
	return out, total, nil
}

// File is a stored document to stream back.
type File struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Name        string
}

// CaptureFile opens the stored file of a document capture.
func (s *Service) CaptureFile(ctx context.Context, sc engagementctx.Scope, id uuid.UUID) (File, error) {
	r, err := s.q.GetWhatsAppCaptureFile(ctx, db.GetWhatsAppCaptureFileParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("whatsapp: capture file: %w", err)
	}
	if s.store == nil {
		return File{}, errors.New("whatsapp: storage is not configured")
	}
	rc, size, err := s.store.Download(ctx, r.StorageKey)
	if err != nil {
		return File{}, fmt.Errorf("whatsapp: download: %w", err)
	}
	f := File{Body: rc, Size: size, ContentType: "application/octet-stream", Name: "belge"}
	if r.ContentType.Valid && r.ContentType.String != "" {
		f.ContentType = r.ContentType.String
	}
	if r.FileName.Valid && r.FileName.String != "" {
		f.Name = r.FileName.String
	}
	return f, nil
}

func captureView(r db.ListWhatsAppCapturesRow) Capture {
	c := Capture{
		UUID: r.Uuid, Target: r.Target, FileName: r.FileName, ContentType: r.ContentType, SizeBytes: r.SizeBytes,
		Taxpayer:      Taxpayer{EngagementUUID: r.EngagementUuid, TaxpayerUUID: r.TaxpayerUuid, Title: r.TaxpayerTitle},
		CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt.Time,
		HasFile: r.AttachmentUuid.Valid,
	}
	if r.ReceiptUuid.Valid {
		id := uuid.UUID(r.ReceiptUuid.Bytes)
		c.ReceiptUUID = &id
	}
	if r.ReceiptStatus.Valid {
		st := r.ReceiptStatus.String
		c.ReceiptStatus = &st
	}
	if r.TaskUuid.Valid {
		id := uuid.UUID(r.TaskUuid.Bytes)
		c.TaskUUID = &id
	}
	if r.TaskTitle.Valid {
		t := r.TaskTitle.String
		c.TaskTitle = &t
	}
	return c
}

// sniff accepts what WhatsApp sends as a photo or a document.
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
		name = "whatsapp"
	}
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	if !strings.HasSuffix(strings.ToLower(name), ext) {
		name += ext
	}
	return name
}

func cleanLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120])
	}
	return s
}
