package usecase

import (
	"context"
	"fmt"
	"time"
)

// DefaultReportDay is the day of month the month-end VAT report is shown
// as ready (#780: "ayın 29'u"); a shorter month uses its last day.
const DefaultReportDay = 29

// TrackingSettings are the office's VAT tracking settings (#780 B). New
// features stay switchable per office.
type TrackingSettings struct {
	// ReportEnabled: the month-end VAT report of all taxpayers (alınan,
	// satılan, devreden) is shown. Default on.
	ReportEnabled bool `json:"report_enabled"`
	// ReportDay: from this day of the month on the report is "ready"
	// (1-31; a shorter month uses its last day). Default 29.
	ReportDay int `json:"report_day"`
	// ReportReadyOn is this month's ready date (YYYY-MM-DD, Istanbul);
	// computed, ignored on save.
	ReportReadyOn string     `json:"report_ready_on"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

// DefaultTrackingSettings is what an office without a row gets.
func DefaultTrackingSettings() TrackingSettings {
	return TrackingSettings{ReportEnabled: true, ReportDay: DefaultReportDay}
}

// Tracking returns the office's VAT tracking settings.
func (s *Service) Tracking(ctx context.Context, orgID int64) (TrackingSettings, error) {
	return s.withReady(s.store.TrackingSettings(ctx, orgID))
}

func (s *Service) withReady(t TrackingSettings, err error) (TrackingSettings, error) {
	if err != nil {
		return t, err
	}
	t.ReportReadyOn = ReportReadyOn(s.now(), t.ReportDay).Format(time.DateOnly)
	return t, nil
}

// SaveTracking validates and stores the office's VAT tracking settings.
func (s *Service) SaveTracking(ctx context.Context, orgID, userID int64, in TrackingSettings) (TrackingSettings, error) {
	if in.ReportDay < 1 || in.ReportDay > 31 {
		return TrackingSettings{}, fmt.Errorf("%w: report_day must be 1-31", ErrInvalid)
	}
	return s.withReady(s.store.SaveTrackingSettings(ctx, orgID, userID, in))
}

// ReportReadyOn is the day of t's month from which the month-end report
// is ready: day, or the month's last day when the month is shorter.
func ReportReadyOn(t time.Time, day int) time.Time {
	t = t.In(Istanbul)
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, Istanbul)
	if last := first.AddDate(0, 1, -1).Day(); day > last {
		day = last
	}
	return first.AddDate(0, 0, day-1)
}
