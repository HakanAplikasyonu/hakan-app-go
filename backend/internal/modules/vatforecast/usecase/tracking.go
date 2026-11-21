package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
	"github.com/shopspring/decimal"
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
	// WatchEnabled: the daily VAT watch ("KDV nöbetçisi") raises "KDV
	// durumu değişti" and "KDV eşiği aşıldı" for the office. Default on.
	WatchEnabled bool `json:"watch_enabled"`
	// WatchEmail: the watch's alerts also go by e-mail to the taxpayer's
	// assignees (owners when nobody is assigned); otherwise in-app only.
	// Default off.
	WatchEmail bool `json:"watch_email"`
	// PayableLimit is the office's "ödenecek KDV şu tutarı geçerse uyar"
	// threshold: the office layer of VAT_PAYABLE_THRESHOLD.limit in
	// warning_rule_configs, which the watch and the warning rule read. nil:
	// not set (no threshold alert). Computed here; saved by
	// SavePayableLimit.
	PayableLimit *Money `json:"payable_limit"`
	// ReportReadyOn is this month's ready date (YYYY-MM-DD, Istanbul);
	// computed, ignored on save.
	ReportReadyOn string     `json:"report_ready_on"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

// DefaultTrackingSettings is what an office without a row gets.
func DefaultTrackingSettings() TrackingSettings {
	return TrackingSettings{ReportEnabled: true, ReportDay: DefaultReportDay, WatchEnabled: true}
}

// Tracking returns the office's VAT tracking settings.
func (s *Service) Tracking(ctx context.Context, orgID int64) (TrackingSettings, error) {
	t, err := s.store.TrackingSettings(ctx, orgID)
	return s.withReady(ctx, orgID, t, err)
}

func (s *Service) withReady(ctx context.Context, orgID int64, t TrackingSettings, err error) (TrackingSettings, error) {
	if err != nil {
		return t, err
	}
	t.ReportReadyOn = ReportReadyOn(s.now(), t.ReportDay).Format(time.DateOnly)
	if s.config != nil {
		st, err := s.Settings(ctx, Subject{OrganizationID: orgID})
		if err != nil {
			return t, err
		}
		if st.PayableLimit != nil {
			l := M(*st.PayableLimit)
			t.PayableLimit = &l
		}
	}
	return t, nil
}

// SaveTracking validates and stores the office's VAT tracking settings.
func (s *Service) SaveTracking(ctx context.Context, orgID, userID int64, in TrackingSettings) (TrackingSettings, error) {
	if in.ReportDay < 1 || in.ReportDay > 31 {
		return TrackingSettings{}, fmt.Errorf("%w: report_day must be 1-31", ErrInvalid)
	}
	t, err := s.store.SaveTrackingSettings(ctx, orgID, userID, in)
	return s.withReady(ctx, orgID, t, err)
}

// ThresholdStore reads and writes warning_rule_configs layers
// (warnings/repository.Config).
type ThresholdStore interface {
	Layers(ctx context.Context, organizationID, engagementID int64, codes []string) ([]warningsrepository.CodeLayers, error)
	Upsert(ctx context.Context, organizationID, engagementID int64, code string, l engine.Layer, userID int64) error
}

// WithThresholds enables SavePayableLimit.
func (s *Service) WithThresholds(ts ThresholdStore) *Service { s.thresholds = ts; return s }

// MaxPayableLimit bounds the threshold (NUMERIC(18,2) amounts).
var MaxPayableLimit = decimal.New(1, 15)

// SavePayableLimit writes the office's VAT threshold into the office layer
// of VAT_PAYABLE_THRESHOLD (warning_rule_configs), the one place the watch
// and the warning rule read; there is no second copy. limit nil clears it.
// The layer's other fields (severity, other params) are kept; setting a
// limit also switches the code back on, since the office asked to be
// warned.
func (s *Service) SavePayableLimit(ctx context.Context, orgID, userID int64, limit *decimal.Decimal) error {
	if limit != nil && (!limit.IsPositive() || limit.GreaterThanOrEqual(MaxPayableLimit)) {
		return fmt.Errorf("%w: payable_limit must be a positive amount", ErrInvalid)
	}
	if s.thresholds == nil {
		return fmt.Errorf("vatforecast: threshold store not configured")
	}
	layers, err := s.thresholds.Layers(ctx, orgID, 0, []string{CodePayableThreshold})
	if err != nil {
		return err
	}
	l, err := PayableLimitLayer(layers, limit)
	if err != nil {
		return err
	}
	return s.thresholds.Upsert(ctx, orgID, 0, CodePayableThreshold, l, userID)
}

// PayableLimitLayer is the new office layer of VAT_PAYABLE_THRESHOLD: the
// current office layer with params.limit replaced.
func PayableLimitLayer(layers []warningsrepository.CodeLayers, limit *decimal.Decimal) (engine.Layer, error) {
	var l engine.Layer
	params := map[string]any{}
	for _, c := range layers {
		if c.Code == CodePayableThreshold && c.Office != nil {
			l = *c.Office
			if len(l.Params) > 0 {
				if err := json.Unmarshal(l.Params, &params); err != nil {
					return l, fmt.Errorf("vat threshold params: %w", err)
				}
			}
		}
	}
	if params == nil {
		params = map[string]any{}
	}
	if limit == nil {
		params["limit"] = nil
	} else {
		params["limit"] = limit.StringFixed(2)
		on := true
		l.Enabled = &on
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return l, err
	}
	l.Params = raw
	return l, nil
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
