package easytrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// VaultPurpose tags source credentials (02 §4).
const VaultPurpose = "source_connection"

// SyncThrottle: once per 5 minutes per connection (02 §3.3).
const SyncThrottle = 5 * time.Minute

var (
	ErrNoVault  = errors.New("easytrade: credential vault is not configured")
	ErrNotFound = errors.New("easytrade: connection not found")
	ErrBadState = errors.New("easytrade: action not allowed in this state")
	ErrThrottle = errors.New("easytrade: sync requested too often")
	ErrBadRange = errors.New("easytrade: invalid date range")
)

// Service runs the connection on source_connections (source_system easytrade).
type Service struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	vault secrets.Vault
	base  string
	hc    *http.Client
	now   func() time.Time
	// interval spaces requests (0 in tests).
	interval time.Duration
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, vault secrets.Vault, baseURL string, hc *http.Client) *Service {
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	return &Service{pool: pool, q: database.QueriesFor(pool), vault: vault, base: BaseURL(baseURL), hc: hc, now: time.Now, interval: 200 * time.Millisecond}
}

type meta struct {
	Branches        []Branch        `json:"branches,omitempty"`
	LastSyncAt      *time.Time      `json:"last_sync_at,omitempty"`
	LastSyncRequest *time.Time      `json:"last_sync_request_at,omitempty"`
	LastReport      *sources.Report `json:"last_report,omitempty"`
	LastWebhookAt   *time.Time      `json:"last_webhook_at,omitempty"`
	CompanyName     string          `json:"company_name,omitempty"`
}

// Connection is the API view.
type Connection struct {
	UUID       uuid.UUID `json:"uuid"`
	SetupState string    `json:"setup_state"`
	Status     string    `json:"status"`
	CompanyID  string    `json:"company_id"`
	// CompanyName: the company Easytrade named at sign-in.
	CompanyName string   `json:"company_name,omitempty"`
	Branches    []Branch `json:"branches"`
	// EnteredCompanyID: only in the connect answer, when the entered ID was
	// not the user's company and the sign-in company was used instead.
	EnteredCompanyID string          `json:"entered_company_id,omitempty"`
	Error            string          `json:"error,omitempty"`
	LastSyncAt       *time.Time      `json:"last_sync_at,omitempty"`
	LastReport       *sources.Report `json:"last_report,omitempty"`
	// Schedule (issue #385): nil interval = manual only.
	SyncIntervalMinutes *int32     `json:"sync_interval_minutes"`
	NextSyncAt          *time.Time `json:"next_sync_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	SyncFailures        int32      `json:"sync_failures"`
	LastRun             *Run       `json:"last_run,omitempty"`
	// LastWebhookAt: the last signed push from Easytrade (issue #385).
	LastWebhookAt *time.Time `json:"last_webhook_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// view adds the last run to the row's view.
func (s *Service) view(ctx context.Context, row db.SourceConnection) Connection {
	c := view(row)
	if rs, err := s.q.ListSyncRuns(ctx, db.ListSyncRunsParams{TaxpayerID: row.TaxpayerID, ConnectionID: row.ID, Lim: 1}); err == nil && len(rs) > 0 {
		r := runView(rs[0])
		c.LastRun = &r
	}
	return c
}

func view(row db.SourceConnection) Connection {
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	c := Connection{UUID: row.Uuid, SetupState: row.SetupState, Status: row.Status, CompanyID: row.ExternalCompanyID.String,
		CompanyName: m.CompanyName, Branches: m.Branches, LastSyncAt: m.LastSyncAt, LastReport: m.LastReport, LastWebhookAt: m.LastWebhookAt, CreatedAt: row.CreatedAt.Time}
	if c.Branches == nil {
		c.Branches = []Branch{}
	}
	if row.SyncIntervalMinutes.Valid {
		v := row.SyncIntervalMinutes.Int32
		c.SyncIntervalMinutes = &v
	}
	if row.NextSyncAt.Valid {
		t := row.NextSyncAt.Time
		c.NextSyncAt = &t
	}
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time
		c.LastSuccessAt = &t
	}
	c.SyncFailures = row.SyncFailures
	var le struct {
		Message string `json:"message"`
	}
	if len(row.LastError) > 0 && json.Unmarshal(row.LastError, &le) == nil {
		c.Error = le.Message
	}
	return c
}

// Connect logs in with the taxpayer's Easytrade user (09 §1.1; the password
// is not stored, only the 30-day token), checks the company by listing its
// branches and activates the connection.
func (s *Service) Connect(ctx context.Context, taxpayerID int64, email, password, companyID string) (Connection, error) {
	if s.vault == nil {
		return Connection{}, ErrNoVault
	}
	email, companyID = NormalizeEmail(email), strings.TrimSpace(companyID)
	if email == "" || password == "" {
		return Connection{}, ErrBadState
	}
	sess, branches, err := s.probe(ctx, email, password, companyID)
	if err != nil {
		return Connection{}, err
	}
	// The company is the one Easytrade named at sign-in (or the entered ID
	// when Easytrade names none).
	entered := companyID
	companyID = sess.CompanyID
	tok := sess.Token
	raw, _ := json.Marshal(tok)
	ref, err := s.vault.Put(ctx, VaultPurpose, secrets.NewSecret(raw))
	if err != nil {
		return Connection{}, err
	}
	m, _ := json.Marshal(meta{Branches: branches, CompanyName: sess.CompanyName})
	row, err := s.q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
		TaxpayerID: taxpayerID, SourceSystem: SourceSystem, ExternalCompanyID: pgtype.Text{String: companyID, Valid: true},
		CredentialsRef: pgtype.Text{String: string(ref), Valid: true}, SetupState: "active", Meta: m,
	})
	if err != nil {
		return Connection{}, err
	}
	row, err = s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: "active", Status: "active", MetaPatch: []byte("{}"), MarkSuccess: false, TaxpayerID: taxpayerID, ID: row.ID,
	})
	if err != nil {
		return Connection{}, err
	}
	// Scheduled by default (issue #385): the first sweep reads the open periods.
	row, err = s.q.SetSourceConnectionSchedule(ctx, db.SetSourceConnectionScheduleParams{
		SyncIntervalMinutes: pgtype.Int4{Int32: int32(DefaultSyncInterval / time.Minute), Valid: true},
		NextSyncAt:          pgtype.Timestamptz{Time: s.now(), Valid: true}, TaxpayerID: taxpayerID, ID: row.ID,
	})
	if err != nil {
		return Connection{}, err
	}
	c := s.view(ctx, row)
	if entered != "" && entered != companyID {
		c.EnteredCompanyID = entered
	}
	return c, nil
}

// companyIDRe: Easytrade's company ID is its numeric record id (the "ID"
// the user sees; not the 6-digit "Müşteri No").
var companyIDRe = regexp.MustCompile(`^[0-9]{1,18}$`)

// ErrBadCompanyID: the entered ID is not a number.
var ErrBadCompanyID = errors.New("easytrade: the company ID is a number")

// ErrNoCompany: no ID was entered and Easytrade did not name the user's
// company at sign-in either.
var ErrNoCompany = errors.New("easytrade: the user's company is unknown; enter the ID")

// probe signs in and resolves the company: the one Easytrade names at
// sign-in (user.company.id of the login / refresh answer) wins; an entered
// ID that differs is not an error (the user cannot pick another company
// with this login) and is only used when Easytrade names none. Its branches
// must be readable with it. The returned session's CompanyID is the
// resolved ID.
func (s *Service) probe(ctx context.Context, email, password, companyID string) (Session, []Branch, error) {
	companyID = strings.TrimSpace(companyID)
	if companyID != "" && !companyIDRe.MatchString(companyID) {
		return Session{}, nil, ErrBadCompanyID
	}
	sess, err := SignIn(ctx, s.base, s.hc, NormalizeEmail(email), password)
	if err != nil {
		return Session{}, nil, err
	}
	switch {
	case companyID == "" && sess.CompanyID == "":
		return sess, nil, ErrNoCompany
	case sess.CompanyID != "":
		companyID = sess.CompanyID
	}
	sess.CompanyID = companyID
	c := NewClient(s.base, s.hc, companyID, sess.Token)
	c.Interval = 0
	branches, err := c.Branches(ctx)
	switch {
	case errors.Is(err, ErrForbidden) || errors.Is(err, ErrLicense):
		return sess, nil, err
	case errors.Is(err, ErrAuth):
		// The fresh token works, yet the branch list was refused (401
		// after a refresh): a separate error with Easytrade's status.
		return sess, nil, &BranchesDeniedError{Host: hostOf(s.base), Status: http.StatusUnauthorized, CompanyID: companyID}
	case err != nil:
		return sess, nil, err
	}
	return sess, branches, nil
}

// TestResult is a successful connection test: nothing is stored.
type TestResult struct {
	OK          bool     `json:"ok"`
	CompanyID   string   `json:"company_id"`
	CompanyName string   `json:"company_name,omitempty"`
	Branches    []Branch `json:"branches"`
	// EnteredCompanyID: set when the entered ID was not the user's company
	// and the sign-in company was used instead (the screen says so).
	EnteredCompanyID string `json:"entered_company_id,omitempty"`
}

// Test checks the email, password and ID against Easytrade without storing
// anything (the "Bağlantıyı test et" button). The ID is optional: the
// result names the company found (resolved ID and name).
func (s *Service) Test(ctx context.Context, email, password, companyID string) (TestResult, error) {
	email = NormalizeEmail(email)
	if email == "" || password == "" {
		return TestResult{}, ErrBadState
	}
	sess, branches, err := s.probe(ctx, email, password, companyID)
	if err != nil {
		return TestResult{}, err
	}
	if branches == nil {
		branches = []Branch{}
	}
	res := TestResult{OK: true, CompanyID: sess.CompanyID, CompanyName: sess.CompanyName, Branches: branches}
	if id := strings.TrimSpace(companyID); id != "" && id != sess.CompanyID {
		res.EnteredCompanyID = id
	}
	return res, nil
}

// List returns the taxpayer's Easytrade connections.
func (s *Service) List(ctx context.Context, taxpayerID int64) ([]Connection, error) {
	rows, err := s.q.ListSourceConnectionsBySystem(ctx, db.ListSourceConnectionsBySystemParams{TaxpayerID: taxpayerID, SourceSystem: SourceSystem})
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(ctx, r))
	}
	return out, nil
}

// row loads the taxpayer's Easytrade connection.
func (s *Service) row(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.SourceConnection, error) {
	row, err := s.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.SourceSystem != SourceSystem) {
		return row, ErrNotFound
	}
	return row, err
}

func (s *Service) client(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.SourceConnection, *Client, error) {
	if s.vault == nil {
		return db.SourceConnection{}, nil, ErrNoVault
	}
	row, err := s.row(ctx, taxpayerID, id)
	if err != nil {
		return row, nil, err
	}
	if row.SetupState != "active" || !row.CredentialsRef.Valid {
		return row, nil, ErrBadState
	}
	sec, err := s.vault.Get(ctx, VaultPurpose, secrets.Ref(row.CredentialsRef.String))
	if err != nil {
		return row, nil, err
	}
	var t Token
	if err := json.Unmarshal(sec.Reveal(), &t); err != nil {
		return row, nil, err
	}
	c := NewClient(s.base, s.hc, row.ExternalCompanyID.String, t)
	c.Interval = s.interval
	c.OnToken = func(nt Token) error {
		raw, _ := json.Marshal(nt)
		return s.vault.Replace(ctx, VaultPurpose, secrets.Ref(row.CredentialsRef.String), secrets.NewSecret(raw))
	}
	return row, c, nil
}

func (s *Service) fail(ctx context.Context, row db.SourceConnection, err error) {
	status, msg := "", ""
	switch {
	case errors.Is(err, ErrLicense):
		status, msg = "disabled", "Mükellefin Easytrade lisansı bitmiş. Senkron durduruldu."
	case errors.Is(err, ErrForbidden):
		status, msg = "auth_failed", "Easytrade kullanıcısının bu firmada yetkisi yok. Yetkili bir kullanıcıyla bağlantıyı yeniden kurun."
	case errors.Is(err, ErrAuth):
		status, msg = "auth_failed", "Easytrade oturumu geçersiz. Bağlantıyı yeniden kurun."
	default:
		return
	}
	le, _ := json.Marshal(map[string]any{"message": msg, "at": s.now()})
	_, _ = s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: row.SetupState, Status: status, LastError: le, MetaPatch: []byte("{}"), TaxpayerID: row.TaxpayerID, ID: row.ID,
	})
}

// Sync reads [from, to] and writes it through the common writer.
func (s *Service) Sync(ctx context.Context, taxpayerID int64, id uuid.UUID, from, to time.Time) (sources.Report, error) {
	row, c, err := s.client(ctx, taxpayerID, id)
	if err != nil {
		return sources.Report{}, err
	}
	if row.Status == "disabled" {
		return sources.Report{}, ErrLicense
	}
	if to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		return sources.Report{}, ErrBadRange
	}
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	now := s.now()
	if m.LastSyncRequest != nil && now.Sub(*m.LastSyncRequest) < SyncThrottle {
		return sources.Report{}, ErrThrottle
	}
	p, _ := json.Marshal(map[string]any{"last_sync_request_at": now})
	if _, err := s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: row.SetupState, Status: row.Status, LastError: row.LastError, MetaPatch: p, TaxpayerID: taxpayerID, ID: row.ID,
	}); err != nil {
		return sources.Report{}, err
	}
	// The open periods (previous and current month up to today) are always
	// read too: the button is pressed to see what was just entered in
	// Easytrade, while the office's period picker usually shows the
	// declaration month (the previous one).
	openFrom, openTo := openPeriods(now)
	res, err := s.run(ctx, row, c, RunManual, [2]time.Time{from, to}, [2]time.Time{openFrom, openTo})
	return res.Counters.Report, err
}

// Reconcile compares Easytrade's VAT report of period with the portal (09 §8).
func (s *Service) Reconcile(ctx context.Context, taxpayerID int64, id uuid.UUID, period string) (Reconciliation, error) {
	row, c, err := s.client(ctx, taxpayerID, id)
	if err != nil {
		return Reconciliation{}, err
	}
	rec, err := c.Reconcile(ctx, s.pool, taxpayerID, period)
	if err != nil {
		s.fail(ctx, row, err)
	}
	return rec, err
}
