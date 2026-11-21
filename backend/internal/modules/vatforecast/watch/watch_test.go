package watch

import (
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func forecast(payable, carryOut string) usecase.Forecast {
	return usecase.Forecast{
		Period: "2026-11",
		Totals: usecase.Totals{
			OutputVAT: usecase.M(d("18000")), InputVAT: usecase.M(d("20000")), CarriedIn: usecase.M(d("0")),
			Payable: usecase.M(d(payable)), CarryOut: usecase.M(d(carryOut)),
		},
		Calendar: usecase.Calendar{DueDate: "2026-12-28"},
	}
}

// Toplantı örneği (21.11): ayın 1'inde mükellefe "bu ay ödeme çıkmayacak"
// denir (2.000 TL devreden). 3'ünde SSK'ya kesilen fatura satış KDV'sini
// artırır; şimdi 2.400 TL ödeme çıkıyor. Nöbetçi "durum değişti" der.
func TestDetectStatusChangedMeetingExample(t *testing.T) {
	eng := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	told := &LastNotice{
		UUID: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Outcome: "no_payment",
		Payable: decimal.Zero, NotifiedAt: time.Date(2026, 11, 1, 9, 0, 0, 0, usecase.Istanbul),
	}
	// Before the SSK invoice: still no payment, nothing raised.
	if got := Detect(eng, forecast("0", "2000"), nil, told); len(got) != 0 {
		t.Fatalf("unchanged status raised %v", got)
	}
	after := forecast("2400", "0")
	got := Detect(eng, after, nil, told)
	if len(got) != 1 || got[0].Event != events.VATStatusChanged {
		t.Fatalf("got %+v, want one status change", got)
	}
	if got[0].Dedupe != "vat.status_changed:"+eng.String()+":2026-11:"+told.UUID.String() {
		t.Fatalf("dedupe = %s", got[0].Dedupe)
	}
	if got[0].Extra["previous_outcome"] != "no_payment" || got[0].Extra["notice_uuid"] != told.UUID.String() {
		t.Fatalf("extra = %v", got[0].Extra)
	}
	title, body := Text("SSK Tedarik A.Ş.", after, got[0])
	if title != "KDV durumu değişti: SSK Tedarik A.Ş." {
		t.Fatalf("title = %q", title)
	}
	for _, want := range []string{"Kasım 2026", "1 Kasım 2026 günü", "\"ödeme çıkmayacak\"", "2.400,00 TL KDV ödemesi çıkıyor", "yeni bilgi"} {
		if !strings.Contains(body, want) {
			t.Errorf("body %q lacks %q", body, want)
		}
	}
}

func TestDetectNoNoticeNoChange(t *testing.T) {
	if got := Detect(uuid.New(), forecast("2400", "0"), nil, nil); len(got) != 0 {
		t.Fatalf("raised without a notice: %v", got)
	}
}

func TestDetectPaymentToNoPayment(t *testing.T) {
	told := &LastNotice{UUID: uuid.New(), Outcome: "payment", Payable: d("900")}
	got := Detect(uuid.New(), forecast("0", "300"), nil, told)
	if len(got) != 1 || got[0].Event != events.VATStatusChanged {
		t.Fatalf("got %+v", got)
	}
	_, body := Text("X", forecast("0", "300"), got[0])
	if !strings.Contains(body, "\"ödeme çıkacak\"") || !strings.Contains(body, "ödemesi çıkmıyor") {
		t.Fatalf("body = %q", body)
	}
}

func TestDetectThreshold(t *testing.T) {
	limit := d("2000")
	eng := uuid.New()
	got := Detect(eng, forecast("2400", "0"), &limit, nil)
	if len(got) != 1 || got[0].Event != events.VATThresholdExceeded || got[0].Extra["limit"] != "2000.00" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Dedupe != "vat.threshold_exceeded:"+eng.String()+":2026-11" {
		t.Fatalf("dedupe = %s", got[0].Dedupe)
	}
	title, body := Text("X", forecast("2400", "0"), got[0])
	if title != "KDV eşiği aşıldı: X" || !strings.Contains(body, "2.400,00 TL") || !strings.Contains(body, "2.000,00 TL") {
		t.Fatalf("text = %q / %q", title, body)
	}
	if got := Detect(eng, forecast("1999.99", "0"), &limit, nil); len(got) != 0 {
		t.Fatalf("under the limit raised %v", got)
	}
}
