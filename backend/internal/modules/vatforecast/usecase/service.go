package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Errors.
var (
	ErrInvalid         = errors.New("vatforecast: invalid input")
	ErrOpeningNotFound = errors.New("vatforecast: opening carry-forward not found")
)

// EngagementRef is an active engagement for the office-wide list.
type EngagementRef struct {
	ID           int64
	UUID         uuid.UUID
	TaxpayerID   int64
	TaxpayerUUID uuid.UUID
}

// OfficeFilter narrows the office-wide list.
type OfficeFilter struct {
	OrganizationID int64
	// AllEngagements: tenant.engagements.read_all; otherwise only UserID's.
	AllEngagements bool
	UserID         int64
	Limit, Offset  int32
}

// Store is what the service needs from Postgres. Taxpayer-scoped reads take
// the taxpayer id of the resolved engagement; office work product is keyed
// by organization and engagement.
type Store interface {
	Lines(ctx context.Context, taxpayerID int64, period string) ([]Line, error)
	Excluded(ctx context.Context, taxpayerID int64, period string) (Excluded, error)
	// History returns period totals in [from, before), ascending.
	History(ctx context.Context, taxpayerID int64, from, before string) ([]PeriodTotal, error)
	OpeningAtOrBefore(ctx context.Context, organizationID, engagementID int64, period string) (*Opening, error)
	ListOpenings(ctx context.Context, organizationID, engagementID int64) ([]Opening, error)
	UpsertOpening(ctx context.Context, organizationID, engagementID int64, o Opening, userID int64) (Opening, error)
	DeleteOpening(ctx context.Context, organizationID, engagementID int64, period string) (bool, error)
	ActiveEngagements(ctx context.Context, f OfficeFilter) ([]EngagementRef, int64, error)
}

// Service computes forecasts and evaluates the warning rule.
type Service struct {
	store  Store
	config engine.ConfigResolver
	engine *engine.Engine
	now    func() time.Time
}

// New builds the service. eng may be nil (Evaluate then fails).
func New(store Store, config engine.ConfigResolver, eng *engine.Engine) *Service {
	return &Service{store: store, config: config, engine: eng, now: time.Now}
}

// WithClock overrides the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service { s.now = now; return s }

// CurrentPeriod is this month (Istanbul): the in-period forecast default.
func (s *Service) CurrentPeriod() string { return PeriodOf(s.now()) }

func (s *Service) subject(sub Subject) engine.Subject {
	return engine.Subject{OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, TaxpayerID: sub.TaxpayerID}
}

// Settings resolves the effective configuration of the engagement.
func (s *Service) Settings(ctx context.Context, sub Subject) (Settings, error) {
	cfg, err := s.config.Resolve(ctx, s.subject(sub), Rule{}.Codes())
	if err != nil {
		return Settings{}, fmt.Errorf("resolve vat config: %w", err)
	}
	return SettingsFrom(cfg)
}

// Forecast computes the forecast of period for the engagement.
func (s *Service) Forecast(ctx context.Context, sub Subject, period string) (Forecast, error) {
	if _, err := ParsePeriod(period); err != nil {
		return Forecast{}, err
	}
	settings, err := s.Settings(ctx, sub)
	if err != nil {
		return Forecast{}, err
	}
	lines, err := s.store.Lines(ctx, sub.TaxpayerID, period)
	if err != nil {
		return Forecast{}, err
	}
	excluded, err := s.store.Excluded(ctx, sub.TaxpayerID, period)
	if err != nil {
		return Forecast{}, err
	}
	opening, err := s.store.OpeningAtOrBefore(ctx, sub.OrganizationID, sub.EngagementID, period)
	if err != nil {
		return Forecast{}, err
	}
	var history []PeriodTotal
	if opening == nil || opening.Period < period {
		from := "0000-01"
		if opening != nil {
			from = opening.Period
		}
		if history, err = s.store.History(ctx, sub.TaxpayerID, from, period); err != nil {
			return Forecast{}, err
		}
	}
	return Compute(Input{
		Subject: sub, Period: period, Lines: lines, History: history, Opening: opening,
		Excluded: excluded, Settings: settings, Now: s.now(),
	})
}

// Evaluation is what Evaluate did for one period.
type Evaluation struct {
	Period       string `json:"period"`
	Payable      Money  `json:"payable"`
	DueDate      string `json:"due_date"`
	DaysLeft     int    `json:"days_left"`
	Created      int    `json:"created"`
	Refreshed    int    `json:"refreshed"`
	Suppressed   int    `json:"suppressed"`
	Disabled     int    `json:"disabled"`
	AutoResolved int    `json:"auto_resolved"`
}

// DefaultEvaluationPeriods are the open periods: last month (its return is
// due this month) and this month (in-period estimate).
func (s *Service) DefaultEvaluationPeriods() []string {
	cur := s.CurrentPeriod()
	return []string{PreviousPeriod(cur), cur}
}

// Evaluate computes the forecast of each period and applies the warning rule.
func (s *Service) Evaluate(ctx context.Context, sub Subject, periods []string) ([]Evaluation, error) {
	if s.engine == nil {
		return nil, errors.New("vatforecast: warning engine not configured")
	}
	out := make([]Evaluation, 0, len(periods))
	for _, p := range periods {
		f, err := s.Forecast(ctx, sub, p)
		if err != nil {
			return nil, err
		}
		rep, err := engine.Run[Forecast](ctx, s.engine, s.subject(sub), Rule{}, f)
		if err != nil {
			return nil, fmt.Errorf("evaluate %s: %w", p, err)
		}
		out = append(out, Evaluation{
			Period: p, Payable: f.Totals.Payable, DueDate: f.Calendar.DueDate, DaysLeft: f.Calendar.DaysLeft,
			Created: rep.Created, Refreshed: rep.Refreshed, Suppressed: rep.Suppressed,
			Disabled: rep.Disabled, AutoResolved: rep.AutoResolved,
		})
	}
	return out, nil
}

// OfficeItem is one engagement in the office-wide list.
type OfficeItem struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	TaxpayerUUID   uuid.UUID `json:"taxpayer_uuid"`
	Totals         Totals    `json:"totals"`
	Calendar       Calendar  `json:"calendar"`
	Checks         Checks    `json:"checks"`
	Excluded       Excluded  `json:"excluded"`
}

// Office lists the forecast of period for the office's active engagements.
func (s *Service) Office(ctx context.Context, f OfficeFilter, period string) ([]OfficeItem, int64, error) {
	if _, err := ParsePeriod(period); err != nil {
		return nil, 0, err
	}
	refs, total, err := s.store.ActiveEngagements(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]OfficeItem, 0, len(refs))
	for _, r := range refs {
		fc, err := s.Forecast(ctx, Subject{
			OrganizationID: f.OrganizationID, EngagementID: r.ID, EngagementUUID: r.UUID,
			TaxpayerID: r.TaxpayerID, TaxpayerUUID: r.TaxpayerUUID,
		}, period)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, OfficeItem{
			EngagementUUID: r.UUID, TaxpayerUUID: r.TaxpayerUUID, Totals: fc.Totals,
			Calendar: fc.Calendar, Checks: fc.Checks, Excluded: fc.Excluded,
		})
	}
	return out, total, nil
}

// OpeningInput is a PUT body for an opening carry-forward.
type OpeningInput struct {
	Period string  `json:"period"`
	Amount string  `json:"amount"`
	Note   *string `json:"note"`
}

// ListOpenings lists the engagement's opening carry-forwards.
func (s *Service) ListOpenings(ctx context.Context, sub Subject) ([]Opening, error) {
	return s.store.ListOpenings(ctx, sub.OrganizationID, sub.EngagementID)
}

// SetOpening writes the opening carry-forward into a period.
func (s *Service) SetOpening(ctx context.Context, sub Subject, in OpeningInput, userID int64) (Opening, error) {
	if _, err := ParsePeriod(in.Period); err != nil {
		return Opening{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	amt, err := decimal.NewFromString(strings.TrimSpace(in.Amount))
	if err != nil {
		return Opening{}, fmt.Errorf("%w: amount must be a decimal string", ErrInvalid)
	}
	if amt.IsNegative() || amt.Exponent() < -2 || amt.GreaterThanOrEqual(decimal.New(1, 16)) {
		return Opening{}, fmt.Errorf("%w: amount must be >= 0 with at most two decimals", ErrInvalid)
	}
	o := Opening{Period: in.Period, Amount: M(amt)}
	if in.Note != nil {
		n := strings.TrimSpace(*in.Note)
		if len(n) > 1000 {
			return Opening{}, fmt.Errorf("%w: note is too long", ErrInvalid)
		}
		o.Note = n
	}
	return s.store.UpsertOpening(ctx, sub.OrganizationID, sub.EngagementID, o, userID)
}

// DeleteOpening removes the opening carry-forward of a period.
func (s *Service) DeleteOpening(ctx context.Context, sub Subject, period string) error {
	if _, err := ParsePeriod(period); err != nil {
		return err
	}
	ok, err := s.store.DeleteOpening(ctx, sub.OrganizationID, sub.EngagementID, period)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOpeningNotFound
	}
	return nil
}
