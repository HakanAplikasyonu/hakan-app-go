package statement

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
)

// testdata/banks/<bank>.csv: synthetic exports in each built-in layout
// (title, IBAN, period above the table; a totals line below). They stand in
// for masked real statements until those arrive (docs/00 M6).
var bankFixtures = map[string]struct {
	bank, name string
}{
	"ziraat":      {"ziraat", "Ziraat"},
	"isbank":      {"isbank", "İş Bankası"},
	"garanti":     {"garanti", "Garanti"},
	"yapikredi":   {"yapikredi", "Yapı Kredi"},
	"akbank":      {"akbank", "Akbank"},
	"vakifbank":   {"vakifbank", "VakıfBank"},
	"halkbank":    {"halkbank", "Halkbank"},
	"qnb":         {"qnb", "QNB"},
	"enpara":      {"qnb", "Enpara"},
	"denizbank":   {"denizbank", "DenizBank"},
	"teb":         {"teb", "TEB"},
	"kuveyt_turk": {"kuveyt_turk", "Kuveyt"},
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "banks", name+".csv"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func checkBankStatement(t *testing.T, st Statement, bank, name string) {
	t.Helper()
	if st.Template == nil || st.Template.Source != SourceBuiltin || !strings.Contains(st.Template.Name, name) {
		t.Fatalf("template = %+v, want built-in %s", st.Template, name)
	}
	if st.BankID != bank {
		t.Fatalf("bank = %q, want %q", st.BankID, bank)
	}
	if st.AccountIBAN == "" || (BankFromIBAN(st.AccountIBAN) != bank) {
		t.Fatalf("iban %q not of %s", st.AccountIBAN, bank)
	}
	if len(st.Lines) != 5 || len(st.Errors) != 0 {
		t.Fatalf("lines %d errors %v", len(st.Lines), st.Errors)
	}
	in, out := decimal.Zero, decimal.Zero
	for _, l := range st.Lines {
		a := decimal.RequireFromString(l.Amount)
		if l.Direction == banking.DirIn {
			in = in.Add(a)
		} else {
			out = out.Add(a)
		}
	}
	if !in.Equal(decimal.RequireFromString("12500")) || !out.Equal(decimal.RequireFromString("21679.68")) {
		t.Fatalf("in %s out %s", in, out)
	}
	if g := CheckRunning(st.Lines); g != nil {
		t.Fatalf("running balance: %s", g.Reason)
	}
	if !strings.Contains(st.Lines[0].Description, "YILDIZ YAZILIM") {
		t.Fatalf("description %q", st.Lines[0].Description)
	}
}

func TestBuiltinBankTemplatesCSV(t *testing.T) {
	for file, want := range bankFixtures {
		t.Run(file, func(t *testing.T) {
			sts, format, err := Parse(file+".csv", readFixture(t, file))
			if err != nil {
				t.Fatal(err)
			}
			if format != "csv" || len(sts) != 1 {
				t.Fatalf("format %s, %d statements", format, len(sts))
			}
			checkBankStatement(t, sts[0], want.bank, want.name)
		})
	}
}

func TestBuiltinBankTemplatesExcel(t *testing.T) {
	for _, file := range []string{"ziraat", "halkbank", "kuveyt_turk", "yapikredi"} {
		t.Run(file, func(t *testing.T) {
			rows, err := ReadCSV(string(readFixture(t, file)))
			if err != nil {
				t.Fatal(err)
			}
			f := excelize.NewFile()
			for i, r := range rows {
				for j, c := range r {
					cellName, _ := excelize.CoordinatesToCellName(j+1, i+1)
					_ = f.SetCellStr("Sheet1", cellName, c)
				}
			}
			var buf bytes.Buffer
			if err := f.Write(&buf); err != nil {
				t.Fatal(err)
			}
			sts, format, err := Parse(file+".xlsx", buf.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if format != "excel" {
				t.Fatalf("format %s", format)
			}
			want := bankFixtures[file]
			checkBankStatement(t, sts[0], want.bank, want.name)
		})
	}
}

func TestTemplateFieldsReachTheLine(t *testing.T) {
	sts, _, err := Parse("kuveyt.csv", readFixture(t, "kuveyt_turk"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sts[0].Lines[0].CounterpartyTaxID; got != "9100000021" {
		t.Fatalf("tax id %q", got)
	}
	sts, _, err = Parse("qnb.csv", readFixture(t, "qnb"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sts[0].Lines[0].CounterpartyIBAN; got != "TR290006200000006299001234" {
		t.Fatalf("iban %q", got)
	}
	if sts[0].Lines[0].ValueDate == nil {
		t.Fatal("value date not read")
	}
	// Yapı Kredi's "İşlem" column is not the description (a synonym would
	// have taken it): the template ignores it.
	sts, _, err = Parse("yk.csv", readFixture(t, "yapikredi"))
	if err != nil {
		t.Fatal(err)
	}
	if l := sts[0].Lines[0]; l.Description == "Havale/EFT" || !l.HasTime || l.Channel != "İnternet" {
		t.Fatalf("line %+v", l.Transaction)
	}
}

func TestBuiltinTemplatesAreValidAndDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, tpl := range BuiltinTemplates() {
		if err := tpl.Validate(); err != nil {
			t.Fatalf("%s: %v", tpl.Name, err)
		}
		fp := tpl.Fingerprint()
		if other, ok := seen[fp]; ok {
			t.Fatalf("%s and %s share a fingerprint", tpl.Name, other)
		}
		seen[fp] = tpl.Name
	}
	if len(seen) < 12 {
		t.Fatalf("%d built-in templates", len(seen))
	}
}

const oddCSV = "Firma Hesap Dökümü\n" +
	"Gün;Hareket;Giriş;Çıkış;Kalan\n" +
	"01.10.2026;KIRA ODEMESI EKIM;;15.000,00;85.000,00\n" +
	"02.10.2026;TAHSILAT ABC LTD;4.000,00;;89.000,00\n"

func TestUnknownLayoutAndOfficeTemplate(t *testing.T) {
	if _, _, err := Parse("odd.csv", []byte(oddCSV)); !errors.Is(err, ErrLayoutUnknown) {
		t.Fatalf("err = %v, want ErrLayoutUnknown", err)
	}
	a, err := AnalyzeFile(context.Background(), "odd.csv", []byte(oddCSV), nil, nil, ColumnRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if a.HeaderRow != 2 || len(a.Headers) != 5 || a.Matched != nil {
		t.Fatalf("analysis %+v", a)
	}
	// Heuristics: Giriş → credit, Çıkış → debit; Gün / Hareket / Kalan are
	// not known words and stay for the accountant.
	got := map[string]Field{}
	for _, c := range a.Columns {
		got[c.Header] = c.Field
	}
	if got["Giriş"] != FieldCredit || got["Çıkış"] != FieldDebit {
		t.Fatalf("heuristic %v", got)
	}
	if a.Lines != 0 || len(a.Warnings) == 0 {
		t.Fatalf("an incomplete proposal must warn: %+v", a)
	}
	// The accountant completes and saves; the next upload is read.
	tpl := Template{Format: "csv", Name: "Firma dökümü", Source: SourceOffice, ID: 7, Columns: []TemplateColumn{
		{"Gün", FieldDate}, {"Hareket", FieldDescription}, {"Giriş", FieldCredit}, {"Çıkış", FieldDebit}, {"Kalan", FieldBalance}}}
	if err := tpl.Validate(); err != nil {
		t.Fatal(err)
	}
	sts, _, err := ParseWith("odd.csv", []byte(oddCSV), []Template{tpl})
	if err != nil {
		t.Fatal(err)
	}
	st := sts[0]
	if st.Template == nil || st.Template.Source != SourceOffice || st.Template.ID != 7 || len(st.Lines) != 2 {
		t.Fatalf("statement %+v", st)
	}
	if st.Lines[0].Direction != banking.DirOut || st.Lines[1].Direction != banking.DirIn {
		t.Fatal("directions")
	}
	// Analysis with the saved template matches it.
	a, err = AnalyzeFile(context.Background(), "odd.csv", []byte(oddCSV), []Template{tpl}, nil, ColumnRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Matched == nil || a.Matched.Source != SourceOffice || a.Lines != 2 {
		t.Fatalf("analysis %+v", a)
	}
	// A template for another format does not apply.
	tpl.Format = "pdf"
	if _, _, err := ParseWith("odd.csv", []byte(oddCSV), []Template{tpl}); !errors.Is(err, ErrLayoutUnknown) {
		t.Fatalf("err = %v", err)
	}
}

type fakeAI struct {
	got ColumnRequest
	out []ColumnSuggestion
	err error
}

func (f *fakeAI) SuggestColumns(_ context.Context, in ColumnRequest) ([]ColumnSuggestion, error) {
	f.got = in
	return f.out, f.err
}

func TestAnalyzeWithAI(t *testing.T) {
	ai := &fakeAI{out: []ColumnSuggestion{
		{Index: 0, Field: FieldDate, Confidence: 0.95},
		{Index: 1, Field: FieldDescription, Confidence: 0.9},
		{Index: 4, Field: FieldBalance, Confidence: 0.8},
		{Index: 3, Field: FieldBalance, Confidence: 0.5}, // second use is dropped
	}}
	a, err := AnalyzeFile(context.Background(), "odd.csv", []byte(oddCSV), nil, ai, ColumnRequest{OrganizationID: 3})
	if err != nil {
		t.Fatal(err)
	}
	if ai.got.OrganizationID != 3 || len(ai.got.Headers) != 5 {
		t.Fatalf("request %+v", ai.got)
	}
	for _, r := range ai.got.Sample {
		for _, c := range r {
			if strings.Contains(c, "ABC LTD") {
				t.Fatalf("sample not masked: %q", c)
			}
		}
	}
	byHeader := map[string]ColumnSuggestion{}
	for _, c := range a.Columns {
		byHeader[c.Header] = c
	}
	if c := byHeader["Gün"]; c.Field != FieldDate || c.Origin != OriginAI || c.Confidence > 0.9 {
		t.Fatalf("Gün %+v", c)
	}
	if byHeader["Kalan"].Field != FieldBalance || byHeader["Giriş"].Field != FieldCredit || byHeader["Çıkış"].Field != FieldDebit {
		t.Fatalf("columns %+v", a.Columns)
	}
	if a.Lines != 2 {
		t.Fatalf("lines %d", a.Lines)
	}
	// No AI configured: heuristics and a warning.
	a, err = AnalyzeFile(context.Background(), "odd.csv", []byte(oddCSV), nil, &fakeAI{err: mapping.ErrAIUnavailable}, ColumnRequest{})
	if err != nil || len(a.Warnings) == 0 {
		t.Fatalf("a %+v err %v", a, err)
	}
}

func TestParseColumnAnswer(t *testing.T) {
	got, err := parseColumnAnswer("tamam: {\"columns\":[{\"index\":0,\"field\":\"date\",\"confidence\":0.9},"+
		"{\"index\":1,\"field\":null,\"confidence\":0.1},{\"index\":9,\"field\":\"amount\",\"confidence\":1},"+
		"{\"index\":2,\"field\":\"nonsense\",\"confidence\":1}]}", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Field != FieldDate {
		t.Fatalf("got %+v", got)
	}
	if _, err := parseColumnAnswer("no json", 3); err == nil {
		t.Fatal("want error")
	}
}

func TestTemplateValidate(t *testing.T) {
	bad := []Template{
		{Columns: []TemplateColumn{{"Tarih", FieldDate}}},
		{Columns: []TemplateColumn{{"Tarih", FieldDate}, {"Açıklama", FieldDescription}}},
		{Columns: []TemplateColumn{{"Tarih", FieldDate}, {"Tutar", FieldAmount}, {"Tutar 2", FieldAmount}}},
		{Columns: []TemplateColumn{{"Tarih", FieldDate}, {"Tutar", "money"}}},
		{Columns: []TemplateColumn{{"Tarih", FieldDate}, {" ", FieldAmount}}},
		{Format: "doc", Columns: []TemplateColumn{{"Tarih", FieldDate}, {"Tutar", FieldAmount}}},
	}
	for i, tpl := range bad {
		if err := tpl.Validate(); !errors.Is(err, ErrInvalidTemplate) {
			t.Fatalf("%d: err = %v", i, err)
		}
	}
	ok := Template{Columns: []TemplateColumn{{"Tarih", FieldDate}, {"Borç", FieldDebit}, {"Alacak", FieldCredit}, {"Not", FieldNone}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBankFromText(t *testing.T) {
	cases := map[string]string{
		"T.C. ZİRAAT BANKASI A.Ş.":         "ziraat",
		"Türkiye İş Bankası":               "isbank",
		"Enpara.com Hesap Hareketleri":     "qnb",
		"TÜRK EKONOMİ BANKASI A.Ş.":        "teb",
		"Kuveyt Türk Katılım Bankası A.Ş.": "kuveyt_turk",
		"Vakıf Katılım":                    "vakif_katilim",
		"Bir market":                       "",
	}
	for text, want := range cases {
		if got := bankFromText([][]string{{text}}, 1); got != want {
			t.Fatalf("%q → %q, want %q", text, got, want)
		}
	}
}
