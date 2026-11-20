package kolaybi

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
	ErrNotConfigured = errors.New("kolaybi: KOLAYBI_CHANNEL is not configured")
	ErrNoVault       = errors.New("kolaybi: credential vault is not configured")
	ErrNotFound      = errors.New("kolaybi: connection not found")
	ErrBadState      = errors.New("kolaybi: action not allowed in this state")
	ErrUnknownCo     = errors.New("kolaybi: company is not accessible with this api key")
	ErrThrottle      = errors.New("kolaybi: sync requested too often")
	ErrBadRange      = errors.New("kolaybi: invalid date range")
)

// Service runs the connection on source_connections (source_system kolaybi).
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	vault    secrets.Vault
	app      App
	hc       *http.Client
	now      func() time.Time
	interval time.Duration
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, vault secrets.Vault, app App, hc *http.Client) *Service {
	return &Service{pool: pool, q: database.QueriesFor(pool), vault: vault, app: app, hc: hc, now: time.Now, interval: time.Second}
}

type meta struct {
	CompanyName     string          `json:"company_name,omitempty"`
	LastSyncAt      *time.Time      `json:"last_sync_at,omitempty"`
	LastSyncRequest *time.Time      `json:"last_sync_request_at,omitempty"`
	LastReport      *sources.Report `json:"last_report,omitempty"`
}

// Connection is the API view.
type Connection struct {
	UUID        uuid.UUID       `json:"uuid"`
	SetupState  string          `json:"setup_state"`
	Status      string          `json:"status"`
	CompanyID   string          `json:"company_id,omitempty"`
	CompanyName string          `json:"company_name,omitempty"`
	Error       string          `json:"error,omitempty"`
	LastSyncAt  *time.Time      `json:"last_sync_at,omitempty"`
	LastReport  *sources.Report `json:"last_report,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

func view(row db.SourceConnection) Connection {
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	c := Connection{UUID: row.Uuid, SetupState: row.SetupState, Status: row.Status, CompanyID: row.ExternalCompanyID.String,
		CompanyName: m.CompanyName, LastSyncAt: m.LastSyncAt, LastReport: m.LastReport, CreatedAt: row.CreatedAt.Time}
	var le struct {
		Message string `json:"message"`
	}
	if len(row.LastError) > 0 && json.Unmarshal(row.LastError, &le) == nil {
		c.Error = le.Message
	}
	return c
}

// Configured reports whether the server can open and use this connection
// (application settings and the secret vault). The connection list carries
// it so the web panel shows an empty state instead of calling routes that
// would answer 503 (#639).
func (s *Service) Configured() bool { return s.app.Channel != "" && s.vault != nil }

func (s *Service) ready() error {
	if s.app.Channel == "" {
		return ErrNotConfigured
	}
	if s.vault == nil {
		return ErrNoVault
	}
	return nil
}

func (s *Service) newClient(apiKey, companyID string) *Client {
	c := NewClient(s.app, s.hc, apiKey, companyID)
	c.Interval = s.interval
	return c
}

// Connect stores the taxpayer's api_key (11 §1.1) after a token and the
// company list prove it works. A single company (or the given one) is bound
// at once; otherwise the companies are returned for SelectCompany.
func (s *Service) Connect(ctx context.Context, taxpayerID int64, apiKey, companyID string) (Connection, []Company, error) {
	if err := s.ready(); err != nil {
		return Connection{}, nil, err
	}
	if apiKey == "" {
		return Connection{}, nil, ErrBadState
	}
	cos, err := s.newClient(apiKey, "").Companies(ctx)
	if err != nil {
		return Connection{}, nil, err
	}
	ref, err := s.vault.Put(ctx, VaultPurpose, secrets.NewSecret([]byte(apiKey)))
	if err != nil {
		return Connection{}, nil, err
	}
	row, err := s.q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
		TaxpayerID: taxpayerID, SourceSystem: SourceSystem, CredentialsRef: pgtype.Text{String: string(ref), Valid: true},
		SetupState: "credentials_entered", Meta: []byte("{}"),
	})
	if err != nil {
		return Connection{}, nil, err
	}
	if companyID == "" && len(cos) == 1 {
		companyID = cos[0].ID
	}
	if companyID != "" {
		c, err := s.bind(ctx, row, cos, companyID)
		return c, cos, err
	}
	return view(row), cos, nil
}

func (s *Service) bind(ctx context.Context, row db.SourceConnection, cos []Company, companyID string) (Connection, error) {
	for _, co := range cos {
		if co.ID != companyID {
			continue
		}
		p, _ := json.Marshal(map[string]any{"company_name": co.Name})
		row, err := s.q.SetSourceConnectionCompany(ctx, db.SetSourceConnectionCompanyParams{
			ExternalCompanyID: pgtype.Text{String: co.ID, Valid: true}, SetupState: "active", Status: "active",
			MetaPatch: p, TaxpayerID: row.TaxpayerID, ID: row.ID,
		})
		if err != nil {
			return Connection{}, err
		}
		return view(row), nil
	}
	return view(row), ErrUnknownCo
}

func (s *Service) load(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.SourceConnection, string, error) {
	if err := s.ready(); err != nil {
		return db.SourceConnection{}, "", err
	}
	row, err := s.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.SourceSystem != SourceSystem) {
		return row, "", ErrNotFound
	}
	if err != nil {
		return row, "", err
	}
	sec, err := s.vault.Get(ctx, VaultPurpose, secrets.Ref(row.CredentialsRef.String))
	if err != nil {
		return row, "", err
	}
	return row, string(sec.Reveal()), nil
}

// SelectCompany binds a multi-company key to one firm.
func (s *Service) SelectCompany(ctx context.Context, taxpayerID int64, id uuid.UUID, companyID string) (Connection, error) {
	row, key, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	cos, err := s.newClient(key, "").Companies(ctx)
	if err != nil {
		return Connection{}, err
	}
	return s.bind(ctx, row, cos, companyID)
}

// List returns the taxpayer's KolayBi connections.
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

// Sync reads [from, to] and writes it through the common writer.
func (s *Service) Sync(ctx context.Context, taxpayerID int64, id uuid.UUID, from, to time.Time) (sources.Report, error) {
	row, key, err := s.load(ctx, taxpayerID, id)
	if err != nil {
		return sources.Report{}, err
	}
	if row.SetupState != "active" || !row.ExternalCompanyID.Valid {
		return sources.Report{}, ErrBadState
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
	b, err := s.newClient(key, row.ExternalCompanyID.String).Read(ctx, from, to)
	if err != nil {
		if errors.Is(err, ErrAuth) {
			le, _ := json.Marshal(map[string]any{"message": "KolayBi API anahtarı geçersiz. Yeni anahtarla yeniden bağlanın.", "at": now})
			_, _ = s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
				SetupState: row.SetupState, Status: "auth_failed", LastError: le, MetaPatch: []byte("{}"), TaxpayerID: taxpayerID, ID: row.ID,
			})
		}
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
