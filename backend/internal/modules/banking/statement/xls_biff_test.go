package statement

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/xls/xlstest"
)

// Binary Excel 97-2003 (.xls, BIFF8) statements. The workbooks are written
// in the test with xlstest from the made-up testdata/banks fixtures: dates
// become Excel serial numbers, times day fractions and amounts numbers,
// as a bank's "Excel'e aktar" stores them.

var (
	reCellDate     = regexp.MustCompile(`^(\d{2})\.(\d{2})\.(\d{4})$`)
	reCellDateTime = regexp.MustCompile(`^(\d{2})\.(\d{2})\.(\d{4}) (\d{2}):(\d{2})$`)
	reCellClock    = regexp.MustCompile(`^(\d{2}):(\d{2})$`)
	reCellAmount   = regexp.MustCompile(`^[+-]?\d{1,3}(\.\d{3})*,\d{2}$`)
)

var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// excelSerial: the 1900-system serial of a day (2025-01-01 is 45658).
func excelSerial(d time.Time) int { return int(d.Sub(excelEpoch).Hours() / 24) }

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// biffCells turns a text table into typed workbook cells.
func biffCells(t *testing.T, rows [][]string) [][]any {
	t.Helper()
	out := make([][]any, len(rows))
	for i, r := range rows {
		out[i] = make([]any, len(r))
		for j, c := range r {
			switch {
			case reCellDate.MatchString(c):
				m := reCellDate.FindStringSubmatch(c)
				out[i][j] = excelSerial(time.Date(atoi(m[3]), time.Month(atoi(m[2])), atoi(m[1]), 0, 0, 0, 0, time.UTC))
			case reCellDateTime.MatchString(c):
				m := reCellDateTime.FindStringSubmatch(c)
				day := excelSerial(time.Date(atoi(m[3]), time.Month(atoi(m[2])), atoi(m[1]), 0, 0, 0, 0, time.UTC))
				out[i][j] = float64(day) + float64(atoi(m[4])*60+atoi(m[5]))/1440
			case reCellClock.MatchString(c):
				m := reCellClock.FindStringSubmatch(c)
				out[i][j] = float64(atoi(m[1])*60+atoi(m[2])) / 1440
			case reCellAmount.MatchString(c):
				d, err := parseAmount(c)
				if err != nil {
					t.Fatal(err)
				}
				out[i][j] = d.InexactFloat64()
			default:
				out[i][j] = c
			}
		}
	}
	return out
}

// biffXLS writes rows as a binary .xls (a second, empty-ish sheet that
// must be ignored).
func biffXLS(t *testing.T, rows [][]string) []byte {
	t.Helper()
	return xlstest.Write(
		xlstest.Sheet{Name: "Hesap Hareketleri", Rows: biffCells(t, rows)},
		xlstest.Sheet{Name: "Bilgi", Rows: [][]any{{"x"}}},
	)
}

// Every bank's layout reads the same from a binary .xls as from the HTML
// ".xls": same bank, account, template and movements.
func TestBIFF8AllBanksLikeHTML(t *testing.T) {
	for file, want := range bankFixtures {
		t.Run(file, func(t *testing.T) {
			rows := fixtureRows(t, file)
			data := biffXLS(t, rows)
			if Detect(file+".xls", data) != "excel" {
				t.Fatalf("detect %s", Detect(file+".xls", data))
			}
			sts, format, err := Parse(file+".xls", data)
			if err != nil {
				t.Fatal(err)
			}
			if format != "excel" {
				t.Fatalf("format %s", format)
			}
			checkBankStatement(t, sts[0], want.bank, want.name)
			ref, _, err := Parse(file+".xls", htmlXLS(rows))
			if err != nil {
				t.Fatal(err)
			}
			got, exp := sts[0], ref[0]
			if got.BankID != exp.BankID || got.AccountIBAN != exp.AccountIBAN || !reflect.DeepEqual(got.Template, exp.Template) {
				t.Fatalf("bank %s iban %s template %+v, html: %s %s %+v", got.BankID, got.AccountIBAN, got.Template, exp.BankID, exp.AccountIBAN, exp.Template)
			}
			// Raw keeps the file's own cell texts ("12.500,00" vs
			// "12500"); everything read from them, the row key (SourceID)
			// included, is the same.
			for i := range got.Lines {
				got.Lines[i].Raw = nil
			}
			for i := range exp.Lines {
				exp.Lines[i].Raw = nil
			}
			if !reflect.DeepEqual(got.Lines, exp.Lines) {
				t.Fatalf("lines differ from HTML:\n%+v\n%+v", got.Lines, exp.Lines)
			}
		})
	}
}

// Ziraat (single signed amount, receipt number) and Yapı Kredi (separate
// time column stored as a day fraction) through BIFF8.
func TestBIFF8ZiraatAndYapiKredi(t *testing.T) {
	for file, bank := range map[string]string{"ziraat": "ziraat", "yapikredi": "yapikredi"} {
		t.Run(file, func(t *testing.T) {
			sts, _, err := Parse(file+".xls", biffXLS(t, fixtureRows(t, file)))
			if err != nil {
				t.Fatal(err)
			}
			checkMoves(t, sts[0], bank, fixtureMoves)
			if g := CheckRunning(sts[0].Lines); g != nil {
				t.Fatalf("running balance: %s", g.Reason)
			}
			l := sts[0].Lines[1]
			switch file {
			case "ziraat":
				if l.ReceiptNumber != "880001" || l.HasTime {
					t.Fatalf("receipt %q has time %v", l.ReceiptNumber, l.HasTime)
				}
			case "yapikredi":
				if !l.HasTime || l.BookedAt.Format("2006-01-02 15:04:05") != "2026-10-02 10:40:00" {
					t.Fatalf("booked at %s (%v)", l.BookedAt, l.HasTime)
				}
			}
			// The table preview reads the same file without row errors.
			a, err := AnalyzeFile(context.Background(), file+".xls", biffXLS(t, fixtureRows(t, file)), nil, nil, ColumnRequest{})
			if err != nil || a.Format != "excel" || a.Lines != 5 || a.RowErrors != 0 {
				t.Fatalf("analysis %s lines %d row errors %d: %v", a.Format, a.Lines, a.RowErrors, err)
			}
		})
	}
}

// Excel serial dates: whole days, a time fraction (rounded to the second,
// not truncated to 08:59:59) and days after the 1900 leap-year bug.
func TestExcelSerialDates(t *testing.T) {
	for _, c := range []struct {
		in, want string
		hasTime  bool
	}{
		{"45658", "2025-01-01 00:00:00", false},
		{"44927", "2023-01-01 00:00:00", false},
		{"46296", "2026-10-01 00:00:00", false},
		{"46081", "2026-02-28 00:00:00", false},
		{"46082", "2026-03-01 00:00:00", false},
		{"45658.375", "2025-01-01 09:00:00", true},
		{"45658.38333333333", "2025-01-01 09:12:00", true},
		{"45658.99998842592", "2025-01-01 23:59:59", true},
	} {
		d, hasTime, err := parseDate(c.in)
		if err != nil || d.Format("2006-01-02 15:04:05") != c.want || hasTime != c.hasTime {
			t.Errorf("%s: %s %v %v, want %s", c.in, d, hasTime, err, c.want)
		}
	}
	if excelSerial(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) != 46296 {
		t.Fatal("test serial helper")
	}
	for in, want := range map[string]string{"09:12": "09:12", "0.38333333333333336": "09:12", "0.5": "12:00", "0": "00:00"} {
		h, m, ok := parseClock(in)
		if got := time.Date(2026, 1, 1, h, m, 0, 0, time.UTC).Format("15:04"); !ok || got != want {
			t.Errorf("clock %s: %s %v", in, got, ok)
		}
	}
	if _, _, ok := parseClock("1.5"); ok {
		t.Error("clock 1.5 read")
	}
}

// Amounts stored as numbers come out as exact decimals: no float rounding
// (1234.56, -0.10, a value with no exact binary form, a formula result).
func TestBIFF8DecimalAmounts(t *testing.T) {
	day := excelSerial(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	data := xlstest.Write(xlstest.Sheet{Name: "Hareketler", Rows: [][]any{
		{"ÖRNEK BANKASI"},
		{"Tarih", "Açıklama", "Tutar", "Bakiye"},
		{day, "GELEN HAVALE ACME", 1234.56, 11234.56},
		{day + 1, "HESAP ISLETIM UCRETI", -0.10, 11234.46},
		{day + 1, "POS SATIS", 0.3, xlstest.FormulaNum(11234.76)},
		{day + 2, "KIRA ODEMESI", -9876543.21, -9865308.45},
	}})
	sts, format, err := Parse("ekstre.xls", data)
	if err != nil {
		t.Fatal(err)
	}
	st := sts[0]
	if format != "excel" || len(st.Lines) != 4 || len(st.Errors) != 0 {
		t.Fatalf("format %s lines %+v errors %v", format, st.Lines, st.Errors)
	}
	want := []struct {
		amount  string
		dir     banking.Direction
		balance string
		date    string
	}{
		{"1234.56", banking.DirIn, "11234.56", "2026-11-02"},
		{"0.1", banking.DirOut, "11234.46", "2026-11-03"},
		{"0.3", banking.DirIn, "11234.76", "2026-11-03"},
		{"9876543.21", banking.DirOut, "-9865308.45", "2026-11-04"},
	}
	for i, w := range want {
		l := st.Lines[i]
		if l.Amount != w.amount || l.Direction != w.dir || l.BalanceAfter == nil || *l.BalanceAfter != w.balance ||
			l.TransactionDate.Format("2006-01-02") != w.date {
			t.Errorf("line %d: %s %s %v %s, want %+v", i, l.Amount, l.Direction, l.BalanceAfter, l.TransactionDate, w)
		}
	}
}

// Routing by the first bytes: OLE (D0 CF 11 E0) is BIFF8, markup with a
// <table> the HTML reader, an Excel 2003 XML workbook the SpreadsheetML
// reader; the file name does not matter.
func TestXLSRoutingByMagic(t *testing.T) {
	rows := fixtureRows(t, "ziraat")
	for name, data := range map[string][]byte{
		"biff8":         biffXLS(t, rows),
		"html":          htmlXLS(rows),
		"spreadsheetml": spreadsheetML(rows),
	} {
		t.Run(name, func(t *testing.T) {
			if isBIFF(data) != (name == "biff8") || isHTMLTable(data) != (name == "html") || isSpreadsheetML(data) != (name == "spreadsheetml") {
				t.Fatalf("biff %v html %v xml %v", isBIFF(data), isHTMLTable(data), isSpreadsheetML(data))
			}
			for _, fn := range []string{"ekstre.xls", "ekstre.xlsx", "ekstre"} {
				sts, format, err := Parse(fn, data)
				if err != nil {
					t.Fatalf("%s: %v", fn, err)
				}
				if format != "excel" {
					t.Fatalf("%s: format %s", fn, format)
				}
				checkMoves(t, sts[0], "ziraat", fixtureMoves)
			}
		})
	}
}

// A binary .xls that cannot be read (damaged, password protected) is the
// one ".xls" error left; the office saves it again or downloads CSV.
func TestBIFF8Unreadable(t *testing.T) {
	junk := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 512)...)
	if Detect("ekstre.xls", junk) != "excel" {
		t.Fatal("detect")
	}
	if _, _, err := Parse("ekstre.xls", junk); !errors.Is(err, ErrLegacyXLS) || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("damaged: %v", err)
	}
	// Password protected: the first record after the globals BOF becomes
	// a FILEPASS record.
	data := biffXLS(t, fixtureRows(t, "ziraat"))
	bof := []byte{0x09, 0x08, 0x10, 0x00, 0x00, 0x06, 0x05, 0x00}
	i := bytes.Index(data, bof)
	if i < 0 {
		t.Fatal("no globals BOF")
	}
	data[i+20], data[i+21] = 0x2F, 0x00
	if _, _, err := Parse("ekstre.xls", data); !errors.Is(err, ErrLegacyXLS) {
		t.Fatalf("password: %v", err)
	}
	if _, err := AnalyzeFile(context.Background(), "ekstre.xls", data, nil, nil, ColumnRequest{}); !errors.Is(err, ErrLegacyXLS) {
		t.Fatalf("analyze password: %v", err)
	}
}
