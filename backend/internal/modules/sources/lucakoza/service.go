package lucakoza

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// SyncThrottle: once per 5 minutes per connection (02 §3.3).
const SyncThrottle = 5 * time.Minute

var (
	ErrOff       = errors.New("lucakoza: turned off in the office settings")
	ErrNotFound  = errors.New("lucakoza: connection not found")
	ErrBadState  = errors.New("lucakoza: action not allowed in this state")
	ErrMissing   = errors.New("lucakoza: address, organisation code, user name and password are required")
	ErrUnknownCo = errors.New("lucakoza: company is not in the user's list")
	ErrThrottle  = errors.New("lucakoza: sync requested too often")
	ErrBadRange  = errors.New("lucakoza: invalid date range")
)

// Service runs Luca Koza connections on source_connections (source_system
// luca_koza). No credential is stored: credentials_ref stays empty.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	hc   *http.Client
	now  func() time.Time
}

// NewService builds the service; hc nil uses GuardedHTTP.
func NewService(pool *pgxpool.Pool, hc *http.Client) *Service {
	if hc == nil {
		hc = GuardedHTTP()
	}
	return &Service{pool: pool, q: database.QueriesFor(pool), hc: hc, now: time.Now}
}

// Settings is the office's "Luca Koza bağlantısı" setting.
type Settings struct {
	Enabled   bool       `json:"enabled"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// Settings returns the office's setting; without a row it is off.
func (s *Service) Settings(ctx context.Context, orgID int64) (Settings, error) {
	row, err := s.q.GetLucaKozaSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("luca koza settings: %w", err)
	}
	return settingsOf(row), nil
}

// SaveSettings stores the office's setting.
func (s *Service) SaveSettings(ctx context.Context, orgID, actorID int64, enabled bool) (Settings, error) {
	var by pgtype.Int8
	if actorID > 0 {
		by = pgtype.Int8{Int64: actorID, Valid: true}
	}
	row, err := s.q.UpsertLucaKozaSettings(ctx, db.UpsertLucaKozaSettingsParams{OrganizationID: orgID, Enabled: enabled, UpdatedBy: by})
	if err != nil {
		return Settings{}, fmt.Errorf("save luca koza settings: %w", err)
	}
	return settingsOf(row), nil
}

func settingsOf(row db.LucaKozaSetting) Settings {
	out := Settings{Enabled: row.Enabled}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time
		out.UpdatedAt = &t
	}
	return out
}

func (s *Service) on(ctx context.Context, orgID int64) error {
	st, err := s.Settings(ctx, orgID)
	if err != nil {
		return err
	}
	if !st.Enabled {
		return ErrOff
	}
	return nil
}

// meta is what the connection keeps: no password, ever.
type meta struct {
	BaseURL         string          `json:"base_url,omitempty"`
	OrgCode         string          `json:"org_code,omitempty"`
	Username        string          `json:"username,omitempty"`
	Companies       []Company       `json:"companies,omitempty"`
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
	BaseURL     string          `json:"base_url"`
	OrgCode     string          `json:"org_code"`
	Username    string          `json:"username"`
	CompanyID   string          `json:"company_id,omitempty"`
	CompanyName string          `json:"company_name,omitempty"`
	Companies   []Company       `json:"companies"`
	LastSyncAt  *time.Time      `json:"last_sync_at,omitempty"`
	LastReport  *sources.Report `json:"last_report,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

func view(row db.SourceConnection) Connection {
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	if m.Companies == nil {
		m.Companies = []Company{}
	}
	return Connection{UUID: row.Uuid, SetupState: row.SetupState, Status: row.Status, BaseURL: m.BaseURL,
		OrgCode: m.OrgCode, Username: m.Username, CompanyID: row.ExternalCompanyID.String, CompanyName: m.CompanyName,
		Companies: m.Companies, LastSyncAt: m.LastSyncAt, LastReport: m.LastReport, CreatedAt: row.CreatedAt.Time}
}

// Credentials are one request's sign-in data. The password lives only in
// this value for the request; it is never stored or logged.
type Credentials struct {
	BaseURL  string
	OrgCode  string
	Username string
	Password string
}

func (cr Credentials) complete() bool {
	return strings.TrimSpace(cr.BaseURL) != "" && strings.TrimSpace(cr.OrgCode) != "" &&
		strings.TrimSpace(cr.Username) != "" && cr.Password != ""
}

// session signs in to Koza with cr.
func (s *Service) session(ctx context.Context, cr Credentials) (*Client, error) {
	c, err := NewClient(cr.BaseURL, s.hc)
	if err != nil {
		return nil, err
	}
	if err := c.Login(ctx, strings.TrimSpace(cr.OrgCode), strings.TrimSpace(cr.Username), cr.Password); err != nil {
		return nil, err
	}
	return c, nil
}

// Connect signs in once, reads the user's companies/branches and opens the
// connection with the non-secret settings. A single company (or the given
// one) is selected at once; otherwise the list is returned for
// SelectCompany. The password is dropped when the call returns.
func (s *Service) Connect(ctx context.Context, orgID, taxpayerID int64, cr Credentials, companyID string) (Connection, []Company, error) {
	if err := s.on(ctx, orgID); err != nil {
		return Connection{}, nil, err
	}
	if !cr.complete() {
		return Connection{}, nil, ErrMissing
	}
	base, err := NormalizeBase(cr.BaseURL)
	if err != nil {
		return Connection{}, nil, err
	}
	c, err := s.session(ctx, cr)
	if err != nil {
		return Connection{}, nil, err
	}
	cos, err := c.Companies(ctx)
	if err != nil {
		return Connection{}, nil, err
	}
	if cos == nil {
		cos = []Company{}
	}
	m, _ := json.Marshal(meta{BaseURL: base, OrgCode: strings.TrimSpace(cr.OrgCode), Username: strings.TrimSpace(cr.Username), Companies: cos})
	row, err := s.q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
		TaxpayerID: taxpayerID, SourceSystem: SourceSystem, SetupState: "credentials_entered", Meta: m,
	})
	if err != nil {
		return Connection{}, nil, err
	}
	if companyID == "" && len(cos) == 1 {
		companyID = cos[0].ID
	}
	if companyID != "" {
		conn, err := s.bind(ctx, row, companyID)
		return conn, cos, err
	}
	return view(row), cos, nil
}

func (s *Service) bind(ctx context.Context, row db.SourceConnection, companyID string) (Connection, error) {
	var m meta
	_ = json.Unmarshal(row.Meta, &m)
	for _, co := range m.Companies {
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

func (s *Service) load(ctx context.Context, orgID, taxpayerID int64, id uuid.UUID) (db.SourceConnection, error) {
	if err := s.on(ctx, orgID); err != nil {
		return db.SourceConnection{}, err
	}
	row, err := s.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.SourceSystem != SourceSystem) {
		return row, ErrNotFound
	}
	return row, err
}

// SelectCompany binds the connection to one company/branch from the list
// read at Connect (no sign-in needed).
func (s *Service) SelectCompany(ctx context.Context, orgID, taxpayerID int64, id uuid.UUID, companyID string) (Connection, error) {
	row, err := s.load(ctx, orgID, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	return s.bind(ctx, row, companyID)
}

// List returns the taxpayer's Luca Koza connections.
func (s *Service) List(ctx context.Context, orgID, taxpayerID int64) ([]Connection, error) {
	if err := s.on(ctx, orgID); err != nil {
		return nil, err
	}
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

// Sync signs in with the stored address, code and user name plus the
// password of this request, selects the bound company/branch, reads
// [from, to] and writes it through the common writer.
func (s *Service) Sync(ctx context.Context, orgID, taxpayerID int64, id uuid.UUID, from, to time.Time, password string) (sources.Report, error) {
	row, err := s.load(ctx, orgID, taxpayerID, id)
	if err != nil {
		return sources.Report{}, err
	}
	if row.SetupState != "active" || !row.ExternalCompanyID.Valid {
		return sources.Report{}, ErrBadState
	}
	if password == "" {
		return sources.Report{}, ErrMissing
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
	c, err := s.session(ctx, Credentials{BaseURL: m.BaseURL, OrgCode: m.OrgCode, Username: m.Username, Password: password})
	if err != nil {
		return sources.Report{}, err // a mistyped password does not start the throttle
	}
	p, _ := json.Marshal(map[string]any{"last_sync_request_at": now})
	if _, err := s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: row.SetupState, Status: row.Status, LastError: row.LastError, MetaPatch: p, TaxpayerID: taxpayerID, ID: row.ID,
	}); err != nil {
		return sources.Report{}, err
	}
	if err := c.SelectCompany(ctx, row.ExternalCompanyID.String); err != nil {
		return sources.Report{}, err
	}
	b, err := c.Read(ctx, from, to)
	if err != nil {
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
