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

// The buyer printed on a receipt (Fiş yapay zekâ kontrolleri): a receipt
// cut in someone else's name is flagged. Read from expense / fuel receipts,
// not sent to the review queue (the draft has no buyer column).
const (
	FieldBuyerTaxNumber = "buyer_tax_number"
	FieldBuyerName      = "buyer_name"
)

// BuyerFields are the buyer fields.
var BuyerFields = []string{FieldBuyerTaxNumber, FieldBuyerName}

// Fields lists every field the model is asked for.
var Fields = []string{FieldDate, FieldVendorTaxNumber, FieldVendorName, FieldReceiptNumber, FieldTotal,
	FieldTax, FieldTaxRate, FieldPlate, FieldZNumber, FieldOKCSerial, FieldPaymentMethod}

// Z report breakdown (#384): payment types and, per VAT rate, the total
// including VAT (vat_<rate>_total) and the VAT (vat_<rate>_tax). They are
// stored with the draft but not sent to the review queue one by one: the
// office corrects the breakdown on the Z report screen.
const (
	FieldCashAmount   = "cash_amount"
	FieldCardAmount   = "card_amount"
	FieldReceiptCount = "receipt_count"
)

// ZRates are the VAT rates read from a Z report (8 / 18: before 07.2023).
var ZRates = []string{"0", "1", "8", "10", "18", "20"}

// VatFields are the per-rate fields (incl. VAT total and VAT) read from a Z
// report and, since #780, from an expense / fuel receipt too.
var VatFields = func() []string {
	var out []string
	for _, r := range ZRates {
		out = append(out, "vat_"+r+"_total", "vat_"+r+"_tax")
	}
	return out
}()

// VatField splits a per-rate field ("vat_20_total", "vat_1_tax") into its
// rate and part ("total" incl. VAT, or "tax"). ok is false for any other
// field or an unknown rate.
func VatField(f string) (rate, part string, ok bool) {
	rest, found := strings.CutPrefix(f, "vat_")
	if !found {
		return "", "", false
	}
	rate, part, found = strings.Cut(rest, "_")
	if !found || (part != "total" && part != "tax") {
		return "", "", false
	}
	for _, r := range ZRates {
		if r == rate {
			return rate, part, true
		}
	}
	return "", "", false
}

// ZFields lists the breakdown fields.
var ZFields = append([]string{FieldCashAmount, FieldCardAmount, FieldReceiptCount}, VatFields...)

// ZVat is one VAT rate line of a Z report or receipt.
type ZVat struct {
	Rate  decimal.Decimal
	Total decimal.Decimal // incl. VAT
	Tax   decimal.Decimal
	// TaxRead: the rate's VAT was read. When false Tax is zero and the line
	// must not be stored as is (the value would be made up, #780).
	TaxRead bool
}

// ZVatLines returns the rate lines read from a Z report (rates with a total).
func (r Result) ZVatLines() []ZVat {
	var out []ZVat
	for _, rate := range ZRates {
		total, ok := r.Decimal("vat_" + rate + "_total")
		if !ok || !total.IsPositive() {
			continue
		}
		tax, taxOK := r.Decimal("vat_" + rate + "_tax")
		out = append(out, ZVat{Rate: decimal.RequireFromString(rate), Total: total, Tax: tax, TaxRead: taxOK})
	}
	return out
}

// VatLines is ZVatLines for any kind (expense receipts carry the same
// per-rate fields).
func (r Result) VatLines() []ZVat { return r.ZVatLines() }

// Fields that must be read for the record to be complete (#780): a missing
// one is flagged "kontrol et" on the office screen.
var (
	requiredExpense = []string{FieldDate, FieldVendorTaxNumber, FieldVendorName, FieldReceiptNumber, FieldTotal, FieldTax}
	requiredZ       = []string{FieldDate, FieldZNumber, FieldTotal, FieldTax, FieldCashAmount, FieldCardAmount}
)

// ReviewFields lists the fields the office should check (#780): a required
// field that was not read, a value we could not normalize, a value read
// under ReviewBelow confidence, and a VAT rate whose total was read but its
// VAT was not. current holds the record's present value per field ("" =
// empty); a low-confidence field whose value was since changed (corrected
// in the review queue) is no longer listed, and a required field that now
// has a value is not either. fields is the stored OCR answer; nil (not a
// photo) returns nil.
func ReviewFields(kind string, fields map[string]Value, current map[string]string) []string {
	if fields == nil {
		return nil
	}
	required := requiredExpense
	if kind == KindZReport {
		required = requiredZ
	}
	seen := map[string]bool{}
	var out []string
	add := func(f string) {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	has := func(f string) bool {
		if v, ok := current[f]; ok {
			return v != ""
		}
		v, ok := fields[f]
		return ok && v.Valid && v.Value != ""
	}
	for _, f := range required {
		if !has(f) {
			add(f)
		}
	}
	check := append(append([]string{}, Fields...), VatFields...)
	if kind == KindZReport {
		check = append(append([]string{}, Fields...), ZFields...)
	}
	for _, f := range check {
		v, ok := fields[f]
		if !ok || v.Raw == "" || v.Reviewed {
			continue
		}
		if cur, tracked := current[f]; tracked && v.Valid && cur != "" && !sameValue(cur, v.Value) {
			continue // corrected since
		}
		if !v.Valid || v.Confidence < ReviewBelow {
			add(f)
		}
	}
	for _, rate := range ZRates {
		t, okT := fields["vat_"+rate+"_total"]
		x, okX := fields["vat_"+rate+"_tax"]
		if okT && t.Valid && (!okX || !x.Valid) && current["vat_"+rate+"_tax"] == "" {
			add("vat_" + rate + "_tax")
		}
	}
	return out
}

// Stored reads the OCR answer stored with a record (expenses.ocr_fields,
// retail_sales.ocr_fields); nil when there is none.
func Stored(raw []byte) map[string]Value {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]Value
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// sameValue compares a stored value with a normalized OCR value (amounts by
// number: "1250.0000" equals "1250.00").
func sameValue(a, b string) bool {
	if a == b {
		return true
	}
	da, errA := decimal.NewFromString(a)
	db, errB := decimal.NewFromString(b)
	return errA == nil && errB == nil && da.Equal(db)
}

// ReviewBelow: fields read under this confidence are flagged for review
// (the use case sends them to the review queue too).
const ReviewBelow = 0.80

// Value is one extracted field: the normalized text and its confidence.
// Valid=false means the model gave a value we could not normalize.
type Value struct {
	Raw        string  `json:"raw"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Valid      bool    `json:"valid"`
	// Reviewed: the office corrected the field in the review queue (set on
	// the stored copy, receipts.sql UpdateExpenseField).
	Reviewed bool `json:"reviewed,omitempty"`
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
total_amount KDV dahil toplamdır (TOPLAM). tax_amount toplam KDV'dir (TOPKDV). tax_rate tek oranlıysa yüzde olarak (20). payment_method: cash, credit_card veya boş. Z raporunda total_amount günlük toplam satış, tax_amount toplam KDV'dir.
Z raporunda ayrıca fields içine şunları ekle (yoksa boş): "cash_amount" (NAKİT), "card_amount" (KREDİ KARTI / BANKA KARTI toplamı), "receipt_count" (FİŞ SAYISI), her KDV oranı için "vat_<oran>_total" (o oranın KDV dahil toplamı) ve "vat_<oran>_tax" (o oranın KDV'si); oran 0, 1, 8, 10, 18 veya 20 (ör. "vat_20_total", "vat_20_tax").
Masraf ve yakıt fişinde fişte alıcı (müşteri) bilgisi yazıyorsa (ör. "ALICI", "SAYIN", "MÜŞTERİ VKN/TCKN") "buyer_tax_number" (alıcının VKN ya da TCKN'si) ve "buyer_name" (alıcının adı / unvanı) ekle; satıcıyı alıcı yazma, yoksa hiç ekleme. Yakıt fişinde plaka yazıyorsa vehicle_plate'e aktar.
Masraf ve yakıt fişinde de fişte KDV oranı satırları varsa (ör. %1, %10, %20 ayrı ayrı) her oran için aynı "vat_<oran>_total" ve "vat_<oran>_tax" alanlarını ekle; fişte yazmayan oranı hiç ekleme. Tutarları birbirinden hesaplayarak doldurma.`

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
			MaxTokens: 1536,
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
	all := Fields
	switch a.Kind {
	case KindZReport:
		all = append(append([]string{}, Fields...), ZFields...)
	case KindExpense, KindFuel:
		all = append(append(append([]string{}, Fields...), VatFields...), BuyerFields...)
	}
	for _, f := range all {
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
	case FieldReceiptCount:
		d, ok := parseAmount(raw)
		if !ok || !d.IsInteger() {
			return "", false
		}
		return d.String(), true
	case FieldTotal, FieldTax, FieldCashAmount, FieldCardAmount:
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
	case FieldVendorTaxNumber, FieldBuyerTaxNumber:
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
		if strings.HasPrefix(field, "vat_") {
			d, ok := parseAmount(raw)
			if !ok {
				return "", false
			}
			return d.StringFixed(2), true
		}
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
