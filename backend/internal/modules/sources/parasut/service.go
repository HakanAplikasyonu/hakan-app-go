package parasut

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

// VaultPurpose tags source-program credentials in the vault (02 §4).
const VaultPurpose = "source_connection"

// SyncThrottle: "şimdi senkronla" once per 5 minutes per connection (02 §3.3).
const SyncThrottle = 5 * time.Minute

var (
	ErrNotConfigured = errors.New("parasut: integrator application is not configured")
	ErrNoVault       = errors.New("parasut: credential vault is not configured")
	ErrNotFound      = errors.New("parasut: connection not found")
	ErrBadState      = errors.New("parasut: action not allowed in this state")
	ErrUnknownCo     = errors.New("parasut: company is not accessible with this login")
	ErrThrottled     = errors.New("parasut: sync requested too often")
	ErrBadRange      = errors.New("parasut: invalid date range")
)

// Service runs the connection flow of 10 §1.2 on source_connections.
type Service struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	vault secrets.Vault
	app   App
	hc    *http.Client
	now   func() time.Time
}

// NewService builds the service; hc nil → default client.
func NewService(pool *pgxpool.Pool, vault secrets.Vault, app App, hc *http.Client) *Service {
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	return &Service{pool: pool, q: database.QueriesFor(pool), vault: vault, app: app, hc: hc, now: time.Now}
}

type meta struct {
	CompanyName     string     `json:"company_name,omitempty"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	LastSyncRequest *time.Time `json:"last_sync_request_at,omitempty"`
	LastReport      *Report    `json:"last_report,omitempty"`
}

// Connection is the API view.
type Connection struct {
	UUID         uuid.UUID  `json:"uuid"`
	SetupState   string     `json:"setup_state"`
	Status       string     `json:"status"`
	CompanyID    string     `json:"company_id,omitempty"`
	CompanyName  string     `json:"company_name,omitempty"`
	AuthorizeURL string     `json:"authorize_url,omitempty"`
	Error        string     `json:"error,omitempty"`
	LastSyncAt   *time.Time `json:"last_sync_at,omitempty"`
	LastReport   *Report    `json:"last_report,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (s *Service) view(row db.SourceConnection) Connection {
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	c := Connection{UUID: row.Uuid, SetupState: row.SetupState, Status: row.Status, CompanyID: row.ExternalCompanyID.String,
		CompanyName: m.CompanyName, LastSyncAt: m.LastSyncAt, LastReport: m.LastReport, CreatedAt: row.CreatedAt.Time}
	if row.SetupState == "draft" || row.Status == "auth_failed" {
		c.AuthorizeURL = s.app.AuthorizeURL(row.Uuid.String())
	}
	var le struct {
		Message string `json:"message"`
	}
	if len(row.LastError) > 0 && json.Unmarshal(row.LastError, &le) == nil {
		c.Error = le.Message
	}
	return c
}

func (s *Service) ready() error {
	if !s.app.Configured() {
		return ErrNotConfigured
	}
	if s.vault == nil {
		return ErrNoVault
	}
	return nil
}

// Create opens a draft and returns the Paraşüt authorize URL (state = the
// connection uuid; the callback page posts the code back with auth).
func (s *Service) Create(ctx context.Context, taxpayerID int64) (Connection, error) {
	if err := s.ready(); err != nil {
		return Connection{}, err
	}
	row, err := s.q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
		TaxpayerID: taxpayerID, SourceSystem: SourceSystem, SetupState: "draft", Meta: []byte("{}"),
	})
	if err != nil {
		return Connection{}, err
	}
	return s.view(row), nil
}

// List returns the taxpayer's Paraşüt connections.
func (s *Service) List(ctx context.Context, taxpayerID int64) ([]Connection, error) {
	rows, err := s.q.ListSourceConnectionsBySystem(ctx, db.ListSourceConnectionsBySystemParams{TaxpayerID: taxpayerID, SourceSystem: SourceSystem})
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(r))
	}
	return out, nil
}

func (s *Service) load(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.SourceConnection, error) {
	row, err := s.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.SourceSystem != SourceSystem) {
		return db.SourceConnection{}, ErrNotFound
	}
	return row, err
}

func (s *Service) saveTokens(ctx context.Context, row db.SourceConnection, t Tokens) (db.SourceConnection, error) {
	raw, _ := json.Marshal(t)
	sec := secrets.NewSecret(raw)
	if row.CredentialsRef.Valid {
		return row, s.vault.Replace(ctx, VaultPurpose, secrets.Ref(row.CredentialsRef.String), sec)
	}
	ref, err := s.vault.Put(ctx, VaultPurpose, sec)
	if err != nil {
		return row, err
	}
	if _, err := s.q.SetSourceConnectionCredentialsRef(ctx, db.SetSourceConnectionCredentialsRefParams{
		CredentialsRef: pgtype.Text{String: string(ref), Valid: true}, SetupState: "credentials_entered",
		TaxpayerID: row.TaxpayerID, ID: row.ID,
	}); err != nil {
		return row, err
	}
	return s.load(ctx, row.TaxpayerID, row.Uuid)
}

func (s *Service) client(ctx context.Context, row db.SourceConnection) (*Client, error) {
	sec, err := s.vault.Get(ctx, VaultPurpose, secrets.Ref(row.CredentialsRef.String))
	if err != nil {
		return nil, err
	}
	var t Tokens
	if err := json.Unmarshal(sec.Reveal(), &t); err != nil {
		return nil, err
	}
	c := NewClient(s.app, s.hc, row.ExternalCompanyID.String, t)
	// 10 §1.3: the new refresh token replaces the old one at once.
	c.OnTokens = func(nt Tokens) error {
		raw, _ := json.Marshal(nt)
		return s.vault.Replace(ctx, VaultPurpose, secrets.Ref(row.CredentialsRef.String), secrets.NewSecret(raw))
	}
	return c, nil
}

// Authorize exchanges the code from the redirect and lists the companies;
// a login with a single company is bound to it at once.
func (s *Service) Authorize(ctx context.Context, taxpayerID int64, id uuid.UUID, code string) (Connection, []Company, error) {
	if err := s.ready(); err != nil {
		return Connection{}, nil, err
	}
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, nil, err
	}
	t, err := s.app.Exchange(ctx, s.hc, code)
	if err != nil {
		if errors.Is(err, ErrAuth) {
			row, _ = s.setError(ctx, row, "auth_failed", "Paraşüt izni alınamadı. Bağlantıyı yeniden başlatın.")
			return s.view(row), nil, err
		}
		return Connection{}, nil, err
	}
	if row, err = s.saveTokens(ctx, row, t); err != nil {
		return Connection{}, nil, err
	}
	cos, err := s.companies(ctx, row)
	if err != nil {
		return Connection{}, nil, err
	}
	if len(cos) == 1 {
		c, err := s.bind(ctx, row, cos[0])
		return c, cos, err
	}
	return s.view(row), cos, nil
}

func (s *Service) companies(ctx context.Context, row db.SourceConnection) ([]Company, error) {
	c, err := s.client(ctx, row)
	if err != nil {
		return nil, err
	}
	return c.Companies(ctx)
}

// Companies lists the firms the stored login can read.
func (s *Service) Companies(ctx context.Context, taxpayerID int64, id uuid.UUID) ([]Company, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return nil, err
	}
	if !row.CredentialsRef.Valid {
		return nil, ErrBadState
	}
	return s.companies(ctx, row)
}

// SelectCompany binds the connection to one firm (external_company_id).
func (s *Service) SelectCompany(ctx context.Context, taxpayerID int64, id uuid.UUID, companyID string) (Connection, error) {
	cos, err := s.Companies(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	for _, c := range cos {
		if c.ID == companyID {
			return s.bind(ctx, row, c)
		}
	}
	return Connection{}, ErrUnknownCo
}

func (s *Service) bind(ctx context.Context, row db.SourceConnection, co Company) (Connection, error) {
	p, _ := json.Marshal(map[string]any{"company_name": co.Name})
	row, err := s.q.SetSourceConnectionCompany(ctx, db.SetSourceConnectionCompanyParams{
		ExternalCompanyID: pgtype.Text{String: co.ID, Valid: true}, SetupState: "active", Status: "active", MetaPatch: p, TaxpayerID: row.TaxpayerID, ID: row.ID,
	})
	if err != nil {
		return Connection{}, err
	}
	return s.view(row), nil
}

func (s *Service) setError(ctx context.Context, row db.SourceConnection, status, msg string) (db.SourceConnection, error) {
	le, _ := json.Marshal(map[string]any{"message": msg, "at": s.now()})
	return s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: row.SetupState, Status: status, LastError: le, MetaPatch: []byte("{}"), TaxpayerID: row.TaxpayerID, ID: row.ID,
	})
}

// Sync reads [from, to] and writes it (02 §3: the accountant's
// "şimdi senkronla"; the 4-hourly schedule of 10 §8 uses the same call).
func (s *Service) Sync(ctx context.Context, taxpayerID int64, id uuid.UUID, from, to time.Time) (Report, error) {
	if err := s.ready(); err != nil {
		return Report{}, err
	}
	row, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return Report{}, err
	}
	if row.SetupState != "active" || !row.ExternalCompanyID.Valid {
		return Report{}, ErrBadState
	}
	if to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		return Report{}, ErrBadRange
	}
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	now := s.now()
	if m.LastSyncRequest != nil && now.Sub(*m.LastSyncRequest) < SyncThrottle {
		return Report{}, ErrThrottled
	}
	p, _ := json.Marshal(map[string]any{"last_sync_request_at": now})
	if row, err = s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: row.SetupState, Status: row.Status, LastError: row.LastError, MetaPatch: p, TaxpayerID: taxpayerID, ID: row.ID,
	}); err != nil {
		return Report{}, err
	}
	c, err := s.client(ctx, row)
	if err != nil {
		return Report{}, err
	}
	b, err := c.Read(ctx, from, to)
	if err != nil {
		if errors.Is(err, ErrAuth) {
			_, _ = s.setError(ctx, row, "auth_failed", "Paraşüt bağlantısının izni sona ermiş. Bağlantıyı yeniden yetkilendirin.")
		}
		return Report{}, fmt.Errorf("parasut read: %w", err)
	}
	rep, err := sources.Write(ctx, s.pool, taxpayerID, SourceSystem, b)
	if err != nil {
		return Report{}, err
	}
	p, _ = json.Marshal(map[string]any{"last_sync_at": s.now(), "last_report": rep})
	_, err = s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: "active", Status: "active", MetaPatch: p, MarkSuccess: true, TaxpayerID: taxpayerID, ID: row.ID,
	})
	return rep, err
}
