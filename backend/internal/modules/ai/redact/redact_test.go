package redact

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/mask"
)

type fakeOCR struct {
	words []Word
	err   error
	got   []byte
}

func (f *fakeOCR) Words(_ context.Context, png []byte) ([]Word, error) {
	f.got = png
	return f.words, f.err
}

// receipt draws a white image with a grey bar for every word, so the test
// can tell painted (black) boxes from untouched ones.
func receipt(t *testing.T, words []Word, enc string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	grey := image.NewUniform(color.Gray{Y: 128})
	for _, w := range words {
		draw.Draw(img, w.Box, grey, image.Point{}, draw.Src)
	}
	var b bytes.Buffer
	var err error
	if enc == "jpeg" {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	} else {
		err = png.Encode(&b, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

var sample = []Word{
	{Text: "KUZEY", Line: 1, Box: image.Rect(10, 10, 60, 25)},
	{Text: "GIDA", Line: 1, Box: image.Rect(65, 10, 110, 25)},
	{Text: "LTD.", Line: 1, Box: image.Rect(115, 10, 150, 25)},
	{Text: "ŞTİ.", Line: 1, Box: image.Rect(155, 10, 190, 25)},
	{Text: "VKN:", Line: 2, Box: image.Rect(10, 40, 50, 55)},
	{Text: "1234567890", Line: 2, Box: image.Rect(55, 40, 160, 55)},
	{Text: "TOPLAM", Line: 3, Box: image.Rect(10, 70, 80, 85)},
	{Text: "*125,00", Line: 3, Box: image.Rect(250, 70, 320, 85)},
	{Text: "Kasiyer:", Line: 4, Box: image.Rect(10, 100, 80, 115)},
	{Text: "Zarife", Line: 4, Box: image.Rect(85, 100, 140, 115)},
	{Text: "Kocabaş", Line: 4, Box: image.Rect(145, 100, 215, 115)},
}

func center(r image.Rectangle) image.Point {
	return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

func isBlack(img image.Image, p image.Point) bool {
	r, g, b, _ := img.At(p.X, p.Y).RGBA()
	return r < 0x2000 && g < 0x2000 && b < 0x2000
}

func TestRedactBlacksOutIdentifiersOnly(t *testing.T) {
	for _, enc := range []string{"png", "jpeg"} {
		t.Run(enc, func(t *testing.T) {
			ocr := &fakeOCR{words: sample}
			m := mask.New()
			res, err := New(ocr).Redact(context.Background(), m, "image/"+enc, receipt(t, sample, enc))
			if err != nil {
				t.Fatal(err)
			}
			if res.MediaType != "image/"+enc {
				t.Fatalf("media type = %q", res.MediaType)
			}
			if len(ocr.got) == 0 {
				t.Fatal("OCR did not get the image")
			}
			out, _, err := image.Decode(bytes.NewReader(res.Data))
			if err != nil {
				t.Fatal(err)
			}
			for _, i := range []int{0, 1, 2, 3, 5, 9, 10} { // company, VKN, cashier name
				if !isBlack(out, center(sample[i].Box)) {
					t.Errorf("%q not blacked out", sample[i].Text)
				}
			}
			for _, i := range []int{4, 6, 7, 8} { // labels and amounts stay
				if isBlack(out, center(sample[i].Box)) {
					t.Errorf("%q blacked out", sample[i].Text)
				}
			}
			note := Note(res.Regions)
			for _, tok := range []string{"[VKN_1]", "[AD_1]", "[AD_2]"} {
				if !strings.Contains(note, tok) {
					t.Errorf("note misses %s: %q", tok, note)
				}
			}
			if strings.Contains(note, "1234567890") || strings.Contains(note, "Zarife") {
				t.Fatalf("note leaks a value: %q", note)
			}
			// The model's answer with tokens is restored on the server.
			if got := m.Unmask(`{"vendor_tax_number":"[VKN_1]"}`); got != `{"vendor_tax_number":"1234567890"}` {
				t.Fatalf("unmask = %q", got)
			}
		})
	}
}

func TestRedactWithoutIdentifiersKeepsPixels(t *testing.T) {
	words := []Word{{Text: "TOPLAM", Line: 1, Box: image.Rect(10, 10, 80, 25)}}
	res, err := New(&fakeOCR{words: words}).Redact(context.Background(), mask.New(), "image/png", receipt(t, words, "png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Regions) != 0 || Note(res.Regions) != "" {
		t.Fatalf("regions = %+v", res.Regions)
	}
}

func TestRedactErrors(t *testing.T) {
	r := New(&fakeOCR{})
	if _, err := r.Redact(context.Background(), mask.New(), "image/png", []byte("not an image")); !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("garbage: %v", err)
	}
	boom := errors.New("boom")
	r = New(&fakeOCR{err: boom})
	if _, err := r.Redact(context.Background(), mask.New(), "image/png", receipt(t, nil, "png")); !errors.Is(err, boom) {
		t.Fatalf("ocr failure: %v", err)
	}
}

func TestParseTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"1\t1\t0\t0\t0\t0\t0\t0\t400\t200\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t10\t10\t50\t15\t96\tKUZEY\n" +
		"5\t1\t1\t1\t1\t2\t65\t10\t45\t15\t95\tGIDA\n" +
		"5\t1\t1\t1\t2\t1\t10\t40\t40\t15\t90\tVKN:\n" +
		"5\t1\t2\t1\t1\t1\t10\t70\t70\t15\t91\t \n" +
		"5\t1\t2\t1\t1\t2\t250\t70\t70\t15\t91\t*125,00\n"
	words, err := ParseTSV([]byte(tsv))
	if err != nil {
		t.Fatal(err)
	}
	if len(words) != 4 {
		t.Fatalf("words = %+v", words)
	}
	if words[1].Line != 1 || words[2].Line != 2 || words[3].Line != 3 {
		t.Fatalf("lines = %+v", words)
	}
	if words[3].Box != image.Rect(250, 70, 320, 85) {
		t.Fatalf("box = %v", words[3].Box)
	}
}
