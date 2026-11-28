package entegrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
		{ID: "edm", Name: "EDM Bilişim", Fields: []Field{fUser, fPass}},
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
	// http and endpoints serve the connection test (probe.go).
	http      *http.Client
	endpoints map[string]string
	now       func() time.Time
}

// NewConnections builds the setup service. vault may be nil: entering
// credentials answers ErrNoVault. enqueue may be nil (QUEUE_ENABLED=false):
// the connection is set up but no sync is queued, and Sync answers
// ErrNoQueue. The API never calls the integrator itself.
func NewConnections(pool *pgxpool.Pool, vault secrets.Vault, enqueue Enqueuer) *Connections {
	return &Connections{q: database.QueriesFor(pool), vault: vault, enqueue: enqueue, now: time.Now}
}

// WithClock replaces time.Now (tests).
func (c *Connections) WithClock(now func() time.Time) *Connections { c.now = now; return c }

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
	Kind string `json:"kind,omitempty"`
	// Code is the integrator's error code when known (technical, for support).
	Code    string `json:"code,omitempty"`
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
	// Progress of the first load (backfill), while it is queued or the
	// latest run is a backfill (#780).
	Progress *SyncProgress `json:"progress,omitempty"`
	// Schedule: the regular sync (hourly incremental + nightly scan) of a
	// scheduled integrator (İşNet, EDM); nil for the others.
	Schedule  *ScheduleView `json:"schedule,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

// ScheduleView is the regular sync of a connection as the office sees it.
type ScheduleView struct {
	// Active: the connection is synced on its own (set up and not stopped
	// by a credential failure).
	Active bool `json:"active"`
	// IntervalMinutes is the pace of the incremental runs.
	IntervalMinutes int `json:"interval_minutes"`
	// LastSyncAt: the last run that brought documents in (success or
	// partial), when it finished.
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	// NextSyncAt: the next incremental run (after a failure: the back-off).
	NextSyncAt *time.Time `json:"next_sync_at,omitempty"`
	// NextNightlyAt: the next nightly scan of the open months.
	NextNightlyAt *time.Time `json:"next_nightly_at,omitempty"`
	// ConsecutiveFailures of the latest runs (0 after a success).
	ConsecutiveFailures int `json:"consecutive_failures"`
	// LastError: the last failure while ConsecutiveFailures > 0 (or the
	// connection is stopped); Message is a fixed Turkish text, never the
	// integrator's raw answer.
	LastError *ScheduleError `json:"last_error,omitempty"`
}

// ScheduleError is a sync failure without secrets: the error kind and a
// fixed Turkish text.
type ScheduleError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// SyncProgress is the live progress of a backfill (#780): the last two
// months come first (listed and turned into invoices), then the older
// months, newest first.
type SyncProgress struct {
	// Phase: queued (the worker has not started), recent (the last two
	// months are being fetched), history (the last two months are in, older
	// months are being listed), done.
	Phase string `json:"phase"`
	// RecentReady: the invoices of the last two months are in the list.
	RecentReady bool `json:"recent_ready"`
	// ThisMonthDocuments: documents of the current month seen at the
	// integrator; ThisMonthInvoices of them are already invoices.
	ThisMonthDocuments int `json:"this_month_documents"`
	ThisMonthInvoices  int `json:"this_month_invoices"`
	// HistoryMonthsDone of HistoryMonthsTotal older months are listed.
	HistoryMonthsDone  int `json:"history_months_done"`
	HistoryMonthsTotal int `json:"history_months_total"`
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
	if Scheduled(row.SourceSystem) {
		sv, err := c.schedule(ctx, row, info.IPFiltered)
		if err != nil {
			return ConnectionView{}, err
		}
		v.Schedule = sv
	}
	switch {
	case v.LastRun == nil && v.State == "waiting":
		v.Progress = &SyncProgress{Phase: "queued", HistoryMonthsTotal: historyMonths(c.clock())}
	case v.LastRun != nil && v.LastRun.Kind == RunBackfill && v.LastRun.Status != "failed":
		// A failed first load shows its error instead of a progress.
		p, err := c.progress(ctx, row, v.LastRun.FinishedAt != nil)
		if err != nil {
			return ConnectionView{}, err
		}
		v.Progress = p
	}
	return v, nil
}

func (c *Connections) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

// schedule builds the regular-sync view of a scheduled connection.
func (c *Connections) schedule(ctx context.Context, row db.SourceConnection, ipFiltered bool) (*ScheduleView, error) {
	now := c.clock()
	sv := &ScheduleView{
		Active:              row.SetupState != "draft" && (row.Status == "active" || row.Status == "rate_limited"),
		IntervalMinutes:     int(SyncInterval / time.Minute),
		ConsecutiveFailures: int(row.SyncFailures),
	}
	last, err := c.q.LastSuccessfulSyncRunFinishedAt(ctx, db.LastSuccessfulSyncRunFinishedAtParams{TaxpayerID: row.TaxpayerID, ConnectionID: row.ID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if last.Valid {
		t := last.Time
		sv.LastSyncAt = &t
	}
	if sv.Active {
		next := now
		if row.NextSyncAt.Valid {
			next = row.NextSyncAt.Time
		}
		n := NextNightly(now)
		sv.NextSyncAt, sv.NextNightlyAt = &next, &n
	}
	if row.SyncFailures > 0 || !sv.Active {
		if e := errorView(row.LastError); e != nil && len(row.LastError) > 0 {
			sv.LastError = &ScheduleError{Kind: e.Kind, Message: FailureText(e.Kind, ipFiltered)}
		}
	}
	return sv, nil
}

// FailureText is the fixed Turkish text of a sync failure kind (go-ubltr
// provider.ErrorKind). The integrator's own message is never shown: it may
// carry addresses, ids or parts of the request.
func FailureText(kind string, ipFiltered bool) string {
	switch kind {
	case "auth":
		return "Entegratör kullanıcı adını veya şifreyi kabul etmedi."
	case "permission":
		if ipFiltered {
			return "Entegratör yanıt vermedi: sunucumuzun IP adresi tanımlı olmayabilir ya da bağlantı geçici olarak kesildi."
		}
		return "Web servis kullanıcısının bu işlem için yetkisi yok."
	case "timeout":
		return "Entegratör zamanında yanıt vermedi; bir sonraki çekimde yeniden denenecek."
	case "unavailable", "transport":
		return "Entegratöre ulaşılamadı; bir sonraki çekimde yeniden denenecek."
	case "rate-limit":
		return "Entegratör çok sık istek nedeniyle bekletti; bir sonraki çekimde yeniden denenecek."
	}
	return "Çekim tamamlanamadı; bir sonraki çekimde yeniden denenecek."
}

// historyMonths is the number of backfill months before the last two.
func historyMonths(now time.Time) int {
	return max(len(months(backfillStart(now), now))-RecentMonths, 0)
}

// progress reads the backfill progress from sync_windows and
// entegrator_documents.
func (c *Connections) progress(ctx context.Context, row db.SourceConnection, finished bool) (*SyncProgress, error) {
	now := c.clock()
	rows, err := c.q.SyncWindowMonths(ctx, db.SyncWindowMonthsParams{
		TaxpayerID: row.TaxpayerID, ConnectionID: row.ID, Since: ts(backfillStart(now)),
	})
	if err != nil {
		return nil, err
	}
	local := now.In(istanbul)
	docs, err := c.q.EntegratorDocumentProgress(ctx, db.EntegratorDocumentProgressParams{
		TaxpayerID: row.TaxpayerID, ConnectionID: row.ID, Since: ts(recentStart(now)),
		MonthStart:  ts(time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul)),
		MaxAttempts: defaultUBLAttempts,
	})
	if err != nil {
		return nil, err
	}
	p := ProgressFrom(now, rows, int(docs.UblPending))
	p.ThisMonthDocuments = int(docs.ThisMonth)
	p.ThisMonthInvoices = int(docs.ThisMonthInvoices)
	if finished {
		p.Phase = "done"
		p.RecentReady = true
	}
	return p, nil
}

// defaultUBLAttempts mirrors Config.MaxUBLAttempts' default.
const defaultUBLAttempts = 5

// ProgressFrom derives the backfill phase from the per-month window counts
// (SyncWindowMonths) and the number of recent documents still waiting for
// their UBL. The last two months are ready when every one of their windows
// is settled and no recent UBL is pending.
func ProgressFrom(now time.Time, rows []db.SyncWindowMonthsRow, ublPending int) *SyncProgress {
	local := now.In(istanbul)
	recent := map[string]bool{}
	for i := range RecentMonths {
		recent[time.Date(local.Year(), local.Month()-time.Month(i), 1, 0, 0, 0, 0, istanbul).Format("2006-01")] = false
	}
	p := &SyncProgress{Phase: "recent", HistoryMonthsTotal: historyMonths(now)}
	for _, r := range rows {
		settled := r.Total > 0 && r.Settled >= r.Total
		if _, ok := recent[r.Month]; ok {
			recent[r.Month] = settled
			continue
		}
		if settled {
			p.HistoryMonthsDone++
		}
	}
	p.HistoryMonthsDone = min(p.HistoryMonthsDone, p.HistoryMonthsTotal)
	p.RecentReady = ublPending == 0
	for _, ok := range recent {
		p.RecentReady = p.RecentReady && ok
	}
	if p.RecentReady {
		p.Phase = "history"
	}
	return p
}

// errorView reads errorJSON's shape ({"kind", "message"}) or a plain message.
func errorView(raw []byte) *ConnectionError {
	var e struct {
		Kind    string `json:"kind"`
		Code    string `json:"code"`
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
	return &ConnectionError{Kind: e.Kind, Code: e.Code, Message: msg}
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
