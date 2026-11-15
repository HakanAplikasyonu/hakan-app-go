package extract

import (
	"context"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
)

const fuelAnswer = `İşte: {"kind":"fuel_receipt","kind_confidence":0.97,"fields":{
"date":{"value":"14.10.2026","confidence":0.95},
"vendor_tax_number":{"value":"123 456 7890","confidence":0.9},
"vendor_name":{"value":"ÖRNEK  PETROL A.Ş.","confidence":0.88},
"receipt_number":{"value":"0042","confidence":0.7},
"total_amount":{"value":"*1.250,40","confidence":0.93},
"tax_amount":{"value":"208,40","confidence":0.6},
"tax_rate":{"value":"%20","confidence":0.9},
"vehicle_plate":{"value":"34 abc 123","confidence":0.55},
"payment_method":{"value":"KREDİ KARTI","confidence":0.8},
"z_report_number":{"value":"","confidence":0}}}`

func TestParseNormalizes(t *testing.T) {
	r, err := Parse(fuelAnswer)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		FieldDate: "2026-10-14", FieldVendorTaxNumber: "1234567890", FieldVendorName: "ÖRNEK PETROL A.Ş.",
		FieldReceiptNumber: "0042", FieldTotal: "1250.40", FieldTax: "208.40", FieldTaxRate: "20",
		FieldPlate: "34ABC123", FieldPaymentMethod: "credit_card",
	}
	for f, v := range want {
		if r.Get(f) != v {
			t.Errorf("%s = %q, want %q", f, r.Get(f), v)
		}
	}
	if _, ok := r.Fields[FieldZNumber]; ok {
		t.Fatal("empty field kept")
	}
	if r.Kind != KindFuel || r.MinConfidence() != 0.55 {
		t.Fatalf("kind %s min %v", r.Kind, r.MinConfidence())
	}
	if d, ok := r.Date(); !ok || d.Day() != 14 {
		t.Fatal("date")
	}
	if _, err := Parse(`{"kind":"letter","fields":{}}`); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestPlate(t *testing.T) {
	for in, want := range map[string]string{"34 ABC 123": "34ABC123", "06-a-1234": "06A1234", "35 ış 99": "35IS99"} {
		if got, ok := NormalizePlate(in); !ok || got != want {
			t.Errorf("%q -> %q %v", in, got, ok)
		}
	}
	for _, bad := range []string{"99ABC123", "ABC123", "34 1234"} {
		if _, ok := NormalizePlate(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

type fakeAI struct {
	answers []string
	calls   []aiusecase.Call
}

func (f *fakeAI) Enabled() bool { return true }
func (f *fakeAI) Complete(_ context.Context, c aiusecase.Call) (aiusecase.Result, error) {
	f.calls = append(f.calls, c)
	a := f.answers[0]
	if len(f.answers) > 1 {
		f.answers = f.answers[1:]
	}
	return aiusecase.Result{Model: "claude-x", Message: provider.Message{Role: provider.RoleAssistant, Content: []provider.Block{provider.TextBlock(a)}}}, nil
}

func TestReadSendsImageAndRetries(t *testing.T) {
	ai := &fakeAI{answers: []string{"oops", fuelAnswer}}
	r, err := New(ai).Read(context.Background(), Request{OrganizationID: 1, TaxpayerID: 2, MediaType: "image/jpeg", Image: []byte("JPEGDATA")})
	if err != nil || len(ai.calls) != 2 || r.Model != "claude-x" {
		t.Fatalf("%v calls=%d", err, len(ai.calls))
	}
	img := ai.calls[0].Messages[0].Content[0]
	if img.Type != provider.BlockImage || img.MediaType != "image/jpeg" || img.Data != "SlBFR0RBVEE=" || ai.calls[0].Purpose != "receipt_ocr" {
		t.Fatalf("image block %+v", img)
	}
	if _, err := New(&fakeAI{answers: []string{"no"}}).Read(context.Background(), Request{MediaType: "image/png", Image: []byte("x")}); err == nil {
		t.Fatal("invalid answers accepted")
	}
}

func TestScore(t *testing.T) {
	exp := []Expected{
		{Name: "a", Kind: KindFuel, Fields: map[string]string{FieldTotal: "1.250,40", FieldPlate: "34 ABC 123", FieldDate: "14.10.2026"}},
		{Name: "b", Kind: KindExpense, Fields: map[string]string{FieldTotal: "10,00"}},
	}
	r1, _ := Parse(fuelAnswer)
	r2, _ := Parse(`{"kind":"expense_receipt","fields":{"total_amount":{"value":"11,00","confidence":0.5}}}`)
	rep := Score(exp, []Result{r1, r2}, 0.8)
	if rep.KindCorrect != 2 {
		t.Fatal("kind")
	}
	var total FieldScore
	for _, f := range rep.Fields {
		if f.Field == FieldTotal {
			total = f
		}
	}
	if total.Samples != 2 || total.Correct != 1 || total.Wrong != 1 || total.LowConfidenceWrong != 1 {
		t.Fatalf("total score %+v", total)
	}
	if md := rep.Markdown(); !strings.Contains(md, "| total_amount | 2 | 1 | 0 | 1 | 1 | %50 |") {
		t.Fatalf("markdown:\n%s", md)
	}
}
