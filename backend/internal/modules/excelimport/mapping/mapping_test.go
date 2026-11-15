package mapping

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

func TestMaskCell(t *testing.T) {
	cases := map[string]string{
		"1234567890":                       "1234******",
		"12345678901":                      "1234*******",
		"0532 123 45 67":                   "0532 *** ** **",
		"TR33 0006 1005 1978 6457 8413 26": "TR33 **** **** **** **** **** **",
		"ali.veli@ornek.com":               "a*******@***",
		"Demir Gıda A.Ş.":                  "Xxxxx Xxxx X.X.",
		"1.234,56":                         "1.234,56",
		"15.10.2026":                       "15.10.2026",
		"Satış":                            "Satış",
		"e-Arşiv":                          "e-Arşiv",
		"250,00 TL":                        "250,00 TL",
	}
	for in, want := range cases {
		if got := MaskCell(in); got != want {
			t.Errorf("MaskCell(%q) = %q, want %q", in, got, want)
		}
	}
}

type fakeAI struct {
	answers []string
	calls   []aiusecase.Call
	err     error
}

func (f *fakeAI) Enabled() bool { return true }
func (f *fakeAI) Complete(_ context.Context, c aiusecase.Call) (aiusecase.Result, error) {
	f.calls = append(f.calls, c)
	if f.err != nil {
		return aiusecase.Result{}, f.err
	}
	a := f.answers[0]
	if len(f.answers) > 1 {
		f.answers = f.answers[1:]
	}
	return aiusecase.Result{Message: provider.Message{Role: provider.RoleAssistant, Content: []provider.Block{provider.TextBlock(a)}}}, nil
}

func sheet() (parse.Sheet, parse.Layout) {
	s := parse.Sheet{Name: "S", Rows: [][]string{
		{"Tarih", "Belge No", "Vergi No", "Ünvan", "KDV %20", "Toplam", "Plasiyer"},
		{"15.10.2026", "A-1", "1234567890", "Demir Gıda", "20,00", "120,00", "Ali"},
		{"16.10.2026", "A-2", "9876543210", "Kaya Ltd", "40,00", "240,00", "Veli"},
	}}
	l, _ := parse.DetectLayout(s)
	return s, l
}

const goodAnswer = `Tabii: {"data_type":"outgoing_invoices","source_guess":"bizimhesap","columns":[
{"column":"A","header":"Tarih","field":"issue_date","confidence":0.98},
{"column":"B","header":"Belge No","field":"invoice_number","confidence":0.95},
{"column":"C","header":"Vergi No","field":"customer_tax_number","confidence":0.93},
{"column":"D","header":"Ünvan","field":"customer_name","confidence":0.9},
{"column":"E","header":"KDV %20","field":"tax_summary[20].vat","confidence":0.70},
{"column":"F","header":"Toplam","field":"amounts.payable_amount","confidence":0.88},
{"column":"G","header":"Plasiyer","field":null,"confidence":0.99}],"warnings":[]}`

func TestAIMapperMasksAndFlagsLowConfidence(t *testing.T) {
	ai := &fakeAI{answers: []string{goodAnswer}}
	s, l := sheet()
	mp, err := NewAIMapper(ai).Map(context.Background(), AIRequest{OrganizationID: 1, TaxpayerID: 2, Sheet: s, Layout: l})
	if err != nil {
		t.Fatal(err)
	}
	sent := ai.calls[0].Messages[0].Text()
	for _, leak := range []string{"1234567890", "Demir Gıda", "Kaya"} {
		if strings.Contains(sent, leak) {
			t.Fatalf("%q sent to AI", leak)
		}
	}
	if ai.calls[0].Purpose != "excel_column_mapping" || *ai.calls[0].TaxpayerID != 2 {
		t.Fatalf("call = %+v", ai.calls[0])
	}
	if mp.DataType != OutgoingInvoices || mp.SourceSystem != "bizimhesap" || len(mp.Columns) != 7 {
		t.Fatalf("mapping = %+v", mp)
	}
	var low []string
	for _, c := range mp.Columns {
		if c.NeedsReview {
			low = append(low, c.Column)
		}
	}
	if len(low) != 1 || low[0] != "E" || !mp.NeedsReview() {
		t.Fatalf("needs review = %v", low)
	}
	if len(MissingRequired(mp.DataType, mp)) != 0 {
		t.Fatal("required fields reported missing")
	}
}

func TestAIMapperRetriesOnceThenGivesUp(t *testing.T) {
	s, l := sheet()
	ai := &fakeAI{answers: []string{"not json", goodAnswer}}
	mp, err := NewAIMapper(ai).Map(context.Background(), AIRequest{OrganizationID: 1, Sheet: s, Layout: l})
	if err != nil || len(ai.calls) != 2 || mp.Columns[0].Field != "issue_date" {
		t.Fatalf("retry: %v calls=%d", err, len(ai.calls))
	}
	ai = &fakeAI{answers: []string{`{"data_type":"outgoing_invoices","columns":[{"column":"A","field":"bogus","confidence":0.9}]}`}}
	mp, err = NewAIMapper(ai).Map(context.Background(), AIRequest{OrganizationID: 1, DataType: OutgoingInvoices, Sheet: s, Layout: l})
	if err != nil || len(ai.calls) != 2 || len(mp.Warnings) != 1 || !mp.NeedsReview() {
		t.Fatalf("give up: %v calls=%d %+v", err, len(ai.calls), mp)
	}
	for _, c := range mp.Columns {
		if c.Field != "" {
			t.Fatal("give-up mapping must be empty")
		}
	}
	if _, err := NewAIMapper(&fakeAI{err: errors.New("down")}).Map(context.Background(), AIRequest{Sheet: s, Layout: l}); err == nil {
		t.Fatal("provider error swallowed")
	}
}

func TestMappingValidate(t *testing.T) {
	m := Mapping{DataType: OutgoingInvoices, Columns: []ColumnMap{{Column: "A", Field: "issue_date"}, {Column: "B", Field: "issue_date"}}}
	if m.Validate() == nil {
		t.Fatal("duplicate field accepted")
	}
	m.Columns[1].Field = "tax_summary[10].base"
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if r, p, ok := TaxSummaryField("tax_summary[1].vat"); !ok || r != "1" || p != "vat" {
		t.Fatal("tax summary field")
	}
	if (Mapping{DataType: "bank_statement"}).Validate() == nil {
		t.Fatal("unsupported data type accepted")
	}
}
