package statement

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/ioengine"
)

// Synthetic PDF statements (#780): no masked real PDF has arrived yet, so
// each bank's statement is drawn here from its testdata/banks/<bank>.csv
// fixture in the layout its internet branch prints: bank title, customer,
// IBAN and period above, the movement table with right-aligned amounts and
// wrapped long descriptions, a footer below. Names, IBANs and amounts are
// fake.

var reNumCell = regexp.MustCompile(`^-?[\d.]+,\d{2}$`)

type pdfBankLayout struct {
	widths  []float64 // column widths in mm, in the fixture's column order
	unicode bool      // embedded DejaVu (Identity-H) vs. core Helvetica (ASCII)
	perPage int       // movement rows per page (0: all on one)
}

var pdfBankLayouts = map[string]pdfBankLayout{
	// Tarih;Açıklama;Etiket;Tutar;Bakiye;Dekont No
	"garanti": {widths: []float64{22, 70, 25, 30, 32, 25}, unicode: true},
	// Tarih/Saat;Kanal;Açıklama;İşlem Tutarı;Bakiye;Ek Bilgi
	"isbank": {widths: []float64{30, 22, 110, 30, 32, 20}, unicode: true, perPage: 3},
	// Tarih;Fiş No;Açıklama;İşlem Tutarı;Bakiye
	"ziraat": {widths: []float64{22, 20, 75, 35, 35}},
	// Tarih;Saat;Tutar;Bakiye;Açıklama;Fiş/Dekont No
	"akbank": {widths: []float64{22, 14, 30, 32, 120, 25}, unicode: true},
	// Tarih;Saat;İşlem;Kanal;Açıklama;İşlem Tutarı;Bakiye
	"yapikredi": {widths: []float64{22, 14, 24, 20, 117, 30, 32}, unicode: true},
	// Tarih;Saat;İşlem No;Tutar;Bakiye;İşlem Adı;Açıklama
	"vakifbank": {widths: []float64{22, 14, 20, 30, 32, 20, 80}},
}

var asciiFold = strings.NewReplacer("ç", "c", "Ç", "C", "ğ", "g", "Ğ", "G", "ı", "i", "İ", "I", "ö", "o", "Ö", "O", "ş", "s", "Ş", "S", "ü", "u", "Ü", "U")

// syntheticPDF draws a fixture as a statement PDF.
func syntheticPDF(t *testing.T, file string, lay pdfBankLayout) []byte {
	t.Helper()
	return syntheticPDFRows(t, file, fixtureRows(t, file), lay)
}

func syntheticPDFRows(t *testing.T, file string, rows [][]string, lay pdfBankLayout) []byte {
	t.Helper()
	hdr := -1
	for i, r := range rows {
		if hdr < 0 && len(r) >= len(lay.widths) && strings.Contains(r[0], "Tarih") {
			hdr = i
		}
	}
	if hdr < 0 {
		t.Fatalf("%s: no header row", file)
	}
	var data [][]string
	for _, r := range rows[hdr+1:] {
		if len(r) < 2 || strings.TrimSpace(r[0]) == "" || strings.HasPrefix(r[0], "Toplam") {
			break
		}
		data = append(data, r)
	}

	p := gofpdf.New("L", "mm", "A4", "")
	p.SetMargins(10, 12, 10)
	p.SetAutoPageBreak(false, 10)
	family := "Helvetica"
	// Core-font statements print Turkish letters folded to ASCII.
	tr := asciiFold.Replace
	if lay.unicode {
		if err := ioengine.RegisterPDFFonts(p); err != nil {
			t.Fatal(err)
		}
		family = ioengine.PDFFontFamily
		tr = func(s string) string { return s }
	}
	text := func(s string) string { return tr(s) }

	pages := [][][]string{data}
	if lay.perPage > 0 {
		pages = nil
		for i := 0; i < len(data); i += lay.perPage {
			pages = append(pages, data[i:min(i+lay.perPage, len(data))])
		}
	}
	for pi, page := range pages {
		p.AddPage()
		p.SetFont(family, "B", 13)
		p.CellFormat(0, 7, text(rows[0][0]), "", 1, "L", false, 0, "")
		p.SetFont(family, "", 9)
		p.CellFormat(0, 5, text("Müşteri: ÖRNEK DANIŞMANLIK LTD. ŞTİ.  Şube: 0999 - Test Şubesi"), "", 1, "L", false, 0, "")
		for _, r := range rows[1:hdr] {
			if line := strings.TrimSpace(strings.Join(r, " ")); line != "" && line != "Hesap Hareketleri" {
				p.CellFormat(0, 5, text(strings.ReplaceAll(line, ";", "")), "", 1, "L", false, 0, "")
			}
		}
		p.Ln(3)
		// Table header.
		p.SetFont(family, "B", 8)
		for j, w := range lay.widths {
			align := "L"
			if j < len(data[0]) && reNumCell.MatchString(data[0][j]) {
				align = "R"
			}
			p.CellFormat(w, 6, text(rows[hdr][j]), "B", 0, align, false, 0, "")
		}
		p.Ln(-1)
		p.SetFont(family, "", 8)
		for _, r := range page {
			y := p.GetY()
			x := p.GetX()
			lines := 1
			for j, w := range lay.widths {
				c := ""
				if j < len(r) {
					c = r[j]
				}
				parts := wrapPDF(p, text(c), w-1)
				if len(parts) == 0 {
					parts = []string{""}
				}
				align := "L"
				if reNumCell.MatchString(c) {
					align = "R"
				}
				for k, s := range parts {
					p.SetXY(x, y+float64(k)*4)
					p.CellFormat(w, 4, s, "", 0, align, false, 0, "")
				}
				lines = max(lines, len(parts))
				x += w
			}
			p.SetXY(10, y+float64(lines)*4+1)
		}
		p.SetY(-15)
		p.SetFont(family, "", 7)
		p.CellFormat(0, 4, text("Bu belge elektronik ortamda üretilmiştir; ıslak imza gerektirmez."), "", 0, "L", false, 0, "")
		p.CellFormat(0, 4, text(fmt.Sprintf("Sayfa %d/%d", pi+1, len(pages))), "", 0, "R", false, 0, "")
	}
	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// wrapPDF splits s into lines that fit width w (word wrap, like the
// banks' description column).
func wrapPDF(p *gofpdf.Fpdf, s string, w float64) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if line != "" && p.GetStringWidth(next) > w {
			out = append(out, line)
			next = word
		}
		line = next
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

func TestSyntheticPDFBanks(t *testing.T) {
	for file, lay := range pdfBankLayouts {
		t.Run(file, func(t *testing.T) {
			data := syntheticPDF(t, file, lay)
			sts, format, err := Parse(file+".pdf", data)
			if err != nil {
				t.Fatal(err)
			}
			if format != "pdf" || len(sts) != 1 {
				t.Fatalf("format %s, %d statements", format, len(sts))
			}
			st := sts[0]
			if BankFromIBAN(st.AccountIBAN) != bankFixtures[file].bank {
				t.Fatalf("iban %q", st.AccountIBAN)
			}
			checkMoves(t, st, bankFixtures[file].bank, fixtureMoves)
			// The preview (template analysis) shows no row errors either:
			// before #780 the footer, wrapped descriptions and page 2's
			// title block were 1-7 "tarih okunamadı / tarih boş" rows.
			a, err := AnalyzeFile(context.Background(), file+".pdf", data, nil, nil, ColumnRequest{})
			if err != nil || a.Lines != 5 || a.RowErrors != 0 {
				t.Fatalf("analysis lines %d row errors %d: %v", a.Lines, a.RowErrors, err)
			}
			for _, l := range st.Lines {
				if l.CaptureMethod != banking.CapturePDF {
					t.Fatalf("capture method %q", l.CaptureMethod)
				}
			}
		})
	}
}

// The wrapped description joins its movement; the page footer and the
// repeated title block of page 2 are no movement rows.
func TestSyntheticPDFWrapAndPages(t *testing.T) {
	rows, err := ReadPDFRows(syntheticPDF(t, "ziraat", pdfBankLayouts["ziraat"]))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		for _, c := range r {
			found = found || strings.Contains(c, "GONDEREN: YILDIZ YAZILIM TEKNOLOJILERI A.S. FATURA 2026-118")
			if strings.Contains(c, "elektronik ortamda") {
				t.Fatalf("footer kept: %q", r)
			}
		}
	}
	if !found {
		t.Fatalf("wrapped description not joined: %q", rows)
	}
	rows, err = ReadPDFRows(syntheticPDF(t, "isbank", pdfBankLayouts["isbank"]))
	if err != nil {
		t.Fatal(err)
	}
	headers := 0
	for _, r := range rows {
		if r[0] == "Tarih/Saat" {
			headers++
		}
	}
	if headers != 1 {
		t.Fatalf("%d header rows, want 1: %q", headers, rows)
	}
}

func TestUnreadablePDFs(t *testing.T) {
	good := syntheticPDF(t, "garanti", pdfBankLayouts["garanti"])

	blank := gofpdf.New("P", "mm", "A4", "")
	blank.AddPage()
	blank.Rect(20, 20, 100, 60, "F") // a scanned page: no text layer
	var scanned bytes.Buffer
	if err := blank.Output(&scanned); err != nil {
		t.Fatal(err)
	}

	letter := gofpdf.New("P", "mm", "A4", "")
	letter.AddPage()
	letter.SetFont("Helvetica", "", 11)
	letter.CellFormat(0, 6, "Sayin musterimiz, hesap ozetiniz ektedir.", "", 1, "L", false, 0, "")
	var noTable bytes.Buffer
	if err := letter.Output(&noTable); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"garbage", []byte("%PDF-1.4\nbu bir pdf degil\n%%EOF"), ErrPDFUnreadable},
		{"truncated", good[:len(good)/2], ErrPDFUnreadable},
		{"scanned", scanned.Bytes(), ErrScannedPDF},
		{"no table", noTable.Bytes(), ErrLayoutUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := Parse("ekstre.pdf", c.data); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			_, err := AnalyzeFile(context.Background(), "ekstre.pdf", c.data, nil, nil, ColumnRequest{})
			if !errors.Is(err, c.want) {
				t.Fatalf("analyze err = %v, want %v", err, c.want)
			}
		})
	}
}

// A PDF whose running balance does not add up is refused (23 §5.3).
func TestSyntheticPDFBalanceMismatch(t *testing.T) {
	rows := fixtureRows(t, "akbank")
	for _, r := range rows {
		for j, c := range r {
			if c == "59.240,75" {
				r[j] = "59.240,70"
			}
		}
	}
	data := syntheticPDFRows(t, "akbank", rows, pdfBankLayouts["akbank"])
	if _, _, err := Parse("akbank.pdf", data); !errors.Is(err, ErrPDFBalance) {
		t.Fatalf("err = %v", err)
	}
}
