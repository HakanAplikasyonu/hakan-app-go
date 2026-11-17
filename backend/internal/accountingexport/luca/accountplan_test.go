package luca

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

func planTemplate(t *testing.T) ae.CSVTemplate {
	t.Helper()
	tpl, err := AccountPlanTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

const planHeader = "Hesap Kodu;Hesap Adı;Vergi No;Vergi Dairesi;T.C. Kimlik No;Adres;Döviz;E-Posta;Kdv Oran;Kdv Hesap Kodu"

// win1254 is the expected Windows-1254 bytes of an ASCII + Turkish string.
func win1254(s string) []byte {
	tr := map[rune]byte{'ı': 0xFD, 'İ': 0xDD, 'ş': 0xFE, 'Ş': 0xDE, 'ğ': 0xF0, 'Ğ': 0xD0, 'ü': 0xFC, 'Ü': 0xDC, 'ö': 0xF6, 'Ö': 0xD6, 'ç': 0xE7, 'Ç': 0xC7}
	var out []byte
	for _, r := range s {
		if b, ok := tr[r]; ok {
			out = append(out, b)
			continue
		}
		if r > 0x7F {
			panic("unexpected rune in test string")
		}
		out = append(out, byte(r))
	}
	return out
}

func TestAccountPlanTemplateUnverified(t *testing.T) {
	tpl := planTemplate(t)
	if tpl.Verified || tpl.Version != AccountPlanTemplateDefault || len(tpl.Columns) != 10 {
		t.Fatalf("template: %+v", tpl)
	}
	if !slices.Contains(AccountPlanVersions(), AccountPlanTemplateDefault) {
		t.Fatalf("versions: %v", AccountPlanVersions())
	}
	if _, err := AccountPlanTemplate("nope"); !errors.Is(err, ae.ErrTemplate) {
		t.Fatalf("unknown version: %v", err)
	}
}

// Header only: Windows-1254 bytes, ";" between fields, no line ending at
// the end of the header row.
func TestAccountPlanHeaderBytes(t *testing.T) {
	res, err := WriteAccountPlan(planTemplate(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.Data, win1254(planHeader)) {
		t.Fatalf("header bytes:\n% X\nwant\n% X", res.Data, win1254(planHeader))
	}
	// "Adı" → 41 64 FD; "Döviz" → 44 F6 76 69 7A.
	if !bytes.Contains(res.Data, []byte{0x41, 0x64, 0xFD}) || !bytes.Contains(res.Data, []byte{0x44, 0xF6, 0x76, 0x69, 0x7A}) {
		t.Fatalf("Turkish characters not in Windows-1254: % X", res.Data)
	}
	if bytes.HasSuffix(res.Data, []byte("\r\n")) || bytes.HasSuffix(res.Data, []byte("\n")) {
		t.Fatal("header row ends with a line ending")
	}
}

func TestAccountPlanRows(t *testing.T) {
	accounts := []PlanAccount{
		{Code: "120.01.002", Name: "Şirin Gıda Ltd. Şti.", TaxNo: "1234567890", TaxOffice: "Kadıköy", Address: "Bağdat Cad. 5; Kat 2", Email: "muhasebe@sirin.example"},
		{Code: "120.01.001", Name: "Öztürk İnşaat", TCKN: "12345678901", Currency: "TRY"},
		{Code: "120.01.010", Name: `Çağ "Büyük" Market`},
	}
	res, err := WriteAccountPlan(planTemplate(t), accounts)
	if err != nil {
		t.Fatal(err)
	}
	want := win1254(planHeader + "\r\n" +
		"120.01.001;Öztürk İnşaat;;;12345678901;;TRY;;;\r\n" +
		"120.01.002;Şirin Gıda Ltd. Şti.;1234567890;Kadıköy;;\"Bağdat Cad. 5; Kat 2\";;muhasebe@sirin.example;;\r\n" +
		"120.01.010;\"Çağ \"\"Büyük\"\" Market\";;;;;;;;")
	if !bytes.Equal(res.Data, want) {
		t.Fatalf("rows:\n%q\nwant\n%q", res.Data, want)
	}
	if res.Rows != 3 || res.Replaced != 0 {
		t.Fatalf("result: %+v", res)
	}
	// ı = FD, ö = F6, ş = FE, İ = DD.
	for _, b := range []byte{0xFD, 0xF6, 0xFE, 0xDD} {
		if !bytes.Contains(res.Data, []byte{b}) {
			t.Fatalf("byte %X missing", b)
		}
	}
	if bytes.Contains(res.Data, []byte{0xC4}) || bytes.Contains(res.Data, []byte{0xC5}) {
		t.Fatal("UTF-8 lead bytes in a Windows-1254 file")
	}
}

func TestAccountPlanRequiredFields(t *testing.T) {
	_, err := WriteAccountPlan(planTemplate(t), []PlanAccount{{Code: "120.01.001", Name: "A"}, {Code: "120.01.002", Name: "  "}})
	var fe *ae.FieldError
	if !errors.As(err, &fe) || !errors.Is(err, ae.ErrMissingField) || fe.Row != 2 || fe.Column != "Hesap Adı" {
		t.Fatalf("missing name: %v", err)
	}
	if _, err := WriteAccountPlan(planTemplate(t), []PlanAccount{{Name: "Kodsuz"}}); !errors.Is(err, ae.ErrMissingField) {
		t.Fatalf("missing code: %v", err)
	}
}

// Delimiter, final line ending, trailing delimiter, encoding are template
// data: a variant changes the bytes without code changes.
func TestAccountPlanFormatOptions(t *testing.T) {
	tpl := planTemplate(t)
	tpl.CSV.Delimiter = ","
	tpl.CSV.FinalLineEnd = true
	tpl.CSV.TrailingDelimiter = true
	tpl.Columns = tpl.Columns[:2]
	res, err := WriteAccountPlan(tpl, []PlanAccount{{Code: "320.01.001", Name: "Kara, Ak ve Oğulları"}})
	if err != nil {
		t.Fatal(err)
	}
	want := win1254("Hesap Kodu,Hesap Adı,\r\n320.01.001,\"Kara, Ak ve Oğulları\",\r\n")
	if !bytes.Equal(res.Data, want) {
		t.Fatalf("comma variant:\n%q\nwant\n%q", res.Data, want)
	}
	tpl.CSV.Encoding = "utf-8"
	tpl.CSV.LineEnding = "lf"
	tpl.CSV.TrailingDelimiter = false
	res, err = WriteAccountPlan(tpl, []PlanAccount{{Code: "320.01.001", Name: "Işık"}})
	if err != nil || string(res.Data) != "Hesap Kodu,Hesap Adı\n320.01.001,Işık\n" {
		t.Fatalf("utf-8 variant: %q %v", res.Data, err)
	}
	// A character Windows-1254 cannot hold becomes "?" and is counted.
	tpl.CSV.Encoding = ""
	res, err = WriteAccountPlan(tpl, []PlanAccount{{Code: "320.01.001", Name: "Ā Ltd"}})
	if err != nil || res.Replaced != 1 || !bytes.Contains(res.Data, []byte("? Ltd")) {
		t.Fatalf("replacement: %q %+v %v", res.Data, res, err)
	}
}

func TestCompareCodes(t *testing.T) {
	codes := []string{"120.10", "120.2", "120", "120.01.001", "100", "120.01"}
	slices.SortFunc(codes, CompareCodes)
	if want := []string{"100", "120", "120.01", "120.01.001", "120.2", "120.10"}; !slices.Equal(codes, want) {
		t.Fatalf("order: %v", codes)
	}
}

func TestNextSubaccount(t *testing.T) {
	plan := []string{"120", "120.01", "120.01.001", "120.01.002", "120.01.007", "120.02", "320", "320.001", "320.002", "159.A1"}
	cases := []struct {
		name, main string
		plan       []string
		taken      []string
		want       string
		parent     string
	}{
		{"three levels inferred", "120", plan, nil, "120.01.008", "120.01"},
		{"handed out codes are skipped", "120", plan, []string{"120.01.008", "120.01.009"}, "120.01.010", "120.01"},
		{"two levels inferred", "320", plan, nil, "320.003", "320"},
		{"no sub account: fallback layout", "329", plan, nil, "329.01.001", "329.01"},
		{"letters ignored", "159", plan, nil, "159.01.001", "159.01"},
		{"latest parent wins", "120", append(plan, "120.02.001"), nil, "120.02.002", "120.02"},
	}
	for _, c := range cases {
		taken := map[string]bool{}
		for _, p := range c.plan {
			taken[p] = true
		}
		for _, x := range c.taken {
			taken[x] = true
		}
		got, parent, err := NextSubaccount(c.main, c.plan, taken, SubaccountLayout{2, 3})
		if err != nil || got != c.want || parent != c.parent {
			t.Errorf("%s: got %q under %q (%v), want %q under %q", c.name, got, parent, err, c.want, c.parent)
		}
	}
	// A full level (one digit: 770.1 … 770.9 used).
	full := []string{"770"}
	taken := map[string]bool{}
	for i := 1; i <= 9; i++ {
		c := "770." + string(rune('0'+i))
		full = append(full, c)
		taken[c] = true
	}
	if _, _, err := NextSubaccount("770", full, taken, nil); !errors.Is(err, ErrNoFreeCode) {
		t.Fatalf("full level: %v", err)
	}
	if _, _, err := NextSubaccount("12", nil, nil, nil); !errors.Is(err, ErrInvalidMain) {
		t.Fatalf("bad main: %v", err)
	}
	if got := DefaultSegments(planTemplate(t)); !slices.Equal(got, SubaccountLayout{2, 3}) {
		t.Fatalf("default segments: %v", got)
	}
}
