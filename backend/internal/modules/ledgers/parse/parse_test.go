package parse

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

const yevmiyeXML = `<?xml version="1.0" encoding="UTF-8"?>
<edefter:defter xmlns:edefter="http://www.edefter.gov.tr" xmlns:xbrli="http://www.xbrl.org/2003/instance" xmlns:gl-cor="http://www.xbrl.org/int/gl/cor/2006-10-25">
 <xbrli:xbrl>
  <gl-cor:accountingEntries>
   <gl-cor:documentInfo>
    <gl-cor:entriesType>journal</gl-cor:entriesType>
    <gl-cor:periodCoveredStart>2026-10-01</gl-cor:periodCoveredStart>
    <gl-cor:periodCoveredEnd>2026-10-31</gl-cor:periodCoveredEnd>
   </gl-cor:documentInfo>
   <gl-cor:entryHeader>
    <gl-cor:entryNumberCounter>1</gl-cor:entryNumberCounter>
    <gl-cor:entryDetail>
     <gl-cor:lineNumber>1</gl-cor:lineNumber>
     <gl-cor:account>
      <gl-cor:accountMainID>153</gl-cor:accountMainID>
      <gl-cor:accountMainDescription>TİCARİ MALLAR</gl-cor:accountMainDescription>
      <gl-cor:accountSub><gl-cor:accountSubDescription>Emtia</gl-cor:accountSubDescription><gl-cor:accountSubID>153.01</gl-cor:accountSubID></gl-cor:accountSub>
     </gl-cor:account>
     <gl-cor:amount>1000.00</gl-cor:amount>
     <gl-cor:debitCreditCode>D</gl-cor:debitCreditCode>
    </gl-cor:entryDetail>
    <gl-cor:entryDetail>
     <gl-cor:lineNumber>2</gl-cor:lineNumber>
     <gl-cor:account>
      <gl-cor:accountMainID>191</gl-cor:accountMainID>
      <gl-cor:accountMainDescription>İNDİRİLECEK KDV</gl-cor:accountMainDescription>
     </gl-cor:account>
     <gl-cor:amount>200.00</gl-cor:amount>
     <gl-cor:debitCreditCode>D</gl-cor:debitCreditCode>
    </gl-cor:entryDetail>
    <gl-cor:entryDetail>
     <gl-cor:lineNumber>3</gl-cor:lineNumber>
     <gl-cor:account>
      <gl-cor:accountMainID>320</gl-cor:accountMainID>
      <gl-cor:accountMainDescription>SATICILAR</gl-cor:accountMainDescription>
     </gl-cor:account>
     <gl-cor:amount>1200.00</gl-cor:amount>
     <gl-cor:debitCreditCode>C</gl-cor:debitCreditCode>
    </gl-cor:entryDetail>
   </gl-cor:entryHeader>
   <gl-cor:entryHeader>
    <gl-cor:entryDetail>
     <gl-cor:account><gl-cor:accountMainID>320.01.005</gl-cor:accountMainID></gl-cor:account>
     <gl-cor:amount>500,00</gl-cor:amount>
     <gl-cor:debitCreditCode>D</gl-cor:debitCreditCode>
    </gl-cor:entryDetail>
    <gl-cor:entryDetail>
     <gl-cor:account><gl-cor:accountMainID>102</gl-cor:accountMainID><gl-cor:accountMainDescription>BANKALAR</gl-cor:accountMainDescription></gl-cor:account>
     <gl-cor:amount>500.00</gl-cor:amount>
     <gl-cor:debitCreditCode>C</gl-cor:debitCreditCode>
    </gl-cor:entryDetail>
   </gl-cor:entryHeader>
  </gl-cor:accountingEntries>
 </xbrli:xbrl>
</edefter:defter>`

func TestEDefterYevmiye(t *testing.T) {
	s := Summarize(KindYevmiye, "1234567890-202610-Y-000001.xml", []byte(yevmiyeXML))
	if !s.Readable || s.EDefter == nil {
		t.Fatalf("summary = %+v", s)
	}
	e := s.EDefter
	if e.Entries != 2 || e.Lines != 5 {
		t.Fatalf("entries %d lines %d", e.Entries, e.Lines)
	}
	if e.DebitTotal != "1700.00" || e.CreditTotal != "1700.00" {
		t.Fatalf("totals %s / %s", e.DebitTotal, e.CreditTotal)
	}
	if e.PeriodStart != "2026-10-01" || e.PeriodEnd != "2026-10-31" {
		t.Fatalf("period %s %s", e.PeriodStart, e.PeriodEnd)
	}
	want := map[string][3]string{
		"102": {"BANKALAR", "0.00", "500.00"},
		"153": {"TİCARİ MALLAR", "1000.00", "0.00"},
		"191": {"İNDİRİLECEK KDV", "200.00", "0.00"},
		"320": {"SATICILAR", "500.00", "1200.00"},
	}
	if len(e.Accounts) != len(want) {
		t.Fatalf("accounts %+v", e.Accounts)
	}
	for _, a := range e.Accounts {
		w, ok := want[a.Code]
		if !ok || a.Name != w[0] || a.Debit != w[1] || a.Credit != w[2] {
			t.Errorf("account %+v, want %v", a, w)
		}
	}
	if e.Accounts[0].Code != "102" {
		t.Errorf("accounts not sorted: %+v", e.Accounts)
	}
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, c := range files {
		f, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(c))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestEDefterZip(t *testing.T) {
	data := zipOf(t, map[string]string{
		"1234567890-202610-Y-000001.xml":  yevmiyeXML,
		"1234567890-202610-YB-000001.xml": `<?xml version="1.0"?><edefter:berat xmlns:edefter="x"/>`,
		"readme.txt":                      "x",
	})
	if k := DetectKind("defter.zip", data); k != KindYevmiye {
		t.Fatalf("kind = %s", k)
	}
	s := Summarize(KindYevmiye, "defter.zip", data)
	if !s.Readable || s.Format != "zip" || s.EDefter == nil || s.EDefter.Lines != 5 {
		t.Fatalf("summary = %+v", s)
	}
	if len(s.Files) != 2 {
		t.Fatalf("files = %v", s.Files)
	}
}

func TestDetectKind(t *testing.T) {
	cases := map[string]string{
		"1234567890-202610-K-000001.xml":  KindKebir,
		"1234567890-202610-KB-000001.xml": KindBerat,
		"1234567890-202610-Y-000001.xml":  KindYevmiye,
		"bilinmeyen.xml":                  KindYevmiye, // content has gl-cor
	}
	for name, want := range cases {
		if got := DetectKind(name, []byte(yevmiyeXML)); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
	if got := DetectKind("defter.csv", []byte("Tarih;Açıklama;Gider\n")); got != KindIsletme {
		t.Errorf("csv: %s", got)
	}
}

func TestIsletmeCSV(t *testing.T) {
	csv := strings.Join([]string{
		"İŞLETME DEFTERİ - EKİM 2026",
		"Tarih;Evrak No;Açıklama;Satın Alınan Mallar;Giderler;KDV;Satılan Mallar",
		"01.10.2026;A1;Toptancı mal alışı;10.000,00;;2.000,00;",
		"05.10.2026;A2;Kira;;5.000,00;;",
		"10.10.2026;S1;Perakende satış;;;;25.500,50",
		"31.10.2026;;Toplam;10.000,00;5.000,00;2.000,00;25.500,50",
	}, "\n")
	s := Summarize(KindIsletme, "isletme.csv", []byte(csv))
	if !s.Readable || s.Isletme == nil {
		t.Fatalf("summary = %+v", s)
	}
	is := s.Isletme
	if is.Purchases != "10000.00" || is.Expenses != "5000.00" || is.Sales != "25500.50" || is.Income != "0.00" {
		t.Fatalf("totals %+v", is)
	}
	if is.Rows != 3 || is.FirstDate != "2026-10-01" || is.LastDate != "2026-10-10" {
		t.Fatalf("rows %d %s %s", is.Rows, is.FirstDate, is.LastDate)
	}
	if len(is.Top) != 3 || is.Top[0].Side != SideSale || is.Top[0].Description != "Perakende satış" {
		t.Fatalf("top %+v", is.Top)
	}
}

func TestIsletmeXLSXTwoRowHeader(t *testing.T) {
	f := excelize.NewFile()
	rows := [][]any{
		{"", "", "GİDER", "", "GELİR", ""},
		{"Tarih", "Açıklama", "Tutar", "KDV", "Tutar", "Diğer"},
		{"03.10.2026", "Elektrik", "1500,00", "300", "", ""},
		{"12.10.2026", "Hizmet satışı", "", "", "8000,00", "250,00"},
	}
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Sheet1", cell, &r); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	s := Summarize(KindIsletme, "defter.xlsx", b.Bytes())
	if !s.Readable || s.Format != "xlsx" || s.Isletme == nil {
		t.Fatalf("summary = %+v", s)
	}
	if s.Isletme.Expenses != "1500.00" || s.Isletme.Income != "8250.00" {
		t.Fatalf("totals %+v", s.Isletme)
	}
}

func TestUnreadable(t *testing.T) {
	cases := []struct {
		name, kind string
		data       []byte
		reason     string
	}{
		{"eski.xls", KindIsletme, append([]byte{0xD0, 0xCF, 0x11, 0xE0}, make([]byte, 64)...), ReasonLegacyXLS},
		{"bos.csv", KindIsletme, nil, ReasonEmpty},
		{"tablo.csv", KindIsletme, []byte("a;b;c\n1;2;3\n"), ReasonNoColumns},
		{"bozuk.xml", KindYevmiye, []byte("<?xml version=\"1.0\"?><a><b/></a>"), ReasonNoEntries},
		{"bos.zip", KindYevmiye, zipOf(t, map[string]string{"a.txt": "x"}), ReasonNoEntries},
		{"resim.png", KindOther, []byte("\x89PNG\r\n\x1a\n\x00\x00"), ReasonUnsupported},
	}
	for _, c := range cases {
		s := Summarize(c.kind, c.name, c.data)
		if s.Readable || s.Reason != c.reason {
			t.Errorf("%s: %+v, want reason %s", c.name, s, c.reason)
		}
	}
	big := make([]byte, MaxBytes+1)
	if s := Summarize(KindIsletme, "big.csv", big); s.Readable || s.Reason != ReasonTooLarge {
		t.Errorf("big: %+v", s)
	}
}

func TestBeratReadable(t *testing.T) {
	s := Summarize(KindBerat, "1234567890-202610-YB-000001.xml", []byte(`<?xml version="1.0"?><edefter:berat xmlns:edefter="x"/>`))
	if !s.Readable || s.EDefter != nil {
		t.Fatalf("berat: %+v", s)
	}
}
