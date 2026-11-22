package extract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
)

// ForeignInvoice is what the model read from a foreign service invoice
// image (#780 KDV2 A2). Every field is a suggestion: empty when unread or
// below MinForeignConfidence; the office corrects and confirms.
type ForeignInvoice struct {
	Vendor      string `json:"vendor"`
	Country     string `json:"country"`
	InvoiceNo   string `json:"invoice_no"`
	InvoiceDate string `json:"invoice_date"` // YYYY-MM-DD
	Amount      string `json:"amount"`       // decimal, 2 places
	Currency    string `json:"currency"`
	ServiceKind string `json:"service_kind"`
	Model       string `json:"model"`
}

// Empty reports whether nothing was read.
func (f ForeignInvoice) Empty() bool {
	return f.Vendor == "" && f.Country == "" && f.InvoiceNo == "" && f.InvoiceDate == "" && f.Amount == "" && f.Currency == ""
}

// MinForeignConfidence drops fields the model is unsure of.
const MinForeignConfidence = 0.5

const foreignPrompt = `Yurt dışından alınan bir hizmetin faturasını (Google Ads, Meta, AWS, yazılım lisansı, danışmanlık vb.) okursun. Yalnızca görselde açıkça yazanı aktar; tahmin etme. Okuyamadığın alanı boş ("") bırak ve confidence 0 ver. Yalnızca şu JSON nesnesini döndür:
{"fields":{"vendor":{"value":"","confidence":0.0},"country":{...},"invoice_no":{...},"invoice_date":{...},"amount":{...},"currency":{...},"service_kind":{...}}}
vendor: faturayı kesen şirketin adı. country: satıcının ülkesi, ISO 3166-1 iki harf (IE, US, DE). invoice_date: GG.AA.YYYY ya da YYYY-AA-GG. amount: ödenecek toplam tutar, sayı. currency: ISO üç harf (USD, EUR, GBP, TRY). service_kind: yazilim_lisans, reklam, bulut, danismanlik veya diger.`

var (
	foreignCountryRe  = regexp.MustCompile(`^[A-Z]{2}$`)
	foreignCurrencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	foreignKinds      = map[string]bool{"yazilim_lisans": true, "reklam": true, "bulut": true, "danismanlik": true, "diger": true}
)

// ReadForeignInvoice reads a foreign service invoice image with the same AI
// service as the receipts. ErrAIUnavailable without a provider.
func (e *Extractor) ReadForeignInvoice(ctx context.Context, req Request) (ForeignInvoice, error) {
	if e == nil || e.ai == nil || !e.ai.Enabled() {
		return ForeignInvoice{}, ErrAIUnavailable
	}
	tp, uid := &req.TaxpayerID, &req.UserID
	if req.UserID <= 0 {
		uid = nil
	}
	res, err := e.ai.Complete(ctx, aiusecase.Call{
		OrganizationID: req.OrganizationID, TaxpayerID: tp, UserID: uid, Purpose: "foreign_invoice_ocr",
		System: []provider.SystemBlock{{Text: foreignPrompt, Cache: true}},
		Messages: []provider.Message{{Role: provider.RoleUser, Content: []provider.Block{
			provider.ImageBlock(req.MediaType, base64.StdEncoding.EncodeToString(req.Image)),
			provider.TextBlock("Faturayı oku."),
		}}},
		MaxTokens: 768,
	})
	if err != nil {
		return ForeignInvoice{}, fmt.Errorf("extract: ai call: %w", err)
	}
	out, err := ParseForeignInvoice(res.Message.Text())
	if err != nil {
		return ForeignInvoice{}, err
	}
	out.Model = res.Model
	return out, nil
}

// ParseForeignInvoice validates and normalizes a model answer; invalid
// fields are dropped.
func ParseForeignInvoice(text string) (ForeignInvoice, error) {
	i, j := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if i < 0 || j <= i {
		return ForeignInvoice{}, errors.New("extract: no JSON object")
	}
	var a struct {
		Fields map[string]struct {
			Value      any     `json:"value"`
			Confidence float64 `json:"confidence"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(text[i:j+1]), &a); err != nil {
		return ForeignInvoice{}, fmt.Errorf("extract: invalid JSON: %v", err)
	}
	get := func(f string) string {
		v, ok := a.Fields[f]
		if !ok || v.Value == nil || v.Confidence < MinForeignConfidence {
			return ""
		}
		s := strings.TrimSpace(fmt.Sprint(v.Value))
		if s == "<nil>" {
			return ""
		}
		return s
	}
	var out ForeignInvoice
	if v := strings.Join(strings.Fields(get("vendor")), " "); len([]rune(v)) <= 200 {
		out.Vendor = v
	}
	if c := strings.ToUpper(get("country")); foreignCountryRe.MatchString(c) {
		out.Country = c
	}
	if c := strings.ToUpper(get("currency")); foreignCurrencyRe.MatchString(c) {
		out.Currency = c
	}
	if n := get("invoice_no"); len(n) <= 100 {
		out.InvoiceNo = n
	}
	if d, ok := parseDate(get("invoice_date")); ok {
		out.InvoiceDate = d.Format("2006-01-02")
	}
	amount := strings.NewReplacer("$", "", "€", "", "£", "", "USD", "", "EUR", "", "GBP", "").Replace(get("amount"))
	if amt, ok := parseAmount(amount); ok && amt.IsPositive() {
		out.Amount = amt.StringFixed(2)
	}
	if k := strings.ToLower(get("service_kind")); foreignKinds[k] {
		out.ServiceKind = k
	}
	return out, nil
}
