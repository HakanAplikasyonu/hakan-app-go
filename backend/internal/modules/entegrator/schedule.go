package entegrator

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/isnet"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Düzenli senkron (çapraz denetim A1 "Kalan", 30 §7). İlk çekimden sonra
// İşNet ve EDM bağlantıları kendiliğinden çekilir:
//
//   - Artımlı: saatte bir (SyncInterval). Pencere son başarılı çalışmanın
//     başından IncrementalOverlap önce başlar, ama en az son
//     IncrementalLookback'i (30 §7: son 3 gün) kapsar; geç düşen belge kaçmaz.
//   - Gece taraması: her gece (queue: CRON_TZ=Europe/Istanbul) içinde
//     bulunulan ay + önceki ay yeniden listelenir (RunFullScan); kaçan ya da
//     sonradan iptal edilen belge yakalanır.
//
// Zamanlama 000094'ün kolonlarıyla tutulur (Easytrade ile aynı): işçinin
// taraması next_sync_at'i gelmiş bağlantıları kuyruğa atar, çalışma bitince
// next_sync_at ve sync_failures yazılır. Aynı bağlantıda aynı anda tek
// çalışma advisory lock ile (Service.lock), kuyrukta tek iş Unique ile.

// ScheduledIntegrators are the integrators synced on a schedule. Others are
// still pulled only by "şimdi çek" until their behaviour is checked against
// a real account.
var ScheduledIntegrators = []string{"isnet", "edm"}

// Schedule tuning.
const (
	// SyncInterval is the pace of incremental runs (30 §7: 60 dakikada 1).
	SyncInterval = time.Hour
	// IncrementalOverlap: an incremental window starts this long before the
	// last successful run began, so a document created around that moment
	// is listed again (upserts are idempotent).
	IncrementalOverlap = 15 * time.Minute
	// MaxIncrementalSpan caps how far back an incremental run reaches after
	// a long outage; the nightly scan covers the two open months anyway.
	MaxIncrementalSpan = 35 * 24 * time.Hour
	// RetryBase is the first retry after a failed run; it doubles with each
	// consecutive failure up to MaxBackoff.
	RetryBase = 5 * time.Minute
	// MaxBackoff is the longest wait between attempts of a failing
	// connection.
	MaxBackoff = 4 * time.Hour
	// IPNotAllowedAfter: İşNet drops calls from IPs it has not allowlisted,
	// so an unanswered call is the only sign of a missing IP definition, and
	// a one-off network outage looks the same (32 §9). Only this many
	// consecutive such failures turn the connection auth_failed (and stop
	// the schedule); fewer are transient.
	IPNotAllowedAfter = 3
	// DueBatch caps the connections one sweep enqueues.
	DueBatch = 200
	// NightlyHour is the hour (Europe/Istanbul) of the nightly scan; the
	// queue's cron uses the same value.
	NightlyHour = 3
)

// scheduledSystems are the source_system values of ScheduledIntegrators.
func scheduledSystems() []string {
	out := make([]string, 0, len(ScheduledIntegrators))
	for _, id := range ScheduledIntegrators {
		out = append(out, SourcePrefix+id)
	}
	return out
}

// Scheduled reports whether a source system is synced on a schedule.
func Scheduled(sourceSystem string) bool {
	for _, s := range scheduledSystems() {
		if s == sourceSystem {
			return true
		}
	}
	return false
}

// NextNightly is the next nightly scan after now.
func NextNightly(now time.Time) time.Time {
	l := now.In(istanbul)
	next := time.Date(l.Year(), l.Month(), l.Day(), NightlyHour, 0, 0, 0, istanbul)
	if !next.After(now) {
		next = time.Date(l.Year(), l.Month(), l.Day()+1, NightlyHour, 0, 0, 0, istanbul)
	}
	return next
}

// Backoff is the wait after the failures-th consecutive failure:
// RetryBase, 2×, 4× … at most MaxBackoff.
func Backoff(failures int) time.Duration {
	if failures <= 0 {
		return SyncInterval
	}
	d := RetryBase << min(failures-1, 16)
	return min(d, MaxBackoff)
}

// incrementalStart is where an incremental window begins: IncrementalOverlap
// before the last successful run started, at least lookback back from now,
// at most MaxIncrementalSpan back.
func incrementalStart(now, lastSuccess time.Time, lookback time.Duration) time.Time {
	from := now.Add(-lookback)
	if !lastSuccess.IsZero() {
		if s := lastSuccess.Add(-IncrementalOverlap); s.Before(from) {
			from = s
		}
	}
	if floor := now.Add(-MaxIncrementalSpan); from.Before(floor) {
		from = floor
	}
	return from
}

// lastSuccess is when the connection's last successful (or partial) run began.
func (s *Service) lastSuccess(ctx context.Context, conn db.SourceConnection) (time.Time, error) {
	t, err := s.q.LastSuccessfulSyncRunStartedAt(ctx, db.LastSuccessfulSyncRunStartedAtParams{TaxpayerID: conn.TaxpayerID, ConnectionID: conn.ID})
	if errors.Is(err, pgx.ErrNoRows) || !t.Valid {
		return time.Time{}, nil
	}
	return t.Time, err
}

// runFailed: the run brought nothing in. A run whose every listing window
// failed (a network outage) fails although no call aborted it.
func runFailed(rep Report, runErr error) bool {
	return runErr != nil || (rep.Counters.WindowsFailed > 0 && rep.Counters.Windows == 0)
}

// ipNotAllowed: İşNet's "unanswered call" error (IP not allowlisted, or a
// network outage that looks the same).
func ipNotAllowed(err error) bool { return err != nil && isnet.IsIPNotAllowed(err) }

// connectionStatus is the status a finished run leaves on the connection:
// "" keeps the current one (a transient failure). Only a real credential
// error (or a permission error, or İşNet's unanswered calls repeated
// IPNotAllowedAfter times) turns it auth_failed. During setup (the first
// load has not succeeded yet) İşNet's unanswered call is reported at once:
// a missing IP definition is the likely cause and the office must act.
func connectionStatus(runErr error, failures int, current string) string {
	setUp := current == "active" || current == "rate_limited"
	switch kindOf(runErr) {
	case provider.KindAuth:
		return "auth_failed"
	case provider.KindPermission:
		if setUp && ipNotAllowed(runErr) && failures < IPNotAllowedAfter {
			return ""
		}
		return "auth_failed"
	case provider.KindRateLimit:
		return "rate_limited"
	}
	if runErr != nil {
		return ""
	}
	return "active"
}

// reschedule writes the next incremental run and the consecutive failures
// of a scheduled connection after a run. errJSON is the failure written to
// last_error when the status is left unchanged (nil after a success).
func (s *Service) reschedule(ctx context.Context, conn db.SourceConnection, failed bool, failures int, errJSON []byte) {
	if !Scheduled(conn.SourceSystem) {
		return
	}
	next := s.now().Add(SyncInterval)
	if failed {
		next = s.now().Add(Backoff(failures))
	}
	if err := s.q.SetEntegratorSyncSchedule(context.WithoutCancel(ctx), db.SetEntegratorSyncScheduleParams{
		SyncIntervalMinutes: pgtype.Int4{Int32: int32(SyncInterval / time.Minute), Valid: true},
		NextSyncAt:          ts(next),
		SyncFailures:        int32(min(failures, 1<<20)), //nolint:gosec // bounded above
		Succeeded:           !failed,
		LastError:           errJSON,
		TaxpayerID:          conn.TaxpayerID, ID: conn.ID,
	}); err != nil {
		s.log.Error("entegrator: reschedule", "err", err)
	}
}

// EnqueueDue hands every scheduled connection whose next incremental run
// has come to enqueue (the worker's sweep, run across offices inside
// rls.System) and returns how many it handed.
func (s *Service) EnqueueDue(ctx context.Context, enqueue Enqueuer) (int, error) {
	rows, err := s.q.ListDueEntegratorConnections(ctx, db.ListDueEntegratorConnectionsParams{
		SourceSystems: scheduledSystems(), Now: ts(s.now()), Lim: DueBatch,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := enqueue(ctx, r.TaxpayerID, r.Uuid, RunIncremental); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// EnqueueNightly hands every scheduled connection to enqueue for the nightly
// scan (RunFullScan) and returns how many it handed.
func (s *Service) EnqueueNightly(ctx context.Context, enqueue Enqueuer) (int, error) {
	rows, err := s.q.ListScheduledEntegratorConnections(ctx, scheduledSystems())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := enqueue(ctx, r.TaxpayerID, r.Uuid, RunFullScan); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// RunScheduledTask is the queue handler of a scheduled run (incremental or
// nightly). Integrator failures are recorded on the run and the connection
// and retried by the schedule (next_sync_at, back-off), not by the queue.
// A nightly scan that finds the connection busy returns ErrBusy so the
// queue retries it shortly; a busy incremental run is simply skipped (the
// sweep picks the connection again).
func (s *Service) RunScheduledTask(ctx context.Context, taxpayerID int64, connUUID uuid.UUID, kind string) error {
	_, err := s.Run(ctx, taxpayerID, connUUID, kind)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrBusy):
		if kind == RunFullScan {
			return err
		}
		return nil
	case errors.Is(err, ErrTooSoon), errors.Is(err, ErrNoCredentials), errors.Is(err, pgx.ErrNoRows), kindOf(err) != "":
		s.log.Info("entegrator: scheduled sync ended", "connection", connUUID, "kind", kind, "reason", err)
		return nil
	}
	return err
}
