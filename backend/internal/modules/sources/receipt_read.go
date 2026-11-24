package sources

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

// Fişten KDV oranı okuma (#780). A pre-accounting program (Easytrade) sends
// one tax_rate per expense, but a market receipt often has lines at several
// rates (%1 and %20). When the expense comes with a receipt image the
// portal reads the image (receipt reader, same KVKK masking as a photo
// upload) and, if the read total matches the program's total, writes the
// read rate lines instead of the single rate.
//
// The state lives in expenses.ocr_fields under ReceiptReadKey (a source
// expense has no photo OCR of its own): the receipt's stable key, the
// status and what was read. The same receipt is never read twice; a sync
// that rewrites the expense keeps the read lines.

// ReceiptReadKey is the ocr_fields key of the state.
const ReceiptReadKey = "receipt_read"

// Receipt read statuses.
const (
	// ReceiptPending: the receipt waits for the reader.
	ReceiptPending = "pending"
	// ReceiptRead: the lines read from the receipt are the breakdown.
	ReceiptRead = "read"
	// ReceiptMismatch: the read total does not match the program's total;
	// the program's single rate is kept ("Fiş okunamadı / tutmadı").
	ReceiptMismatch = "mismatch"
	// ReceiptUnreadable: the image could not be fetched or read; the
	// program's single rate is kept ("Fiş okunamadı / tutmadı").
	ReceiptUnreadable = "unreadable"
)

// ReceiptReadLine is one rate read from the receipt.
type ReceiptReadLine struct {
	Rate decimal.Decimal `json:"tax_rate"`
	Base decimal.Decimal `json:"tax_base"`
	Tax  decimal.Decimal `json:"tax_amount"`
}

// ReceiptReadState is the stored state of an expense's receipt read.
type ReceiptReadState struct {
	// Key identifies the receipt image without its expiring signature
	// (scheme, host and path of the URL).
	Key    string `json:"key"`
	Status string `json:"status"`
	// Attempts counts failed downloads; after MaxReceiptAttempts the
	// receipt is given up as unreadable.
	Attempts int               `json:"attempts,omitempty"`
	Lines    []ReceiptReadLine `json:"lines,omitempty"`
	// Total is the receipt's KDV dahil total as read.
	Total  *decimal.Decimal `json:"total,omitempty"`
	Reason string           `json:"reason,omitempty"`
	Model  string           `json:"model,omitempty"`
	ReadAt *time.Time       `json:"read_at,omitempty"`
}

// MaxReceiptAttempts: failed downloads before a receipt is unreadable.
const MaxReceiptAttempts = 3

// ParseReceiptRead reads the state from expenses.ocr_fields.
func ParseReceiptRead(ocrFields []byte) (ReceiptReadState, bool) {
	if len(ocrFields) == 0 {
		return ReceiptReadState{}, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(ocrFields, &m) != nil {
		return ReceiptReadState{}, false
	}
	raw, ok := m[ReceiptReadKey]
	if !ok {
		return ReceiptReadState{}, false
	}
	var s ReceiptReadState
	if json.Unmarshal(raw, &s) != nil || s.Key == "" || s.Status == "" {
		return ReceiptReadState{}, false
	}
	return s, true
}

// OcrFields is the ocr_fields value holding s.
func (s ReceiptReadState) OcrFields() []byte {
	b, _ := json.Marshal(map[string]ReceiptReadState{ReceiptReadKey: s})
	return b
}

// ReceiptTolerance is how far the read total may be from the program's
// total: 0,10 TL or 0,2 %, whichever is larger (rounding per rate line,
// kuruş differences of the receipt printer).
func ReceiptTolerance(amount decimal.Decimal) decimal.Decimal {
	t := amount.Abs().Mul(decimal.RequireFromString("0.002"))
	if floor := decimal.RequireFromString("0.10"); t.LessThan(floor) {
		return floor
	}
	return t
}

// ReceiptMatches reports whether the read lines add up to amount (KDV
// dahil) within ReceiptTolerance. No lines never match.
func ReceiptMatches(lines []ReceiptReadLine, amount decimal.Decimal) bool {
	if len(lines) == 0 || !amount.IsPositive() {
		return false
	}
	sum := decimal.Zero
	for _, l := range lines {
		if l.Rate.IsNegative() || l.Base.IsNegative() || l.Tax.IsNegative() {
			return false
		}
		sum = sum.Add(l.Base).Add(l.Tax)
	}
	return sum.Sub(amount).Abs().LessThanOrEqual(ReceiptTolerance(amount))
}

// VatLines turns the read lines into expense VAT lines.
func (s ReceiptReadState) VatLines() []ExpenseVatLine {
	out := make([]ExpenseVatLine, 0, len(s.Lines))
	for _, l := range s.Lines {
		out = append(out, ExpenseVatLine{Rate: l.Rate, Base: l.Base, Tax: l.Tax})
	}
	return out
}

// NextReceiptRead decides, when a source expense is written, its receipt
// state and its VAT lines:
//
//   - no receipt: no state, the program's lines;
//   - the receipt already read (same key): the read lines when they still
//     match the (possibly changed) total, else the program's lines and the
//     status mismatch; a receipt given up stays given up;
//   - a new or changed receipt: pending, the program's lines until it is
//     read.
func NextReceiptRead(prev ReceiptReadState, hasPrev bool, e Expense) (*ReceiptReadState, []ExpenseVatLine) {
	if e.ReceiptKey == "" {
		return nil, e.VatLines
	}
	if hasPrev && prev.Key == e.ReceiptKey {
		next := prev
		switch prev.Status {
		case ReceiptRead, ReceiptMismatch:
			if ReceiptMatches(prev.Lines, e.Amount) {
				next.Status = ReceiptRead
				return &next, next.VatLines()
			}
			if len(prev.Lines) > 0 {
				next.Status = ReceiptMismatch
			}
		}
		return &next, e.VatLines
	}
	return &ReceiptReadState{Key: e.ReceiptKey, Status: ReceiptPending}, e.VatLines
}
