package reports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
)

// ScheduleQueries is the db subset of scheduled reports.
type ScheduleQueries interface {
	DeliveryQueries
	GetReportSchedule(ctx context.Context, arg db.GetReportScheduleParams) (db.ReportSchedule, error)
	UpsertReportSchedule(ctx context.Context, arg db.UpsertReportScheduleParams) (db.ReportSchedule, error)
	DueReportSchedules(ctx context.Context, arg db.DueReportSchedulesParams) ([]db.DueReportSchedulesRow, error)
	MarkReportScheduleRun(ctx context.Context, arg db.MarkReportScheduleRunParams) error
}

// Schedule is the API view of an engagement's automatic monthly report.
type Schedule struct {
	Enabled bool `json:"enabled"`
	// DayOfMonth: the previous month's report goes out from this day on
	// (1-31; a shorter month uses its last day, #780).
	DayOfMonth int32   `json:"day_of_month"`
	Recipient  *string `json:"recipient"`
	// LastPeriod is the last month sent automatically.
	LastPeriod *string `json:"last_period"`
	LastError  *string `json:"last_error"`
	// NextRun is the next send date (Europe/Istanbul) and NextPeriod the
	// month it reports; null when disabled.
	NextRun    *string `json:"next_run"`
	NextPeriod *string `json:"next_period"`
}

// ScheduleInput is the PUT body.
type ScheduleInput struct {
	Enabled    bool   `json:"enabled"`
	DayOfMonth int32  `json:"day_of_month"`
	Recipient  string `json:"recipient"`
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid || strings.TrimSpace(t.String) == "" {
		return nil
	}
	s := strings.TrimSpace(t.String)
	return &s
}

func scheduleView(r db.ReportSchedule, now time.Time) Schedule {
	v := Schedule{Enabled: r.Enabled, DayOfMonth: r.DayOfMonth, Recipient: textPtr(r.Recipient),
		LastPeriod: textPtr(r.LastPeriod), LastError: textPtr(r.LastError)}
	if r.Enabled {
		run, period := NextRun(now, int(r.DayOfMonth), v.LastPeriod)
		d, p := run.Format("2006-01-02"), period
		v.NextRun, v.NextPeriod = &d, &p
	}
	return v
}

// NextRun is when the schedule sends next and which month: on day `day`
// of a month it sends the month before, unless that one was already sent.
func NextRun(now time.Time, day int, last *string) (time.Time, string) {
	t := now.In(istanbul)
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, istanbul)
	for i := 0; i < 3; i++ {
		month := first.AddDate(0, i, 0)
		period := month.AddDate(0, -1, 0).Format("2006-01")
		if last != nil && *last >= period {
			continue
		}
		run := time.Date(month.Year(), month.Month(), clampDay(month, day), 0, 0, 0, 0, istanbul)
		if i == 0 && t.After(run) {
			// Due already: the sweep sends it within the minute.
			return t, period
		}
		return run, period
	}
	m := first.AddDate(0, 1, 0)
	return time.Date(m.Year(), m.Month(), clampDay(m, day), 0, 0, 0, 0, istanbul), first.Format("2006-01")
}

// clampDay is day, or the last day of month's month when it is shorter
// (29-31 in February, 31 in a 30-day month).
func clampDay(month time.Time, day int) int {
	if last := lastDay(month); day > last {
		return last
	}
	return day
}

func lastDay(t time.Time) int {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, 1, -1).Day()
}

// GetSchedule returns the engagement's schedule (disabled default when none).
func (s *DeliveryService) GetSchedule(ctx context.Context, q ScheduleQueries, sc engagementctx.Scope) (Schedule, error) {
	r, err := q.GetReportSchedule(ctx, db.GetReportScheduleParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{Enabled: false, DayOfMonth: 1}, nil
	}
	if err != nil {
		return Schedule{}, err
	}
	return scheduleView(r, s.now()), nil
}

// PutSchedule saves the engagement's schedule.
func (s *DeliveryService) PutSchedule(ctx context.Context, q ScheduleQueries, sc engagementctx.Scope, userID int64, in ScheduleInput) (Schedule, error) {
	if in.DayOfMonth < 1 || in.DayOfMonth > 31 {
		return Schedule{}, fmt.Errorf("%w: day_of_month must be 1-31", ErrInvalid)
	}
	var to pgtype.Text
	if r := strings.TrimSpace(in.Recipient); r != "" {
		addr, err := ValidEmail(r)
		if err != nil {
			return Schedule{}, err
		}
		to = pgtype.Text{String: addr, Valid: true}
	} else if in.Enabled {
		// Without a recipient the taxpayer's contact email must exist.
		if _, err := s.recipient(ctx, sc.TaxpayerID, ""); err != nil {
			return Schedule{}, err
		}
	}
	var by pgtype.Int8
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	r, err := q.UpsertReportSchedule(ctx, db.UpsertReportScheduleParams{OrganizationID: sc.OrganizationID, EngagementID: sc.ID,
		TaxpayerID: sc.TaxpayerID, Enabled: in.Enabled, DayOfMonth: in.DayOfMonth, Recipient: to, UpdatedBy: by})
	if err != nil {
		return Schedule{}, err
	}
	return scheduleView(r, s.now()), nil
}

// Scheduler queues the due scheduled reports (worker sweep, every office:
// run it inside rls.System).
type Scheduler struct {
	Q   ScheduleQueries
	Log *slog.Logger
	Now func() time.Time
}

// RunDue queues last month's report for every schedule whose day has come
// and returns how many were queued. A taxpayer without a usable address is
// marked as run with the error, so it is not retried every minute; the
// office sees the error on the schedule.
func (s *Scheduler) RunDue(ctx context.Context) (int, error) {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	t := now().In(istanbul)
	period := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, -1, 0).Format("2006-01")
	rows, err := s.Q.DueReportSchedules(ctx, db.DueReportSchedulesParams{Today: intconv.Int32(t.Day()), LastDay: intconv.Int32(lastDay(t)), Period: period, BatchLimit: 50})
	if err != nil {
		return 0, fmt.Errorf("reports: due schedules: %w", err)
	}
	svc := &DeliveryService{q: s.Q, now: now}
	rg, _ := ParseRange(period, "")
	queued := 0
	for _, r := range rows {
		sc := engagementctx.Scope{ID: r.EngagementID, UUID: r.EngagementUuid, OrganizationID: r.OrganizationID, TaxpayerID: r.TaxpayerID}
		recipient := ""
		if r.Recipient.Valid {
			recipient = r.Recipient.String
		}
		_, err := svc.Queue(ctx, sc, QueueInput{Range: rg, Recipient: recipient, OfficeName: r.OfficeName, Origin: OriginScheduled})
		var lastErr pgtype.Text
		switch {
		case errors.Is(err, ErrNoRecipient), errors.Is(err, ErrBadRecipient):
			lastErr = pgtype.Text{String: err.Error(), Valid: true}
			log.Warn("report_schedule_skipped", "engagement", r.EngagementUuid.String(), "period", period, "error", err)
		case err != nil:
			return queued, err
		default:
			queued++
		}
		if err := s.Q.MarkReportScheduleRun(ctx, db.MarkReportScheduleRunParams{
			OrganizationID: r.OrganizationID, EngagementID: r.EngagementID, Period: pgtype.Text{String: period, Valid: true}, LastError: lastErr,
		}); err != nil {
			return queued, err
		}
	}
	return queued, nil
}
