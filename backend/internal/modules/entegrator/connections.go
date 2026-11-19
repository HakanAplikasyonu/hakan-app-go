package entegrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
)

// Integrator connection setup from the office panel (#518, çapraz denetim
// A1). The office picks an integrator for the taxpayer, enters the
// web-service user; the credentials go to the vault (CredentialsPurpose) and
// the first sync (backfill) runs on the fixed-egress worker. The connection
// becomes active (or auth_failed) when that run finishes: the API process
// never calls the integrator itself, because integrators allowlist the
// worker's fixed IP (issue #45).

// Field is one credential input of an integrator form.
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	// Option: the value goes to Credentials.Options[Key] instead of a
	// top-level credential field.
	Option bool   `json:"option,omitempty"`
	Help   string `json:"help,omitempty"`
}

// IntegratorInfo is one entry of the integrator catalog.
type IntegratorInfo struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
	// IPFiltered: the integrator authorises by the caller's fixed IP and the
	// taxpayer's VKN; there is no user to enter (İşNet).
	IPFiltered bool `json:"ip_filtered"`
}

var (
	fUser      = Field{Key: "username", Label: "Web servis kullanıcı adı", Required: true}
	fPass      = Field{Key: "password", Label: "Web servis şifresi", Secret: true, Required: true}
	fAPIKey    = Field{Key: "api_key", Label: "API anahtarı", Secret: true, Required: true}
	catalogAll = []IntegratorInfo{
		{ID: "izibiz", Name: "İzibiz", Fields: []Field{fUser, fPass}},
		{ID: "isnet", Name: "İşNet", IPFiltered: true, Fields: []Field{}},
		{ID: "nes", Name: "NES Bilgi", Fields: []Field{fAPIKey}},
		{ID: "nilvera", Name: "Nilvera", Fields: []Field{fAPIKey}},
		{ID: "sovos", Name: "Sovos (Foriba)", Fields: []Field{fUser, fPass,
			{Key: "sender_alias", Label: "Gönderici birim etiketi", Option: true, Help: "urn:mail:defaultgb@… biçiminde"},
			{Key: "postbox_alias", Label: "Posta kutusu etiketi", Option: true, Help: "urn:mail:defaultpk@… biçiminde"},
		}},
		{ID: "turkcell", Name: "Turkcell e-Şirket", Fields: []Field{fAPIKey}},
		{ID: "uyumsoft", Name: "Uyumsoft", Fields: []Field{fUser, fPass}},
		{ID: "muhasebeizleme", Name: "Muhasebe İzleme (Uyumsoft)", Fields: []Field{
			{Key: "password", Label: "API anahtarı", Secret: true, Required: true},
		}},
		{ID: "trendyol", Name: "Trendyol e-Faturam", Fields: []Field{
			{Key: "username", Label: "Giriş e-postası", Required: true}, fPass,
		}},
	}
)

// Catalog lists the integrators the office can connect, by name.
func Catalog() []IntegratorInfo {
	out := append([]IntegratorInfo(nil), catalogAll...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func integrator(id string) (IntegratorInfo, bool) {
	for _, i := range catalogAll {
		if i.ID == id {
			return i, true
		}
	}
	return IntegratorInfo{}, false
}

var (
	ErrUnknownIntegrator  = errors.New("entegrator: unknown integrator")
	ErrConnectionNotFound = errors.New("entegrator: connection not found")
	ErrDuplicate          = errors.New("entegrator: the taxpayer already has a connection to this integrator")
	ErrBadCredentials     = errors.New("entegrator: credentials incomplete")
	ErrNoVault            = errors.New("entegrator: credential vault not configured")
	ErrNotReady           = errors.New("entegrator: the connection has no credentials yet")
	ErrNoQueue            = errors.New("entegrator: the job queue is not configured")
)

// Enqueuer starts a sync of kind on the fixed-egress worker.
type Enqueuer func(ctx context.Context, taxpayerID int64, connUUID uuid.UUID, kind string) error

// Connections manages integrator connections for the office panel.
type Connections struct {
	q       *db.Queries
	vault   secrets.Vault
	enqueue Enqueuer
}

// NewConnections builds the setup service. vault may be nil: entering
// credentials answers ErrNoVault. enqueue may be nil (QUEUE_ENABLED=false):
// the connection is set up but no sync is queued, and Sync answers
// ErrNoQueue. The API never calls the integrator itself.
func NewConnections(pool *pgxpool.Pool, vault secrets.Vault, enqueue Enqueuer) *Connections {
	return &Connections{q: database.QueriesFor(pool), vault: vault, enqueue: enqueue}
}

// connMeta is the integrator part of source_connections.meta.
type connMeta struct {
	Environment string `json:"environment,omitempty"`
	// LastSyncRequestAt: the last "şimdi çek" (ManualInterval applies at
	// the worker; the API only records it).
	LastSyncRequestAt *time.Time `json:"last_sync_request_at,omitempty"`
}

// RunView is the last sync run of a connection.
type RunView struct {
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Counters   Counters   `json:"counters"`
}

// ConnectionError is the last failure shown to the office.
type ConnectionError struct {
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message"`
}

// ConnectionView is the API view of one integrator connection.
type ConnectionView struct {
	UUID           uuid.UUID `json:"uuid"`
	IntegratorID   string    `json:"integrator_id"`
	IntegratorName string    `json:"integrator_name"`
	Environment    string    `json:"environment"`
	// State: draft (no credentials), waiting (first sync queued or
	// running), active, auth_failed, rate_limited, disabled.
	State          string           `json:"state"`
	HasCredentials bool             `json:"has_credentials"`
	IPFiltered     bool             `json:"ip_filtered"`
	Fields         []Field          `json:"fields"`
	Error          *ConnectionError `json:"error,omitempty"`
	LastSuccessAt  *time.Time       `json:"last_success_at,omitempty"`
	LastRun        *RunView         `json:"last_run,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}

// State derives the office-facing state of a connection row.
func State(setupState, status string, hasCredentials, ipFiltered bool) string {
	switch status {
	case "active", "auth_failed", "rate_limited", "disabled":
		return status
	}
	if !hasCredentials && !ipFiltered && setupState == "draft" {
		return "draft"
	}
	if setupState == "draft" {
		return "draft"
	}
	return "waiting"
}

func (c *Connections) view(ctx context.Context, row db.SourceConnection) (ConnectionView, error) {
	id, _ := ProviderName(row.SourceSystem)
	info, _ := integrator(id)
	var m connMeta
	_ = json.Unmarshal(row.Meta, &m)
	if m.Environment == "" {
		m.Environment = "production"
	}
	v := ConnectionView{
		UUID: row.Uuid, IntegratorID: id, IntegratorName: info.Name, Environment: m.Environment,
		HasCredentials: row.CredentialsRef.Valid, IPFiltered: info.IPFiltered, Fields: info.Fields,
		State:     State(row.SetupState, row.Status, row.CredentialsRef.Valid, info.IPFiltered),
		CreatedAt: row.CreatedAt.Time,
	}
	if v.IntegratorName == "" {
		v.IntegratorName = id
	}
	if v.Fields == nil {
		v.Fields = []Field{}
	}
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time
		v.LastSuccessAt = &t
	}
	if len(row.LastError) > 0 && row.Status != "active" {
		v.Error = errorView(row.LastError)
	}
	runs, err := c.q.ListSyncRuns(ctx, db.ListSyncRunsParams{TaxpayerID: row.TaxpayerID, ConnectionID: row.ID, Lim: 1})
	if err != nil {
		return ConnectionView{}, err
	}
	if len(runs) > 0 {
		r := runs[0]
		rv := RunView{Kind: r.Kind, Status: r.Status, StartedAt: r.StartedAt.Time}
		if r.FinishedAt.Valid {
			t := r.FinishedAt.Time
			rv.FinishedAt = &t
		}
		_ = json.Unmarshal(r.Counters, &rv.Counters)
		v.LastRun = &rv
		if v.Error == nil && r.Status == "failed" && len(r.Error) > 0 {
			v.Error = errorView(r.Error)
		}
	}
	return v, nil
}

// errorView reads errorJSON's shape ({"kind", "message"}) or a plain message.
func errorView(raw []byte) *ConnectionError {
	var e struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return nil
	}
	msg := e.Message
	if msg == "" {
		msg = e.Error
	}
	if msg == "" && e.Kind == "" {
		return nil
	}
	return &ConnectionError{Kind: e.Kind, Message: msg}
}

// List returns the taxpayer's integrator connections.
func (c *Connections) List(ctx context.Context, taxpayerID int64) ([]ConnectionView, error) {
	rows, err := c.q.ListSourceConnections(ctx, taxpayerID)
	if err != nil {
		return nil, err
	}
	out := []ConnectionView{}
	for _, r := range rows {
		if _, ok := ProviderName(r.SourceSystem); !ok {
			continue
		}
		v, err := c.view(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (c *Connections) load(ctx context.Context, taxpayerID int64, id uuid.UUID) (db.SourceConnection, error) {
	row, err := c.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SourceConnection{}, ErrConnectionNotFound
	}
	if err != nil {
		return db.SourceConnection{}, err
	}
	if _, ok := ProviderName(row.SourceSystem); !ok {
		return db.SourceConnection{}, ErrConnectionNotFound
	}
	return row, nil
}

// Get returns one connection.
func (c *Connections) Get(ctx context.Context, taxpayerID int64, id uuid.UUID) (ConnectionView, error) {
	row, err := c.load(ctx, taxpayerID, id)
	if err != nil {
		return ConnectionView{}, err
	}
	return c.view(ctx, row)
}

// Create opens a draft connection. environment is production (default) or
// test. One connection per integrator and taxpayer. An IP-filtered
// integrator needs no credentials: its first sync is queued at once.
func (c *Connections) Create(ctx context.Context, taxpayerID int64, integratorID, environment string) (ConnectionView, error) {
	info, ok := integrator(strings.TrimSpace(integratorID))
	if !ok {
		return ConnectionView{}, ErrUnknownIntegrator
	}
	switch environment {
	case "":
		environment = "production"
	case "production", "test":
	default:
		return ConnectionView{}, fmt.Errorf("%w: environment", ErrBadCredentials)
	}
	system := SourcePrefix + info.ID
	if _, err := c.q.GetSourceConnectionBySystem(ctx, db.GetSourceConnectionBySystemParams{TaxpayerID: taxpayerID, SourceSystem: system}); err == nil {
		return ConnectionView{}, ErrDuplicate
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ConnectionView{}, err
	}
	meta, _ := json.Marshal(connMeta{Environment: environment})
	state := "draft"
	if info.IPFiltered {
		state = "credentials_entered"
	}
	row, err := c.q.CreateSourceConnection(ctx, db.CreateSourceConnectionParams{
		TaxpayerID: taxpayerID, SourceSystem: system, SetupState: state, Meta: meta,
	})
	if err != nil {
		return ConnectionView{}, err
	}
	if info.IPFiltered && c.enqueue != nil {
		if err := c.enqueue(ctx, taxpayerID, row.Uuid, RunBackfill); err != nil {
			return ConnectionView{}, err
		}
	}
	return c.view(ctx, row)
}

// SetCredentials stores the web-service user in the vault, clears the last
// error and queues the first sync (backfill). The values are never written
// to the connection row or returned.
func (c *Connections) SetCredentials(ctx context.Context, taxpayerID int64, id uuid.UUID, values map[string]string) (ConnectionView, error) {
	if c.vault == nil {
		return ConnectionView{}, ErrNoVault
	}
	row, err := c.load(ctx, taxpayerID, id)
	if err != nil {
		return ConnectionView{}, err
	}
	name, _ := ProviderName(row.SourceSystem)
	info, ok := integrator(name)
	if !ok {
		return ConnectionView{}, ErrUnknownIntegrator
	}
	var m connMeta
	_ = json.Unmarshal(row.Meta, &m)
	creds, err := buildCredentials(info, values, m.Environment == "test")
	if err != nil {
		return ConnectionView{}, err
	}
	plain, _ := json.Marshal(creds)
	if row.CredentialsRef.Valid {
		if err := c.vault.Replace(ctx, CredentialsPurpose, secrets.Ref(row.CredentialsRef.String), secrets.NewSecret(plain)); err != nil {
			return ConnectionView{}, err
		}
		if _, err := c.q.SetSourceConnectionCredentialsRef(ctx, db.SetSourceConnectionCredentialsRefParams{
			CredentialsRef: row.CredentialsRef, SetupState: "credentials_entered", TaxpayerID: taxpayerID, ID: row.ID,
		}); err != nil {
			return ConnectionView{}, err
		}
	} else {
		ref, err := c.vault.Put(ctx, CredentialsPurpose, secrets.NewSecret(plain))
		if err != nil {
			return ConnectionView{}, err
		}
		if _, err := c.q.SetSourceConnectionCredentialsRef(ctx, db.SetSourceConnectionCredentialsRefParams{
			CredentialsRef: pgtype.Text{String: string(ref), Valid: true}, SetupState: "credentials_entered",
			TaxpayerID: taxpayerID, ID: row.ID,
		}); err != nil {
			return ConnectionView{}, err
		}
	}
	// New credentials: the old failure no longer applies.
	if _, err := c.q.SetSourceConnectionStatus(ctx, db.SetSourceConnectionStatusParams{
		TaxpayerID: taxpayerID, ID: row.ID, Status: "pending",
	}); err != nil {
		return ConnectionView{}, err
	}
	if c.enqueue != nil {
		if err := c.enqueue(ctx, taxpayerID, row.Uuid, RunBackfill); err != nil {
			return ConnectionView{}, err
		}
	}
	return c.Get(ctx, taxpayerID, id)
}

// buildCredentials checks the form against the integrator's fields.
func buildCredentials(info IntegratorInfo, values map[string]string, test bool) (Credentials, error) {
	cr := Credentials{Test: test}
	for _, f := range info.Fields {
		v := strings.TrimSpace(values[f.Key])
		if v == "" {
			if f.Required {
				return Credentials{}, fmt.Errorf("%w: %s", ErrBadCredentials, f.Key)
			}
			continue
		}
		if f.Option {
			if cr.Options == nil {
				cr.Options = map[string]string{}
			}
			cr.Options[f.Key] = v
			continue
		}
		switch f.Key {
		case "username":
			cr.Username = v
		case "password":
			cr.Password = v
		case "api_key":
			cr.APIKey = v
		}
	}
	return cr, nil
}

// Sync queues a manual sync ("şimdi çek"). The worker skips it when the
// integrator's interval or ManualInterval has not passed (RunTask).
func (c *Connections) Sync(ctx context.Context, taxpayerID int64, id uuid.UUID) (ConnectionView, error) {
	if c.enqueue == nil {
		return ConnectionView{}, ErrNoQueue
	}
	row, err := c.load(ctx, taxpayerID, id)
	if err != nil {
		return ConnectionView{}, err
	}
	name, _ := ProviderName(row.SourceSystem)
	info, _ := integrator(name)
	if !row.CredentialsRef.Valid && !info.IPFiltered {
		return ConnectionView{}, ErrNotReady
	}
	if err := c.enqueue(ctx, taxpayerID, row.Uuid, RunManual); err != nil {
		return ConnectionView{}, err
	}
	return c.view(ctx, row)
}
