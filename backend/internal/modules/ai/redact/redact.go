// Package redact blacks out personal data in images before they leave the
// server for an AI provider (issue #169, KVKK, docs/01 G11).
//
// An OCR pre-pass reads the words of the image with their boxes. Each line is
// run through the call's mask.Masker; the words of every span it finds (VKN,
// TCKN, IBAN, names, e-mail, phone) are painted black. The provider gets the
// redacted image plus a note that lists the tokens of the blacked-out
// regions, so it can still answer "the vendor's VKN is [VKN_1]" and the
// answer is unmasked on the server like any other text.
//
// Re-encoding also drops the image's metadata (EXIF, GPS).
package redact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // decoder registration
	"image/jpeg"
	"image/png"
	"sort"
	"strings"

	_ "golang.org/x/image/webp" // decoder registration

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/mask"
)

// Word is one OCR word: its text, its box in image pixels and the line it
// belongs to (lines are numbered from 1, top to bottom as the OCR reads).
type Word struct {
	Text string
	Box  image.Rectangle
	Line int
}

// OCR reads the words of a PNG image.
type OCR interface {
	Words(ctx context.Context, png []byte) ([]Word, error)
}

// Region is one blacked-out area.
type Region struct {
	Token string
	Kind  mask.Kind
	Line  int
	Box   image.Rectangle
}

// Result is a redacted image.
type Result struct {
	MediaType string
	Data      []byte
	Regions   []Region
}

// ErrUnsupportedImage: the image could not be decoded.
var ErrUnsupportedImage = errors.New("redact: unsupported or corrupt image")

// MaxPixels bounds decoded images (a 48 MP photo is 48e6).
const MaxPixels = 50_000_000

// Redactor blacks out personal data with an OCR engine.
type Redactor struct {
	ocr OCR
	// Pad grows every box on each side, in pixels, so glyph edges do not
	// survive.
	Pad int
}

// New returns a redactor.
func New(ocr OCR) *Redactor { return &Redactor{ocr: ocr, Pad: 3} }

// Redact decodes data, finds identifiers with m and paints them black.
// JPEG stays JPEG; every other format becomes PNG.
func (r *Redactor) Redact(ctx context.Context, m *mask.Masker, mediaType string, data []byte) (Result, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Result{}, ErrUnsupportedImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return Result{}, fmt.Errorf("%w: %dx%d", ErrUnsupportedImage, cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Result{}, ErrUnsupportedImage
	}
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)

	// The OCR reads the decoded pixels, so its boxes match what is painted
	// (EXIF orientation is ignored on both sides).
	var plain bytes.Buffer
	if err := png.Encode(&plain, img); err != nil {
		return Result{}, err
	}
	words, err := r.ocr.Words(ctx, plain.Bytes())
	if err != nil {
		return Result{}, fmt.Errorf("redact: ocr: %w", err)
	}
	regions := Find(m, words, r.Pad, img.Bounds())
	black := image.NewUniform(color.Black)
	for _, reg := range regions {
		draw.Draw(img, reg.Box, black, image.Point{}, draw.Src)
	}

	var out bytes.Buffer
	outType := "image/png"
	if mediaType == "image/jpeg" {
		outType = mediaType
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(&out, img)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{MediaType: outType, Data: out.Bytes(), Regions: regions}, nil
}

// Find maps the masker's spans on each OCR line back to word boxes.
func Find(m *mask.Masker, words []Word, pad int, bounds image.Rectangle) []Region {
	lines := map[int][]Word{}
	var order []int
	for _, w := range words {
		if strings.TrimSpace(w.Text) == "" {
			continue
		}
		if _, ok := lines[w.Line]; !ok {
			order = append(order, w.Line)
		}
		lines[w.Line] = append(lines[w.Line], w)
	}
	sort.Ints(order)
	var out []Region
	for _, ln := range order {
		ws := lines[ln]
		sort.SliceStable(ws, func(i, j int) bool { return ws[i].Box.Min.X < ws[j].Box.Min.X })
		var b strings.Builder
		offs := make([][2]int, len(ws))
		for i, w := range ws {
			if i > 0 {
				b.WriteByte(' ')
			}
			offs[i][0] = b.Len()
			b.WriteString(w.Text)
			offs[i][1] = b.Len()
		}
		for _, sp := range m.Find(b.String()) {
			var box image.Rectangle
			for i, o := range offs {
				if o[0] < sp.End && sp.Start < o[1] {
					box = box.Union(ws[i].Box)
				}
			}
			if box.Empty() {
				continue
			}
			box = image.Rect(box.Min.X-pad, box.Min.Y-pad, box.Max.X+pad, box.Max.Y+pad).Intersect(bounds)
			out = append(out, Region{Token: sp.Token, Kind: sp.Kind, Line: ln, Box: box})
		}
	}
	return out
}

// Note tells the model which tokens stand for the blacked-out regions.
func Note(regions []Region) string {
	if len(regions) == 0 {
		return ""
	}
	parts := make([]string, 0, len(regions))
	for _, r := range regions {
		parts = append(parts, fmt.Sprintf("%s (satır %d)", r.Token, r.Line))
	}
	return "Görseldeki kişisel veriler gizlilik için siyahla kapatıldı. Kapatılan alanların değeri yerine şu belirteçleri aynen kullan: " +
		strings.Join(parts, ", ") + "."
}
