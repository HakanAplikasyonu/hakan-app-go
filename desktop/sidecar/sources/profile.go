package sources

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// defaultsJSON, ⚠️ işaretli bütün tablo / kolon adlarının, sorgu metinlerinin
// ve tür kodu eşlemelerinin tek kaynağıdır (CLAUDE.md §6 kural 1).
//
//go:embed defaults.json
var defaultsJSON []byte

// Mantıksal alan adları (Profile.Fields anahtarları). Her biri kaynaktaki
// aday kolon adlarına eşlenir; satırda ilk bulunan kolon kullanılır.
const (
	FieldSourceID          = "source_id"
	FieldGUID              = "guid"
	FieldDocNo             = "doc_no"
	FieldInternalNo        = "internal_no"
	FieldDate              = "date"
	FieldDocDate           = "doc_date"
	FieldType              = "type"        // Logo TRCODE / TYPE
	FieldEvrakTip          = "evrak_tip"   // Mikro cha_evrak_tip
	FieldCinsi             = "cinsi"       // Mikro cha_cinsi
	FieldTip               = "tip"         // Mikro cha_tip (borç / alacak)
	FieldNormalIade        = "normal_iade" // Mikro cha_normal_Iade
	FieldCancelled         = "cancelled"
	FieldCounterpartyCode  = "counterparty_code"
	FieldCounterpartyTitle = "counterparty_title"
	FieldCounterpartyTaxID = "counterparty_tax_id"
	FieldCurrency          = "currency"
	FieldETTN              = "ettn"
	FieldGross             = "gross"
	FieldDiscounts         = "discounts"
	FieldVAT               = "vat"
	FieldAdditionalTax     = "additional_tax"
	FieldNet               = "net"
	FieldAmount            = "amount"
	FieldSubtotal          = "subtotal"
)

var knownFields = map[string]bool{
	FieldSourceID: true, FieldGUID: true, FieldDocNo: true, FieldInternalNo: true,
	FieldDate: true, FieldDocDate: true, FieldType: true, FieldEvrakTip: true,
	FieldCinsi: true, FieldTip: true, FieldNormalIade: true, FieldCancelled: true,
	FieldCounterpartyCode: true, FieldCounterpartyTitle: true, FieldCounterpartyTaxID: true,
	FieldCurrency: true, FieldETTN: true, FieldGross: true, FieldDiscounts: true,
	FieldVAT: true, FieldAdditionalTax: true, FieldNet: true, FieldAmount: true,
	FieldSubtotal: true,
}

// TypeRule, tür kodu alanlarının bir birleşimini yön / türe eşler. Match'teki
// bütün alanlar eşleşmelidir; birden çok kural eşleşirse en çok alanı olan
// (en özgül) kazanır, eşitlikte ilk sıradaki.
type TypeRule struct {
	Match         map[string]string `json:"match"`
	Direction     Direction         `json:"direction,omitempty"`
	Kind          DocKind           `json:"kind,omitempty"`
	DocumentClass string            `json:"document_class,omitempty"`
	// Skip: satır çekilmez (ör. Logo proforma, Mikro'da fatura olmayan evrak).
	Skip bool   `json:"skip,omitempty"`
	Note string `json:"note,omitempty"`
}

// Profile bir kaynağın sorgu ve eşleme tanımıdır.
type Profile struct {
	// Verified: ⚠️ adlar pilot mükellefte doğrulandı mı (bilgi amaçlı).
	Verified bool   `json:"verified"`
	Note     string `json:"note,omitempty"`
	// Tables, Test'in varlığını denetlediği tablolar ({FFF}/{PP} şablonlu).
	Tables []string `json:"tables"`
	// InvoiceQuery fatura başlıklarını [@from, @to) penceresinde okur.
	InvoiceQuery string `json:"invoice_query"`
	// DropColumns ham satıra bile alınmayan kolonlar (ör. kart no, 14 §4.2).
	DropColumns []string `json:"drop_columns"`
	// TypeFields tür kodunu oluşturan mantıksal alanlar (TypeRule.Match anahtarları).
	TypeFields []string `json:"type_fields"`
	// TaxBreakdownPrefix oran bazında KDV kolonlarının ön eki (Mikro
	// "cha_vergi" → cha_vergi1..N). Doluysa ve değer varsa TAX_RATE_UNMAPPED.
	TaxBreakdownPrefix string              `json:"tax_breakdown_prefix,omitempty"`
	Fields             map[string][]string `json:"fields"`
	TypeRules          []TypeRule          `json:"type_rules"`
}

var tableNameRe = regexp.MustCompile(`^[A-Za-z0-9_{}]+$`)

func decodeStrict(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// DefaultProfile gömülü varsayılan profili döner (her çağrıda yeni kopya).
func DefaultProfile(kind Kind) (*Profile, error) {
	return LoadProfile(kind, "")
}

// LoadProfile gömülü varsayılanı okur, overridePath doluysa o dosyadaki
// aynı anahtarlı profili ("logo" ya da "mikro") üstüne yazar. Geçersiz kılma
// kuralları: verilen üst düzey alanlar değiştirilir; "fields" anahtar
// bazında birleşir; listeler (tables, type_rules, ...) bütünüyle değişir.
// Bilinmeyen alan adı hata verir (yazım hatası sessizce yutulmasın).
func LoadProfile(kind Kind, overridePath string) (*Profile, error) {
	if !kind.Valid() {
		return nil, rpc.Errorf(rpc.CodeInvalidParams, "bilinmeyen kaynak türü: %q", kind)
	}
	key := kind.profileKey()
	var all map[string]json.RawMessage
	if err := json.Unmarshal(defaultsJSON, &all); err != nil {
		return nil, rpc.Errorf(rpc.CodeInternal, "gömülü varsayılanlar okunamadı: %v", err)
	}
	p := &Profile{}
	if err := decodeStrict(all[key], p); err != nil {
		return nil, rpc.Errorf(rpc.CodeInternal, "gömülü %s profili okunamadı: %v", key, err)
	}
	if overridePath != "" {
		data, err := os.ReadFile(overridePath)
		if err != nil {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "geçersiz kılma dosyası okunamadı: %v", err)
		}
		var over map[string]json.RawMessage
		if err := json.Unmarshal(data, &over); err != nil {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "geçersiz kılma dosyası JSON değil: %v", err)
		}
		for k := range over {
			if k != "logo" && k != "mikro" {
				return nil, rpc.Errorf(rpc.CodeInvalidParams, "geçersiz kılma dosyasında bilinmeyen profil: %q (logo, mikro)", k)
			}
		}
		if raw, ok := over[key]; ok {
			if err := decodeStrict(raw, p); err != nil {
				return nil, rpc.Errorf(rpc.CodeInvalidParams, "geçersiz kılma dosyasındaki %s profili geçersiz: %v", key, err)
			}
		}
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Profile) validate() error {
	bad := func(format string, args ...any) error {
		return rpc.Errorf(rpc.CodeInvalidParams, "profil geçersiz: "+format, args...)
	}
	if strings.TrimSpace(p.InvoiceQuery) == "" {
		return bad("invoice_query boş")
	}
	if len(p.Tables) == 0 {
		return bad("tables boş")
	}
	for _, t := range p.Tables {
		if !tableNameRe.MatchString(t) {
			return bad("tablo adı geçersiz: %q", t)
		}
	}
	for f := range p.Fields {
		if !knownFields[f] {
			return bad("bilinmeyen alan: %q", f)
		}
	}
	for _, f := range p.TypeFields {
		if !knownFields[f] {
			return bad("type_fields içinde bilinmeyen alan: %q", f)
		}
	}
	for i, r := range p.TypeRules {
		if len(r.Match) == 0 {
			return bad("type_rules[%d]: match boş", i)
		}
		for f := range r.Match {
			if !knownFields[f] {
				return bad("type_rules[%d]: bilinmeyen alan %q", i, f)
			}
		}
		if r.Skip {
			continue
		}
		if r.Direction != DirectionSales && r.Direction != DirectionPurchase {
			return bad("type_rules[%d]: direction sales ya da purchase olmalı", i)
		}
		if r.Kind != DocKindInvoice && r.Kind != DocKindReturn {
			return bad("type_rules[%d]: kind invoice ya da return olmalı", i)
		}
	}
	return nil
}
