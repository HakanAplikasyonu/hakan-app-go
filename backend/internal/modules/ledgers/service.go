// Package ledgers serves the "Defterler" section of a taxpayer (#780): the
// office uploads each period the işletme defteri export of Luca and the
// e-defter files, sees which periods have which files (and which are
// missing), and a summary read from the file (parse package).
//
// Defter dönemi: aylık (default) or 3 aylık. With 3 aylık a period is a
// quarter, keyed by its last month (2026-03, 2026-06, 2026-09, 2026-12); a
// file uploaded to any month of the quarter covers it. Missing periods are
// counted from the engagement start, or the earliest period a book was
// uploaded for when that is earlier (at most 36 months / 12 quarters back),
// up to the last completed period (Europe/Istanbul).
//
// No Luca password or e-defter credential is ever stored: the office
// exports the files itself and uploads them.
package ledgers

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
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ledgers/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/periodq"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Errors mapped to HTTP by the handler.
var (
	ErrNotFound       = errors.New("ledgers: file not found")
	ErrInvalid        = errors.New("ledgers: invalid request")
	ErrUnsupported    = errors.New("ledgers: only Excel (xlsx, xls), CSV, XML and ZIP files are accepted")
	ErrStorageMissing = errors.New("ledgers: storage is not configured")
)

// Period kinds (Defter dönemi).
const (
	PeriodMonthly   = "monthly"
	PeriodQuarterly = "quarterly"
)

const (
	maxMonthsBack   = 36
	maxQuartersBack = 12
)

// Queries is the db.Querier subset.
type Queries interface {
	ListLedgerFiles(ctx context.Context, arg db.ListLedgerFilesParams) ([]db.ListLedgerFilesRow, error)
	GetLedgerFile(ctx context.Context, arg db.GetLedgerFileParams) (db.LedgerFile, error)
	GetLedgerFileBySha(ctx context.Context, arg db.GetLedgerFileByShaParams) (db.LedgerFile, error)
	InsertLedgerFile(ctx context.Context, arg db.InsertLedgerFileParams) (db.LedgerFile, error)
	DeleteLedgerFile(ctx context.Context, arg db.DeleteLedgerFileParams) (string, error)
	GetLedgerSettings(ctx context.Context, arg db.GetLedgerSettingsParams) (string, error)
	UpsertLedgerSettings(ctx context.Context, arg db.UpsertLedgerSettingsParams) (string, error)
}

// ObjectStore is the object storage subset.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// Service is the Defterler logic.
type Service struct {
	q     Queries
	store ObjectStore
	log   *slog.Logger
	now   func() time.Time
}

// New builds the service; store nil answers uploads and downloads with
// ErrStorageMissing (503 STORAGE_UNAVAILABLE).
func New(q Queries, store ObjectStore, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{q: q, store: store, log: log, now: time.Now}
}

// Subject is one engagement.
type Subject struct {
	OrganizationID, EngagementID int64
	StartedAt                    time.Time
}

// Item is one uploaded file.
type Item struct {
	UUID           uuid.UUID `json:"uuid"`
	Kind           string    `json:"kind"`
	Period         string    `json:"period"`
	FileName       string    `json:"file_name"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	Readable       bool      `json:"readable"`
	UploadedByName string    `json:"uploaded_by_name"`
	CreatedAt      time.Time `json:"created_at"`
}

// Detail is a file with its summary.
type Detail struct {
	Item
	Summary parse.Summary `json:"summary"`
}

// PeriodRow is one month (aylık) or quarter (3 aylık, keyed by its last
// month) of the period list.
type PeriodRow struct {
	Period string `json:"period"`
	// Kinds: the kinds of the files in the period (sorted, unique).
	Kinds []string `json:"kinds"`
	Files int      `json:"files"`
	// Expected: the period is between the engagement start and the last
	// completed period; Missing: expected and no book file.
	Expected bool `json:"expected"`
	Missing  bool `json:"missing"`
}

// List is the Defterler screen.
type List struct {
	PeriodKind    string      `json:"period_kind"`
	LastCompleted string      `json:"last_completed"`
	Items         []Item      `json:"items"`
	Periods       []PeriodRow `json:"periods"`
	Missing       []string    `json:"missing"`
}

// Settings is the Defter dönemi of an engagement.
type Settings struct {
	PeriodKind string `json:"period_kind"`
}

// UploadInput is one file.
type UploadInput struct {
	FileName string
	Data     []byte
	Period   string
	Kind     string // empty: detected
	UserID   int64
}

// UploadResult: Replayed when the same file was uploaded before (the first
// row is answered).
type UploadResult struct {
	Item     Detail `json:"item"`
	Replayed bool   `json:"replayed"`
}

func istanbul() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*3600)
}

func monthOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func parsePeriod(p string) (time.Time, bool) {
	t, err := time.Parse("2006-01", p)
	return t, err == nil
}

// Bucket is the period a file counts for: the month (aylık) or the last
// month of its quarter (3 aylık).
func Bucket(kind, period string) string {
	if kind != PeriodQuarterly {
		return period
	}
	t, ok := parsePeriod(period)
	if !ok {
		return period
	}
	end := ((int(t.Month())-1)/3 + 1) * 3
	return time.Date(t.Year(), time.Month(end), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
}

// LastCompleted is the last month that has ended at now (Europe/Istanbul).
func LastCompleted(now time.Time) string {
	return monthOf(now.In(istanbul())).AddDate(0, -1, 0).Format("2006-01")
}

// ExpectedPeriods lists the periods that should have a book, newest first:
// from the engagement start (bounded) to the last completed period.
func ExpectedPeriods(kind string, started, now time.Time) []string {
	last, _ := parsePeriod(LastCompleted(now))
	step, back := 1, maxMonthsBack
	if kind == PeriodQuarterly {
		step, back = 3, maxQuartersBack
		// The last completed quarter ends at or before the last completed month.
		for int(last.Month())%3 != 0 {
			last = last.AddDate(0, -1, 0)
		}
	}
	var first time.Time
	if !started.IsZero() {
		first = monthOf(started.In(istanbul()))
		if kind == PeriodQuarterly {
			first, _ = parsePeriod(Bucket(PeriodQuarterly, first.Format("2006-01")))
		}
	}
	var out []string
	for m := last; len(out) < back && !m.Before(first); m = m.AddDate(0, -step, 0) {
		out = append(out, m.Format("2006-01"))
	}
	return out
}

// startOf is where the missing periods start: the engagement start, or an
// earlier period a book was uploaded for (the office brought older books:
// from then on every period is expected).
func startOf(started time.Time, items []Item) time.Time {
	for _, it := range items {
		if !parse.IsBook(it.Kind) {
			continue
		}
		if t, ok := parsePeriod(it.Period); ok && (started.IsZero() || t.Before(monthOf(started.In(istanbul())))) {
			started = t
		}
	}
	return started
}

// BuildPeriods joins the expected periods and the uploaded files.
// "other", mizan and bordro files are listed but do not cover a period.
func BuildPeriods(kind string, expected []string, items []Item) ([]PeriodRow, []string) {
	rows := map[string]*PeriodRow{}
	get := func(p string) *PeriodRow {
		r := rows[p]
		if r == nil {
			r = &PeriodRow{Period: p, Kinds: []string{}}
			rows[p] = r
		}
		return r
	}
	for _, p := range expected {
		get(p).Expected = true
	}
	covered := map[string]bool{}
	for _, it := range items {
		b := Bucket(kind, it.Period)
		r := get(b)
		r.Files++
		if !contains(r.Kinds, it.Kind) {
			r.Kinds = append(r.Kinds, it.Kind)
		}
		if parse.IsBook(it.Kind) {
			covered[b] = true
		}
	}
	out := make([]PeriodRow, 0, len(rows))
	missing := []string{}
	for _, r := range rows {
		sort.Strings(r.Kinds)
		r.Missing = r.Expected && !covered[r.Period]
		if r.Missing {
			missing = append(missing, r.Period)
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Period > out[j].Period })
	sort.Sort(sort.Reverse(sort.StringSlice(missing)))
	return out, missing
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (s *Service) periodKind(ctx context.Context, sub Subject) (string, error) {
	k, err := s.q.GetLedgerSettings(ctx, db.GetLedgerSettingsParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return PeriodMonthly, nil
	case err != nil:
		return "", fmt.Errorf("ledgers: settings: %w", err)
	}
	return k, nil
}

// List answers the Defterler screen.
func (s *Service) List(ctx context.Context, sub Subject) (List, error) {
	kind, err := s.periodKind(ctx, sub)
	if err != nil {
		return List{}, err
	}
	rows, err := s.q.ListLedgerFiles(ctx, db.ListLedgerFilesParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID})
	if err != nil {
		return List{}, fmt.Errorf("ledgers: list: %w", err)
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, Item{
			UUID: r.Uuid, Kind: r.Kind, Period: r.Period, FileName: r.FileName, ContentType: r.ContentType,
			SizeBytes: r.SizeBytes, Readable: r.Readable, UploadedByName: r.UploadedByName, CreatedAt: r.CreatedAt.Time,
		})
	}
	now := s.now()
	periods, missing := BuildPeriods(kind, ExpectedPeriods(kind, startOf(sub.StartedAt, items), now), items)
	return List{PeriodKind: kind, LastCompleted: LastCompleted(now), Items: items, Periods: periods, Missing: missing}, nil
}

// SaveSettings sets the Defter dönemi.
func (s *Service) SaveSettings(ctx context.Context, sub Subject, in Settings, userID int64) (Settings, error) {
	if in.PeriodKind != PeriodMonthly && in.PeriodKind != PeriodQuarterly {
		return Settings{}, fmt.Errorf("%w: period_kind must be monthly or quarterly", ErrInvalid)
	}
	k, err := s.q.UpsertLedgerSettings(ctx, db.UpsertLedgerSettingsParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, PeriodKind: in.PeriodKind,
		UpdatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
	})
	if err != nil {
		return Settings{}, fmt.Errorf("ledgers: save settings: %w", err)
	}
	return Settings{PeriodKind: k}, nil
}

func detail(f db.LedgerFile, by string) Detail {
	d := Detail{Item: Item{
		UUID: f.Uuid, Kind: f.Kind, Period: f.Period, FileName: f.FileName, ContentType: f.ContentType,
		SizeBytes: f.SizeBytes, Readable: f.Readable, UploadedByName: by, CreatedAt: f.CreatedAt.Time,
	}}
	if len(f.Summary) > 0 {
		_ = json.Unmarshal(f.Summary, &d.Summary)
	}
	return d
}

// Get answers one file with its summary.
func (s *Service) Get(ctx context.Context, sub Subject, id uuid.UUID) (Detail, error) {
	f, err := s.q.GetLedgerFile(ctx, db.GetLedgerFileParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("ledgers: get: %w", err)
	}
	return detail(f, ""), nil
}

var formats = map[string]struct{ ext, contentType string }{
	"xlsx": {".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	"xls":  {".xls", "application/vnd.ms-excel"},
	"csv":  {".csv", "text/csv"},
	"xml":  {".xml", "application/xml"},
	"zip":  {".zip", "application/zip"},
}

func cleanFileName(name, ext string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "defter"
	}
	if r := []rune(name); len(r) > 150 {
		name = string(r[:150])
	}
	if !strings.HasSuffix(strings.ToLower(name), ext) {
		name += ext
	}
	return name
}

// Upload stores a file and its summary.
func (s *Service) Upload(ctx context.Context, sub Subject, in UploadInput) (UploadResult, error) {
	if !periodq.Valid(in.Period) {
		return UploadResult{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	if cur := monthOf(s.now().In(istanbul())).Format("2006-01"); in.Period > cur {
		return UploadResult{}, fmt.Errorf("%w: period is in the future", ErrInvalid)
	}
	if in.Kind != "" && !parse.ValidKind(in.Kind) {
		return UploadResult{}, fmt.Errorf("%w: unknown kind", ErrInvalid)
	}
	if len(in.Data) == 0 || len(in.Data) > parse.MaxBytes {
		return UploadResult{}, fmt.Errorf("%w: the file is empty or larger than 20 MB", ErrInvalid)
	}
	f, ok := formats[parse.DetectFormat(in.FileName, in.Data)]
	if !ok {
		return UploadResult{}, ErrUnsupported
	}
	if s.store == nil {
		return UploadResult{}, ErrStorageMissing
	}
	sum := sha256.Sum256(in.Data)
	sha := hex.EncodeToString(sum[:])
	if old, err := s.q.GetLedgerFileBySha(ctx, db.GetLedgerFileByShaParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Sha256: sha}); err == nil {
		return UploadResult{Item: detail(old, ""), Replayed: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return UploadResult{}, fmt.Errorf("ledgers: lookup: %w", err)
	}
	kind := in.Kind
	if kind == "" {
		kind = parse.DetectKind(in.FileName, in.Data)
	}
	summary := parse.Summarize(kind, in.FileName, in.Data)
	raw, err := json.Marshal(summary)
	if err != nil {
		return UploadResult{}, fmt.Errorf("ledgers: summary: %w", err)
	}
	name := cleanFileName(in.FileName, f.ext)
	key := fmt.Sprintf("ledgers/%d/%d/%s%s", sub.OrganizationID, sub.EngagementID, sha, f.ext)
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(in.Data), Size: int64(len(in.Data)), ContentType: f.contentType, Filename: name}, key); err != nil {
		return UploadResult{}, fmt.Errorf("ledgers: store file: %w", err)
	}
	row, err := s.q.InsertLedgerFile(ctx, db.InsertLedgerFileParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Kind: kind, Period: in.Period,
		FileName: name, ContentType: f.contentType, StorageKey: key, Sha256: sha, SizeBytes: int64(len(in.Data)),
		Readable: summary.Readable, Summary: raw, UploadedBy: pgtype.Int8{Int64: in.UserID, Valid: in.UserID > 0},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A parallel upload of the same file won.
		old, gerr := s.q.GetLedgerFileBySha(ctx, db.GetLedgerFileByShaParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Sha256: sha})
		if gerr != nil {
			return UploadResult{}, fmt.Errorf("ledgers: lookup: %w", gerr)
		}
		return UploadResult{Item: detail(old, ""), Replayed: true}, nil
	}
	if err != nil {
		return UploadResult{}, fmt.Errorf("ledgers: insert: %w", err)
	}
	return UploadResult{Item: detail(row, "")}, nil
}

// Delete removes a file (the row, then the object, best effort).
func (s *Service) Delete(ctx context.Context, sub Subject, id uuid.UUID) error {
	key, err := s.q.DeleteLedgerFile(ctx, db.DeleteLedgerFileParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("ledgers: delete: %w", err)
	}
	if s.store != nil {
		if err := s.store.Delete(ctx, key); err != nil {
			s.log.WarnContext(ctx, "ledgers: delete object", "key", key, "error", err)
		}
	}
	return nil
}

// File opens the stored file.
func (s *Service) File(ctx context.Context, sub Subject, id uuid.UUID) (io.ReadCloser, Item, error) {
	f, err := s.q.GetLedgerFile(ctx, db.GetLedgerFileParams{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, Item{}, ErrNotFound
	}
	if err != nil {
		return nil, Item{}, fmt.Errorf("ledgers: get: %w", err)
	}
	if s.store == nil {
		return nil, Item{}, ErrStorageMissing
	}
	rc, _, err := s.store.Download(ctx, f.StorageKey)
	if err != nil {
		// The row outlived its object (a demo row without a store, a
		// manual cleanup): nothing to show.
		s.log.WarnContext(ctx, "ledgers: download", "key", f.StorageKey, "error", err)
		return nil, Item{}, ErrNotFound
	}
	return rc, detail(f, "").Item, nil
}
