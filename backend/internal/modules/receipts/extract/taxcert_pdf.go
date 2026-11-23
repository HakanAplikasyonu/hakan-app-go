package extract

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// ErrScannedPDF: the PDF has no text layer (a scan). No AI channel reads
// PDF pages as images (#831) and the repo has no PDF rasterizer, so a
// scanned certificate must be uploaded as a photo.
var ErrScannedPDF = errors.New("extract: PDF has no text layer")

// maxCertText caps the text sent to the model (a certificate is one page).
const maxCertText = 6000

// PDFText returns the text of a PDF's first page. e-Vergi levhası PDFs from
// GİB carry a text layer, so their text goes to the model instead of an
// image; ErrScannedPDF when there is none.
func PDFText(data []byte) (text string, err error) {
	defer func() {
		// The PDF reader panics on some malformed files.
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("extract: pdf: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("extract: pdf: %w", err)
	}
	if r.NumPage() < 1 {
		return "", ErrScannedPDF
	}
	p := r.Page(1)
	if p.V.IsNull() {
		return "", ErrScannedPDF
	}
	s, err := p.GetPlainText(nil)
	if err != nil {
		return "", fmt.Errorf("extract: pdf: %w", err)
	}
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) < 20 {
		return "", ErrScannedPDF
	}
	if utf8.RuneCountInString(s) > maxCertText {
		s = string([]rune(s)[:maxCertText])
	}
	return s, nil
}
