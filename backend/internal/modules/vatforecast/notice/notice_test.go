package notice

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
)

func m(s string) usecase.Money { return usecase.M(decimal.RequireFromString(s)) }

func TestText(t *testing.T) {
	pay := usecase.Forecast{Period: "2026-11", Totals: usecase.Totals{
		OutputVAT: m("40000"), InputVAT: m("25000"), CarriedIn: m("2500"), Payable: m("12500"),
	}, Calendar: usecase.Calendar{DueDate: "2026-12-28"}}
	title, body := Text(pay, "Demo Mali Müşavirlik", "Sorunuz olursa arayın.")
	if Outcome(pay.Totals) != OutcomePayment || title != "Kasım 2026: KDV ödemeniz çıkıyor" {
		t.Fatalf("title = %q", title)
	}
	for _, want := range []string{"tahmini 12.500,00 TL KDV", "Son ödeme günü 28 Aralık 2026", "Satış KDV'si 40.000,00 TL", "devreden 2.500,00 TL", "Sorunuz olursa arayın.", "Demo Mali Müşavirlik"} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}

	none := usecase.Forecast{Period: "2026-11", Totals: usecase.Totals{OutputVAT: m("1000"), InputVAT: m("3000"), CarryOut: m("2000")}}
	title, body = Text(none, "", "")
	if Outcome(none.Totals) != OutcomeNoPayment || title != "Kasım 2026: KDV ödemeniz çıkmıyor" || !strings.Contains(body, "Sonraki aya devreden tahmini 2.000,00 TL") {
		t.Errorf("no payment = %q %q", title, body)
	}
}

func TestTL(t *testing.T) {
	for in, want := range map[string]string{"0": "0,00", "999.5": "999,50", "1234567.891": "1.234.567,89", "-1000": "-1.000,00"} {
		if got := tl(m(in)); got != want {
			t.Errorf("tl(%s) = %s, want %s", in, got, want)
		}
	}
}
