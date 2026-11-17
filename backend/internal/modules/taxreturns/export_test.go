package taxreturns

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func readBook(t *testing.T, data []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func sheetText(t *testing.T, f *excelize.File, sheet string) string {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strings.Join(r, "|"))
		b.WriteString("\n")
	}
	return b.String()
}

func TestWorkbookKDV1(t *testing.T) {
	k, items := ComputeKDV1(KDV1Input{
		Invoices: []InvoiceLine{
			{Direction: "outgoing", Type: "SATIS", Rate: d("20"), Count: 1, Base: d("1000"), VAT: d("200")},
			{Direction: "outgoing", Type: "TEVKIFAT", Rate: d("20"), WithholdingCode: "603", Count: 1, Base: d("500"), VAT: d("100"), Withholding: d("70")},
			{Direction: "incoming", Type: "ALIS", Rate: d("20"), Count: 1, Base: d("400"), VAT: d("80")},
		},
		Carry: Carry{Source: CarryNone},
	})
	p := Preview{Kind: KindKDV1, Period: "2026-10", Status: StatusDraft, Calendar: Calendar{DueDate: "2026-11-30"}}
	p.KDV1, p.Checklist = &k, items
	data, err := Workbook(p)
	if err != nil {
		t.Fatal(err)
	}
	f := readBook(t, data)
	if got := f.GetSheetList(); len(got) != 2 || got[0] != "KDV1" || got[1] != "Kontrol listesi" {
		t.Fatalf("sheets = %v", got)
	}
	txt := sheetText(t, f, "KDV1")
	for _, want := range []string{"1 No.lu KDV Beyannamesi — 2026-10", "Taslak", "Kısmi tevkifat", "603", "7/10",
		"Ödenmesi gereken katma değer vergisi", "bilinmiyor (0 alındı)"} {
		if !strings.Contains(txt, want) {
			t.Errorf("KDV1 sheet misses %q:\n%s", want, txt)
		}
	}
	// Tutarlar sayı olarak yazılır: ödenecek 200 + 30 − 80 = 150.
	rows, _ := f.GetRows("KDV1", excelize.Options{RawCellValue: true})
	found := false
	for _, r := range rows {
		if len(r) > 1 && r[0] == "Ödenmesi gereken katma değer vergisi" && r[1] == "150" {
			found = true
		}
	}
	if !found {
		t.Errorf("payable row not found as a number")
	}
	if !strings.Contains(sheetText(t, f, "Kontrol listesi"), "devreden KDV 0 alındı") {
		t.Error("checklist sheet misses CARRY_UNKNOWN")
	}
}

func TestWorkbookMUHSGKAndBABS(t *testing.T) {
	days := int32(30)
	m, items := ComputeMUHSGK(MUHSGKInput{
		Run: &PayrollRun{EmployeeCount: 1},
		Lines: []PayrollLine{{LineNo: 1, Name: "Ayşe Kaya", NationalIDMasked: "100******46", SGKNo: "1", DaysWorked: &days,
			Gross: d("30000"), IncomeTaxBase: d("25500"), IncomeTax: d("1500"), StampTax: d("100")}},
	})
	p := Preview{Kind: KindMUHSGK, Period: "2026-10", Status: StatusPrepared}
	p.MUHSGK, p.Checklist = &m, items
	data, err := Workbook(p)
	if err != nil {
		t.Fatal(err)
	}
	f := readBook(t, data)
	if got := f.GetSheetList(); len(got) != 3 || got[1] != "Çalışanlar" {
		t.Fatalf("sheets = %v", got)
	}
	if txt := sheetText(t, f, "Çalışanlar"); !strings.Contains(txt, "100******46") || strings.Contains(txt, "10000000146") {
		t.Errorf("employees sheet: %s", txt)
	}

	x, items := ComputeBABS(BABSInput{Counterparties: []Counterparty{
		{Direction: "incoming", ID: "1234567890", Name: "Tedarikçi A.Ş.", Count: 2, Amount: d("9000")},
		{Direction: "outgoing", ID: "12345678901", Name: "Oğuz Yılmaz", Count: 1, Amount: d("6000")},
	}})
	p = Preview{Kind: KindBABS, Period: "2026-10", Status: StatusDraft}
	p.BABS, p.Checklist = &x, items
	data, err = Workbook(p)
	if err != nil {
		t.Fatal(err)
	}
	f = readBook(t, data)
	if got := f.GetSheetList(); len(got) != 3 || got[0] != "Form Ba" || got[1] != "Form Bs" {
		t.Fatalf("sheets = %v", got)
	}
	ba := sheetText(t, f, "Form Ba")
	if !strings.Contains(ba, "Tedarikçi A.Ş.|TÜRKİYE|1234567890||2") {
		t.Errorf("Form Ba: %s", ba)
	}
	if bs := sheetText(t, f, "Form Bs"); !strings.Contains(bs, "Oğuz Yılmaz|TÜRKİYE||12345678901|1") {
		t.Errorf("Form Bs: %s", bs)
	}
}
