package facts

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestRender(t *testing.T) {
	cases := map[string]string{
		Render(d("1234567.891"), UnitTRY): "1.234.567,89 TL",
		Render(d("12"), UnitTRY):          "12,00 TL",
		Render(d("12.46"), UnitPercent):   "%12,5",
		Render(d("1500"), UnitCount):      "1.500",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}

func TestTemplateFillVerify(t *testing.T) {
	fs := []Fact{
		New("F1", "satış", d("125000.50"), UnitTRY, Source{}),
		New("F2", "fatura", d("14"), UnitCount, Source{}),
		New("F3", "değişim", d("-12.46"), UnitPercent, Source{}),
	}
	tpl := "Ekim döneminde {{F2}} satış faturasıyla {{F1}} matrah oluştu; önceki aya göre {{F3}} azaldı."
	if err := CheckTemplate(tpl, fs); err != nil {
		t.Fatal(err)
	}
	body := Fill(tpl, fs)
	if body != "Ekim döneminde 14 satış faturasıyla 125.000,50 TL matrah oluştu; önceki aya göre %12,5 azaldı." {
		t.Fatalf("body %q", body)
	}
	if bad := Verify(body+" (Ekim 2026, 10. ay)", fs, "2026-10"); len(bad) != 0 {
		t.Fatalf("unexpected unmatched %v", bad)
	}
	// An edited body with a made-up number is caught.
	if bad := Verify("Satışlar 130.000 TL oldu, 14 fatura.", fs, "2026-10"); len(bad) != 1 || bad[0] != "130.000" {
		t.Fatalf("bad = %v", bad)
	}
	for _, wrong := range []string{"{{F9}} arttı", "Satış 120 bin TL oldu {{F1}}", "rakam yok"} {
		if CheckTemplate(wrong, fs) == nil {
			t.Errorf("template accepted: %q", wrong)
		}
	}
	if Sorted([]Fact{{ID: "F10"}, {ID: "F2"}})[0].ID != "F2" {
		t.Fatal("sort")
	}
}
