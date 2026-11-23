package extract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
	aiusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
)

// Vergi levhası (tax certificate) fields read when the office adds a
// mükellef (Oğuz'un geri bildirimi). Every value is a suggestion: the office
// checks the form and saves it itself.
const (
	CertTaxID        = "tax_id"
	CertTitle        = "title"
	CertTaxOffice    = "tax_office"
	CertAddress      = "address"
	CertActivityCode = "activity_code"
)

// CertFields lists the fields in the order the form shows them.
var CertFields = []string{CertTaxID, CertTitle, CertTaxOffice, CertAddress, CertActivityCode}

// Reasons a certificate field must be checked ("kontrol et").
const (
	CertMissing       = "missing"        // not read
	CertLowConfidence = "low_confidence" // read under ReviewBelow
	CertInvalid       = "invalid"        // VKN/TCKN check digit or format, bad activity code
)

// Field length limits (taxpayers / taxpayer_onboarding_requests columns).
const (
	certMaxTitle     = 255
	certMaxTaxOffice = 128
	certMaxAddress   = 500
)

// CertField is one read field. Check is true when the office must look at
// it: Problem says why. Value is "" when nothing usable was read; an
// invalid VKN/TCKN keeps the digits read so one wrong digit is easy to fix.
type CertField struct {
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Check      bool    `json:"check"`
	Problem    string  `json:"problem,omitempty"`
}

// TaxCertificate is what was read from a vergi levhası.
type TaxCertificate struct {
	Fields map[string]CertField `json:"fields"`
	// TaxIDType is "vkn" or "tckn" for a valid number, "" otherwise.
	TaxIDType string `json:"tax_id_type"`
	// Kind suggests the form's taxpayer kind ("legal", "individual"); ""
	// when the certificate does not tell (keep what the office chose).
	Kind  string `json:"kind"`
	Model string `json:"model"`
}

// CheckFields lists the fields to check, in form order.
func (c TaxCertificate) CheckFields() []string {
	var out []string
	for _, f := range CertFields {
		if c.Fields[f].Check {
			out = append(out, f)
		}
	}
	return out
}

// CertRequest is one certificate to read: an image (MediaType + Image) or
// the text of a PDF (Text, from PDFText).
type CertRequest struct {
	OrganizationID int64
	UserID         int64
	MediaType      string
	Image          []byte
	Text           string
}

const taxCertPrompt = `Türkiye'de Gelir İdaresi Başkanlığı'nın verdiği bir vergi levhasını okursun (fotoğraf ya da PDF metni). Yalnızca belgede açıkça yazanı aktar; tahmin etme. Okuyamadığın alanı boş ("") bırak ve confidence 0 ver. Yalnızca şu JSON nesnesini döndür:
{"fields":{"tax_id":{"value":"","confidence":0.0},"title":{...},"tax_office":{...},"address":{...},"activity_code":{...}}}
tax_id: VERGİ KİMLİK NO (10 hane) ya da T.C. KİMLİK NO (11 hane); yalnız rakamlar. title: TİCARET ÜNVANI ya da ADI SOYADI, belgede yazdığı gibi. tax_office: VERGİ DAİRESİ adı ("Vergi Dairesi" eki olmadan, ör. "Kadıköy"). address: İŞ YERİ ADRESİ tek satır. activity_code: ANA FAALİYET KODU (NACE, 6 hane, ör. 471101); birden çok faaliyet varsa yalnız ana faaliyetin kodu.`

// ReadTaxCertificate reads a vergi levhası with the same AI service (and
// so the same masking, image redaction and usage record) as the receipts.
// The call is not tied to a taxpayer: the mükellef is not added yet.
func (e *Extractor) ReadTaxCertificate(ctx context.Context, req CertRequest) (TaxCertificate, error) {
	if e == nil || e.ai == nil || !e.ai.Enabled() {
		return TaxCertificate{}, ErrAIUnavailable
	}
	var uid *int64
	if req.UserID > 0 {
		uid = &req.UserID
	}
	var content []provider.Block
	if req.Text != "" {
		content = []provider.Block{provider.TextBlock("Vergi levhasının PDF metni:\n" + req.Text + "\n\nLevhayı oku.")}
	} else {
		content = []provider.Block{
			provider.ImageBlock(req.MediaType, base64.StdEncoding.EncodeToString(req.Image)),
			provider.TextBlock("Levhayı oku."),
		}
	}
	res, err := e.ai.Complete(ctx, aiusecase.Call{
		OrganizationID: req.OrganizationID, UserID: uid, Purpose: "tax_certificate_ocr",
		System:    []provider.SystemBlock{{Text: taxCertPrompt, Cache: true}},
		Messages:  []provider.Message{{Role: provider.RoleUser, Content: content}},
		MaxTokens: 768,
	})
	if err != nil {
		return TaxCertificate{}, fmt.Errorf("extract: ai call: %w", err)
	}
	out, err := ParseTaxCertificate(res.Message.Text())
	if err != nil {
		return TaxCertificate{}, err
	}
	out.Model = res.Model
	return out, nil
}

// ParseTaxCertificate validates and normalizes a model answer. Every field
// of CertFields is present in the result; a missing, unsure or invalid one
// has Check set.
func ParseTaxCertificate(text string) (TaxCertificate, error) {
	i, j := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if i < 0 || j <= i {
		return TaxCertificate{}, errors.New("extract: no JSON object")
	}
	var a struct {
		Fields map[string]struct {
			Value      any     `json:"value"`
			Confidence float64 `json:"confidence"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(text[i:j+1]), &a); err != nil {
		return TaxCertificate{}, fmt.Errorf("extract: invalid JSON: %v", err)
	}
	out := TaxCertificate{Fields: map[string]CertField{}}
	for _, f := range CertFields {
		raw, ok := a.Fields[f]
		s := ""
		if ok && raw.Value != nil {
			s = strings.Join(strings.Fields(fmt.Sprint(raw.Value)), " ")
		}
		if s == "" || s == "<nil>" {
			out.Fields[f] = CertField{Check: true, Problem: CertMissing}
			continue
		}
		v := CertField{Confidence: clamp(raw.Confidence)}
		var valid bool
		v.Value, valid = normalizeCert(f, s)
		switch {
		case !valid:
			v.Check, v.Problem = true, CertInvalid
		case v.Confidence < ReviewBelow:
			v.Check, v.Problem = true, CertLowConfidence
		}
		if v.Value == "" && v.Problem == "" {
			v.Check, v.Problem = true, CertMissing
		}
		out.Fields[f] = v
	}
	if id := out.Fields[CertTaxID]; id.Problem != CertInvalid && id.Value != "" {
		if _, typ, err := taxid.Parse(id.Value); err == nil {
			out.TaxIDType = string(typ)
		}
	}
	out.Kind = suggestKind(out.TaxIDType, out.Fields[CertTitle].Value)
	return out, nil
}

// normalizeCert turns a raw certificate field into the form's text; ok is
// false when the value cannot be right (the office must correct it).
func normalizeCert(field, raw string) (string, bool) {
	switch field {
	case CertTaxID:
		digits := onlyDigits(raw)
		if digits == "" {
			return "", false
		}
		if _, _, err := taxid.Parse(digits); err != nil {
			return digits, false
		}
		return digits, true
	case CertActivityCode:
		d := onlyDigits(raw)
		if len(d) != 6 {
			return "", false
		}
		return d[0:2] + "." + d[2:4] + "." + d[4:6], true
	case CertTaxOffice:
		v := raw
		for _, suffix := range []string{" Vergi Dairesi Müdürlüğü", " VERGİ DAİRESİ MÜDÜRLÜĞÜ", " Vergi Dairesi", " VERGİ DAİRESİ", " V.D.", " VD"} {
			v = strings.TrimSuffix(v, suffix)
		}
		return limitRunes(v, certMaxTaxOffice)
	case CertTitle:
		return limitRunes(raw, certMaxTitle)
	case CertAddress:
		return limitRunes(raw, certMaxAddress)
	}
	return raw, true
}

func limitRunes(s string, max int) (string, bool) {
	if utf8.RuneCountInString(s) <= max {
		return s, true
	}
	return string([]rune(s)[:max]), false
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// companyMarkers in a title mean a legal entity (tüzel kişi).
var companyMarkers = []string{"a.ş", "a.s.", "anonim", "ltd", "limited", "şti", "sti.", "kooperatif", "vakf", "derneğ", "dernegi", "kollektif", "komandit"}

// suggestKind: a TCKN is always a gerçek kişi; a VKN whose title names a
// company is a tüzel kişi; otherwise the certificate does not tell.
func suggestKind(typ, title string) string {
	switch typ {
	case string(taxid.TCKN):
		return "individual"
	case string(taxid.VKN):
		t := strings.ToLower(strings.NewReplacer("İ", "i", "I", "ı").Replace(title))
		for _, m := range companyMarkers {
			if strings.Contains(t, m) {
				return "legal"
			}
		}
	}
	return ""
}
