package easytrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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
	return &Service{pool: pool, q: db.New(pool), vault: vault, base: baseURL, hc: hc, now: time.Now, interval: 200 * time.Millisecond}
}

type meta struct {
	Branches        []Branch        `json:"branches,omitempty"`
	LastSyncAt      *time.Time      `json:"last_sync_at,omitempty"`
	LastSyncRequest *time.Time      `json:"last_sync_request_at,omitempty"`
	LastReport      *sources.Report `json:"last_report,omitempty"`
}

// Connection is the API view.
type Connection struct {
	UUID       uuid.UUID       `json:"uuid"`
	SetupState string          `json:"setup_state"`
	Status     string          `json:"status"`
	CompanyID  string          `json:"company_id"`
	Branches   []Branch        `json:"branches"`
	Error      string          `json:"error,omitempty"`
	LastSyncAt *time.Time      `json:"last_sync_at,omitempty"`
	LastReport *sources.Report `json:"last_report,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

func view(row db.SourceConnection) Connection {
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	c := Connection{UUID: row.Uuid, SetupState: row.SetupState, Status: row.Status, CompanyID: row.ExternalCompanyID.String,
		Branches: m.Branches, LastSyncAt: m.LastSyncAt, LastReport: m.LastReport, CreatedAt: row.CreatedAt.Time}
	if c.Branches == nil {
		c.Branches = []Branch{}
	}
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
	if email == "" || password == "" || companyID == "" {
		return Connection{}, ErrBadState
	}
	tok, err := Login(ctx, s.base, s.hc, email, password)
	if err != nil {
		return Connection{}, err
	}
	c := NewClient(s.base, s.hc, companyID, tok)
	branches, err := c.Branches(ctx)
	if err != nil {
		return Connection{}, err
	}
	raw, _ := json.Marshal(tok)
	ref, err := s.vault.Put(ctx, VaultPurpose, secrets.NewSecret(raw))
	if err != nil {
		return Connection{}, err
	}
	m, _ := json.Marshal(meta{Branches: branches})
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
	return view(row), nil
}

// List returns the taxpayer's Easytrade connections.
func (s *Service) List(ctx context.Context, taxpayerID int64) ([]Connection, error) {
	rows, err := s.q.ListSourceConnectionsBySystem(ctx, db.ListSourceConnectionsBySystemParams{TaxpayerID: taxpayerID, SourceSystem: SourceSystem})
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(r))
	}
	return out, nil
}

func (s *Service) client(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.SourceConnection, *Client, error) {
	if s.vault == nil {
		return db.SourceConnection{}, nil, ErrNoVault
	}
	row, err := s.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.SourceSystem != SourceSystem) {
		return row, nil, ErrNotFound
	}
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
	b, err := c.Read(ctx, from, to)
	if err != nil {
		s.fail(ctx, row, err)
		return sources.Report{}, err
	}
	rep, err := sources.Write(ctx, s.pool, taxpayerID, SourceSystem, b)
	if err != nil {
		return rep, err
	}
	p, _ = json.Marshal(map[string]any{"last_sync_at": s.now(), "last_report": rep})
	_, err = s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: "active", Status: "active", MetaPatch: p, MarkSuccess: true, TaxpayerID: taxpayerID, ID: row.ID,
	})
	return rep, err
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
