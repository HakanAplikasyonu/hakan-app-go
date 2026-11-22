package foreigninvoices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/exchangerates"
)

// Errors.
var (
	ErrNotFound = errors.New("foreigninvoices: invoice not found")
	ErrInvalid  = errors.New("foreigninvoices: invalid input")
	// ErrConflict: the row is confirmed (undo first) or not confirmed.
	ErrConflict = errors.New("foreigninvoices: conflict")
	// ErrDisabled: the office turned the section off.
	ErrDisabled = errors.New("foreigninvoices: turned off by the office")
)

// Subject is one engagement (office × taxpayer).
type Subject struct {
	OrganizationID, EngagementID int64
}

// Input is a new invoice (POST). Amounts and rates are decimal strings
// ("100.00", "34.5"; a comma decimal separator is accepted). Treatment,
// StopajRate, GrossUp and VATRate default to the office's setting of the
// service kind. ExchangeRate empty / null: kur eksik (TRY: always 1).
type Input struct {
	Period       string  `json:"period"`
	Vendor       string  `json:"vendor"`
	Country      string  `json:"country"`
	InvoiceNo    string  `json:"invoice_no"`
	InvoiceDate  string  `json:"invoice_date"`
	Amount       string  `json:"amount"`
	Currency     string  `json:"currency"`
	ExchangeRate *string `json:"exchange_rate"`
	ServiceKind  string  `json:"service_kind"`
	Treatment    *string `json:"treatment"`
	StopajRate   *string `json:"stopaj_rate"`
	GrossUp      *bool   `json:"gross_up"`
	VATRate      *string `json:"vat_rate"`
	Note         string  `json:"note"`
}

// Nullable is a JSON field that can be absent, null or a string.
type Nullable struct {
	Set   bool
	Value *string
}

// UnmarshalJSON is called only when the key is present.
func (n *Nullable) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n.Value = &s
	return nil
}

// Patch changes a draft invoice (PATCH): absent fields keep their value;
// "exchange_rate": null clears the kur.
type Patch struct {
	Period       *string  `json:"period"`
	Vendor       *string  `json:"vendor"`
	Country      *string  `json:"country"`
	InvoiceNo    *string  `json:"invoice_no"`
	InvoiceDate  *string  `json:"invoice_date"`
	Amount       *string  `json:"amount"`
	Currency     *string  `json:"currency"`
	ExchangeRate Nullable `json:"exchange_rate"`
	ServiceKind  *string  `json:"service_kind"`
	Treatment    *string  `json:"treatment"`
	StopajRate   *string  `json:"stopaj_rate"`
	GrossUp      *bool    `json:"gross_up"`
	VATRate      *string  `json:"vat_rate"`
	Note         *string  `json:"note"`
}

// Item is one invoice as the API shows it, with the calculation.
type Item struct {
	UUID         uuid.UUID `json:"uuid"`
	Period       string    `json:"period"`
	Vendor       string    `json:"vendor"`
	Country      string    `json:"country"`
	InvoiceNo    string    `json:"invoice_no"`
	InvoiceDate  *string   `json:"invoice_date"`
	Amount       string    `json:"amount"`
	Currency     string    `json:"currency"`
	ExchangeRate *string   `json:"exchange_rate"`
	// ExchangeRateSource: "tcmb" (automatic, #780; ExchangeRateDate is the
	// TCMB bulletin day), "manual" (entered in the office); null for TL or
	// without a kur.
	ExchangeRateSource *string `json:"exchange_rate_source"`
	ExchangeRateDate   *string `json:"exchange_rate_date"`
	ServiceKind        string  `json:"service_kind"`
	ServiceKindLabel   string  `json:"service_kind_label"`
	Treatment          string  `json:"treatment"`
	StopajRate         string  `json:"stopaj_rate"`
	GrossUp            bool    `json:"gross_up"`
	VATRate            string  `json:"vat_rate"`
	// Calculated in the invoice currency.
	Gross  string `json:"gross"`
	Stopaj string `json:"stopaj"`
	KDV2   string `json:"kdv2"`
	// Calculated in TL; null without a kur.
	AmountTRY   *string    `json:"amount_try"`
	GrossTRY    *string    `json:"gross_try"`
	StopajTRY   *string    `json:"stopaj_try"`
	KDV2TRY     *string    `json:"kdv2_try"`
	MissingRate bool       `json:"missing_rate"`
	Status      string     `json:"status"`
	SourceKind  *string    `json:"source_kind"`
	SourceUUID  *uuid.UUID `json:"source_uuid"`
	// HasFile: the invoice came with a file (GET .../{uuid}/file, #780 A2).
	HasFile bool `json:"has_file"`
	// Prefilled: the fields were suggested by AI from the file; the office
	// checks and confirms (#780 A2).
	Prefilled   bool       `json:"prefilled"`
	Note        *string    `json:"note"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Totals are the period's TL sums; rows without a kur are counted in
// MissingRate and left out of the sums.
type Totals struct {
	Count       int    `json:"count"`
	Confirmed   int    `json:"confirmed"`
	Draft       int    `json:"draft"`
	MissingRate int    `json:"missing_rate"`
	GrossTRY    string `json:"gross_try"`
	KDV2TRY     string `json:"kdv2_try"`
	StopajTRY   string `json:"stopaj_try"`
}

// List is the engagement's invoices of one period.
type List struct {
	Period string `json:"period"`
	Items  []Item `json:"items"`
	Totals Totals `json:"totals"`
}

// KindSetting is one service kind of the office.
type KindSetting struct {
	ServiceKind string `json:"service_kind"`
	Label       string `json:"label"`
	Treatment   string `json:"treatment"`
	StopajRate  string `json:"stopaj_rate"`
	GrossUp     bool   `json:"gross_up"`
	VATRate     string `json:"vat_rate"`
	Enabled     bool   `json:"enabled"`
	// Customized: the office changed the ready-made default.
	Customized bool `json:"customized"`
}

// Settings is the office's section switch and service kinds.
type Settings struct {
	Enabled bool          `json:"enabled"`
	Items   []KindSetting `json:"items"`
}

// KindInput is one service kind in PUT.
type KindInput struct {
	ServiceKind string `json:"service_kind"`
	Treatment   string `json:"treatment"`
	StopajRate  string `json:"stopaj_rate"`
	GrossUp     bool   `json:"gross_up"`
	VATRate     string `json:"vat_rate"`
	Enabled     bool   `json:"enabled"`
}

// SettingsInput is PUT /v1/tenant/foreign-service-kinds. Items absent:
// only the switch changes.
type SettingsInput struct {
	Enabled bool        `json:"enabled"`
	Items   []KindInput `json:"items"`
}

// Queries is the subset of db.Querier used here.
type Queries interface {
	ListForeignInvoices(ctx context.Context, arg db.ListForeignInvoicesParams) ([]db.ForeignServiceInvoice, error)
	GetForeignInvoice(ctx context.Context, arg db.GetForeignInvoiceParams) (db.ForeignServiceInvoice, error)
	CreateForeignInvoice(ctx context.Context, arg db.CreateForeignInvoiceParams) (db.ForeignServiceInvoice, error)
	UpdateForeignInvoice(ctx context.Context, arg db.UpdateForeignInvoiceParams) (db.ForeignServiceInvoice, error)
	DeleteForeignInvoice(ctx context.Context, arg db.DeleteForeignInvoiceParams) (int64, error)
	ConfirmForeignInvoice(ctx context.Context, arg db.ConfirmForeignInvoiceParams) (db.ForeignServiceInvoice, error)
	UnconfirmForeignInvoice(ctx context.Context, arg db.UnconfirmForeignInvoiceParams) (db.ForeignServiceInvoice, error)
	ListForeignServiceKindSettings(ctx context.Context, organizationID int64) ([]db.ForeignServiceKindSetting, error)
	UpsertForeignServiceKindSetting(ctx context.Context, arg db.UpsertForeignServiceKindSettingParams) error
	GetForeignInvoiceSettings(ctx context.Context, organizationID int64) (db.ForeignInvoiceSetting, error)
	UpsertForeignInvoiceSettings(ctx context.Context, arg db.UpsertForeignInvoiceSettingsParams) (db.ForeignInvoiceSetting, error)
	GetAppSettings(ctx context.Context) (db.AppSetting, error)
	// TCMB kurları (#780).
	exchangerates.RateReader
}

// Service manages the foreign service invoices.
type Service struct {
	q   Queries
	now func() time.Time
}

// New builds the service.
func New(q Queries) *Service { return &Service{q: q, now: time.Now} }

// Istanbul is the month boundary of the current period.
var Istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*60*60)
	}
	return loc
}()

// CurrentPeriod is the month of now in Europe/Istanbul (YYYY-MM).
func CurrentPeriod(now time.Time) string { return now.In(Istanbul).Format("2006-01") }

// normalized is a checked input ready for the database.
type normalized struct {
	Period, Vendor, Country, InvoiceNo, Currency, ServiceKind, Treatment, Note string
	InvoiceDate                                                                *time.Time
	Amount, StopajRate, VATRate                                                decimal.Decimal
	Rate                                                                       *decimal.Decimal
	GrossUp                                                                    bool
	// RateSource: exchangerates.SourceTCMB / SourceManual, "" for TL;
	// RateDate is the TCMB bulletin day of an automatic rate.
	RateSource string
	RateDate   *time.Time
}

func parseDec(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ",") && !strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", ".")
	}
	return decimal.NewFromString(s)
}

var maxAmount = decimal.New(1, 15)

// normalize checks an input; kind holds the defaults for absent treatment,
// rates and gross-up. It returns the row to store or a short reason.
func normalize(in Input, kind KindDefault) (normalized, string) {
	n := normalized{
		Period: strings.TrimSpace(in.Period), Vendor: strings.TrimSpace(in.Vendor),
		Country: normalizeCode(in.Country), InvoiceNo: strings.TrimSpace(in.InvoiceNo),
		Currency: normalizeCode(in.Currency), ServiceKind: strings.TrimSpace(in.ServiceKind),
		Note:      strings.TrimSpace(in.Note),
		Treatment: kind.Treatment, StopajRate: kind.StopajRate, GrossUp: kind.GrossUp, VATRate: kind.VATRate,
	}
	if n.Currency == "" {
		n.Currency = "USD"
	}
	switch {
	case !ValidPeriod(n.Period):
		return n, "period must be YYYY-MM"
	case n.Vendor == "" || len(n.Vendor) > 200:
		return n, "vendor is required (at most 200 characters)"
	case !countryRe.MatchString(n.Country):
		return n, "country must be a two-letter ISO code"
	case !currencyRe.MatchString(n.Currency):
		return n, "currency must be a three-letter code"
	case len(n.InvoiceNo) > 100:
		return n, "invoice_no is too long"
	case len(n.Note) > 1000:
		return n, "note is too long"
	}
	if _, ok := DefaultOf(n.ServiceKind); !ok {
		return n, "service_kind is unknown"
	}
	if d := strings.TrimSpace(in.InvoiceDate); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return n, "invoice_date must be YYYY-MM-DD"
		}
		n.InvoiceDate = &t
	}
	amount, err := parseDec(in.Amount)
	if err != nil || !amount.GreaterThan(decimal.Zero) || amount.GreaterThanOrEqual(maxAmount) {
		return n, "amount must be a positive number"
	}
	n.Amount = amount.Round(2)
	if n.Currency == "TRY" {
		one := decimal.NewFromInt(1)
		n.Rate = &one
	} else if in.ExchangeRate != nil && strings.TrimSpace(*in.ExchangeRate) != "" {
		r, err := parseDec(*in.ExchangeRate)
		if err != nil || !r.GreaterThan(decimal.Zero) || r.GreaterThanOrEqual(decimal.New(1, 9)) {
			return n, "exchange_rate must be a positive number"
		}
		r = r.Round(6)
		n.Rate = &r
		n.RateSource = exchangerates.SourceManual
	}
	if in.Treatment != nil {
		n.Treatment = strings.TrimSpace(*in.Treatment)
	}
	if !ValidTreatment(n.Treatment) {
		return n, "treatment must be kdv2 or kdv2_stopaj"
	}
	if in.StopajRate != nil {
		r, err := parseDec(*in.StopajRate)
		if err != nil {
			return n, "stopaj_rate must be a number"
		}
		n.StopajRate = r
	}
	if in.GrossUp != nil {
		n.GrossUp = *in.GrossUp
	}
	if in.VATRate != nil {
		r, err := parseDec(*in.VATRate)
		if err != nil {
			return n, "vat_rate must be a number"
		}
		n.VATRate = r
	}
	if n.Treatment == TreatmentKDV2 {
		n.StopajRate, n.GrossUp = decimal.Zero, false
	}
	if reason := checkRates(n.Treatment, n.StopajRate, n.VATRate); reason != "" {
		return n, reason
	}
	n.StopajRate, n.VATRate = n.StopajRate.Round(2), n.VATRate.Round(2)
	return n, ""
}

func checkRates(treatment string, stopaj, vat decimal.Decimal) string {
	if stopaj.IsNegative() || stopaj.GreaterThanOrEqual(hundred) {
		return "stopaj_rate must be between 0 and 99.99"
	}
	if treatment == TreatmentKDV2Stopaj && stopaj.IsZero() {
		return "stopaj_rate is required with kdv2_stopaj"
	}
	if vat.IsNegative() || vat.GreaterThan(hundred) {
		return "vat_rate must be between 0 and 100"
	}
	return ""
}

// enabled reports whether the office has the section on (default on).
func (s *Service) enabled(ctx context.Context, orgID int64) (bool, error) {
	row, err := s.q.GetForeignInvoiceSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("foreign invoice settings: %w", err)
	}
	return row.Enabled, nil
}

// kindDefaults merges the office's rows onto the ready-made defaults.
func (s *Service) kindDefaults(ctx context.Context, orgID int64) (map[string]KindSetting, error) {
	rows, err := s.q.ListForeignServiceKindSettings(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("foreign service kinds: %w", err)
	}
	out := make(map[string]KindSetting, len(Kinds))
	for _, k := range Kinds {
		out[k.Kind] = KindSetting{
			ServiceKind: k.Kind, Label: k.Label, Treatment: k.Treatment, StopajRate: k.StopajRate.String(),
			GrossUp: k.GrossUp, VATRate: k.VATRate.String(), Enabled: true,
		}
	}
	for _, r := range rows {
		k, ok := out[r.ServiceKind]
		if !ok {
			continue
		}
		k.Treatment, k.StopajRate, k.GrossUp = r.Treatment, dec(r.StopajRate).String(), r.GrossUp
		k.VATRate, k.Enabled, k.Customized = dec(r.VatRate).String(), r.Enabled, true
		out[r.ServiceKind] = k
	}
	return out, nil
}

func defaultFrom(k KindSetting) KindDefault {
	return KindDefault{
		Kind: k.ServiceKind, Label: k.Label, Treatment: k.Treatment, GrossUp: k.GrossUp,
		StopajRate: decimal.RequireFromString(k.StopajRate), VATRate: decimal.RequireFromString(k.VATRate),
	}
}

// List returns the engagement's invoices of period (current month when
// empty) with the TL totals.
func (s *Service) List(ctx context.Context, sub Subject, period string) (List, error) {
	if period == "" {
		period = CurrentPeriod(s.now())
	}
	if !ValidPeriod(period) {
		return List{}, fmt.Errorf("%w: period must be YYYY-MM", ErrInvalid)
	}
	rows, err := s.q.ListForeignInvoices(ctx, db.ListForeignInvoicesParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Period: period,
	})
	if err != nil {
		return List{}, fmt.Errorf("list foreign invoices: %w", err)
	}
	out := List{Period: period, Items: make([]Item, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, itemFromRow(r))
	}
	out.Totals = totalsOf(out.Items)
	return out, nil
}

// totalsOf sums the TL amounts of the items with a kur.
func totalsOf(items []Item) Totals {
	t := Totals{Count: len(items)}
	gross, kdv2, stopaj := decimal.Zero, decimal.Zero, decimal.Zero
	for _, it := range items {
		if it.Status == StatusConfirmed {
			t.Confirmed++
		} else {
			t.Draft++
		}
		if it.MissingRate {
			t.MissingRate++
			continue
		}
		gross = gross.Add(decimal.RequireFromString(*it.GrossTRY))
		kdv2 = kdv2.Add(decimal.RequireFromString(*it.KDV2TRY))
		stopaj = stopaj.Add(decimal.RequireFromString(*it.StopajTRY))
	}
	t.GrossTRY, t.KDV2TRY, t.StopajTRY = gross.StringFixed(2), kdv2.StringFixed(2), stopaj.StringFixed(2)
	return t
}

// Create adds a draft invoice.
func (s *Service) Create(ctx context.Context, sub Subject, in Input, userID int64) (Item, error) {
	on, err := s.enabled(ctx, sub.OrganizationID)
	if err != nil {
		return Item{}, err
	}
	if !on {
		return Item{}, ErrDisabled
	}
	kinds, err := s.kindDefaults(ctx, sub.OrganizationID)
	if err != nil {
		return Item{}, err
	}
	k, ok := kinds[strings.TrimSpace(in.ServiceKind)]
	if !ok {
		return Item{}, fmt.Errorf("%w: service_kind is unknown", ErrInvalid)
	}
	n, reason := normalize(in, defaultFrom(k))
	if reason != "" {
		return Item{}, fmt.Errorf("%w: %s", ErrInvalid, reason)
	}
	if err := s.autoRate(ctx, sub.OrganizationID, &n); err != nil {
		return Item{}, err
	}
	row, err := s.q.CreateForeignInvoice(ctx, db.CreateForeignInvoiceParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Period: n.Period,
		Vendor: n.Vendor, Country: n.Country, InvoiceNo: n.InvoiceNo, InvoiceDate: date(n.InvoiceDate),
		Amount: num(n.Amount), Currency: n.Currency, ExchangeRate: optNum(n.Rate), ServiceKind: n.ServiceKind,
		Treatment: n.Treatment, StopajRate: num(n.StopajRate), GrossUp: n.GrossUp, VatRate: num(n.VATRate),
		Note: text(n.Note), CreatedBy: optID(userID),
		ExchangeRateSource: text(n.RateSource), ExchangeRateDate: date(n.RateDate),
	})
	if err != nil {
		return Item{}, fmt.Errorf("create foreign invoice: %w", err)
	}
	return itemFromRow(row), nil
}

func (s *Service) get(ctx context.Context, sub Subject, id uuid.UUID) (db.ForeignServiceInvoice, error) {
	row, err := s.q.GetForeignInvoice(ctx, db.GetForeignInvoiceParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get foreign invoice: %w", err)
	}
	return row, nil
}

// inputOf turns a stored row back into an input (for PATCH).
func inputOf(r db.ForeignServiceInvoice) Input {
	in := Input{
		Period: r.Period, Vendor: r.Vendor, Country: r.Country, InvoiceNo: r.InvoiceNo,
		Amount: dec(r.Amount).String(), Currency: r.Currency, ServiceKind: r.ServiceKind, Note: r.Note.String,
	}
	if r.InvoiceDate.Valid {
		in.InvoiceDate = r.InvoiceDate.Time.Format("2006-01-02")
	}
	if r.ExchangeRate.Valid {
		v := dec(r.ExchangeRate).String()
		in.ExchangeRate = &v
	}
	tr, sr, vr, gu := r.Treatment, dec(r.StopajRate).String(), dec(r.VatRate).String(), r.GrossUp
	in.Treatment, in.StopajRate, in.VATRate, in.GrossUp = &tr, &sr, &vr, &gu
	return in
}

func apply(in Input, p Patch) Input {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&in.Period, p.Period)
	set(&in.Vendor, p.Vendor)
	set(&in.Country, p.Country)
	set(&in.InvoiceNo, p.InvoiceNo)
	set(&in.InvoiceDate, p.InvoiceDate)
	set(&in.Amount, p.Amount)
	set(&in.Currency, p.Currency)
	set(&in.ServiceKind, p.ServiceKind)
	set(&in.Note, p.Note)
	if p.ExchangeRate.Set {
		in.ExchangeRate = p.ExchangeRate.Value
	}
	if p.Treatment != nil {
		in.Treatment = p.Treatment
	}
	if p.StopajRate != nil {
		in.StopajRate = p.StopajRate
	}
	if p.GrossUp != nil {
		in.GrossUp = p.GrossUp
	}
	if p.VATRate != nil {
		in.VATRate = p.VATRate
	}
	return in
}

// Update changes a draft invoice; a confirmed one answers ErrConflict.
func (s *Service) Update(ctx context.Context, sub Subject, id uuid.UUID, p Patch) (Item, error) {
	cur, err := s.get(ctx, sub, id)
	if err != nil {
		return Item{}, err
	}
	if cur.Status != StatusDraft {
		return Item{}, fmt.Errorf("%w: the invoice is confirmed; undo the confirmation first", ErrConflict)
	}
	in := apply(inputOf(cur), p)
	keep := keepStoredRate(p.ExchangeRate.Set, cur.ExchangeRate.Valid, cur.ExchangeRateSource.String)
	if !p.ExchangeRate.Set && !keep {
		in.ExchangeRate = nil // an automatic rate follows the new date / currency
	}
	k, ok := DefaultOf(strings.TrimSpace(in.ServiceKind))
	if !ok {
		return Item{}, fmt.Errorf("%w: service_kind is unknown", ErrInvalid)
	}
	n, reason := normalize(in, k)
	if reason != "" {
		return Item{}, fmt.Errorf("%w: %s", ErrInvalid, reason)
	}
	if keep && n.Currency != "TRY" {
		// The stored rate stays with its label (entered by hand, or older).
		n.RateSource = cur.ExchangeRateSource.String
	}
	if err := s.autoRate(ctx, sub.OrganizationID, &n); err != nil {
		return Item{}, err
	}
	row, err := s.q.UpdateForeignInvoice(ctx, db.UpdateForeignInvoiceParams{
		Period: n.Period, Vendor: n.Vendor, Country: n.Country, InvoiceNo: n.InvoiceNo,
		InvoiceDate: date(n.InvoiceDate), Amount: num(n.Amount), Currency: n.Currency,
		ExchangeRate: optNum(n.Rate), ServiceKind: n.ServiceKind, Treatment: n.Treatment,
		StopajRate: num(n.StopajRate), GrossUp: n.GrossUp, VatRate: num(n.VATRate), Note: text(n.Note),
		ExchangeRateSource: text(n.RateSource), ExchangeRateDate: date(n.RateDate),
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, fmt.Errorf("%w: the invoice is confirmed; undo the confirmation first", ErrConflict)
	}
	if err != nil {
		return Item{}, fmt.Errorf("update foreign invoice: %w", err)
	}
	return itemFromRow(row), nil
}

// Delete removes a draft invoice.
func (s *Service) Delete(ctx context.Context, sub Subject, id uuid.UUID) error {
	cur, err := s.get(ctx, sub, id)
	if err != nil {
		return err
	}
	if cur.Status != StatusDraft {
		return fmt.Errorf("%w: the invoice is confirmed; undo the confirmation first", ErrConflict)
	}
	n, err := s.q.DeleteForeignInvoice(ctx, db.DeleteForeignInvoiceParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
	})
	if err != nil {
		return fmt.Errorf("delete foreign invoice: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Confirm marks a draft invoice confirmed by the office. A row without a
// kur cannot be confirmed.
func (s *Service) Confirm(ctx context.Context, sub Subject, id uuid.UUID, userID int64) (Item, error) {
	cur, err := s.get(ctx, sub, id)
	if err != nil {
		return Item{}, err
	}
	if cur.Status == StatusConfirmed {
		return Item{}, fmt.Errorf("%w: the invoice is already confirmed", ErrConflict)
	}
	if cur.Vendor == "" || !countryRe.MatchString(strings.TrimSpace(cur.Country)) || !dec(cur.Amount).IsPositive() {
		// A draft from a taxpayer file is completed by the office first (#780 A2).
		return Item{}, fmt.Errorf("%w: enter the vendor, country and amount before confirming", ErrInvalid)
	}
	if !cur.ExchangeRate.Valid {
		return Item{}, fmt.Errorf("%w: enter the exchange rate before confirming", ErrInvalid)
	}
	row, err := s.q.ConfirmForeignInvoice(ctx, db.ConfirmForeignInvoiceParams{
		ConfirmedBy: optID(userID), OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, fmt.Errorf("%w: the invoice changed, reload", ErrConflict)
	}
	if err != nil {
		return Item{}, fmt.Errorf("confirm foreign invoice: %w", err)
	}
	return itemFromRow(row), nil
}

// Unconfirm takes a confirmed invoice back to draft.
func (s *Service) Unconfirm(ctx context.Context, sub Subject, id uuid.UUID) (Item, error) {
	cur, err := s.get(ctx, sub, id)
	if err != nil {
		return Item{}, err
	}
	if cur.Status != StatusConfirmed {
		return Item{}, fmt.Errorf("%w: the invoice is not confirmed", ErrConflict)
	}
	row, err := s.q.UnconfirmForeignInvoice(ctx, db.UnconfirmForeignInvoiceParams{
		OrganizationID: sub.OrganizationID, EngagementID: sub.EngagementID, Uuid: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, fmt.Errorf("%w: the invoice changed, reload", ErrConflict)
	}
	if err != nil {
		return Item{}, fmt.Errorf("unconfirm foreign invoice: %w", err)
	}
	return itemFromRow(row), nil
}

// Settings returns the office's switch and service kinds.
func (s *Service) Settings(ctx context.Context, orgID int64) (Settings, error) {
	on, err := s.enabled(ctx, orgID)
	if err != nil {
		return Settings{}, err
	}
	kinds, err := s.kindDefaults(ctx, orgID)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{Enabled: on, Items: make([]KindSetting, 0, len(Kinds))}
	for _, k := range Kinds {
		out.Items = append(out.Items, kinds[k.Kind])
	}
	return out, nil
}

// SaveSettings stores the switch and, when given, the service kinds.
func (s *Service) SaveSettings(ctx context.Context, orgID int64, in SettingsInput, userID int64) (Settings, error) {
	type row struct {
		kind, treatment string
		stopaj, vat     decimal.Decimal
		grossUp, on     bool
	}
	rows := make([]row, 0, len(in.Items))
	seen := map[string]bool{}
	for _, it := range in.Items {
		kind := strings.TrimSpace(it.ServiceKind)
		if _, ok := DefaultOf(kind); !ok || seen[kind] {
			return Settings{}, fmt.Errorf("%w: service_kind %q is unknown or repeated", ErrInvalid, kind)
		}
		seen[kind] = true
		if !ValidTreatment(it.Treatment) {
			return Settings{}, fmt.Errorf("%w: treatment must be kdv2 or kdv2_stopaj", ErrInvalid)
		}
		stopaj, err := parseDec(it.StopajRate)
		if err != nil {
			return Settings{}, fmt.Errorf("%w: stopaj_rate must be a number", ErrInvalid)
		}
		vat, err := parseDec(it.VATRate)
		if err != nil {
			return Settings{}, fmt.Errorf("%w: vat_rate must be a number", ErrInvalid)
		}
		grossUp := it.GrossUp
		if it.Treatment == TreatmentKDV2 {
			stopaj, grossUp = decimal.Zero, false
		}
		if reason := checkRates(it.Treatment, stopaj, vat); reason != "" {
			return Settings{}, fmt.Errorf("%w: %s", ErrInvalid, reason)
		}
		rows = append(rows, row{kind, it.Treatment, stopaj.Round(2), vat.Round(2), grossUp, it.Enabled})
	}
	if _, err := s.q.UpsertForeignInvoiceSettings(ctx, db.UpsertForeignInvoiceSettingsParams{
		OrganizationID: orgID, Enabled: in.Enabled, UpdatedBy: optID(userID),
	}); err != nil {
		return Settings{}, fmt.Errorf("save foreign invoice settings: %w", err)
	}
	for _, r := range rows {
		if err := s.q.UpsertForeignServiceKindSetting(ctx, db.UpsertForeignServiceKindSettingParams{
			OrganizationID: orgID, ServiceKind: r.kind, Treatment: r.treatment, StopajRate: num(r.stopaj),
			GrossUp: r.grossUp, VatRate: num(r.vat), Enabled: r.on, UpdatedBy: optID(userID),
		}); err != nil {
			return Settings{}, fmt.Errorf("save foreign service kind: %w", err)
		}
	}
	return s.Settings(ctx, orgID)
}

// keepStoredRate reports whether a PATCH keeps the stored rate as it is.
// A rate the patch sets (or clears) is the office's word. Otherwise a rate
// entered by hand, or stored before automatic rates, stays; an automatic
// TCMB rate is looked up again (the date or currency may have changed).
func keepStoredRate(patchSetsRate, hasRate bool, source string) bool {
	if patchSetsRate {
		return false
	}
	return hasRate && !exchangerates.MayAutoFill(hasRate, source)
}

// autoRate fills a missing rate from the TCMB table (#780): the last
// bulletin before the invoice date, of the office's rate kind. Without an
// invoice date or a fitting bulletin the kur stays missing.
func (s *Service) autoRate(ctx context.Context, orgID int64, n *normalized) error {
	if n.Rate != nil || n.Currency == "TRY" || n.InvoiceDate == nil {
		return nil
	}
	r, d, ok, err := exchangerates.Lookup(ctx, s.q, orgID, n.Currency, *n.InvoiceDate)
	if err != nil || !ok {
		return err
	}
	n.Rate, n.RateSource, n.RateDate = &r, exchangerates.SourceTCMB, &d
	return nil
}

// itemFromRow turns a row into an item with the calculation.
func itemFromRow(r db.ForeignServiceInvoice) Item {
	t := Terms{
		Amount: dec(r.Amount), Treatment: r.Treatment, StopajRate: dec(r.StopajRate),
		GrossUp: r.GrossUp, VATRate: dec(r.VatRate),
	}
	it := Item{
		UUID: r.Uuid, Period: r.Period, Vendor: r.Vendor, Country: strings.TrimSpace(r.Country), InvoiceNo: r.InvoiceNo,
		HasFile: r.SourceUuid.Valid, Prefilled: r.Prefilled,
		Amount: t.Amount.StringFixed(2), Currency: r.Currency, ServiceKind: r.ServiceKind,
		Treatment: r.Treatment, StopajRate: t.StopajRate.String(), GrossUp: r.GrossUp,
		VATRate: t.VATRate.String(), Status: r.Status, CreatedAt: r.CreatedAt.Time,
	}
	if k, ok := DefaultOf(r.ServiceKind); ok {
		it.ServiceKindLabel = k.Label
	}
	if r.InvoiceDate.Valid {
		s := r.InvoiceDate.Time.Format("2006-01-02")
		it.InvoiceDate = &s
	}
	if r.ExchangeRate.Valid {
		rate := dec(r.ExchangeRate)
		t.Rate = &rate
		s := rate.String()
		it.ExchangeRate = &s
		if src := r.ExchangeRateSource.String; src != "" && r.Currency != "TRY" {
			it.ExchangeRateSource = &src
			if src == exchangerates.SourceTCMB && r.ExchangeRateDate.Valid {
				d := r.ExchangeRateDate.Time.Format("2006-01-02")
				it.ExchangeRateDate = &d
			}
		}
	}
	res := Compute(t)
	it.Gross, it.Stopaj, it.KDV2 = res.Gross.StringFixed(2), res.Stopaj.StringFixed(2), res.KDV2.StringFixed(2)
	it.AmountTRY, it.GrossTRY, it.StopajTRY, it.KDV2TRY = fixed(res.AmountTRY), fixed(res.GrossTRY), fixed(res.StopajTRY), fixed(res.KDV2TRY)
	it.MissingRate = res.MissingRate
	if r.SourceKind.Valid {
		s := r.SourceKind.String
		it.SourceKind = &s
	}
	if r.SourceUuid.Valid {
		u := uuid.UUID(r.SourceUuid.Bytes)
		it.SourceUUID = &u
	}
	if r.Note.Valid && r.Note.String != "" {
		s := r.Note.String
		it.Note = &s
	}
	if r.ConfirmedAt.Valid {
		c := r.ConfirmedAt.Time
		it.ConfirmedAt = &c
	}
	return it
}

func fixed(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(2)
	return &s
}

func dec(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func num(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func optNum(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	return num(*d)
}

func date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func optID(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: id > 0} }

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
