// Package doctasks is the missing-document task module (issue #36, docs/01 §3
// Basis row, B2B2C):
//
//  1. Detection (producer control:missing_document): a supplier payment or a
//     customer collection in the bank with no document raises
//     BNK_UNDOCUMENTED_PAYMENT / BNK_UNINVOICED_COLLECTION on the
//     bank_transaction (21 §6.3); once a document is matched the warning is
//     auto-resolved through the warning engine.
//  2. The office turns open warnings into tasks for the mükellef
//     (document_tasks, office work product).
//  3. The mükellef sees the tasks of its active engagement on /v1/taxpayer/*
//     and uploads a photo / PDF. The file becomes an attachment, is matched to
//     the bank transaction (bank_transaction_documents) and detection runs
//     again for that transaction.
//  4. A scheduled job reminds the mükellef's members of open tasks with
//     backoff.
//
// Bank tables are read only; the banking module owns them.
package doctasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	notifmodel "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/notifications/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Producer is the data_warning producer of the detection (05 §1).
const Producer = "control:missing_document"

// Warning codes the control owns (21 §6.3).
const (
	CodeUndocumentedPayment  = "BNK_UNDOCUMENTED_PAYMENT"
	CodeUninvoicedCollection = "BNK_UNINVOICED_COLLECTION"
)

// OwnedCodes are every code the control checks on a bank transaction.
var OwnedCodes = []string{CodeUndocumentedPayment, CodeUninvoicedCollection}

// Task statuses.
const (
	StatusOpen      = "open"
	StatusSubmitted = "submitted"
	StatusMatched   = "matched"
	StatusCancelled = "cancelled"
)

// Notification template / source event codes (000093 seeds the templates).
const (
	TemplateCreated  = events.DocTasksCreated
	TemplateReminder = events.DocTasksReminder
)

// MaxUploadBytes caps one uploaded document (02 §7: en fazla 20 MB).
const MaxUploadBytes = 20 << 20

// Errors.
var (
	ErrNotFound    = errors.New("doctasks: task not found")
	ErrClosed      = errors.New("doctasks: task is closed")
	ErrBadPeriod   = errors.New("doctasks: period must be YYYY-MM")
	ErrBadInput    = errors.New("doctasks: invalid input")
	ErrEmpty       = errors.New("doctasks: empty file")
	ErrTooLarge    = errors.New("doctasks: file too large")
	ErrUnsupported = errors.New("doctasks: file is not an image or PDF")
)

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// ValidPeriod reports whether p is YYYY-MM.
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

// DB is what the service needs from Postgres.
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ObjectStore is the storage.Driver subset the service uses.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// Notifier enqueues notifications (notifications usecase.Service).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Config holds the task and reminder policy.
type Config struct {
	// DueDays is the default time the mükellef has to upload.
	DueDays int
	// ReminderBaseDays is the first reminder delay; the n-th reminder is
	// followed by the next one after ReminderBaseDays * 2^n days.
	ReminderBaseDays int
	// MaxReminders caps reminders per task.
	MaxReminders int
	// FrontendURL builds the link in notifications.
	FrontendURL string
}

// DefaultConfig is the policy used when a field is zero.
var DefaultConfig = Config{DueDays: 7, ReminderBaseDays: 3, MaxReminders: 3}

// Service runs detection, tasks, uploads and reminders.
type Service struct {
	db       DB
	q        *db.Queries
	engine   *engine.Engine
	store    ObjectStore
	notifier Notifier
	cfg      Config
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service. store and notifier may be nil (uploads and
// notifications are then unavailable / skipped).
func New(pool DB, store ObjectStore, notifier Notifier, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if cfg.DueDays <= 0 {
		cfg.DueDays = DefaultConfig.DueDays
	}
	if cfg.ReminderBaseDays <= 0 {
		cfg.ReminderBaseDays = DefaultConfig.ReminderBaseDays
	}
	if cfg.MaxReminders <= 0 {
		cfg.MaxReminders = DefaultConfig.MaxReminders
	}
	q := db.New(pool)
	return &Service{
		db: pool, q: q,
		engine:   engine.New(warningsrepository.NewWarnings(q), warningsrepository.NewConfig(q)),
		store:    store,
		notifier: notifier,
		cfg:      cfg,
		log:      log,
		now:      time.Now,
	}
}

// WithClock overrides the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	s.engine.WithClock(now)
	return s
}

func (s *Service) link(taxpayerUUID string) string {
	base := strings.TrimRight(s.cfg.FrontendURL, "/")
	return base + "/m/" + taxpayerUUID + "/tasks"
}
