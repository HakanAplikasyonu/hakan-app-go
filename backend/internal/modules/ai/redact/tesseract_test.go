package redact

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/mask"
)

// renderReceipt draws lines of text in black on white.
func renderReceipt(t *testing.T, lines []string) []byte {
	t.Helper()
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 32, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 900, 60+len(lines)*60))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	d := font.Drawer{Dst: img, Src: image.NewUniform(color.Black), Face: face}
	for i, l := range lines {
		d.Dot = fixed.P(30, 70+i*60)
		d.DrawString(l)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Gerçek tesseract ile uçtan uca: VKN ve adlar karartılır, kalan metin
// okunur kalır. tesseract kurulu değilse atlanır.
func TestTesseractRedactsReceipt(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract not installed")
	}
	ocr, err := NewTesseract("", "tur")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	src := renderReceipt(t, []string{
		"KUZEY YILDIZI GIDA LTD. ŞTİ.",
		"VKN: 1234567890",
		"TOPLAM 125,00",
		"Kasiyer: Zarife Kocabaş",
	})
	before, err := ocr.Words(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joinWords(before), "1234567890") {
		t.Fatalf("OCR did not read the VKN: %q", joinWords(before))
	}
	res, err := New(ocr).Redact(ctx, mask.New(), "image/png", src)
	if err != nil {
		t.Fatal(err)
	}
	after, err := ocr.Words(ctx, res.Data)
	if err != nil {
		t.Fatal(err)
	}
	text := joinWords(after)
	for _, leaked := range []string{"1234567890", "Zarife", "Kocabaş", "KUZEY", "GIDA"} {
		if strings.Contains(text, leaked) {
			t.Errorf("%q still readable after redaction: %q", leaked, text)
		}
	}
	for _, kept := range []string{"TOPLAM", "125"} {
		if !strings.Contains(text, kept) {
			t.Errorf("%q lost: %q", kept, text)
		}
	}
	if note := Note(res.Regions); !strings.Contains(note, "[VKN_1]") {
		t.Errorf("note = %q", note)
	}
}

func joinWords(ws []Word) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}
