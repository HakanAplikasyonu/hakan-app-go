package model

import "strings"

// Source priorities (`01` §6): entegratör / UBL XML > ön muhasebe programı >
// OCR / elle. The invoice row carries the content of its highest-priority source.
const (
	PriorityLow     = 1 // ocr, manual, qr, bank_match
	PriorityProgram = 2 // program API or program Excel export
	PriorityUBL     = 3 // entegratör or uploaded UBL XML
)

// SourcePriority ranks where an invoice came from.
func SourcePriority(sourceSystem, captureMethod string) int {
	switch {
	case strings.HasPrefix(sourceSystem, "entegrator_"), captureMethod == CaptureUBLUpload:
		return PriorityUBL
	case captureMethod == CaptureAPI, captureMethod == CaptureExcelImport:
		return PriorityProgram
	}
	return PriorityLow
}

// Match rules recorded on invoice_sources.
const (
	MatchNew    = "new"    // first source of a new invoice
	MatchSource = "source" // the same source sent the invoice again
	MatchETTN   = "ettn"   // `01` §6 rule 1
	MatchNumber = "number" // `01` §6 rule 2: VKN/TCKN + number + date
)
