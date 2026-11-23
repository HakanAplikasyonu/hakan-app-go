package parse

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/xls/xlstest"
)

// Fixtures are built here with made-up firms, people and amounts; only the
// row / column structure follows Luca's exports.

// xlsxOf writes rows to Sheet1 starting at A1; a float64 cell stays a
// number cell (Luca writes amounts and the T.C. kimlik no as numbers).
func xlsxOf(t *testing.T, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, r := range rows {
		if len(r) == 0 {
			continue
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		row := r
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// mizanRows: header info rows, then the Luca mizan header.
func mizanRows(info int) [][]any {
	rows := [][]any{
		{"ÖRNEK TİCARET LTD. ŞTİ."},
		{"MİZAN"},
		{"Dönem: 2026"},
		{"Tarih Aralığı: 01.01.2026 - 31.10.2026"},
		{"Para Birimi: TL"},
	}
	rows = rows[:info]
	return append(rows,
		[]any{"HESAP KODU", "HESAP ADI", "", "BORÇ", "ALACAK", "BORÇ BAKİYESİ", "ALACAK BAKİYESİ"},
		[]any{"100", "KASA", "", 5000.0, 3000.0, 2000.0, 0.0},
		[]any{"100.01", "MERKEZ KASA", "", 5000.0, 3000.0, 2000.0, 0.0},
		[]any{"320", "SATICILAR", "", 1000.0, 4000.0, 0.0, 3000.0},
		[]any{"320.01.001", "ÖRNEK TEDARİKÇİ", "", 1000.0, 4000.0, 0.0, 3000.0},
		[]any{"600", "YURT İÇİ SATIŞLAR", "", 0.0, 1000.0, 0.0, 1000.0},
		[]any{"770", "GENEL YÖNETİM GİDERLERİ", "", 2000.0, 0.0, 2000.0, 0.0},
		[]any{},
		[]any{"TOPLAM", "", "", 8000.0, 8000.0, 4000.0, 4000.0},
	)
}

func TestMizanLuca(t *testing.T) {
	data := xlsxOf(t, mizanRows(5))
	if k := DetectKind("mizan.xlsx", data); k != KindMizan {
		t.Fatalf("kind = %s", k)
	}
	s := Summarize(KindMizan, "mizan.xlsx", data)
	if !s.Readable || s.Mizan == nil {
		t.Fatalf("summary = %+v", s)
	}
	m := s.Mizan
	if m.HeaderRow != 6 || m.Period != "2026-10" || m.PeriodStart != "2026-01-01" || m.PeriodEnd != "2026-10-31" {
		t.Fatalf("header / period: %+v", m)
	}
	if m.Accounts != 6 || len(m.Lines) != 4 {
		t.Fatalf("accounts %d lines %+v", m.Accounts, m.Lines)
	}
	// Main accounts only: sub-accounts are not counted twice.
	if m.DebitTotal != "8000.00" || m.CreditTotal != "8000.00" || m.DebitBalance != "4000.00" || m.CreditBalance != "4000.00" || !m.Balanced {
		t.Fatalf("totals: %+v", m)
	}
	if l := m.Lines[1]; l.Code != "320" || l.Name != "SATICILAR" || l.Credit != "4000.00" || l.CreditBalance != "3000.00" {
		t.Fatalf("line: %+v", l)
	}
}

// The header row is found by its text: fewer header info rows (and none
// at all) still read.
func TestMizanShiftedHeader(t *testing.T) {
	for _, info := range []int{2, 0} {
		s := Summarize(KindMizan, "mizan_2026-09.xlsx", xlsxOf(t, mizanRows(info)))
		if !s.Readable || s.Mizan == nil || s.Mizan.HeaderRow != info+1 || s.Mizan.Accounts != 6 {
			t.Fatalf("info %d: %+v", info, s)
		}
		// No date range above the header: the period comes from the name.
		if s.Mizan.Period != "2026-09" {
			t.Fatalf("info %d: period %q", info, s.Mizan.Period)
		}
	}
	s := Summarize(KindMizan, "tablo.xlsx", xlsxOf(t, [][]any{{"a", "b"}, {1.0, 2.0}}))
	if s.Readable || s.Reason != ReasonNoMizanHeader {
		t.Fatalf("no header: %+v", s)
	}
}

// bordroRows: the Luca ücret bordrosu layout with two made-up people.
func bordroRows() [][]any {
	person := func(name string, id any, gross, net float64) [][]any {
		return [][]any{
			{name, "01.01.2025", "", id, "1234567890123", 30.0, "Gün", gross, 0.0, gross, gross, gross * 0.14, gross * 0.01, gross * 0.85, 0.0, 0.0, 0.0, 0.0, 0.0, net, net, ""},
			{"Normal Gün :", "30 Gün", gross, "---", "Hafta Tatili :", "", "", "", "Genel Tatil :", "", "", "", "Yemek :", "", ""},
			{},
		}
	}
	rows := [][]any{
		{},
		{"", "", "", "", "", "", "", "EKİM 2026 ÜCRET BORDROSU", "", "", "", "", "", "", "", "", "", "", "", "Tarih: 05/11/2026"},
		{"İşyeri:", "ÖRNEK TİCARET LTD. ŞTİ."},
		{"İşyeri S.No:", "0000000000000000000000000-00"},
		{"SSK Şube:", "ÖRNEK"},
		{"Vergi D./No:", "ÖRNEK VD / 1234567890"},
		{"MERSİS No:", ""},
		{"Ticaret Sicil No:", ""},
		{},
		{"ADI SOYADI", "GİRİŞ TARİHİ", "ÇIKIŞ TARİHİ", "T.C. KİMLİK NO", "SSK SİCİL NO", "GÜN", "", "NORMAL KAZANÇ", "DİĞER KAZANÇ", "TOPLAM KAZANÇ", "SSK MATRAH", "SSK PRİMİ", "İŞSİZLİK PRİMİ", "G.V.M. (AYLIK)", "GELİR VERGİSİ", "ASGARİ GEÇ.İND.", "KALAN G.VER.", "DAMGA VERGİSİ", "ÖZEL KESİNTİ", "AGI Hariç Ücret", "NET İSTİHKAK", "İMZA"},
		{},
	}
	rows = append(rows, person("AYŞE ÖRNEK", 10000000146.0, 30000, 25000)...)
	// The second id is text with the ".0" Luca's number leaves behind.
	rows = append(rows, person("MEHMET DENEME", "20000000046.0", 40000, 33000)...)
	rows = append(rows,
		[]any{"", "", "", "", "", 60.0, "", 70000.0, 0.0, 70000.0},
		[]any{"Listelenen 2 Personel"},
		[]any{},
		[]any{"BRÜT ÖDEMELER", "", "Gün/Saat", "Tutar", "NET ÖDEMELER", "", "", "", "", "Tutar"},
		[]any{"Normal Kazançlar", "", 60.0, 70000.0, "Gelir Vergisi", "", "", "", "", 4000.0},
		[]any{"Yıllık İzinler", "", 0.0, 0.0, "GV Teşvikleri", "", "", "", "", 1000.0},
		[]any{"Toplam Kişi Sayısı", "", 2.0, "", "Net GV", "", "", "", "", 3000.0},
		[]any{"Toplam SGK Matrahı", "", "", 70000.0, "Damga Vergisi", "", "", "", "", 531.3},
		[]any{"Brüt Kazançlar Toplamı", "", "", 70000.0, "DV Teşvikleri", "", "", "", "", 231.3},
		[]any{"Sigorta İşveren Hissesi", "", "", 15050.0, "Net DV", "", "", "", "", 300.0},
		[]any{"SGK İşveren Teşvikleri", "", "", 3500.0, "Sigorta Toplam Prim (Emekli/Normal)", "", "", "", "", 9800.0},
		[]any{"Net Sigorta İşveren Hissesi", "", "", 11550.0, "İşsizlik Sigortası", "", "", "", "", 700.0},
		[]any{"İşsizlik İşveren Hissesi", "", "", 1400.0, "Net Ödemeler Toplam", "", "", "", "", 56200.0},
		[]any{"TOPLAM", "", "", 82950.0, "Özel Kesintiler", "", "", "", "", 0.0},
		[]any{"", "", "", "", "TOPLAM", "", "", "", "", 70000.0},
	)
	return rows
}

func TestBordroLucaSummary(t *testing.T) {
	data := xlsxOf(t, bordroRows())
	if k := DetectKind("bordro.xlsx", data); k != KindBordro {
		t.Fatalf("kind = %s", k)
	}
	s := Summarize(KindBordro, "bordro.xlsx", data)
	if !s.Readable || s.Bordro == nil {
		t.Fatalf("summary = %+v", s)
	}
	b := s.Bordro
	if b.Period != "2026-10" || b.Persons != 2 || !b.FromTotals {
		t.Fatalf("period / persons: %+v", b)
	}
	want := map[string]string{
		"gross": b.Gross, "sgk_base": b.SGKBase, "sgk_employee": b.SGKEmployee, "sgk_employer": b.SGKEmployer,
		"sgk_incentive": b.SGKIncentive, "unemployment_employee": b.UnemploymentEmployee,
		"unemployment_employer": b.UnemploymentEmployer, "income_tax": b.IncomeTax, "stamp_tax": b.StampTax,
		"net": b.Net, "employer_cost": b.EmployerCost,
	}
	expect := map[string]string{
		"gross": "70000.00", "sgk_base": "70000.00", "sgk_employee": "9800.00", "sgk_employer": "11550.00",
		"sgk_incentive": "3500.00", "unemployment_employee": "700.00", "unemployment_employer": "1400.00",
		"income_tax": "3000.00", "stamp_tax": "300.00", "net": "56200.00", "employer_cost": "82950.00",
	}
	for k, v := range expect {
		if want[k] != v {
			t.Errorf("%s = %s, want %s", k, want[k], v)
		}
	}
	// People: the numeric T.C. kimlik no loses its ".0".
	if len(b.Employees) != 2 {
		t.Fatalf("employees: %+v", b.Employees)
	}
	if e := b.Employees[0]; e.Name != "AYŞE ÖRNEK" || e.NationalID != "10000000146" || e.Gross != "30000.00" || e.Net != "25000.00" || e.Days != "30" {
		t.Fatalf("first: %+v", e)
	}
	if e := b.Employees[1]; e.NationalID != "20000000046" {
		t.Fatalf("second: %+v", e)
	}
}

func TestNationalID(t *testing.T) {
	cases := map[string]string{
		"10000000146":      "10000000146",
		"10000000146.0":    "10000000146",
		"10000000146,0":    "10000000146",
		"1.0000000146E+10": "10000000146",
		" 10000000146 ":    "10000000146",
		"1234":             "",
		"Normal Gün :":     "",
		"":                 "",
	}
	for in, want := range cases {
		if got := NationalID(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// An .xls that cannot be opened (damaged, password protected) is stored
// with its reason; the kind then comes from the file name.
func TestBordroDamagedXLS(t *testing.T) {
	data := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...)
	if k := DetectKind("Ekim bordro.xls", data); k != KindBordro {
		t.Fatalf("kind = %s", k)
	}
	if k := DetectKind("MİZAN.xls", data); k != KindMizan {
		t.Fatalf("kind = %s", k)
	}
	if s := Summarize(KindBordro, "Ekim bordro.xls", data); s.Readable || s.Reason != ReasonLegacyXLS {
		t.Fatalf("summary = %+v", s)
	}
}

// xlsOf writes rows as a binary Excel 97-2003 workbook (BIFF8), the way
// Luca saves the ücret bordrosu: numbers as number cells, labels as text.
func xlsOf(rows [][]any) []byte {
	return xlstest.Write(xlstest.Sheet{Name: "Sayfa1", Rows: rows})
}

// A mizan saved by Luca as .xls is read like the xlsx: the header row by
// its text, the kind from the content (the name says nothing).
func TestMizanXLS(t *testing.T) {
	data := xlsOf(mizanRows(5))
	if f := DetectFormat("rapor.xls", data); f != "xls" {
		t.Fatalf("format = %s", f)
	}
	if k := DetectKind("rapor.xls", data); k != KindMizan {
		t.Fatalf("kind = %s", k)
	}
	s := Summarize(KindMizan, "rapor.xls", data)
	if !s.Readable || s.Format != "xls" || s.Mizan == nil {
		t.Fatalf("summary = %+v", s)
	}
	m := s.Mizan
	if m.HeaderRow != 6 || m.Period != "2026-10" || m.Accounts != 6 || len(m.Lines) != 4 {
		t.Fatalf("mizan: %+v", m)
	}
	if m.DebitTotal != "8000.00" || m.CreditTotal != "8000.00" || m.DebitBalance != "4000.00" || m.CreditBalance != "4000.00" || !m.Balanced {
		t.Fatalf("totals: %+v", m)
	}
	if l := m.Lines[1]; l.Code != "320" || l.Name != "SATICILAR" || l.Credit != "4000.00" {
		t.Fatalf("line: %+v", l)
	}
}

// A Luca ücret bordrosu .xls: the bottom summary totals, the T.C. kimlik
// no from a number cell and from "….0" text, and an amount given as
// formatted text ("1.400,00") next to number cells, all as exact decimals.
func TestBordroXLS(t *testing.T) {
	rows := bordroRows()
	for _, r := range rows {
		if len(r) > 3 && r[0] == "İşsizlik İşveren Hissesi" {
			r[3] = xlstest.Label("1.400,00")
		}
	}
	data := xlsOf(rows)
	if k := DetectKind("Ekim 2026.xls", data); k != KindBordro {
		t.Fatalf("kind = %s", k)
	}
	s := Summarize(KindBordro, "Ekim 2026.xls", data)
	if !s.Readable || s.Format != "xls" || s.Bordro == nil {
		t.Fatalf("summary = %+v", s)
	}
	b := s.Bordro
	if b.Period != "2026-10" || b.Persons != 2 || !b.FromTotals {
		t.Fatalf("period / persons: %+v", b)
	}
	got := map[string]string{
		"gross": b.Gross, "sgk_base": b.SGKBase, "sgk_employee": b.SGKEmployee, "sgk_employer": b.SGKEmployer,
		"sgk_incentive": b.SGKIncentive, "unemployment_employee": b.UnemploymentEmployee,
		"unemployment_employer": b.UnemploymentEmployer, "income_tax": b.IncomeTax, "stamp_tax": b.StampTax,
		"net": b.Net, "employer_cost": b.EmployerCost,
	}
	expect := map[string]string{
		"gross": "70000.00", "sgk_base": "70000.00", "sgk_employee": "9800.00", "sgk_employer": "11550.00",
		"sgk_incentive": "3500.00", "unemployment_employee": "700.00", "unemployment_employer": "1400.00",
		"income_tax": "3000.00", "stamp_tax": "300.00", "net": "56200.00", "employer_cost": "82950.00",
	}
	for k, v := range expect {
		if got[k] != v {
			t.Errorf("%s = %s, want %s", k, got[k], v)
		}
	}
	if len(b.Employees) != 2 {
		t.Fatalf("employees: %+v", b.Employees)
	}
	if e := b.Employees[0]; e.Name != "AYŞE ÖRNEK" || e.NationalID != "10000000146" || e.Gross != "30000.00" || e.Net != "25000.00" || e.Days != "30" {
		t.Fatalf("first: %+v", e)
	}
	if e := b.Employees[1]; e.NationalID != "20000000046" {
		t.Fatalf("second: %+v", e)
	}
}

// The format comes from the magic bytes, not the extension: an .xls named
// .xlsx and an .xlsx named .xls are both read.
func TestXLSMagicBytes(t *testing.T) {
	x := xlsOf(bordroRows())
	if f := DetectFormat("bordro.xlsx", x); f != "xls" {
		t.Fatalf("xls as .xlsx: %s", f)
	}
	if s := Summarize(KindBordro, "bordro.xlsx", x); !s.Readable || s.Bordro == nil || s.Bordro.Net != "56200.00" {
		t.Fatalf("xls as .xlsx: %+v", s)
	}
	y := xlsxOf(t, mizanRows(5))
	if f := DetectFormat("mizan.xls", y); f != "xlsx" {
		t.Fatalf("xlsx as .xls: %s", f)
	}
	if s := Summarize(KindMizan, "mizan.xls", y); !s.Readable || s.Mizan == nil {
		t.Fatalf("xlsx as .xls: %+v", s)
	}
	// Content that tells nothing: the name decides.
	plain := xlsOf([][]any{{"Not"}, {"Ekim dönemi"}})
	if k := DetectKind("Ekim mizan.xls", plain); k != KindMizan {
		t.Fatalf("name fallback: %s", k)
	}
	if k := DetectKind("liste.xls", plain); k != KindIsletme {
		t.Fatalf("default: %s", k)
	}
}

// Without the bottom blocks the person rows are summed.
func TestBordroWithoutSummary(t *testing.T) {
	rows := bordroRows()
	cut := 0
	for i, r := range rows {
		if len(r) > 0 && r[0] == "Listelenen 2 Personel" {
			cut = i + 1
		}
	}
	s := Summarize(KindBordro, "bordro.xlsx", xlsxOf(t, rows[:cut]))
	if !s.Readable || s.Bordro == nil || s.Bordro.FromTotals || s.Bordro.Gross != "70000.00" || s.Bordro.Net != "58000.00" || s.Bordro.Persons != 2 {
		t.Fatalf("summary = %+v", s.Bordro)
	}
}
