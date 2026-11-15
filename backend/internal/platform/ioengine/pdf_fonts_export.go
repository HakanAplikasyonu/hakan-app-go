package ioengine

import "github.com/jung-kurt/gofpdf/v2"

// PDFFontFamily is the embedded DejaVu family (Turkish glyphs included).
const PDFFontFamily = pdfFontFamily

// RegisterPDFFonts adds the embedded DejaVu fonts to pdf, for modules that
// draw their own documents (e.g. the bank instruction letter).
func RegisterPDFFonts(pdf *gofpdf.Fpdf) error { return registerPDFFonts(pdf) }
