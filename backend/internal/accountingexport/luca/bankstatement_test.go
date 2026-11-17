package luca

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

func bankTemplate(t *testing.T, o BankOptions) ae.CSVTemplate {
	t.Helper()
	tpl, err := BankStatementTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	if tpl.Verified {
		t.Fatal("bank statement template must stay unverified")
	}
	tpl, err = ApplyBankOptions(tpl, o)
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

var bankLines = []BankLine{
	{Date: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), DocNo: "DKT-0042", Description: "Şirin Gıda tahsilat; fatura ABC2026", Incoming: true,
		Amount: decimal.RequireFromString("18500.5"), TaxID: "1234567890", IBAN: "TR330006100519786457841326", Currency: "TRY"},
	{Date: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), Description: "Kira ödemesi", Amount: decimal.RequireFromString("25000"), Currency: "TRY"},
	{Date: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC), Description: "Döviz alış", Amount: decimal.RequireFromString("1234567.891"), Currency: "USD"},
}

// Default layout: one signed Tutar column (giriş +, çıkış −), dd.MM.yyyy,
// decimal comma, empty fields left empty, Windows-1254.
func TestBankStatementSigned(t *testing.T) {
	res, err := WriteBankStatement(bankTemplate(t, BankOptions{}), bankLines)
	if err != nil {
		t.Fatal(err)
	}
	want := win1254("Evrak Tarihi;Evrak No;Açıklama;Tutar;VKN/TCKN;IBAN;Döviz\r\n" +
		"03.10.2026;DKT-0042;\"Şirin Gıda tahsilat; fatura ABC2026\";18500,50;1234567890;TR330006100519786457841326;TRY\r\n" +
		"15.10.2026;;Kira ödemesi;-25000,00;;;TRY\r\n" +
		"31.10.2026;;Döviz alış;-1234567,89;;;USD")
	if !bytes.Equal(res.Data, want) {
		t.Fatalf("signed:\n%q\nwant\n%q", res.Data, want)
	}
	// ş = FE, ö = F6, ı = FD in Windows-1254.
	for _, b := range []byte{0xFE, 0xF6, 0xFD} {
		if !bytes.Contains(res.Data, []byte{b}) {
			t.Fatalf("byte %X missing", b)
		}
	}
}

// Opposite sign rule and two-column layout (Borç = çıkış, Alacak = giriş);
// delimiter, quote and thousands separator from options / template.
func TestBankStatementLayouts(t *testing.T) {
	res, err := WriteBankStatement(bankTemplate(t, BankOptions{IncomingSign: "-"}), bankLines[:2])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(res.Data, []byte(";-18500,50;")) || !bytes.Contains(res.Data, []byte(";25000,00;")) {
		t.Fatalf("incoming minus: %q", res.Data)
	}
	tpl := bankTemplate(t, BankOptions{AmountLayout: AmountTwoColumns, Delimiter: ",", Quote: "'"})
	tpl.Options["thousands"] = "."
	res, err = WriteBankStatement(tpl, bankLines)
	if err != nil {
		t.Fatal(err)
	}
	want := win1254("Evrak Tarihi,Evrak No,Açıklama,Borç,Alacak,VKN/TCKN,IBAN,Döviz\r\n" +
		"03.10.2026,DKT-0042,Şirin Gıda tahsilat; fatura ABC2026,,'18.500,50',1234567890,TR330006100519786457841326,TRY\r\n" +
		"15.10.2026,,Kira ödemesi,'25.000,00',,,,TRY\r\n" +
		"31.10.2026,,Döviz alış,'1.234.567,89',,,,USD")
	if !bytes.Equal(res.Data, want) {
		t.Fatalf("two columns:\n%q\nwant\n%q", res.Data, want)
	}
}

func TestBankStatementRequiredAndOptions(t *testing.T) {
	_, err := WriteBankStatement(bankTemplate(t, BankOptions{}), []BankLine{{Description: "tarihsiz", Amount: decimal.NewFromInt(1)}})
	var fe *ae.FieldError
	if !errors.As(err, &fe) || fe.Column != "Evrak Tarihi" {
		t.Fatalf("missing date: %v", err)
	}
	tpl, _ := BankStatementTemplate("")
	for _, o := range []BankOptions{{AmountLayout: "split"}, {IncomingSign: "x"}, {Delimiter: ";;"}} {
		if _, err := ApplyBankOptions(tpl, o); !errors.Is(err, ae.ErrTemplate) {
			t.Errorf("%+v: %v", o, err)
		}
	}
	// The guide fields are template data.
	for _, c := range tpl.Columns {
		if c.ProgramField == "" || c.DataType == "" {
			t.Fatalf("column %s has no mapping guide", c.Key)
		}
	}
}

func TestMoneyFormat(t *testing.T) {
	cases := []struct{ v, dec, th, want string }{
		{"0", ",", "", "0,00"},
		{"-0.005", ",", "", "-0,01"},
		{"999.999", ",", ".", "1.000,00"},
		{"1234567.5", ".", ",", "1,234,567.50"},
		{"12.3", "", "", "12.30"},
	}
	for _, c := range cases {
		if got := money(decimal.RequireFromString(c.v), c.dec, c.th); got != c.want {
			t.Errorf("money(%s, %q, %q) = %q, want %q", c.v, c.dec, c.th, got, c.want)
		}
	}
}
