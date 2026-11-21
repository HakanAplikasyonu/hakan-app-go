package payroll

import (
	"bytes"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
)

const lucaCSV = "LUCA BORDRO DÖKÜMÜ\nDönem: 2026/10\n\n" +
	"Sıra No;Adı Soyadı;TC Kimlik No;SGK Sicil No;Gün;Brüt Ücret;SGK Matrahı;SGK İşçi Payı;İşsizlik İşçi Payı;" +
	"Gelir Vergisi Matrahı;Kümülatif GV Matrahı;GV İstisnası;Ödenecek Gelir Vergisi;DV İstisnası;Damga Vergisi;Net Ücret;" +
	"SGK İşveren Payı;İşsizlik İşveren Payı;SGK Teşvik;Toplam Maliyet\n" +
	"1;Ayşe Yılmaz;10000000146;3400123;30;40.000,00;40.000,00;5.600,00;400,00;34.000,00;300.000,00;3.000,00;2.100,00;150,00;153,60;31.746,40;8.200,00;800,00;2.000,00;47.000,00\n" +
	"2;Mehmet Demir;12345678950;3400124;30;26.005,50;26.005,50;3.640,77;260,06;22.104,67;200.000,00;3.315,70;0,00;197,38;0,00;22.104,67;5.331,13;520,11;1.300,28;30.556,46\n" +
	";Toplam;;;;66.005,50;;9.240,77;660,06;;;;2.100,00;;153,60;53.851,07;13.531,13;1.320,11;3.300,28;77.556,46\n"

func TestHeuristicMappingLuca(t *testing.T) {
	p, err := ReadFile("bordro.csv", []byte(lucaCSV))
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceSystem != SourceLuca || p.Period != "2026-10" {
		t.Fatalf("source %q period %q", p.SourceSystem, p.Period)
	}
	want := map[string]string{
		"Adı Soyadı": "employee_name", "TC Kimlik No": "national_id", "SGK Sicil No": "sgk_no", "Gün": "days_worked",
		"Brüt Ücret": "gross", "SGK Matrahı": "sgk_base", "SGK İşçi Payı": "sgk_employee", "İşsizlik İşçi Payı": "unemployment_employee",
		"Gelir Vergisi Matrahı": "income_tax_base", "Kümülatif GV Matrahı": "", "GV İstisnası": "income_tax_exemption",
		"Ödenecek Gelir Vergisi": "income_tax", "DV İstisnası": "stamp_tax_exemption", "Damga Vergisi": "stamp_tax",
		"Net Ücret": "net", "SGK İşveren Payı": "sgk_employer", "İşsizlik İşveren Payı": "unemployment_employer",
		"SGK Teşvik": "sgk_incentive", "Toplam Maliyet": "employer_cost", "Sıra No": "",
	}
	for _, c := range p.Mapping.Columns {
		if w, ok := want[c.Header]; ok && w != c.Field {
			t.Errorf("%q -> %q, want %q", c.Header, c.Field, w)
		}
	}
	if m := Missing(p.Mapping); len(m) != 0 {
		t.Fatalf("missing %v", m)
	}
	lines, issues, _ := Lines(p)
	if len(lines) != 2 {
		t.Fatalf("lines %d, issues %+v", len(lines), issues)
	}
	a := lines[0]
	if a.EmployeeName != "Ayşe Yılmaz" || a.NationalIDMasked != "100******46" || a.nationalID != "10000000146" ||
		!a.Gross.Equal(decimal.RequireFromString("40000")) || !a.Net.Equal(decimal.RequireFromString("31746.40")) ||
		a.DaysWorked == nil || *a.DaysWorked != 30 {
		t.Fatalf("line 1 %+v", a)
	}
	for _, is := range issues {
		if is.Code == IssueNetMismatch || is.Code == IssueCostMismatch {
			t.Errorf("unexpected issue %+v", is)
		}
	}
	tot := Sum(lines)
	if !tot.Net.Equal(decimal.RequireFromString("53851.07")) || tot.EmployeeCount != 2 {
		t.Fatalf("totals %+v", tot)
	}
	acc, inc, pay := tot.SGKAccrual()
	if !acc.Equal(decimal.RequireFromString("24752.07")) || !inc.Equal(decimal.RequireFromString("3300.28")) ||
		!pay.Equal(decimal.RequireFromString("21451.79")) {
		t.Fatalf("sgk %s %s %s", acc, inc, pay)
	}
}

func TestEntryLinesBalance(t *testing.T) {
	p, _ := ReadFile("bordro.csv", []byte(lucaCSV))
	lines, _, _ := Lines(p)
	ls := EntryLines(Sum(lines), "2026-10", Accounts{Expense: "770.01"}, [16]byte{1})
	var dr, cr decimal.Decimal
	accounts := map[string]bool{}
	for _, l := range ls {
		dr, cr = dr.Add(l.Debit), cr.Add(l.Credit)
		accounts[l.Account] = true
		if l.SourceKind != "payroll" {
			t.Fatalf("source %q", l.SourceKind)
		}
	}
	if !dr.Equal(cr) {
		t.Fatalf("unbalanced: debit %s credit %s", dr, cr)
	}
	for _, a := range []string{"770.01", "335", "360", "361"} {
		if !accounts[a] {
			t.Errorf("account %s missing: %+v", a, ls)
		}
	}
	// 770 = brüt + işveren payları − teşvik = işverene maliyet.
	if !ls[0].Debit.Equal(decimal.RequireFromString("77556.46")) {
		t.Fatalf("770 %s", ls[0].Debit)
	}
}

func TestFreeFormXLSXWithSplitNameAndNoNet(t *testing.T) {
	f := excelize.NewFile()
	rows := [][]any{
		{"Personel Kodu", "Adı", "Soyadı", "Görevi", "İşe Giriş Tarihi", "Brüt", "SSK İşçi Primi", "İşsizlik Sig. İşçi", "Gelir Vergisi", "Damga Vergisi", "Avans", "Net Ödenen"},
		{"P01", "Zeynep", "Kaya", "Muhasebe", "15.03.2024", 30000, 4200, 300, 1500, 100, 1000, 22900},
	}
	for i, r := range rows {
		for j, v := range r {
			c, _ := excelize.CoordinatesToCellName(j+1, i+1)
			_ = f.SetCellValue("Sheet1", c, v)
		}
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	p, err := ReadFile("personel.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceSystem != SourceGeneric || p.Period != "" {
		t.Fatalf("source %q period %q", p.SourceSystem, p.Period)
	}
	fields := p.Mapping.FieldColumns()
	for _, f := range []string{"first_name", "last_name", "job_title", "hire_date", "gross", "sgk_employee", "unemployment_employee", "income_tax", "stamp_tax", "other_deductions", "net"} {
		if fields[f] == "" {
			t.Errorf("%s not mapped: %+v", f, p.Mapping.Columns)
		}
	}
	if fields["employee_name"] != "" {
		t.Errorf("Personel Kodu mapped as name")
	}
	lines, issues, _ := Lines(p)
	if len(lines) != 1 || lines[0].EmployeeName != "Zeynep Kaya" || lines[0].HireDate == nil ||
		!lines[0].HireDate.Equal(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("lines %+v issues %+v", lines, issues)
	}
	if !lines[0].EmployerCost.Equal(decimal.RequireFromString("30000")) {
		t.Fatalf("employer cost %s", lines[0].EmployerCost)
	}
	// GV matrahı sütunu yok (#575): brüt − SGK işçi − işsizlik işçi.
	if !lines[0].IncomeTaxBase.Equal(decimal.RequireFromString("25500")) {
		t.Fatalf("income tax base %s", lines[0].IncomeTaxBase)
	}
	for _, is := range issues {
		if is.Code == IssueNetMismatch {
			t.Fatalf("net mismatch on a consistent row: %+v", is)
		}
	}
}

func TestNetMismatchAndInvalidTCKN(t *testing.T) {
	csv := "Ad Soyad;TCKN;Brüt;SGK İşçi;Net\nAli Veli;12345678901;10000;1400;9000\n"
	p, err := ReadFile("x.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	lines, issues, _ := Lines(p)
	codes := map[string]bool{}
	for _, is := range issues {
		codes[is.Code] = true
	}
	if !codes[IssueNetMismatch] || !codes[IssueInvalidTCKN] || lines[0].NationalIDMasked != "" {
		t.Fatalf("issues %+v line %+v", issues, lines[0])
	}
}

func TestApplyOverride(t *testing.T) {
	p, _ := ReadFile("bordro.csv", []byte(lucaCSV))
	if _, err := ApplyOverride(p.Mapping, map[string]string{"B": "nope"}); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := ApplyOverride(p.Mapping, map[string]string{"ZZ": "gross"}); err == nil {
		t.Fatal("unknown column accepted")
	}
	if _, err := ApplyOverride(p.Mapping, map[string]string{"A": "gross"}); err == nil {
		t.Fatal("field mapped twice accepted")
	}
	mp, err := ApplyOverride(p.Mapping, map[string]string{"F": "", "A": "gross"})
	if err != nil || mp.FieldColumns()["gross"] != "A" {
		t.Fatalf("%v %+v", err, mp.FieldColumns())
	}
	if mp.Columns[0].Origin != mapping.OriginUser {
		t.Fatalf("origin %q", mp.Columns[0].Origin)
	}
}

func TestPeriodAndDueDate(t *testing.T) {
	for in, want := range map[string]string{"Dönem: 2026/10": "2026-10", "Bordro 03.2025": "2025-03", "EKİM 2026 BORDROSU": "2026-10", "Rapor": ""} {
		if got := periodIn(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	d, _ := SGKDueDate("2026-12")
	if d.Format(time.DateOnly) != "2027-01-31" {
		t.Fatalf("due %s", d)
	}
}

func TestDigestIsPerTaxpayerAndKeyed(t *testing.T) {
	a, b := NewDigester("k1"), NewDigester("k2")
	if a.Digest(1, "10000000146") == a.Digest(2, "10000000146") || a.Digest(1, "10000000146") == b.Digest(1, "10000000146") {
		t.Fatal("digest does not depend on taxpayer and key")
	}
	if MaskNationalID("10000000146") != "100******46" {
		t.Fatal(MaskNationalID("10000000146"))
	}
}
