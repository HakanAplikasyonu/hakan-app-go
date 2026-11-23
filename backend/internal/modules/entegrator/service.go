package entegrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/secrets"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Run kinds (sync_runs.kind, 30 §7).
const (
	RunBackfill    = "backfill"    // current + previous year, monthly windows, newest first; last two months (listed + UBL) before the rest
	RunIncremental = "incremental" // last 3 days of creation time
	RunFullScan    = "full_scan"   // current + previous month; marks missing documents
	RunManual      = "manual"      // "şimdi çek": like incremental, at most every 5 minutes
)

// CredentialsPurpose binds source connection credentials in the vault.
const CredentialsPurpose = "source_connection"

// Producer is the data_warning producer name of this channel (`05` §1).
const Producer = "entegrator_sync"

var (
	ErrNotEntegrator  = errors.New("entegrator: connection is not an integrator connection")
	ErrNoCredentials  = errors.New("entegrator: connection has no credentials")
	ErrBusy           = errors.New("entegrator: a sync is already running for this connection")
	ErrTooSoon        = errors.New("entegrator: polled too soon for this integrator")
	ErrUnknownRunKind = errors.New("entegrator: unknown run kind")
)

// Credentials is the vault payload of an integrator connection. It never
// lives in source_connections (credentials_ref points to it).
type Credentials struct {
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	APIKey   string            `json:"api_key,omitempty"`
	Endpoint string            `json:"endpoint,omitempty"`
	Test     bool              `json:"test,omitempty"`
	Options  map[string]string `json:"options,omitempty"`
}

// Opener opens the read-only client of a connection. The default opener
// reads the credentials from the vault and calls provider.Open.
type Opener func(ctx context.Context, conn db.SourceConnection, ownTaxID string) (*Client, error)

// ObjectStore is the storage.Driver subset used for UBL attachments.
type ObjectStore interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Exists(ctx context.Context, path string) (bool, error)
}

// Config tunes the sync. Zero values select the defaults.
type Config struct {
	// PageSize is the listing page-size hint (default 100).
	PageSize int
	// MaxPagesPerWindow: a window still paging after this many pages is
	// split in two (37 §5.3), down to MinWindow (default 10).
	MaxPagesPerWindow int
	// MinWindow is the narrowest window; it pages to the end (default 24h).
	MinWindow time.Duration
	// RequestGap is the pause between integrator calls of one connection
	// (30 §7: 1 request / second by default). Tests set a negative value
	// for no pause.
	RequestGap time.Duration
	// MaxWindowAttempts: a failed window is retried by later runs up to
	// this many times (default 5).
	MaxWindowAttempts int
	// MaxUBLAttempts: a failed UBL download is retried up to this many
	// times (default 5, 37 §5.5).
	MaxUBLAttempts int
	// IncrementalLookback is the incremental window (default 72h).
	IncrementalLookback time.Duration
	// ManualInterval is the floor between manual runs (default 5 minutes).
	ManualInterval time.Duration
	// CallTimeout bounds one integrator call (default 2 minutes). A call
	// that hangs fails its window or document instead of the whole run.
	CallTimeout time.Duration
	// HTTP is the transport for integrator calls (issue #45). The
	// fixed-egress worker sets it to the client that leaves from the
	// allowlisted IP (internal/platform/egress); nil uses go-ubltr's default.
	HTTP *http.Client
}

func (c Config) withDefaults() Config {
	if c.PageSize <= 0 {
		c.PageSize = 100
	}
	if c.MaxPagesPerWindow <= 0 {
		c.MaxPagesPerWindow = 10
	}
	if c.MinWindow <= 0 {
		c.MinWindow = 24 * time.Hour
	}
	if c.RequestGap == 0 {
		c.RequestGap = time.Second
	}
	if c.MaxWindowAttempts <= 0 {
		c.MaxWindowAttempts = 5
	}
	if c.MaxUBLAttempts <= 0 {
		c.MaxUBLAttempts = 5
	}
	if c.IncrementalLookback <= 0 {
		c.IncrementalLookback = 72 * time.Hour
	}
	if c.ManualInterval <= 0 {
		c.ManualInterval = 5 * time.Minute
	}
	if c.CallTimeout <= 0 {
		c.CallTimeout = 2 * time.Minute
	}
	return c
}

// Service runs integrator syncs.
type Service struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	vault secrets.Vault
	store ObjectStore
	open  Opener
	cfg   Config
	now   func() time.Time
	log   *slog.Logger
}

// New builds the service. vault may be nil when WithOpener supplies clients.
func New(pool *pgxpool.Pool, vault secrets.Vault, store ObjectStore, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &Service{pool: pool, q: database.QueriesFor(pool), vault: vault, store: store, cfg: cfg.withDefaults(), now: time.Now, log: log}
	s.open = s.vaultOpener
	return s
}

// WithOpener replaces how clients are opened (tests, fakes).
func (s *Service) WithOpener(o Opener) *Service { s.open = o; return s }

// WithClock replaces time.Now (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

func (s *Service) vaultOpener(ctx context.Context, conn db.SourceConnection, ownTaxID string) (*Client, error) {
	name, ok := ProviderName(conn.SourceSystem)
	if !ok {
		return nil, ErrNotEntegrator
	}
	cr, err := connCredentials(ctx, s.vault, conn)
	if err != nil {
		return nil, err
	}
	return Open(name, provider.Config{
		Username: cr.Username, Password: cr.Password, APIKey: cr.APIKey,
		Endpoint: cr.Endpoint, Test: cr.Test, VKN: ownTaxID, Options: cr.Options,
		HTTP: auditDoer{next: s.transport(), log: s.log, conn: conn.Uuid, system: conn.SourceSystem, vkn: ownTaxID},
	})
}

// connCredentials reads a connection's credentials from the vault. An
// IP-filtered integrator (İşNet) has no stored user: it opens with empty
// credentials and the environment chosen at setup (meta.environment), so
// its syncs no longer stop at ErrNoCredentials.
func connCredentials(ctx context.Context, vault secrets.Vault, conn db.SourceConnection) (Credentials, error) {
	name, _ := ProviderName(conn.SourceSystem)
	if !conn.CredentialsRef.Valid {
		if info, ok := integrator(name); ok && info.IPFiltered {
			var m connMeta
			_ = json.Unmarshal(conn.Meta, &m)
			return Credentials{Test: m.Environment == "test"}, nil
		}
		return Credentials{}, ErrNoCredentials
	}
	if vault == nil {
		return Credentials{}, ErrNoCredentials
	}
	ref, err := secrets.ParseRef(conn.CredentialsRef.String)
	if err != nil {
		return Credentials{}, err
	}
	var cr Credentials
	if err := secrets.GetJSON(ctx, vault, CredentialsPurpose, ref, &cr); err != nil {
		return Credentials{}, err
	}
	return cr, nil
}

// transport is the HTTP client under the audit log: the fixed-egress client
// when configured (issue #45), otherwise the default.
func (s *Service) transport() *http.Client {
	if s.cfg.HTTP != nil {
		return s.cfg.HTTP
	}
	return auditHTTP
}

// Counters are stored on sync_runs.counters.
type Counters struct {
	Windows       int `json:"windows"`
	WindowsFailed int `json:"windows_failed"`
	Splits        int `json:"splits"`
	LimitReached  int `json:"limit_reached"`
	Pages         int `json:"pages"`
	Listed        int `json:"listed"`
	New           int `json:"new"`
	StatusChanged int `json:"status_changed"`
	Skipped       int `json:"skipped"`
	UBLFetched    int `json:"ubl_fetched"`
	UBLFailed     int `json:"ubl_failed"`
	UBLMissing    int `json:"ubl_not_available"`
	Missing       int `json:"missing"`
}

// Report is the result of one Run.
type Report struct {
	RunID    uuid.UUID
	Kind     string
	Status   string // success, partial, failed
	Counters Counters
}

// Run performs one sync of the connection: plans the windows of kind, lists
// every pending window into entegrator_documents, then works the UBL queue.
// Only one run per connection at a time (37 §5.6).
func (s *Service) Run(ctx context.Context, taxpayerID int64, connUUID uuid.UUID, kind string) (Report, error) {
	switch kind {
	case RunBackfill, RunIncremental, RunFullScan, RunManual:
	default:
		return Report{}, ErrUnknownRunKind
	}
	conn, err := s.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: taxpayerID, Uuid: connUUID})
	if err != nil {
		return Report{}, err
	}
	if _, ok := ProviderName(conn.SourceSystem); !ok {
		return Report{}, ErrNotEntegrator
	}
	unlock, err := s.lock(ctx, conn.ID)
	if err != nil {
		return Report{}, err
	}
	defer unlock()

	ownTaxID, err := s.q.GetTaxpayerOwnTaxID(ctx, taxpayerID)
	if err != nil {
		return Report{}, err
	}
	c, err := s.open(ctx, conn, ownTaxID)
	if err != nil {
		return Report{}, err
	}
	defer c.Close(ctx)
	if err := s.checkInterval(ctx, conn, c, kind); err != nil {
		return Report{}, err
	}
	_ = s.saveCapabilities(ctx, conn, c)

	// Database clock: last_seen_at is written with NOW().
	var seenFrom time.Time
	if err := s.pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&seenFrom); err != nil {
		return Report{}, err
	}
	run, err := s.q.StartSyncRun(ctx, db.StartSyncRunParams{TaxpayerID: taxpayerID, ConnectionID: conn.ID, Kind: kind})
	if err != nil {
		return Report{}, err
	}
	rep := Report{RunID: run.Uuid, Kind: kind}
	j := &job{s: s, c: c, conn: conn, ownTaxID: ownTaxID, cnt: &rep.Counters}
	var runErr error
	if kind == RunBackfill {
		// Fast first step (#780): the last two months are listed and their
		// UBLs fetched first, so the invoice list fills while the older
		// history is still being listed.
		now := s.now()
		runErr = j.planWindows(ctx, s.plan(c, kind, now))
		if runErr == nil {
			runErr = j.listResumable(ctx, recentStart(now))
		}
		if runErr == nil {
			runErr = j.ublQueue(ctx)
		}
		if runErr == nil {
			runErr = j.listResumable(ctx, time.Time{})
		}
	} else {
		runErr = j.listWindows(ctx, s.plan(c, kind, s.now()))
	}
	if runErr == nil && kind == RunFullScan {
		runErr = j.markMissing(ctx, seenFrom)
	}
	if runErr == nil {
		runErr = j.ublQueue(ctx)
	}
	rep.Status = "success"
	switch {
	case runErr != nil:
		rep.Status = "failed"
	case rep.Counters.WindowsFailed > 0 || rep.Counters.UBLFailed > 0 || rep.Counters.LimitReached > 0:
		rep.Status = "partial"
	}
	s.finish(ctx, conn, run.ID, rep, runErr)
	return rep, runErr
}

func (s *Service) finish(ctx context.Context, conn db.SourceConnection, runID int64, rep Report, runErr error) {
	counters, _ := json.Marshal(rep.Counters)
	var errJSON []byte
	if runErr != nil {
		errJSON = errorJSON(runErr)
	}
	// The run is closed even when ctx was cancelled.
	ctx = context.WithoutCancel(ctx)
	if _, err := s.q.FinishSyncRun(ctx, db.FinishSyncRunParams{TaxpayerID: conn.TaxpayerID, ID: runID, Status: rep.Status, Counters: counters, Error: errJSON}); err != nil {
		s.log.Error("entegrator: finish run", "err", err)
	}
	status := "active"
	switch kindOf(runErr) {
	case provider.KindAuth, provider.KindPermission:
		status = "auth_failed"
	case provider.KindRateLimit:
		status = "rate_limited"
	default:
		if runErr != nil {
			return // a transient failure does not change the connection
		}
	}
	if _, err := s.q.SetSourceConnectionStatus(ctx, db.SetSourceConnectionStatusParams{TaxpayerID: conn.TaxpayerID, ID: conn.ID, Status: status, LastError: errJSON}); err != nil {
		s.log.Error("entegrator: connection status", "err", err)
	}
}

// lockKey namespaces the advisory lock of a connection.
const lockKey = 0x45474e54 // "EGNT"

// lock takes a session advisory lock on a dedicated pool connection.
func (s *Service) lock(ctx context.Context, connID int64) (func(), error) {
	pc, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var ok bool
	if err := pc.QueryRow(ctx, "SELECT pg_try_advisory_lock($1::int, $2::int)", lockKey, int32(connID)).Scan(&ok); err != nil { //nolint:gosec // lock key half: an id wraps mod 2^32, a collision only serialises two syncs
		pc.Release()
		return nil, err
	}
	if !ok {
		pc.Release()
		return nil, ErrBusy
	}
	return func() {
		_, _ = pc.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1::int, $2::int)", lockKey, int32(connID)) //nolint:gosec // same key as the lock above
		pc.Release()
	}, nil
}

// checkInterval enforces Capabilities.MinPollInterval for polling runs and
// the manual floor.
func (s *Service) checkInterval(ctx context.Context, conn db.SourceConnection, c *Client, kind string) error {
	min := c.Capabilities().MinPollInterval
	switch kind {
	case RunManual:
		min = max(min, s.cfg.ManualInterval)
	case RunIncremental:
	default:
		return nil
	}
	if min <= 0 {
		return nil
	}
	last, err := s.q.LastSyncRunStartedAt(ctx, db.LastSyncRunStartedAtParams{TaxpayerID: conn.TaxpayerID, ConnectionID: conn.ID, Kind: kind})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if last.Valid && s.now().Sub(last.Time) < min {
		return ErrTooSoon
	}
	return nil
}

// saveCapabilities records what the connection can deliver (30 §6 verify).
func (s *Service) saveCapabilities(ctx context.Context, conn db.SourceConnection, c *Client) error {
	caps := c.Capabilities()
	out := map[string]any{"provider": c.Name()}
	for _, k := range []provider.DocumentKind{provider.EInvoice, provider.EArchive} {
		var ops []string
		for _, op := range []provider.Operation{provider.OpListInbox, provider.OpListOutbox, provider.OpFetch, provider.OpStatus} {
			if c.Supports(k, op) {
				ops = append(ops, string(op))
			}
		}
		if len(ops) > 0 {
			out[docKind(k)] = ops
		}
	}
	// Boxes the integrator cannot list are named, so the office sees "not
	// supported" instead of an empty list (#59).
	if nl := NotListable(c); len(nl) > 0 {
		out["not_listable"] = nl
	}
	if caps.MinPollInterval > 0 {
		out["min_poll_interval_seconds"] = int(caps.MinPollInterval.Seconds())
	}
	b, _ := json.Marshal(out)
	_, err := s.q.SetSourceConnectionCapabilities(ctx, db.SetSourceConnectionCapabilitiesParams{TaxpayerID: conn.TaxpayerID, ID: conn.ID, Capabilities: b})
	return err
}

// NotListable names the "<doc_kind>:<direction>" boxes c cannot list.
func NotListable(c *Client) []string {
	var out []string
	for _, k := range []provider.DocumentKind{provider.EInvoice, provider.EArchive} {
		for _, d := range []provider.Direction{provider.Inbound, provider.Outbound} {
			if k == provider.EArchive && d == provider.Inbound {
				continue // e-Arşiv has no inbox
			}
			op := provider.OpListInbox
			if d == provider.Outbound {
				op = provider.OpListOutbox
			}
			if !c.Supports(k, op) {
				out = append(out, docKind(k)+":"+direction(d))
			}
		}
	}
	return out
}

func kindOf(err error) provider.ErrorKind {
	var pe *provider.Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	return ""
}

// abortsRun: errors after which no further call of this run can succeed.
// A single call that ran past CallTimeout does not end the run; the run's
// own context being done does.
func abortsRun(ctx context.Context, err error) bool {
	switch kindOf(err) {
	case provider.KindAuth, provider.KindPermission, provider.KindRateLimit:
		return true
	}
	return ctx.Err() != nil
}

func errorJSON(err error) []byte {
	m := map[string]any{"message": err.Error()}
	var pe *provider.Error
	if errors.As(err, &pe) {
		m["kind"] = string(pe.Kind)
		if pe.Code != "" {
			m["code"] = pe.Code
		}
	}
	b, _ := json.Marshal(m)
	return b
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func docKind(k provider.DocumentKind) string {
	switch k {
	case provider.EInvoice:
		return "e_invoice"
	case provider.EArchive:
		return "e_archive"
	}
	return ""
}

func providerKind(s string) (provider.DocumentKind, error) {
	switch s {
	case "e_invoice":
		return provider.EInvoice, nil
	case "e_archive":
		return provider.EArchive, nil
	}
	return "", fmt.Errorf("entegrator: unknown document kind %q", s)
}

func direction(d provider.Direction) string {
	if d == provider.Inbound {
		return "incoming"
	}
	return "outgoing"
}

func providerDirection(s string) provider.Direction {
	if s == "incoming" {
		return provider.Inbound
	}
	return provider.Outbound
}

// RunTask is the queue handler (queue.TaskEntegratorSync). A run that finds
// the connection busy or polled too recently is not an error: the next
// scheduled task picks the work up.
func (s *Service) RunTask(ctx context.Context, taxpayerID int64, connUUID uuid.UUID, kind string) error {
	_, err := s.Run(ctx, taxpayerID, connUUID, kind)
	if errors.Is(err, ErrBusy) || errors.Is(err, ErrTooSoon) {
		s.log.Info("entegrator: sync skipped", "connection", connUUID, "kind", kind, "reason", err)
		return nil
	}
	return err
}
