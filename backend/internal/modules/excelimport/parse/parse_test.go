package parse

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

func TestNumberStyles(t *testing.T) {
	tr := []string{"1.234,56", "12,50", "₺ 3.000,00", "(150,25)"}
	en := []string{"1,234.56", "12.50", "3000.00"}
	if DetectNumberStyle(tr) != StyleTR || DetectNumberStyle(en) != StyleEN {
		t.Fatal("style detection")
	}
	cases := []struct {
		in    string
		style NumberStyle
		want  string
		neg   bool
	}{
		{"1.234,56", StyleTR, "1234.56", false},
		{"₺ 3.000,00 TL", StyleTR, "3000", false},
		{"(150,25)", StyleTR, "150.25", true},
		{"-12,5", StyleTR, "12.5", true},
		{"1,234.56", StyleEN, "1234.56", false},
		{"1234.5", StyleEN, "1234.5", false},
		{"100-", StyleEN, "100", true},
	}
	for _, c := range cases {
		d, neg, ok := ParseAmount(c.in, c.style)
		if !ok || d.String() != c.want || neg != c.neg {
			t.Errorf("%q: %s %v %v", c.in, d, neg, ok)
		}
	}
	if _, _, ok := ParseAmount("abc", StyleTR); ok {
		t.Fatal("text parsed as number")
	}
}

func TestDates(t *testing.T) {
	for in, want := range map[string]string{
		"15.10.2026": "2026-10-15", "15/10/2026": "2026-10-15", "2026-10-15": "2026-10-15",
		"46310": "2026-10-15", "15.10.2026 14:30": "2026-10-15",
	} {
		d, ok := ParseDate(in)
		if !ok || d.Format("2006-01-02") != want {
			t.Errorf("%s -> %v %v", in, d, ok)
		}
	}
	if _, ok := ParseDate("Fatura"); ok {
		t.Fatal("text parsed as date")
	}
}

func TestSmallConversions(t *testing.T) {
	for in, want := range map[string]string{"%20": "20", "20": "20", "0,20": "20", "0.2": "20", "5/10": "50", "%50": "50", "0,5": "50"} {
		if r, ok := Rate(in); !ok || r.String() != want {
			t.Errorf("rate %s -> %s", in, r)
		}
	}
	if v, _, ok := TaxID("123 456 7890"); !ok || v != "1234567890" {
		t.Fatal("vkn")
	}
	if _, c, ok := TaxID("12345678901"); !ok || c != "12345678901" {
		t.Fatal("tckn")
	}
	if _, _, ok := TaxID("123"); ok {
		t.Fatal("short id")
	}
	if Currency("TL") != "TRY" || Currency("₺") != "TRY" || Currency("eur") != "EUR" {
		t.Fatal("currency")
	}
	if k, _ := ParseInvoiceKind("Satış İade"); k.Direction != "outgoing" || k.Type != "IADE" {
		t.Fatalf("kind %+v", k)
	}
	if k, _ := ParseInvoiceKind("ALIŞ"); k.Direction != "incoming" || k.Type != "ALIS" {
		t.Fatalf("kind %+v", k)
	}
	if p, _ := Profile("e-Arşiv"); p != "EARSIVFATURA" {
		t.Fatal("profile")
	}
}

func grid() [][]string {
	return [][]string{
		{"ACME Yazılım", "", "", ""},
		{"Satış Faturaları Listesi", "", "", ""},
		{},
		{"Fatura Tarihi", "Belge No", "Cari", "KDV", "", "Toplam"},
		{"", "", "", "%10", "%20", ""},
		{"15.10.2026", "A-1", "Demir Gıda", "10,00", "20,00", "230,00"},
		{"16.10.2026", "A-2", "Kaya Ltd", "0,00", "40,00", "240,00"},
		{"Genel Toplam", "", "", "10,00", "60,00", "470,00"},
		{"17.10.2026", "A-9", "after total", "", "", "1"},
	}
}

func TestDetectLayoutSkipsBannerMergesTwoRowHeaderStopsAtTotal(t *testing.T) {
	l, ok := DetectLayout(Sheet{Name: "S", Rows: grid()})
	if !ok {
		t.Fatal("no header")
	}
	if l.HeaderRow != 5 || l.FirstDataRow != 6 || l.LastDataRow != 7 {
		t.Fatalf("layout %+v", l)
	}
	got := map[string]string{}
	for _, c := range l.Columns {
		got[c.Letter] = c.Header
	}
	if got["D"] != "KDV %10" || got["E"] != "KDV %20" || got["A"] != "Fatura Tarihi" || got["F"] != "Toplam" {
		t.Fatalf("headers %v", got)
	}
	// Same headers, different case/punctuation -> same fingerprint.
	g := grid()
	g[3][0] = "FATURA TARİHİ"
	l2, _ := DetectLayout(Sheet{Rows: g})
	if l2.Fingerprint != l.Fingerprint {
		t.Fatal("fingerprint not normalized")
	}
	g[3][1] = "Fatura No"
	l3, _ := DetectLayout(Sheet{Rows: g})
	if l3.Fingerprint == l.Fingerprint {
		t.Fatal("different header, same fingerprint")
	}
}

func TestReadCSVWindows1254Semicolon(t *testing.T) {
	text := "Tarih;Belge No;Ünvan;Tutar\n15.10.2026;A-1;Şükrü Çağ;1.234,56\n"
	enc, _ := charmap.Windows1254.NewEncoder().String(text)
	f, sheets, err := Read("liste.csv", []byte(enc))
	if err != nil || f != FormatCSV {
		t.Fatal(err)
	}
	if sheets[0].Rows[1][2] != "Şükrü Çağ" || sheets[0].Rows[0][2] != "Ünvan" {
		t.Fatalf("rows %v", sheets[0].Rows)
	}
	_, sheets, _ = Read("x.csv", []byte("a,b\n1,2\n"))
	if len(sheets[0].Rows[0]) != 2 {
		t.Fatal("comma csv")
	}
}

func TestReadXLSXRawValues(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "Tarih")
	_ = f.SetCellValue("Sheet1", "B1", "Tutar")
	_ = f.SetCellValue("Sheet1", "A2", 46310)
	_ = f.SetCellValue("Sheet1", "B2", 1234.5)
	var buf bytes.Buffer
	_ = f.Write(&buf)
	format, sheets, err := Read("a.xlsx", buf.Bytes())
	if err != nil || format != FormatXLSX {
		t.Fatal(err)
	}
	r := sheets[0].Rows[1]
	if d, _ := ParseDate(r[0]); d.Format("2006-01-02") != "2026-10-15" || r[1] != "1234.5" {
		t.Fatalf("row %v", r)
	}
	if _, _, err := Read("old.xls", []byte{0xD0, 0xCF, 0x11, 0xE0, 1, 2}); err == nil {
		t.Fatal("legacy xls accepted")
	}
}

func TestColumnLetter(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"} {
		if ColumnLetter(i) != want {
			t.Errorf("%d -> %s", i, ColumnLetter(i))
		}
	}
}
