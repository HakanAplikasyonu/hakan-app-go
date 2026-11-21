package foreigninvoices

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ptr(s string) *decimal.Decimal { v := d(s); return &v }

func str(p *decimal.Decimal) string {
	if p == nil {
		return "<nil>"
	}
	return p.StringFixed(2)
}

func TestComputeGrossUpLicence(t *testing.T) {
	// Görevdeki örnek: 100 USD net, %15 stopaj, brütleştirme, KDV %20, kur 34,50.
	r := Compute(Terms{Amount: d("100"), Treatment: TreatmentKDV2Stopaj, StopajRate: d("15"), GrossUp: true, VATRate: d("20"), Rate: ptr("34.50")})
	for name, got := range map[string]string{
		"gross": r.Gross.StringFixed(2), "stopaj": r.Stopaj.StringFixed(2), "kdv2": r.KDV2.StringFixed(2),
		"amount_try": str(r.AmountTRY), "gross_try": str(r.GrossTRY), "stopaj_try": str(r.StopajTRY), "kdv2_try": str(r.KDV2TRY),
	} {
		want := map[string]string{
			"gross": "117.65", "stopaj": "17.65", "kdv2": "23.53",
			"amount_try": "3450.00", "gross_try": "4058.93", "stopaj_try": "608.84", "kdv2_try": "811.79",
		}[name]
		if got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	if r.MissingRate {
		t.Error("missing rate with a rate")
	}
}

func TestComputeOnlyKDV2(t *testing.T) {
	// Yalnız KDV2: stopaj oranı ve brütleştirme yok sayılır, brüt = tutar.
	r := Compute(Terms{Amount: d("250.40"), Treatment: TreatmentKDV2, StopajRate: d("15"), GrossUp: true, VATRate: d("20"), Rate: ptr("35.1234")})
	if r.Gross.StringFixed(2) != "250.40" || !r.Stopaj.IsZero() || r.KDV2.StringFixed(2) != "50.08" {
		t.Fatalf("got gross %s stopaj %s kdv2 %s", r.Gross, r.Stopaj, r.KDV2)
	}
	// 250,40 × 35,1234 = 8794,899... → 8794,90; KDV2 1758,98.
	if str(r.GrossTRY) != "8794.90" || str(r.KDV2TRY) != "1758.98" || str(r.StopajTRY) != "0.00" {
		t.Fatalf("got TL gross %s kdv2 %s stopaj %s", str(r.GrossTRY), str(r.KDV2TRY), str(r.StopajTRY))
	}
}

func TestComputeStopajWithoutGrossUp(t *testing.T) {
	r := Compute(Terms{Amount: d("1000"), Treatment: TreatmentKDV2Stopaj, StopajRate: d("20"), VATRate: d("20"), Rate: ptr("1")})
	if r.Gross.StringFixed(2) != "1000.00" || r.Stopaj.StringFixed(2) != "200.00" || r.KDV2.StringFixed(2) != "200.00" {
		t.Fatalf("got gross %s stopaj %s kdv2 %s", r.Gross, r.Stopaj, r.KDV2)
	}
}

func TestComputeMissingRate(t *testing.T) {
	r := Compute(Terms{Amount: d("49.99"), Treatment: TreatmentKDV2, VATRate: d("20")})
	if !r.MissingRate || r.GrossTRY != nil || r.KDV2TRY != nil || r.StopajTRY != nil || r.AmountTRY != nil {
		t.Fatalf("missing rate: %+v", r)
	}
	// 49,99 × %20 = 9,998 → 10,00 (kuruşa, yarım yukarı).
	if r.KDV2.StringFixed(2) != "10.00" {
		t.Fatalf("kdv2 = %s", r.KDV2)
	}
}

func TestComputeHalfKurusRoundsUp(t *testing.T) {
	// 0,125 × 20 / 100 = 0,025 → 0,03 (yarım kuruş sıfırdan uzağa).
	r := Compute(Terms{Amount: d("0.125"), Treatment: TreatmentKDV2, VATRate: d("20"), Rate: ptr("1")})
	if r.Gross.StringFixed(2) != "0.13" {
		t.Fatalf("gross = %s", r.Gross)
	}
	if got := pct(d("0.125"), d("20")).StringFixed(2); got != "0.03" {
		t.Fatalf("pct = %s", got)
	}
}

func TestKindsDefaults(t *testing.T) {
	want := map[string]struct {
		treatment string
		rate      string
		grossUp   bool
	}{
		KindSoftware:    {TreatmentKDV2Stopaj, "15", true},
		KindAdvertising: {TreatmentKDV2, "0", false},
		KindCloud:       {TreatmentKDV2, "0", false},
		KindConsulting:  {TreatmentKDV2Stopaj, "20", false},
		KindOther:       {TreatmentKDV2, "0", false},
	}
	if len(Kinds) != len(want) {
		t.Fatalf("kinds = %d", len(Kinds))
	}
	for _, k := range Kinds {
		w := want[k.Kind]
		if k.Treatment != w.treatment || k.StopajRate.String() != w.rate || k.GrossUp != w.grossUp || k.VATRate.String() != "20" || k.Label == "" {
			t.Errorf("%s = %+v", k.Kind, k)
		}
	}
	if _, ok := DefaultOf("yok"); ok {
		t.Error("unknown kind found")
	}
}

func TestNormalizeInput(t *testing.T) {
	kind, _ := DefaultOf(KindAdvertising)
	in := Input{
		Period: "2026-11", Vendor: " Google Ireland Ltd. ", Country: "ie", Amount: "100", Currency: "try",
		ServiceKind: KindAdvertising,
	}
	got, reason := normalize(in, kind)
	if reason != "" {
		t.Fatal(reason)
	}
	if got.Vendor != "Google Ireland Ltd." || got.Country != "IE" || got.Currency != "TRY" || got.Rate == nil || got.Rate.String() != "1" {
		t.Fatalf("normalized = %+v", got)
	}
	if got.Treatment != TreatmentKDV2 || !got.StopajRate.IsZero() || got.VATRate.String() != "20" {
		t.Fatalf("defaults = %+v", got)
	}
	for _, bad := range []Input{
		{Period: "2026-13", Vendor: "x", Country: "IE", Amount: "1", Currency: "USD", ServiceKind: KindAdvertising},
		{Period: "2026-11", Vendor: "", Country: "IE", Amount: "1", Currency: "USD", ServiceKind: KindAdvertising},
		{Period: "2026-11", Vendor: "x", Country: "IRL", Amount: "1", Currency: "USD", ServiceKind: KindAdvertising},
		{Period: "2026-11", Vendor: "x", Country: "IE", Amount: "0", Currency: "USD", ServiceKind: KindAdvertising},
		{Period: "2026-11", Vendor: "x", Country: "IE", Amount: "1", Currency: "US", ServiceKind: KindAdvertising},
		{Period: "2026-11", Vendor: "x", Country: "IE", Amount: "1", Currency: "USD", ServiceKind: KindAdvertising, ExchangeRate: sp("-1")},
		{Period: "2026-11", Vendor: "x", Country: "IE", Amount: "1", Currency: "USD", ServiceKind: KindAdvertising, InvoiceDate: "2026-02-30"},
	} {
		if _, reason := normalize(bad, kind); reason == "" {
			t.Errorf("accepted %+v", bad)
		}
	}
	// Yalnız KDV2 seçilince stopaj oranı ve brütleştirme sıfırlanır.
	soft, _ := DefaultOf(KindSoftware)
	tr := TreatmentKDV2
	got, reason = normalize(Input{Period: "2026-11", Vendor: "x", Country: "DE", Amount: "10", Currency: "EUR",
		ServiceKind: KindSoftware, Treatment: &tr}, soft)
	if reason != "" || got.GrossUp || !got.StopajRate.IsZero() {
		t.Fatalf("kdv2 only = %+v %s", got, reason)
	}
}

func sp(s string) *string { return &s }
