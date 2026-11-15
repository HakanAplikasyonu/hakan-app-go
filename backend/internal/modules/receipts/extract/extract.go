// Package extract reads a receipt photo with the AI module (#24) and turns
// the answer into typed fields, each with its own confidence (issue #23,
// docs/00 K8): masraf fişi (T6), yakıt fişi (T7), Z raporu (T5).
package extract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
)

// Kinds of document.
const (
	KindExpense = "expense_receipt"
	KindFuel    = "fuel_receipt"
	KindZReport = "z_report"
	KindOther   = "other"
)

// Field names (01 §4.12 expense / §4.7 retail_sale).
const (
	FieldDate            = "date"
	FieldVendorTaxNumber = "vendor_tax_number"
	FieldVendorName      = "vendor_name"
	FieldReceiptNumber   = "receipt_number"
	FieldTotal           = "total_amount"
	FieldTax             = "tax_amount"
	FieldTaxRate         = "tax_rate"
	FieldPlate           = "vehicle_plate"
	FieldZNumber         = "z_report_number"
	FieldOKCSerial       = "okc_serial_number"
	FieldPaymentMethod   = "payment_method"
)

// Fields lists every field the model is asked for.
var Fields = []string{FieldDate, FieldVendorTaxNumber, FieldVendorName, FieldReceiptNumber, FieldTotal,
	FieldTax, FieldTaxRate, FieldPlate, FieldZNumber, FieldOKCSerial, FieldPaymentMethod}

// Value is one extracted field: the normalized text and its confidence.
// Valid=false means the model gave a value we could not normalize.
type Value struct {
	Raw        string  `json:"raw"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Valid      bool    `json:"valid"`
}

// Result is a read receipt.
type Result struct {
	Kind           string           `json:"kind"`
	KindConfidence float64          `json:"kind_confidence"`
	Fields         map[string]Value `json:"fields"`
	Model          string           `json:"model"`
}

// Get returns a field ("" when absent or invalid).
func (r Result) Get(f string) string {
	v, ok := r.Fields[f]
	if !ok || !v.Valid {
		return ""
	}
	return v.Value
}

// Date returns the receipt date.
func (r Result) Date() (time.Time, bool) {
	return parseDate(r.Get(FieldDate))
}

// Decimal returns a decimal field.
func (r Result) Decimal(f string) (decimal.Decimal, bool) {
	v := r.Get(f)
	if v == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(v)
	return d, err == nil
}

// MinConfidence is the lowest confidence among the present fields.
func (r Result) MinConfidence() float64 {
	m := 1.0
	for _, v := range r.Fields {
		if v.Raw != "" && v.Confidence < m {
			m = v.Confidence
		}
	}
	return m
}

// Completer is the AI service subset.
type Completer interface {
	Enabled() bool
	Complete(ctx context.Context, call aiusecase.Call) (aiusecase.Result, error)
}

// ErrAIUnavailable: no provider configured.
var ErrAIUnavailable = errors.New("extract: AI is not configured")

// Request is one image to read.
type Request struct {
	OrganizationID int64
	TaxpayerID     int64
	UserID         int64
	MediaType      string
	Image          []byte
	// KindHint is the kind the uploader chose ("" = let the model decide).
	KindHint string
}

// Extractor reads receipts.
type Extractor struct{ ai Completer }

// New builds the extractor.
func New(ai Completer) *Extractor { return &Extractor{ai: ai} }

const systemPrompt = `Türkiye'deki bir fiş, yakıt fişi veya Z raporu fotoğrafını okursun. Yalnızca görselde açıkça yazanı aktar; tahmin etme, hesap yapma. Okuyamadığın alanı boş ("") bırak ve confidence 0 ver. Yalnızca şu JSON nesnesini döndür:
{"kind":"expense_receipt|fuel_receipt|z_report|other","kind_confidence":0.0,"fields":{"date":{"value":"GG.AA.YYYY","confidence":0.0},"vendor_tax_number":{...},"vendor_name":{...},"receipt_number":{...},"total_amount":{...},"tax_amount":{...},"tax_rate":{...},"vehicle_plate":{...},"z_report_number":{...},"okc_serial_number":{...},"payment_method":{...}}}
total_amount KDV dahil toplamdır (TOPLAM). tax_amount toplam KDV'dir (TOPKDV). tax_rate tek oranlıysa yüzde olarak (20). payment_method: cash, credit_card veya boş. Z raporunda total_amount günlük toplam satış, tax_amount toplam KDV'dir.`

type answer struct {
	Kind           string  `json:"kind"`
	KindConfidence float64 `json:"kind_confidence"`
	Fields         map[string]struct {
		Value      any     `json:"value"`
		Confidence float64 `json:"confidence"`
	} `json:"fields"`
}

// Read sends the image and parses the answer; an invalid answer is
// retried once.
func (e *Extractor) Read(ctx context.Context, req Request) (Result, error) {
	if e == nil || e.ai == nil || !e.ai.Enabled() {
		return Result{}, ErrAIUnavailable
	}
	hint := "Belge türünü sen belirle."
	if req.KindHint != "" {
		hint = "Yükleyen belge türünü " + req.KindHint + " olarak seçti."
	}
	tp, uid := &req.TaxpayerID, &req.UserID
	if req.UserID <= 0 {
		uid = nil
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		text := hint
		if lastErr != nil {
			text += " Önceki yanıt geçersizdi (" + lastErr.Error() + "); yalnızca JSON döndür."
		}
		res, err := e.ai.Complete(ctx, aiusecase.Call{
			OrganizationID: req.OrganizationID, TaxpayerID: tp, UserID: uid, Purpose: "receipt_ocr",
			System: []provider.SystemBlock{{Text: systemPrompt, Cache: true}},
			Messages: []provider.Message{{Role: provider.RoleUser, Content: []provider.Block{
				provider.ImageBlock(req.MediaType, base64.StdEncoding.EncodeToString(req.Image)),
				provider.TextBlock(text),
			}}},
			MaxTokens: 1024,
		})
		if err != nil {
			return Result{}, fmt.Errorf("extract: ai call: %w", err)
		}
		out, err := Parse(res.Message.Text())
		if err == nil {
			out.Model = res.Model
			if req.KindHint != "" && out.Kind == KindOther {
				out.Kind = req.KindHint
			}
			return out, nil
		}
		lastErr = err
	}
	return Result{}, fmt.Errorf("extract: invalid answer: %w", lastErr)
}

// Parse validates and normalizes a model answer.
func Parse(text string) (Result, error) {
	i, j := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if i < 0 || j <= i {
		return Result{}, errors.New("no JSON object")
	}
	var a answer
	if err := json.Unmarshal([]byte(text[i:j+1]), &a); err != nil {
		return Result{}, fmt.Errorf("invalid JSON: %v", err)
	}
	switch a.Kind {
	case KindExpense, KindFuel, KindZReport, KindOther:
	default:
		return Result{}, fmt.Errorf("unknown kind %q", a.Kind)
	}
	r := Result{Kind: a.Kind, KindConfidence: clamp(a.KindConfidence), Fields: map[string]Value{}}
	for _, f := range Fields {
		raw, ok := a.Fields[f]
		if !ok {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(raw.Value))
		if raw.Value == nil || s == "" || s == "<nil>" {
			continue
		}
		v := Value{Raw: s, Confidence: clamp(raw.Confidence)}
		v.Value, v.Valid = Normalize(f, s)
		r.Fields[f] = v
	}
	return r, nil
}

var plateRe = regexp.MustCompile(`^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$`)

// NormalizePlate upper-cases and drops spaces / dashes (01 §4.12:
// "34ABC123"); ok=false when the result is not a Turkish plate.
func NormalizePlate(s string) (string, bool) {
	s = strings.NewReplacer("i", "İ", "ı", "I").Replace(s)
	s = strings.ToUpper(s)
	// Plates use no Turkish-only letters; OCR often adds the diacritics.
	s = strings.NewReplacer("İ", "I", "Ş", "S", "Ç", "C", "Ğ", "G", "Ö", "O", "Ü", "U",
		" ", "", "-", "", ".", "").Replace(s)
	return s, plateRe.MatchString(s)
}

// Normalize turns a raw field into its canonical text.
func Normalize(field, raw string) (string, bool) {
	switch field {
	case FieldDate:
		d, ok := parseDate(raw)
		if !ok {
			return "", false
		}
		return d.Format("2006-01-02"), true
	case FieldTotal, FieldTax:
		d, ok := parseAmount(raw)
		if !ok {
			return "", false
		}
		return d.StringFixed(2), true
	case FieldTaxRate:
		d, ok := parseRate(raw)
		if !ok {
			return "", false
		}
		return d.String(), true
	case FieldVendorTaxNumber:
		return parseTaxID(raw)
	case FieldPlate:
		return NormalizePlate(raw)
	case FieldPaymentMethod:
		n := normText(raw)
		switch {
		case strings.Contains(n, "kart") || strings.Contains(n, "credit"):
			return "credit_card", true
		case strings.Contains(n, "nakit") || n == "cash":
			return "cash", true
		}
		return "", false
	default:
		return strings.Join(strings.Fields(raw), " "), true
	}
}

func clamp(f float64) float64 {
	switch {
	case f < 0:
		return 0
	case f > 1:
		return 1
	}
	return f
}
