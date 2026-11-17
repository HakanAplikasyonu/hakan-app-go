package easytrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/sources"
)

// Run kinds (sync_runs.kind).
const (
	RunScheduled = "incremental"
	RunManual    = "manual"
)

// Schedule bounds (issue #385). The default is 09 §10's four hours for
// invoices, retail sales, financial transactions and expenses.
const (
	DefaultSyncInterval = 4 * time.Hour
	MinSyncInterval     = 15 * time.Minute
	MaxSyncInterval     = 7 * 24 * time.Hour
	// RetryBase is the first retry after a failed scheduled run; it doubles
	// with each failure and never passes the connection's interval.
	RetryBase = 5 * time.Minute
	// DueBatch caps the connections one sweep enqueues.
	DueBatch = 200
)

var (
	// ErrBusy: a sync of the connection is already running.
	ErrBusy = errors.New("easytrade: a sync is already running for this connection")
	// ErrBadInterval: the interval is outside [MinSyncInterval, MaxSyncInterval].
	ErrBadInterval = errors.New("easytrade: sync interval out of range")
)

// istanbul draws period (month) boundaries.
var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// Counters are a run's sync_runs.counters: the import report plus windows.
type Counters struct {
	sources.Report
	WindowsDone   int `json:"windows_done"`
	WindowsFailed int `json:"windows_failed"`
	// Unsupported items: Easytrade has no endpoint for them (yet).
	Unsupported []string `json:"unsupported,omitempty"`
}

// RunResult is one finished run.
type RunResult struct {
	RunID    uuid.UUID `json:"run_id"`
	Kind     string    `json:"kind"`
	Status   string    `json:"status"`
	Counters Counters  `json:"counters"`
}

// lockKey namespaces the advisory lock of a connection ("EASY").
const lockKey = 0x45415359

// lock takes a session advisory lock on a dedicated pool connection, so a
// scheduled and a manual run of one connection never overlap.
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

// span is one sync window: [From, To) in whole days.
type span struct{ From, To time.Time }

// windows cuts the inclusive day range [from, to] at month starts.
func windows(from, to time.Time) []span {
	from = day(from)
	end := day(to).AddDate(0, 0, 1)
	var out []span
	for start := from; start.Before(end); {
		next := time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		if next.After(end) {
			next = end
		}
		out = append(out, span{start, next})
		start = next
	}
	return out
}

func day(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }

// openPeriods is what a scheduled run reads: the previous and the current
// month (Istanbul calendar) up to today. Older periods are closed; they are
// read again only by a manual sync.
func openPeriods(now time.Time) (time.Time, time.Time) {
	l := now.In(istanbul)
	today := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
	return time.Date(l.Year(), l.Month()-1, 1, 0, 0, 0, 0, time.UTC), today
}

// fatal errors stop the whole run: every further window would fail alike.
func fatal(err error) bool {
	return errors.Is(err, ErrAuth) || errors.Is(err, ErrLicense) || errors.Is(err, ErrNoBranch) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func errJSON(err error) []byte {
	b, _ := json.Marshal(map[string]string{"message": err.Error()})
	return b
}

// run reads [from, to] month window by month window and item by item,
// recording sync_windows and one sync_runs row. A failed window does not
// stop the others (status partial); a fatal error stops the run.
func (s *Service) run(ctx context.Context, row db.SourceConnection, c *Client, kind string, from, to time.Time) (RunResult, error) {
	unlock, err := s.lock(ctx, row.ID)
	if err != nil {
		return RunResult{}, err
	}
	defer unlock()
	run, err := s.q.StartSyncRun(ctx, db.StartSyncRunParams{TaxpayerID: row.TaxpayerID, ConnectionID: row.ID, Kind: kind})
	if err != nil {
		return RunResult{}, err
	}
	res := RunResult{RunID: run.Uuid, Kind: kind, Counters: Counters{Report: sources.Report{Excluded: sources.Excluded{}}}}
	cnt := &res.Counters
	var runErr error
	branches, err := c.ActiveBranches(ctx)
	if err != nil {
		runErr = err
	}
windows:
	for _, w := range windows(from, to) {
		if runErr != nil {
			break
		}
		for _, item := range Items {
			win, err := s.q.UpsertSyncWindow(ctx, db.UpsertSyncWindowParams{
				TaxpayerID: row.TaxpayerID, ConnectionID: row.ID, Item: item, DateField: DateField(item),
				RangeStart: pgtype.Timestamptz{Time: w.From, Valid: true}, RangeEnd: pgtype.Timestamptz{Time: w.To, Valid: true},
			})
			if err != nil {
				runErr = err
				break windows
			}
			_, _ = s.q.SetSyncWindowStatus(ctx, db.SetSyncWindowStatusParams{Status: "running", TaxpayerID: row.TaxpayerID, ID: win.ID})
			b, err := c.ReadItem(ctx, item, branches, w.From, w.To.AddDate(0, 0, -1))
			if errors.Is(err, ErrNotSupported) {
				if !slices.Contains(cnt.Unsupported, item) {
					cnt.Unsupported = append(cnt.Unsupported, item)
				}
				err = nil
			}
			if err == nil {
				var rep sources.Report
				rep, err = sources.Write(ctx, s.pool, row.TaxpayerID, SourceSystem, b)
				cnt.Add(rep)
			}
			if err != nil {
				cnt.WindowsFailed++
				_, _ = s.q.SetSyncWindowStatus(context.WithoutCancel(ctx), db.SetSyncWindowStatusParams{
					Status: "failed", LastError: errJSON(err), TaxpayerID: row.TaxpayerID, ID: win.ID,
				})
				if fatal(err) {
					runErr = err
					break windows
				}
				continue
			}
			cnt.WindowsDone++
			_, _ = s.q.SetSyncWindowStatus(ctx, db.SetSyncWindowStatusParams{Status: "done", TaxpayerID: row.TaxpayerID, ID: win.ID})
		}
	}
	res.Status = "success"
	switch {
	case runErr != nil || (cnt.WindowsDone == 0 && cnt.WindowsFailed > 0):
		res.Status = "failed"
		if runErr == nil {
			runErr = errAllFailed
		}
	case cnt.WindowsFailed > 0:
		res.Status = "partial"
	}
	s.finish(ctx, row, run.ID, res, runErr)
	if res.Status == "failed" {
		return res, runErr
	}
	return res, nil
}

// finish closes the run and records the last sync on the connection.
func (s *Service) finish(ctx context.Context, row db.SourceConnection, runID int64, res RunResult, runErr error) {
	ctx = context.WithoutCancel(ctx)
	counters, _ := json.Marshal(res.Counters)
	var e []byte
	if runErr != nil {
		e = errJSON(runErr)
	}
	_, _ = s.q.FinishSyncRun(ctx, db.FinishSyncRunParams{TaxpayerID: row.TaxpayerID, ID: runID, Status: res.Status, Counters: counters, Error: e})
	if runErr != nil {
		s.fail(ctx, row, runErr)
	}
	if res.Status == "failed" {
		return
	}
	rep := res.Counters.Report
	p, _ := json.Marshal(map[string]any{"last_sync_at": s.now(), "last_report": rep})
	_, _ = s.q.SetSourceConnectionState(ctx, db.SetSourceConnectionStateParams{
		SetupState: "active", Status: "active", MetaPatch: p, MarkSuccess: res.Status == "success", TaxpayerID: row.TaxpayerID, ID: row.ID,
	})
}

// RunScheduled runs one scheduled sync of the open periods and moves the
// connection's next_sync_at: the interval after a success, an exponential
// retry (RetryBase, 2×, 4× … at most the interval) after a failure. Auth and
// license failures stop the schedule (the sweep only picks active
// connections) until the connection is set up again.
func (s *Service) RunScheduled(ctx context.Context, taxpayerID int64, id uuid.UUID) (RunResult, error) {
	row, c, err := s.client(ctx, taxpayerID, id)
	if errors.Is(err, ErrBadState) && row.ID != 0 && interval(row) > 0 {
		// Scheduled but without credentials (demo office, a half set-up
		// connection): wait one interval instead of being picked by every
		// sweep; not a source failure, so sync_failures stays.
		_, _ = s.q.SetSourceConnectionSchedule(context.WithoutCancel(ctx), db.SetSourceConnectionScheduleParams{
			SyncIntervalMinutes: row.SyncIntervalMinutes, SyncFailures: row.SyncFailures,
			NextSyncAt: pgtype.Timestamptz{Time: s.now().Add(interval(row)), Valid: true},
			TaxpayerID: row.TaxpayerID, ID: row.ID,
		})
	}
	if err != nil {
		return RunResult{}, err
	}
	if row.Status == "disabled" || row.Status == "auth_failed" {
		return RunResult{}, ErrBadState
	}
	from, to := openPeriods(s.now())
	res, runErr := s.run(ctx, row, c, RunScheduled, from, to)
	if errors.Is(runErr, ErrBusy) {
		return res, runErr
	}
	s.reschedule(ctx, row, res.Status == "success")
	return res, runErr
}

// interval of a connection; zero when it is not scheduled.
func interval(row db.SourceConnection) time.Duration {
	if !row.SyncIntervalMinutes.Valid {
		return 0
	}
	return time.Duration(row.SyncIntervalMinutes.Int32) * time.Minute
}

// nextAfter is the next run after a success (failures == 0) or the
// failures-th consecutive failure.
func nextAfter(now time.Time, every time.Duration, failures int) time.Time {
	if failures <= 0 {
		return now.Add(every)
	}
	d := RetryBase << min(failures-1, 10)
	return now.Add(min(d, every))
}

func (s *Service) reschedule(ctx context.Context, row db.SourceConnection, ok bool) {
	every := interval(row)
	if every <= 0 {
		return
	}
	failures := 0
	if !ok {
		failures = int(row.SyncFailures) + 1
	}
	_, _ = s.q.SetSourceConnectionSchedule(context.WithoutCancel(ctx), db.SetSourceConnectionScheduleParams{
		SyncIntervalMinutes: row.SyncIntervalMinutes,
		NextSyncAt:          pgtype.Timestamptz{Time: nextAfter(s.now(), every, failures), Valid: true},
		SyncFailures:        int32(min(failures, 1<<20)), //nolint:gosec // bounded above
		TaxpayerID:          row.TaxpayerID, ID: row.ID,
	})
}

// SetSchedule turns the schedule on with every (MinSyncInterval …
// MaxSyncInterval) or off (0). Turning it on runs the first sync soon.
func (s *Service) SetSchedule(ctx context.Context, taxpayerID int64, id uuid.UUID, every time.Duration) (Connection, error) {
	row, err := s.row(ctx, taxpayerID, id)
	if err != nil {
		return Connection{}, err
	}
	p := db.SetSourceConnectionScheduleParams{TaxpayerID: taxpayerID, ID: row.ID}
	switch {
	case every == 0:
	case every < MinSyncInterval || every > MaxSyncInterval:
		return Connection{}, ErrBadInterval
	default:
		next := s.now()
		if old := interval(row); old > 0 && row.NextSyncAt.Valid {
			// Already scheduled: keep the cadence, never later than the new interval.
			next = row.NextSyncAt.Time
			if lim := s.now().Add(every); next.After(lim) {
				next = lim
			}
		}
		p.SyncIntervalMinutes = pgtype.Int4{Int32: int32(every / time.Minute), Valid: true} //nolint:gosec // bounded by MaxSyncInterval
		p.NextSyncAt = pgtype.Timestamptz{Time: next, Valid: true}
	}
	row, err = s.q.SetSourceConnectionSchedule(ctx, p)
	if err != nil {
		return Connection{}, err
	}
	return s.view(ctx, row), nil
}

// EnqueueDue hands every due connection to enqueue (the worker's sweep,
// run across offices inside rls.System) and returns how many it handed.
func (s *Service) EnqueueDue(ctx context.Context, enqueue func(ctx context.Context, taxpayerID int64, id uuid.UUID) error) (int, error) {
	if s.vault == nil {
		return 0, nil // no vault, no credentials: the feature is off
	}
	rows, err := s.q.ListDueSourceConnections(ctx, db.ListDueSourceConnectionsParams{
		SourceSystem: SourceSystem, Now: pgtype.Timestamptz{Time: s.now(), Valid: true}, Lim: DueBatch,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := enqueue(ctx, r.TaxpayerID, r.Uuid); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// RunTask is the worker's entry for one scheduled sync. Source failures
// are recorded on the run and retried by the schedule (next_sync_at), not
// by the queue; only infrastructure errors (database) fail the task.
func (s *Service) RunTask(ctx context.Context, taxpayerID int64, id uuid.UUID) error {
	_, err := s.RunScheduled(ctx, taxpayerID, id)
	switch {
	case err == nil, errors.Is(err, ErrBusy), errors.Is(err, ErrBadState), errors.Is(err, ErrNotFound),
		errors.Is(err, ErrNoVault), errors.Is(err, errAllFailed), fatal(err), isSource(err):
		return nil
	}
	return err
}

// isSource: Easytrade answered with an error or could not be reached.
func isSource(err error) bool {
	var ue *url.Error
	return errors.Is(err, ErrUpstream) || errors.As(err, &ue)
}

// errAllFailed: no window of the run succeeded.
var errAllFailed = errors.New("easytrade: every window failed")

// Run is one sync_runs row in the API.
type Run struct {
	UUID       uuid.UUID       `json:"uuid"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	Counters   json.RawMessage `json:"counters"`
	Error      string          `json:"error,omitempty"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

func runView(r db.SyncRun) Run {
	out := Run{UUID: r.Uuid, Kind: r.Kind, Status: r.Status, Counters: json.RawMessage(r.Counters), StartedAt: r.StartedAt.Time}
	if len(out.Counters) == 0 {
		out.Counters = json.RawMessage("{}")
	}
	if r.FinishedAt.Valid {
		t := r.FinishedAt.Time
		out.FinishedAt = &t
	}
	var e struct {
		Message string `json:"message"`
	}
	if len(r.Error) > 0 && json.Unmarshal(r.Error, &e) == nil {
		out.Error = e.Message
	}
	return out
}

// Runs lists the connection's last runs, newest first.
func (s *Service) Runs(ctx context.Context, taxpayerID int64, id uuid.UUID, limit int32) ([]Run, error) {
	row, err := s.row(ctx, taxpayerID, id)
	if err != nil {
		return nil, err
	}
	rs, err := s.q.ListSyncRuns(ctx, db.ListSyncRunsParams{TaxpayerID: taxpayerID, ConnectionID: row.ID, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(rs))
	for _, r := range rs {
		out = append(out, runView(r))
	}
	return out, nil
}
