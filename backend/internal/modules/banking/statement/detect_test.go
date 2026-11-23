package statement

import (
	"errors"
	"fmt"
	"html"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// #780: the statement channel reads the ten big banks' internet branch
// exports in every shape they come in (CSV, .xlsx, the HTML or XML ".xls",
// Windows-1254 text, MT940) and names the bank from the layout, the IBAN or
// the title. The fixtures are the synthetic testdata/banks/<bank>.csv
// files; the other shapes are built from them here so every bank is
// covered in every shape.

// fixtureRows reads a CSV fixture into its cell table.
func fixtureRows(t *testing.T, name string) [][]string {
	t.Helper()
	rows, err := ReadCSV(string(readFixture(t, name)))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// htmlXLS writes rows as the HTML ".xls" an internet branch saves: a title
// table, then the movement table (cells escaped, Windows-1254 meta).
func htmlXLS(rows [][]string) []byte {
	var b strings.Builder
	b.WriteString(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=utf-8"></head><body>`)
	b.WriteString("<table>")
	for _, r := range rows {
		b.WriteString("<tr>")
		for _, c := range r {
			b.WriteString("<td>" + html.EscapeString(c) + "</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table></body></html>")
	return []byte(b.String())
}

// spreadsheetML writes rows as an Excel 2003 XML workbook (amounts as
// strings like the banks' exports; a second sheet that must be ignored).
func spreadsheetML(rows [][]string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<?mso-application progid="Excel.Sheet"?>` + "\n")
	b.WriteString(`<Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet" xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet">`)
	b.WriteString(`<Worksheet ss:Name="Hesap Hareketleri"><Table>`)
	for _, r := range rows {
		b.WriteString("<Row>")
		for _, c := range r {
			b.WriteString(`<Cell><Data ss:Type="String">` + html.EscapeString(c) + `</Data></Cell>`)
		}
		b.WriteString("</Row>\n")
	}
	b.WriteString(`</Table></Worksheet><Worksheet ss:Name="Bilgi"><Table><Row><Cell><Data ss:Type="String">x</Data></Cell></Row></Table></Worksheet></Workbook>`)
	return []byte(b.String())
}

func TestHTMLXLSAllBanks(t *testing.T) {
	for file, want := range bankFixtures {
		t.Run(file, func(t *testing.T) {
			sts, format, err := Parse(file+".xls", htmlXLS(fixtureRows(t, file)))
			if err != nil {
				t.Fatal(err)
			}
			if format != "excel" {
				t.Fatalf("format %s", format)
			}
			checkBankStatement(t, sts[0], want.bank, want.name)
		})
	}
}

func TestSpreadsheetMLAllBanks(t *testing.T) {
	for file, want := range bankFixtures {
		t.Run(file, func(t *testing.T) {
			sts, format, err := Parse(file+".xls", spreadsheetML(fixtureRows(t, file)))
			if err != nil {
				t.Fatal(err)
			}
			if format != "excel" {
				t.Fatalf("format %s", format)
			}
			checkBankStatement(t, sts[0], want.bank, want.name)
		})
	}
}

func TestWindows1254CSVAllBanks(t *testing.T) {
	for file, want := range bankFixtures {
		t.Run(file, func(t *testing.T) {
			data, err := charmap.Windows1254.NewEncoder().Bytes(readFixture(t, file))
			if err != nil {
				t.Fatal(err)
			}
			if _, enc := Decode(data); enc != "windows-1254" {
				t.Fatalf("encoding %s", enc)
			}
			sts, _, err := Parse(file+".csv", data)
			if err != nil {
				t.Fatal(err)
			}
			checkBankStatement(t, sts[0], want.bank, want.name)
		})
	}
}

// Checked-in samples of the two ".xls" shapes (Ziraat HTML with colspan
// title cells and &nbsp;, Yapı Kredi XML with ss:Index gaps).
func TestXLSSamples(t *testing.T) {
	for file, bank := range map[string]string{"ziraat_html.xls": "ziraat", "yapikredi_xml.xls": "yapikredi"} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "banks", file))
			if err != nil {
				t.Fatal(err)
			}
			sts, format, err := Parse(file, data)
			if err != nil {
				t.Fatal(err)
			}
			if format != "excel" || sts[0].BankID != bank || len(sts[0].Lines) != 3 || len(sts[0].Errors) != 0 {
				t.Fatalf("format %s bank %s lines %d errors %v", format, sts[0].BankID, len(sts[0].Lines), sts[0].Errors)
			}
			if g := CheckRunning(sts[0].Lines); g != nil {
				t.Fatalf("running balance: %s", g.Reason)
			}
		})
	}
}

func TestLegacyXLSRefused(t *testing.T) {
	data := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 512)...)
	if _, _, err := Parse("ekstre.xls", data); !errors.Is(err, ErrLegacyXLS) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestSlashDates(t *testing.T) {
	text := strings.ReplaceAll(string(readFixture(t, "garanti")), ".10.2026", "/10/2026")
	sts, _, err := Parse("garanti.csv", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	checkBankStatement(t, sts[0], "garanti", "Garanti")
	if d := sts[0].Lines[4].TransactionDate.Format("2006-01-02"); d != "2026-10-05" {
		t.Fatalf("date %s", d)
	}
}

// trIBAN builds a valid Turkish IBAN for a bank code (test accounts only).
func trIBAN(bankCode, account string) string {
	bban := bankCode + "0" + strings.Repeat("0", 16-len(account)) + account
	n, _ := new(big.Int).SetString(bban+"292700", 10) // "TR00" → 29 27 00
	check := 98 - new(big.Int).Mod(n, big.NewInt(97)).Int64()
	return fmt.Sprintf("TR%02d%s", check, bban)
}

// An unknown column layout (no built-in template) still reads through the
// header synonyms; the bank comes from the IBAN's bank code (digits 5–9).
func TestGenericLayoutBankFromIBAN(t *testing.T) {
	codes := map[string]string{
		"00010": "ziraat", "00012": "halkbank", "00015": "vakifbank", "00064": "isbank", "00062": "garanti",
		"00046": "akbank", "00067": "yapikredi", "00111": "qnb", "00134": "denizbank", "00205": "kuveyt_turk",
	}
	for code, bank := range codes {
		t.Run(bank, func(t *testing.T) {
			iban := trIBAN(code, "123456789")
			csv := "Hesap Özeti\n" +
				"Hesap No: " + iban[:4] + " " + iban[4:8] + " " + iban[8:12] + " " + iban[12:16] + " " + iban[16:20] + " " + iban[20:24] + " " + iban[24:] + "\n" +
				"\n" +
				"İşlem Tarihi;İşlem Açıklaması;Borç Tutarı;Alacak Tutarı;İşlem Sonrası Bakiye\n" +
				"01.10.2026;GELEN HAVALE ACME;;1.000,00;11.000,00\n" +
				"02.10.2026;KIRA ODEMESI;2.500,50;;8.499,50\n"
			sts, _, err := Parse("ekstre.csv", []byte(csv))
			if err != nil {
				t.Fatal(err)
			}
			st := sts[0]
			if st.AccountIBAN != iban || st.BankID != bank || st.Template == nil || st.Template.Source != SourceSynonyms {
				t.Fatalf("iban %s bank %s template %+v", st.AccountIBAN, st.BankID, st.Template)
			}
			if len(st.Lines) != 2 || st.Lines[1].Amount != "2500.5" {
				t.Fatalf("lines %+v", st.Lines)
			}
		})
	}
}

func TestSignedAmountColumn(t *testing.T) {
	csv := "Tarih,Açıklama,Tutar,Bakiye\n" +
		"03/10/2026,POS SATIS,\"1.234,56\",\"5.234,56\"\n" +
		"04/10/2026,EFT GIDEN,\"-234,56\",\"5.000,00\"\n"
	sts, _, err := Parse("hareketler.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	ls := sts[0].Lines
	if len(ls) != 2 || ls[0].Amount != "1234.56" || ls[1].Direction != banking.DirOut || ls[1].Amount != "234.56" {
		t.Fatalf("lines %+v", ls)
	}
}

func TestMT940BankFromIBAN(t *testing.T) {
	sts, err := ParseMT940(strings.ReplaceAll(sample940, "\r\n", "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sts[0].BankID != "ziraat" || sts[1].BankID != "akbank" {
		t.Fatalf("banks %s %s", sts[0].BankID, sts[1].BankID)
	}
}
