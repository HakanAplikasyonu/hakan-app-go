package usecase

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

func TestNormalizeCode(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		" 120.01.001 ": "120.01.001",
		"120 01 001":   "120.01.001",
		"120-01-001":   "120.01.001",
		"120.01.001.":  "120.01.001",
		"120..01":      "120.01",
		"120.1":        "120.1",
	} {
		if got := NormalizeCode(in); got != want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"12", "TOPLAM", "", "1200", "12A.01"} {
		if ValidCode(NormalizeCode(bad)) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}

func TestParseCSVWindows1254WithHeader(t *testing.T) {
	t.Parallel()
	src := "LUCA HESAP PLANI\n" +
		"Hesap Kodu;Hesap Adı\n" +
		"100;KASA\n" +
		"100.01;Merkez Kasa\n" +
		"120;ALICILAR\n" +
		"120.01.001;Şirket Ğ Çarşı\n" +
		"120.01.001;tekrar\n" +
		";\n" +
		"TOPLAM;\n"
	enc, err := charmap.Windows1254.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse("plan.csv", enc)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != FormatCSV || p.Kind != KindAccountPlan || p.HeaderRow != 2 {
		t.Fatalf("format/kind/header = %s/%s/%d", p.Format, p.Kind, p.HeaderRow)
	}
	if len(p.Accounts) != 4 || p.Duplicates != 1 || p.SkipCount != 1 {
		t.Fatalf("accounts=%d dup=%d skip=%d %+v", len(p.Accounts), p.Duplicates, p.SkipCount, p.Accounts)
	}
	leaf := map[string]bool{}
	for _, a := range p.Accounts {
		leaf[a.Code] = a.IsLeaf
		if a.Code == "120.01.001" && (a.Name != "Şirket Ğ Çarşı" || a.Level != 3 || a.MainCode != "120") {
			t.Errorf("decoded account = %+v", a)
		}
	}
	// 120 has 120.01.001 under it (no 120.01 row: still a group account).
	if leaf["100"] || !leaf["100.01"] || leaf["120"] || !leaf["120.01.001"] {
		t.Errorf("leaves = %v", leaf)
	}
}

func TestParseXLSXTrialBalance(t *testing.T) {
	t.Parallel()
	f := excelize.NewFile()
	rows := [][]any{
		{"Mizan 01.10.2026 - 31.10.2026"},
		{"Hesap Kodu", "Hesap Adı", "Borç", "Alacak", "Borç Bakiye", "Alacak Bakiye"},
		{"153", "TİCARİ MALLAR", 1000, 0, 1000, 0},
		{"153 01", "Ticari Mal", 1000, 0, 1000, 0},
		{"320-01-005", "Satıcı A.Ş.", 0, 500, 0, 500},
	}
	for i, r := range rows {
		cellName, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Sheet1", cellName, &r); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	p, err := Parse("mizan.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != FormatXLSX || p.Kind != KindTrialBalance || p.HeaderRow != 2 || len(p.Accounts) != 3 {
		t.Fatalf("parsed = %+v", p)
	}
	if p.Accounts[1].Code != "153.01" || p.Accounts[2].Code != "320.01.005" {
		t.Errorf("codes = %+v", p.Accounts)
	}
}

func TestParseWithoutHeaderUsesFirstColumns(t *testing.T) {
	t.Parallel()
	p, err := Parse("x.csv", []byte("100,Kasa\n102,Bankalar\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.HeaderRow != 0 || len(p.Accounts) != 2 || p.Accounts[1].Name != "Bankalar" {
		t.Fatalf("parsed = %+v", p)
	}
}

func TestParseRejectsLegacyXLS(t *testing.T) {
	t.Parallel()
	_, err := Parse("eski.xls", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingTemplateKeepsCodesAsText(t *testing.T) {
	t.Parallel()
	data, err := MissingTemplate([]TemplateAccount{{Code: "120.10", Name: "X"}, {Code: "120 10"}, {Code: "770.01", Name: "Genel Yönetim"}}, timeZero)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.GetRows("Hesap Planı")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0][0] != "Hesap Kodu" || rows[1][0] != "120.10" || rows[2][0] != "770.01" {
		t.Fatalf("rows = %v", rows)
	}
	if _, err := MissingTemplate([]TemplateAccount{{Code: "TOPLAM"}}, timeZero); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid code err = %v", err)
	}
}

var timeZero = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)

func TestParseTrialBalanceAmountsAndPeriod(t *testing.T) {
	t.Parallel()
	csv := "ATLAS GIDA GENEL GEÇİCİ MİZAN 01.01.2026 - 30.09.2026\n" +
		"Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye\n" +
		"100;KASA;125.400,00;118.250,00;7.150,00;\n" +
		"100.01;MERKEZ KASA;125.400,00;118.250,00;7.150,00;\n" +
		"320;SATICILAR;10.000,00;45.500,50;;35.500,50\n"
	p, err := Parse("mizan.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindTrialBalance || p.Period != "2026-09" || len(p.Accounts) != 3 {
		t.Fatalf("parsed = %+v", p)
	}
	got := map[string][3]string{}
	for _, a := range p.Accounts {
		got[a.Code] = [3]string{a.Debit.StringFixed(2), a.Credit.StringFixed(2), a.Balance.StringFixed(2)}
	}
	if got["100.01"] != [3]string{"125400.00", "118250.00", "7150.00"} || got["320"] != [3]string{"10000.00", "45500.50", "-35500.50"} {
		t.Fatalf("amounts = %v", got)
	}
}

func TestParseTrialBalanceWithoutBalanceColumns(t *testing.T) {
	t.Parallel()
	csv := "Hesap Kodu,Hesap Adı,Borç,Alacak\n102,BANKALAR,1500.25,500\n"
	p, err := Parse("mizan-2026-10.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if p.Period != "2026-10" || p.Accounts[0].Balance.StringFixed(2) != "1000.25" {
		t.Fatalf("parsed = %+v", p)
	}
	// A plain account plan has no amounts and no period.
	p, err = Parse("plan-2026-10.csv", []byte("Hesap Kodu;Hesap Adı\n100;KASA\n"))
	if err != nil || p.Kind != KindAccountPlan || !p.Accounts[0].Balance.IsZero() {
		t.Fatalf("plan = %+v %v", p, err)
	}
}
