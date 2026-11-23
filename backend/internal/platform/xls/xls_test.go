package xls

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/xls/xlstest"
)

// Fixtures are built in the test with made-up values.

func TestReadCellKinds(t *testing.T) {
	data := xlstest.Write(
		xlstest.Sheet{Name: "Bordro", Rows: [][]any{
			{"EKİM 2026 ÜCRET BORDROSU", nil, "Tarih: 05/11/2026"},
			{},
			{xlstest.Label("AYŞE ÖRNEK"), 10000000146.0, "20000000046.0", 30.0},
			// One MULRK run: integer, cents (÷100), a float RK.
			{"Tutar", 70000.0, 531.3, 0.5, 1234.56},
			// NUMBER (not RK-exact), formulas, a padded label.
			{4200.000000000001, xlstest.FormulaNum(12.75), xlstest.FormulaStr("TOPLAM"), xlstest.Label("  Net  ")},
			{-5, 0.0},
		}},
		xlstest.Sheet{Name: "Sayfa İki", Rows: [][]any{{nil, "şçğ"}}},
	)
	if !IsXLS(data) {
		t.Fatal("magic")
	}
	sheets, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 || sheets[0].Name != "Bordro" || sheets[1].Name != "Sayfa İki" {
		t.Fatalf("sheets: %+v", sheets)
	}
	want := [][]string{
		{"EKİM 2026 ÜCRET BORDROSU", "", "Tarih: 05/11/2026"},
		{},
		{"AYŞE ÖRNEK", "10000000146", "20000000046.0", "30"},
		{"Tutar", "70000", "531.3", "0.5", "1234.56"},
		{"4200.000000000001", "12.75", "TOPLAM", "Net"},
		{"-5", "0"},
	}
	if !reflect.DeepEqual(sheets[0].Rows, want) {
		t.Fatalf("rows:\n%q\nwant\n%q", sheets[0].Rows, want)
	}
	if !reflect.DeepEqual(sheets[1].Rows, [][]string{{"", "şçğ"}}) {
		t.Fatalf("second: %q", sheets[1].Rows)
	}
}

// A shared string table split over CONTINUE records, also inside the
// characters of a string (compressed and 16-bit).
func TestReadSSTContinue(t *testing.T) {
	var rows [][]any
	var want [][]string
	for i := 0; i < 30; i++ {
		s := strings.Repeat("Hesap adı ", i%4+1) + string(rune('A'+i))
		if i%3 == 0 {
			s = strings.Repeat("abc", 7) + string(rune('A'+i))
		}
		rows = append(rows, []any{s})
		want = append(want, []string{strings.TrimSpace(s)})
	}
	data := xlstest.Book{MaxRecord: 37, Sheets: []xlstest.Sheet{{Name: "Mizan", Rows: rows}}}.Bytes()
	sheets, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sheets[0].Rows, want) {
		t.Fatalf("rows:\n%q\nwant\n%q", sheets[0].Rows, want)
	}
}

// A continued character run starts with its own option byte, which may
// switch from compressed to 16-bit characters.
func TestReadSSTOptionByte(t *testing.T) {
	le := binary.LittleEndian
	seg1 := le.AppendUint32(nil, 1)
	seg1 = le.AppendUint32(seg1, 1)
	seg1 = le.AppendUint16(seg1, 4)
	seg1 = append(seg1, 0, 'a', 'b')
	seg2 := []byte{1}
	seg2 = le.AppendUint16(seg2, 'ş')
	seg2 = le.AppendUint16(seg2, 'c')
	got, err := readSST([][]byte{seg1, seg2})
	if err != nil || len(got) != 1 || got[0] != "abşc" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRKText(t *testing.T) {
	// rkInt encodes a 30-bit integer RK (div100: the value is n / 100).
	rkInt := func(n int32, div100 bool) uint32 {
		x := uint32(n)<<2 | 2
		if div100 {
			x |= 1
		}
		return x
	}
	cases := map[uint32]string{
		rkInt(2, false):                          "2",
		rkInt(53130, true):                       "531.3",
		rkInt(-5, false):                         "-5",
		rkInt(-150, true):                        "-1.5",
		uint32(math.Float64bits(0.5) >> 32):      "0.5",
		uint32(math.Float64bits(1234.5)>>32) | 1: "12.345",
	}
	for rk, want := range cases {
		if got := rkText(rk); got != want {
			t.Errorf("%#x: %q, want %q", rk, got, want)
		}
	}
}

func TestReadNotXLS(t *testing.T) {
	if _, err := Read([]byte("PK\x03\x04 not a workbook")); !errors.Is(err, ErrNotXLS) {
		t.Fatalf("zip: %v", err)
	}
	// The signature with nothing behind it: damaged, not a panic.
	junk := append(append([]byte{}, Magic...), make([]byte, 600)...)
	if _, err := Read(junk); err == nil {
		t.Fatal("junk read")
	}
	if IsXLS([]byte{0xD0, 0xCF, 0x11, 0xE0}) {
		t.Fatal("short magic")
	}
}
