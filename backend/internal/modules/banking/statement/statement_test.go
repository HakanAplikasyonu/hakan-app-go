package statement

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf/v2"
	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Test IBANs (valid check digits, no real account).
const (
	ibanZiraat = "TR760001000519786457841326"
	ibanAkbank = "TR870004600099888000012345"
)

// Two accounts in one file, SWIFT envelope, CRLF, an RC storno, a year-end
// line and a Turkish :86: (23 §7 tests 1–4).
var sample940 = "{1:F01ZIRATRIS0000000000}{2:I940XXXXXXXXXXXXN}{4:\r\n" +
	":20:STMT-1\r\n" +
	":25:" + ibanZiraat + "\r\n" +
	":28C:1/1\r\n" +
	":60F:C261231TRY10000,00\r\n" +
	":61:2612310102C1250,00NTRFNONREF//00012345\r\n" +
	":86:GÖNDEREN: YILDIZ MARKET LTD ŞTİ / VKN 9100000021 / IBAN TR32 0010 0099 9990 1234 5678 90\r\n" +
	"SORGU NO: 778899\r\n" +
	":61:2701020102D12,50NCOMNONREF\r\n" +
	":86:EFT KOMISYONU BSMV DAHIL\r\n" +
	":61:2701020102D300,00NTRFREF1//A1\r\n" +
	":86:FATURA ODEMESI ALICI: KALEM KIRTASIYE\r\n" +
	":61:2701020102RC300,00NTRFREF1//A2\r\n" +
	":86:IPTAL\r\n" +
	":62F:C270102TRY11237,50\r\n" +
	":64:C270102TRY11237,50\r\n" +
	"-}\r\n" +
	":20:STMT-2\r\n" +
	":25:" + ibanAkbank + "\r\n" +
	":60F:C270102USD500,00\r\n" +
	":61:270102C100,NTRFNONREF\r\n" +
	":86:SWIFT GELEN\r\n" +
	":62F:C270102USD600,00\r\n"

func TestMT940(t *testing.T) {
	sts, err := ParseMT940(strings.ReplaceAll(sample940, "\r\n", "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sts) != 2 {
		t.Fatalf("statements = %d", len(sts))
	}
	z := sts[0]
	if z.AccountIBAN != ibanZiraat || z.Currency != "TRY" || len(z.Lines) != 4 {
		t.Fatalf("ziraat block: iban=%s cur=%s lines=%d errs=%v", z.AccountIBAN, z.Currency, len(z.Lines), z.Errors)
	}
	first := z.Lines[0]
	// Year end: value 2026-12-31, entry 0102 → 2027-01-02.
	if first.TransactionDate.Format("2006-01-02") != "2027-01-02" || first.ValueDate.Format("2006-01-02") != "2026-12-31" {
		t.Errorf("year end: tx=%s value=%s", first.TransactionDate, first.ValueDate)
	}
	if first.Direction != banking.DirIn || first.Amount != "1250" || first.MT940Code != "TRF" || first.BankReference != "00012345" {
		t.Errorf("61 fields: %+v", first.Transaction)
	}
	if first.CounterpartyName != "YILDIZ MARKET LTD ŞTİ" || first.CounterpartyTaxID != "9100000021" ||
		first.CounterpartyIBAN != "TR320010009999901234567890" || first.EFTQueryNumber != "778899" {
		t.Errorf("86 extraction: name=%q tax=%q iban=%q sorgu=%q", first.CounterpartyName, first.CounterpartyTaxID, first.CounterpartyIBAN, first.EFTQueryNumber)
	}
	// RC: credit reversal → money out (B), linked with the original D line.
	pay, rc := z.Lines[2], z.Lines[3]
	if rc.Direction != banking.DirOut || !rc.Reversal {
		t.Errorf("RC must be B and reversal: dir=%s rev=%v", rc.Direction, rc.Reversal)
	}
	// The D 300 is not reversed by an RC (both are outflows); RD would be.
	if pay.IsCancelled {
		t.Errorf("RC must not cancel a debit")
	}
	if z.Opening == nil || z.Closing == nil || !z.Closing.Amount.Equal(z.Closing.Amount) {
		t.Fatal("balances")
	}
	if z.Lines[0].SourceID == "" || z.Lines[0].SourceID == z.Lines[1].SourceID {
		t.Error("source ids")
	}
	a := sts[1]
	if a.AccountIBAN != ibanAkbank || a.Currency != "USD" || a.Lines[0].Amount != "100" || a.Lines[0].Currency != "USD" {
		t.Errorf("akbank block: %+v", a)
	}
	if g := CheckBlock(a); g != nil {
		t.Errorf("akbank continuity: %s", g.Reason)
	}
	// Ziraat: 10000 + 1250 − 12,50 − 300 − 300 (RC is an outflow) = 10637,50 ≠ 11237,50
	if g := CheckBlock(z); g == nil {
		t.Error("ziraat block should report a gap")
	}
}

func TestMT940ReversalLinksOriginal(t *testing.T) {
	text := ":20:X\n:25:" + ibanZiraat + "\n:60F:C261101TRY1000,00\n" +
		":61:2611011101D200,00NTRFNONREF\n:86:ODEME\n" +
		":61:2611011101RD200,00NTRFNONREF\n:86:IPTAL\n" +
		":62F:C261101TRY1000,00\n"
	sts, err := ParseMT940(text)
	if err != nil {
		t.Fatal(err)
	}
	l := sts[0].Lines
	if l[1].Direction != banking.DirIn || !l[1].Reversal || !l[0].IsCancelled || !l[1].IsCancelled {
		t.Fatalf("RD storno: %+v / %+v", l[0].Transaction, l[1].Transaction)
	}
	if g := CheckBlock(sts[0]); g != nil {
		t.Fatalf("continuity: %s", g.Reason)
	}
}

func TestMT940Windows1254(t *testing.T) {
	enc, err := charmap.Windows1254.NewEncoder().String(":20:X\n:25:" + ibanZiraat + "\n:61:2611011101C10,00NTRFNONREF\n:86:ÖDEME ŞİRKET ĞÜÇ\n")
	if err != nil {
		t.Fatal(err)
	}
	text, cs := Decode([]byte(enc))
	if cs != "windows-1254" {
		t.Fatalf("charset %s", cs)
	}
	sts, err := ParseMT940(text)
	if err != nil {
		t.Fatal(err)
	}
	if got := sts[0].Lines[0].Description; got != "ÖDEME ŞİRKET ĞÜÇ" {
		t.Fatalf("description %q", got)
	}
}

// Newest-first CSV with the IBAN in the header area (23 §7 tests 8, 9).
const sampleCSV = "Hesap Hareketleri;;;;\n" +
	"IBAN:;TR76 0001 0005 1978 6457 8413 26;;;\n" +
	";;;;\n" +
	"Tarih;Açıklama;Tutar;Bakiye;Dekont No\n" +
	"03.10.2026 14:10;POS GÜN SONU;1.200,00;3.187,50;D3\n" +
	"02.10.2026;HESAP İŞLETİM ÜCRETİ;-12,50;1.987,50;D2\n" +
	"01.10.2026;FAST GELEN YILDIZ MARKET;1.000,00;2.000,00;D1\n" +
	"Toplam;;;;\n"

func TestCSVNewestFirst(t *testing.T) {
	sts, format, err := Parse("ekstre.csv", []byte(sampleCSV))
	if err != nil {
		t.Fatal(err)
	}
	st := sts[0]
	if format != "csv" || st.AccountIBAN != ibanZiraat || BankFromIBAN(st.AccountIBAN) != "ziraat" {
		t.Fatalf("detect: %s %s %s", format, st.AccountIBAN, BankFromIBAN(st.AccountIBAN))
	}
	if len(st.Lines) != 3 || len(st.Errors) != 0 {
		t.Fatalf("lines=%d errs=%v", len(st.Lines), st.Errors)
	}
	if st.Lines[0].Description != "FAST GELEN YILDIZ MARKET" || st.Lines[2].Description != "POS GÜN SONU" {
		t.Fatalf("order not reversed: %s … %s", st.Lines[0].Description, st.Lines[2].Description)
	}
	if g := CheckRunning(st.Lines); g != nil {
		t.Fatalf("running: %s", g.Reason)
	}
	fee := st.Lines[1]
	if fee.Direction != banking.DirOut || fee.Amount != "12.5" || *fee.BalanceAfter != "1987.5" || fee.ReceiptNumber != "D2" {
		t.Errorf("fee row: %+v", fee.Transaction)
	}
	if !st.Lines[2].HasTime || st.Lines[2].BookedAt.Hour() != 14 {
		t.Errorf("time not read: %v", st.Lines[2].BookedAt)
	}
	// Re-parsing gives the same keys (re-upload never duplicates).
	again, _, _ := Parse("ekstre.csv", []byte(sampleCSV))
	for i := range st.Lines {
		if again[0].Lines[i].SourceID != st.Lines[i].SourceID {
			t.Fatal("source ids are not stable")
		}
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]string{
		"1.234,56": "1234.56", "-1.234,56": "-1234.56", "27,299.33": "27299.33", "8208.00": "8208",
		"-555": "-555", "+1234,56": "1234.56", "1.000": "1000", "(12,50)": "-12.5", "1.234,56 TL": "1234.56",
		"12,50-": "-12.5",
	}
	for in, want := range cases {
		d, err := parseAmount(in)
		if err != nil || d.String() != want {
			t.Errorf("%q → %v %v, want %s", in, d, err, want)
		}
	}
}

func TestExcelSeparateDebitCredit(t *testing.T) {
	f := excelize.NewFile()
	rows := [][]any{
		{"Garanti BBVA Hesap Özeti"},
		{"IBAN", "TR87 0004 6000 9988 8000 0123 45"},
		{},
		{"İşlem Tarihi", "Valör", "Açıklama", "Borç", "Alacak", "Bakiye"},
		{"01.10.2026", "01.10.2026", "Kira Ekim", "22.000,00", "", "78.000,00"},
		{"02.10.2026", "02.10.2026", "Tahsilat", "", "5.000,00", "83.000,00"},
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
	sts, format, err := Parse("hareketler.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	st := sts[0]
	if format != "excel" || st.AccountIBAN != ibanAkbank || len(st.Lines) != 2 {
		t.Fatalf("excel: %s %s %d %v", format, st.AccountIBAN, len(st.Lines), st.Errors)
	}
	if st.Lines[0].Direction != banking.DirOut || st.Lines[0].Amount != "22000" || st.Lines[1].Direction != banking.DirIn {
		t.Errorf("directions: %+v %+v", st.Lines[0].Transaction, st.Lines[1].Transaction)
	}
	if st.Lines[0].ValueDate == nil {
		t.Error("value date")
	}
}

func makePDF(t *testing.T, lines [][]string) []byte {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 9)
	pdf.Text(10, 15, "Hesap Ozeti IBAN: "+ibanZiraat)
	xs := []float64{10, 40, 110, 150}
	for i, l := range lines {
		y := 30 + float64(i)*6
		for j, s := range l {
			x := xs[j]
			if j >= 2 && i > 0 { // right-align numbers in their column
				x = xs[j] + 30 - pdf.GetStringWidth(s)
			}
			pdf.Text(x, y, s)
		}
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTextPDF(t *testing.T) {
	data := makePDF(t, [][]string{
		{"Tarih", "Aciklama", "Tutar", "Bakiye"},
		{"01.10.2026", "FAST GELEN", "1.000,00", "2.000,00"},
		{"02.10.2026", "HESAP ISLETIM UCRETI", "-12,50", "1.987,50"},
	})
	sts, format, err := Parse("ekstre.pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	st := sts[0]
	if format != "pdf" || st.AccountIBAN != ibanZiraat || len(st.Lines) != 2 {
		t.Fatalf("pdf: %s iban=%s lines=%d errs=%v", format, st.AccountIBAN, len(st.Lines), st.Errors)
	}
	if st.Lines[1].Direction != banking.DirOut || st.Lines[1].Amount != "12.5" || st.Lines[1].CaptureMethod != banking.CapturePDF {
		t.Errorf("pdf row: %+v", st.Lines[1].Transaction)
	}
	// 23 §5.3: a PDF whose balance does not add up never reaches the preview.
	bad := makePDF(t, [][]string{
		{"Tarih", "Aciklama", "Tutar", "Bakiye"},
		{"01.10.2026", "FAST GELEN", "1.000,00", "2.000,00"},
		{"02.10.2026", "MASRAF", "-12,50", "1.900,00"},
	})
	if _, _, err := Parse("bozuk.pdf", bad); err == nil {
		t.Fatal("broken balance accepted")
	}
}
