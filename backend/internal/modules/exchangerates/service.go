package exchangerates

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// BackfillDays: on every run the business days of this many past days
// that have no bulletin are fetched from the archive (holidays answer 404).
const BackfillDays = 31

// ErrInvalid: a bad settings input.
var ErrInvalid = errors.New("exchangerates: invalid input")

// Istanbul is the office time zone (TCMB publishes at 15:30 Istanbul).
var Istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*60*60)
	}
	return loc
}()

// RateReader reads the stored bulletins and the office setting; used by
// Lookup (foreign invoices, request time).
type RateReader interface {
	ListExchangeRatesBefore(ctx context.Context, arg db.ListExchangeRatesBeforeParams) ([]db.ExchangeRate, error)
	GetExchangeRateSettings(ctx context.Context, organizationID int64) (db.ExchangeRateSetting, error)
}

// Queries is the subset of db.Querier used by the service.
type Queries interface {
	RateReader
	UpsertExchangeRate(ctx context.Context, arg db.UpsertExchangeRateParams) error
	ListExchangeRateDates(ctx context.Context, arg db.ListExchangeRateDatesParams) ([]pgtype.Date, error)
	UpsertExchangeRateSettings(ctx context.Context, arg db.UpsertExchangeRateSettingsParams) (db.ExchangeRateSetting, error)
	FillInvoiceExchangeRates(ctx context.Context, arg db.FillInvoiceExchangeRatesParams) (int64, error)
	FillForeignInvoiceExchangeRates(ctx context.Context, organizationID pgtype.Int8) (int64, error)
}

// Source is the TCMB feed (Client).
type Source interface {
	Today(ctx context.Context) (Bulletin, error)
	Day(ctx context.Context, d time.Time) (Bulletin, error)
}

// Service fetches and stores the rates and keeps the office setting.
type Service struct {
	q   Queries
	src Source
	now func() time.Time
}

// New builds the service; src may be nil (no fetching, API only).
func New(q Queries, src Source) *Service { return &Service{q: q, src: src, now: time.Now} }

// Settings is the office's rate setting.
type Settings struct {
	// RateKind: forex_buying (default), forex_selling, banknote_buying,
	// banknote_selling.
	RateKind string `json:"rate_kind"`
	// AutoFill: missing rates are filled from TCMB (default true).
	AutoFill bool `json:"auto_fill"`
}

// DefaultSettings is the setting of an office without a row.
var DefaultSettings = Settings{RateKind: KindForexBuying, AutoFill: true}

func settingsOf(ctx context.Context, q RateReader, orgID int64) (Settings, error) {
	row, err := q.GetExchangeRateSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultSettings, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("exchange rate settings: %w", err)
	}
	return Settings{RateKind: row.RateKind, AutoFill: row.AutoFill}, nil
}

// Settings returns the office's setting.
func (s *Service) Settings(ctx context.Context, orgID int64) (Settings, error) {
	return settingsOf(ctx, s.q, orgID)
}

// SaveSettings stores the setting and applies it at once: missing rates
// are filled and automatic rates take the new kind (rates entered by hand
// stay). Returns the stored setting.
func (s *Service) SaveSettings(ctx context.Context, orgID int64, in Settings, userID int64) (Settings, error) {
	if !ValidKind(in.RateKind) {
		return Settings{}, fmt.Errorf("%w: rate_kind must be forex_buying, forex_selling, banknote_buying or banknote_selling", ErrInvalid)
	}
	if _, err := s.q.UpsertExchangeRateSettings(ctx, db.UpsertExchangeRateSettingsParams{
		OrganizationID: orgID, RateKind: in.RateKind, AutoFill: in.AutoFill,
		UpdatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
	}); err != nil {
		return Settings{}, fmt.Errorf("save exchange rate settings: %w", err)
	}
	if in.AutoFill {
		org := pgtype.Int8{Int64: orgID, Valid: true}
		if _, err := s.q.FillInvoiceExchangeRates(ctx, db.FillInvoiceExchangeRatesParams{OrganizationID: org}); err != nil {
			return Settings{}, fmt.Errorf("fill invoice rates: %w", err)
		}
		if _, err := s.q.FillForeignInvoiceExchangeRates(ctx, org); err != nil {
			return Settings{}, fmt.Errorf("fill foreign invoice rates: %w", err)
		}
	}
	return s.Settings(ctx, orgID)
}

// Lookup is the automatic rate of an office for currency on an invoice
// dated date: the rate, the bulletin day and true; false when the office
// turned automatic rates off or no bulletin fits (PickRate).
func Lookup(ctx context.Context, q RateReader, orgID int64, currency string, date time.Time) (decimal.Decimal, time.Time, bool, error) {
	set, err := settingsOf(ctx, q, orgID)
	if err != nil || !set.AutoFill || currency == "" || currency == "TRY" {
		return decimal.Zero, time.Time{}, false, err
	}
	d := day(date)
	rows, err := q.ListExchangeRatesBefore(ctx, db.ListExchangeRatesBeforeParams{
		Currency: currency, BeforeDate: pgDate(d), FromDate: pgDate(d.AddDate(0, 0, -LookbackDays)),
	})
	if err != nil {
		return decimal.Zero, time.Time{}, false, fmt.Errorf("exchange rates: %w", err)
	}
	rates := make([]Rate, 0, len(rows))
	for _, r := range rows {
		rates = append(rates, rateFromRow(r))
	}
	r, v, ok := PickRate(rates, d, set.RateKind)
	if !ok {
		return decimal.Zero, time.Time{}, false, nil
	}
	return v, r.Date, true, nil
}

func rateFromRow(r db.ExchangeRate) Rate {
	return Rate{
		Date: r.Date.Time, Currency: r.Currency, Unit: int(r.Unit),
		ForexBuying: decPtr(r.ForexBuying), ForexSelling: decPtr(r.ForexSelling),
		BanknoteBuying: decPtr(r.BanknoteBuying), BanknoteSelling: decPtr(r.BanknoteSelling),
	}
}

// Fetch reads today's bulletin and the archive bulletins of the missing
// business days of the last BackfillDays and stores them. It returns how
// many bulletins it stored; the error is the first network or parse
// failure (the stored ones stay).
func (s *Service) Fetch(ctx context.Context) (int, error) {
	if s.src == nil {
		return 0, nil
	}
	today := day(s.now().In(Istanbul))
	stored := 0
	var firstErr error
	b, err := s.src.Today(ctx)
	if err == nil {
		if err = s.Store(ctx, b, "tcmb"); err != nil {
			return stored, err
		}
		stored++
	} else {
		firstErr = err
	}
	from := today.AddDate(0, 0, -BackfillDays)
	dates, err := s.q.ListExchangeRateDates(ctx, db.ListExchangeRateDatesParams{FromDate: pgDate(from), ToDate: pgDate(today)})
	if err != nil {
		return stored, fmt.Errorf("exchange rate dates: %w", err)
	}
	have := make(map[string]bool, len(dates))
	for _, d := range dates {
		have[d.Time.Format("2006-01-02")] = true
	}
	// Today's archive file appears only after 15:30; today.xml covers it.
	for _, d := range MissingDays(from, today.AddDate(0, 0, -1), have) {
		if ctx.Err() != nil {
			return stored, ctx.Err()
		}
		b, err := s.src.Day(ctx, d)
		if errors.Is(err, ErrNotPublished) {
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			break // network down: try again on the next run
		}
		if err := s.Store(ctx, b, "tcmb"); err != nil {
			return stored, err
		}
		stored++
	}
	return stored, firstErr
}

// Store upserts every rate of a bulletin.
func (s *Service) Store(ctx context.Context, b Bulletin, source string) error {
	for _, r := range b.Rates {
		if err := s.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgDate(b.Date), Currency: r.Currency, Unit: int32(r.Unit),
			ForexBuying: numPtr(r.ForexBuying), ForexSelling: numPtr(r.ForexSelling),
			BanknoteBuying: numPtr(r.BanknoteBuying), BanknoteSelling: numPtr(r.BanknoteSelling),
			Source: source,
		}); err != nil {
			return fmt.Errorf("store exchange rate %s %s: %w", b.Date.Format("2006-01-02"), r.Currency, err)
		}
	}
	return nil
}

// FillAll fills the missing and automatic rates of every office's
// invoices and draft foreign invoices. The caller runs it in rls.System.
func (s *Service) FillAll(ctx context.Context) (int, error) {
	a, err := s.q.FillInvoiceExchangeRates(ctx, db.FillInvoiceExchangeRatesParams{})
	if err != nil {
		return 0, fmt.Errorf("fill invoice rates: %w", err)
	}
	b, err := s.q.FillForeignInvoiceExchangeRates(ctx, pgtype.Int8{})
	if err != nil {
		return int(a), fmt.Errorf("fill foreign invoice rates: %w", err)
	}
	return int(a + b), nil
}

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: day(t), Valid: true} }

func numPtr(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func decPtr(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return nil
	}
	d := decimal.NewFromBigInt(n.Int, n.Exp)
	return &d
}
