// Package foreigninvoices is "Yabancı faturalar / KDV2" (#780 A1): the
// invoices of services bought from abroad (Google Ads, AWS, a foreign
// software licence...). The office collects them per taxpayer and month,
// the sorumlu sıfatıyla KDV (KDV2) and, when the service carries one, the
// withholding (stopaj) are computed here, and the office confirms each row.
// The return itself is filed in Luca; this module only collects and
// computes.
//
// Later work builds on this list: A2 (taxpayer uploads, WhatsApp, AI
// pre-fill; source_kind / source_uuid link the source document), B (journal
// draft and Luca transfer) and C (KDV2 and muhtasar returns fed from it).
//
// Calculation (Compute, pure):
//
//	brüt   = gross_up ? tutar ÷ (1 − stopaj oranı) : tutar   (kdv2_stopaj)
//	brüt   = tutar                                           (yalnız kdv2)
//	stopaj = brüt × stopaj oranı                             (kdv2_stopaj)
//	KDV2   = brüt × KDV oranı
//
// Rounding (kuruş): every amount is rounded to 2 decimals, half away from
// zero (0,005 → 0,01). In the invoice currency the gross is rounded first;
// stopaj and KDV2 are computed on the rounded gross. In TL the matrah is the
// rounded currency gross × kur, rounded; TL stopaj and TL KDV2 are computed
// on that TL matrah and rounded (the return is in TL, so its taxes come from
// the TL matrah). Without a kur the TL amounts are empty and the row counts
// as "kur eksik". A TRY invoice has kur 1.
//
// Example: 100 USD net, %15 stopaj, brütleştirme, KDV %20, kur 34,50 →
// brüt 117,65; stopaj 17,65; KDV2 23,53; TL matrah 4.058,93; TL stopaj
// 608,84; TL KDV2 811,79.
package foreigninvoices

import (
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

// Treatments (000187 chk_foreign_service_invoices_treatment).
const (
	TreatmentKDV2       = "kdv2"
	TreatmentKDV2Stopaj = "kdv2_stopaj"
)

// Statuses.
const (
	StatusDraft     = "draft"
	StatusConfirmed = "confirmed"
)

// Service kinds.
const (
	KindSoftware    = "yazilim_lisans"
	KindAdvertising = "reklam"
	KindCloud       = "bulut"
	KindConsulting  = "danismanlik"
	KindOther       = "diger"
)

// KindDefault is a service kind with its ready-made defaults. The office
// changes them in Ayarlar (foreign_service_kind_settings).
type KindDefault struct {
	Kind       string
	Label      string
	Treatment  string
	StopajRate decimal.Decimal
	GrossUp    bool
	VATRate    decimal.Decimal
}

// Kinds are the ready-made service kinds in display order. Defaults:
//   - yazılım / lisans: KDV2 + %15 stopaj, brütleştirme açık (lisans bedeli
//     çoğu anlaşmada %10–20; yabancı satıcı net ister, stopaj ödeyene kalır);
//   - reklam (Google Ads, Meta) ve bulut (AWS): yalnız KDV2;
//   - danışmanlık: KDV2 + %20 stopaj (KVK 30/1-b serbest meslek), brütleştirme kapalı;
//   - diğer: yalnız KDV2.
//
// KDV oranı hepsinde %20.
var Kinds = []KindDefault{
	{KindSoftware, "Yazılım / lisans", TreatmentKDV2Stopaj, decimal.NewFromInt(15), true, decimal.NewFromInt(20)},
	{KindAdvertising, "Reklam (Google Ads, Meta vb.)", TreatmentKDV2, decimal.Zero, false, decimal.NewFromInt(20)},
	{KindCloud, "Bulut / barındırma (AWS vb.)", TreatmentKDV2, decimal.Zero, false, decimal.NewFromInt(20)},
	{KindConsulting, "Danışmanlık", TreatmentKDV2Stopaj, decimal.NewFromInt(20), false, decimal.NewFromInt(20)},
	{KindOther, "Diğer hizmet", TreatmentKDV2, decimal.Zero, false, decimal.NewFromInt(20)},
}

// DefaultOf returns the ready-made default of kind.
func DefaultOf(kind string) (KindDefault, bool) {
	for _, k := range Kinds {
		if k.Kind == kind {
			return k, true
		}
	}
	return KindDefault{}, false
}

// ValidTreatment reports whether t is a known treatment.
func ValidTreatment(t string) bool { return t == TreatmentKDV2 || t == TreatmentKDV2Stopaj }

var (
	periodRe   = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
	countryRe  = regexp.MustCompile(`^[A-Z]{2}$`)
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	hundred    = decimal.NewFromInt(100)
)

// ValidPeriod reports whether p is YYYY-MM.
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

// Terms are the inputs of the calculation.
type Terms struct {
	Amount     decimal.Decimal
	Treatment  string
	StopajRate decimal.Decimal // percent, 15 = %15
	GrossUp    bool
	VATRate    decimal.Decimal // percent
	// Rate: 1 unit of the invoice currency in TL; nil = kur eksik.
	Rate *decimal.Decimal
}

// Result is the calculated amounts. TL fields are nil without a kur.
type Result struct {
	Gross, Stopaj, KDV2                     decimal.Decimal
	AmountTRY, GrossTRY, StopajTRY, KDV2TRY *decimal.Decimal
	MissingRate                             bool
}

func kurus(d decimal.Decimal) decimal.Decimal { return d.Round(2) }

func pct(d, rate decimal.Decimal) decimal.Decimal { return kurus(d.Mul(rate).Div(hundred)) }

// Compute applies the rules in the package comment.
func Compute(t Terms) Result {
	var r Result
	withStopaj := t.Treatment == TreatmentKDV2Stopaj
	gross := t.Amount
	if withStopaj && t.GrossUp && t.StopajRate.GreaterThan(decimal.Zero) && t.StopajRate.LessThan(hundred) {
		gross = t.Amount.Mul(hundred).Div(hundred.Sub(t.StopajRate))
	}
	r.Gross = kurus(gross)
	if withStopaj {
		r.Stopaj = pct(r.Gross, t.StopajRate)
	}
	r.KDV2 = pct(r.Gross, t.VATRate)
	if t.Rate == nil || !t.Rate.GreaterThan(decimal.Zero) {
		r.MissingRate = true
		return r
	}
	amountTRY := kurus(t.Amount.Mul(*t.Rate))
	grossTRY := kurus(r.Gross.Mul(*t.Rate))
	kdv2TRY := pct(grossTRY, t.VATRate)
	stopajTRY := decimal.Zero
	if withStopaj {
		stopajTRY = pct(grossTRY, t.StopajRate)
	}
	r.AmountTRY, r.GrossTRY, r.StopajTRY, r.KDV2TRY = &amountTRY, &grossTRY, &stopajTRY, &kdv2TRY
	return r
}

// normalizeCode upper-cases and trims a country / currency code.
func normalizeCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
