package sources

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// rowView satırı büyük / küçük harf duyarsız okur (SQL Server kolon adları
// harmanlamaya göre duyarsızdır; Mikro'da cha_normal_Iade gibi karışık
// yazımlar var).
type rowView struct {
	byLower map[string]any
	profile *Profile
}

func newRowView(p *Profile, row Row) rowView {
	m := make(map[string]any, len(row))
	for k, v := range row {
		m[strings.ToLower(k)] = v
	}
	return rowView{byLower: m, profile: p}
}

// get mantıksal alanın değerini döner. configured: profilde aday kolon var
// mı; present: adaylardan biri satırda var mı.
func (rv rowView) get(field string) (v any, col string, configured, present bool) {
	cands := rv.profile.Fields[field]
	for _, c := range cands {
		if val, ok := rv.byLower[strings.ToLower(c)]; ok {
			return val, c, true, true
		}
	}
	return nil, "", len(cands) > 0, false
}

type mapper struct {
	rv       rowView
	warnings []Warning
}

func (m *mapper) warn(code, format string, args ...any) {
	m.warnings = append(m.warnings, Warning{Code: code, Message: fmt.Sprintf(format, args...)})
}

// lookup alanı okur; kolon yapılandırılmış ama satırda hiç yoksa
// FIELD_MISSING üretir (⚠️ kolon adı yanlış olabilir). required alanlar boş
// (NULL) geldiğinde de uyarı verilir.
func (m *mapper) lookup(field string, required bool) (any, string, bool) {
	v, col, configured, present := m.rv.get(field)
	switch {
	case configured && !present:
		m.warn(WarnFieldMissing, "%s alanı için beklenen kolon satırda yok (%s)",
			field, strings.Join(m.rv.profile.Fields[field], ", "))
		return nil, "", false
	case required && !configured:
		m.warn(WarnFieldMissing, "%s alanı için kolon tanımlı değil", field)
		return nil, "", false
	case required && isEmpty(v):
		m.warn(WarnFieldMissing, "%s alanı boş (%s)", field, col)
		return nil, col, false
	}
	return v, col, present && !isEmpty(v)
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []byte:
		return len(strings.TrimSpace(string(x))) == 0
	}
	return false
}

func (m *mapper) str(field string, required bool) string {
	v, col, ok := m.lookup(field, required)
	if !ok {
		return ""
	}
	s, err := valueString(v)
	if err != nil {
		m.warn(WarnValueInvalid, "%s (%s) metne çevrilemedi: %v", field, col, err)
		return ""
	}
	return s
}

func (m *mapper) date(field string, required bool) string {
	v, col, ok := m.lookup(field, required)
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case time.Time:
		return x.Format(time.DateOnly)
	case string:
		s := strings.TrimSpace(x)
		if len(s) >= 10 {
			if _, err := time.Parse(time.DateOnly, s[:10]); err == nil {
				return s[:10]
			}
		}
	}
	m.warn(WarnValueInvalid, "%s (%s) tarih değil: %v", field, col, v)
	return ""
}

func (m *mapper) amount(field string) string {
	v, col, ok := m.lookup(field, false)
	if !ok {
		return ""
	}
	d, err := toDecimal(v)
	if err != nil {
		m.warn(WarnValueInvalid, "%s (%s) tutar değil: %v", field, col, err)
		return ""
	}
	return d.String()
}

func (m *mapper) boolean(field string) bool {
	v, col, ok := m.lookup(field, false)
	if !ok {
		return false
	}
	s, err := valueString(v)
	if err == nil {
		switch strings.TrimSpace(s) {
		case "0":
			return false
		case "1":
			return true
		}
	}
	m.warn(WarnValueInvalid, "%s (%s) beklenmeyen değer: %v", field, col, v)
	return false
}

// valueString değeri karşılaştırma / gösterim için dizgiye çevirir.
// Tam sayı değerli float'lar ondalıksız yazılır ("8", "8.0" değil).
func valueString(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(x), nil
	case []byte:
		return strings.TrimSpace(string(x)), nil
	case bool:
		if x {
			return "1", nil
		}
		return "0", nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int16:
		return strconv.FormatInt(int64(x), 10), nil
	case int8:
		return strconv.FormatInt(int64(x), 10), nil
	case int:
		return strconv.Itoa(x), nil
	case uint8:
		return strconv.FormatUint(uint64(x), 10), nil
	case float64, float32:
		d, err := toDecimal(x)
		if err != nil {
			return "", err
		}
		return d.String(), nil
	case decimal.Decimal:
		return x.String(), nil
	case time.Time:
		return x.Format(time.RFC3339), nil
	case fmt.Stringer:
		return x.String(), nil
	}
	return "", fmt.Errorf("desteklenmeyen tür %T", v)
}

// toDecimal sürücüden gelen değeri ondalığa çevirir. DECIMAL / MONEY
// dizgi olarak gelir ve kayıpsızdır. FLOAT kolonlar (Logo tutarları FLOAT
// olabilir ⚠️) float64 gelir; en kısa ondalık gösterimle (strconv 'f', -1)
// dizgiye, oradan ondalığa çevrilir; ikili kayan nokta aritmetiği yapılmaz.
func toDecimal(v any) (decimal.Decimal, error) {
	switch x := v.(type) {
	case string:
		return decimal.NewFromString(strings.TrimSpace(x))
	case []byte:
		return decimal.NewFromString(strings.TrimSpace(string(x)))
	case int64:
		return decimal.NewFromInt(x), nil
	case int32:
		return decimal.NewFromInt(int64(x)), nil
	case int16:
		return decimal.NewFromInt(int64(x)), nil
	case int:
		return decimal.NewFromInt(int64(x)), nil
	case uint8:
		return decimal.NewFromInt(int64(x)), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return decimal.Decimal{}, fmt.Errorf("sonlu değil: %v", x)
		}
		return decimal.NewFromString(strconv.FormatFloat(x, 'f', -1, 64))
	case float32:
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return decimal.Decimal{}, fmt.Errorf("sonlu değil: %v", x)
		}
		return decimal.NewFromString(strconv.FormatFloat(f, 'f', -1, 32))
	case decimal.Decimal:
		return x, nil
	}
	return decimal.Decimal{}, fmt.Errorf("desteklenmeyen tür %T", v)
}

// typeCode tür kodu alanlarını okur. Tek alanlı profilde yalın değer
// ("8"), çok alanlıda "evrak_tip=..;cinsi=.." biçimi döner.
func (m *mapper) typeCode() (map[string]string, string, bool) {
	vals := map[string]string{}
	parts := make([]string, 0, len(m.rv.profile.TypeFields))
	complete := true
	for _, f := range m.rv.profile.TypeFields {
		v, _, configured, present := m.rv.get(f)
		if !configured || !present {
			complete = false
			if configured {
				m.warn(WarnFieldMissing, "%s alanı için beklenen kolon satırda yok (%s)",
					f, strings.Join(m.rv.profile.Fields[f], ", "))
			}
			parts = append(parts, f+"=?")
			continue
		}
		s, err := valueString(v)
		if err != nil {
			complete = false
			m.warn(WarnValueInvalid, "%s metne çevrilemedi: %v", f, err)
		}
		vals[f] = s
		parts = append(parts, f+"="+s)
	}
	if len(m.rv.profile.TypeFields) == 1 {
		return vals, vals[m.rv.profile.TypeFields[0]], complete
	}
	return vals, strings.Join(parts, ";"), complete
}

// matchRule en özgül eşleşen kuralı bulur.
func matchRule(rules []TypeRule, vals map[string]string) *TypeRule {
	var best *TypeRule
	for i := range rules {
		r := &rules[i]
		ok := true
		for k, want := range r.Match {
			got, has := vals[k]
			if !has || got != strings.TrimSpace(want) {
				ok = false
				break
			}
		}
		if ok && (best == nil || len(r.Match) > len(best.Match)) {
			best = r
		}
	}
	return best
}

// mapRow tek bir satırı tarafsız DTO'ya çevirir. Bilinmeyen değerlerde
// çökmez: uyarı ekler, ham satırı korur. skip=true ise satır kural gereği
// çekilmez.
func mapRow(kind Kind, p *Profile, row Row) (Invoice, bool) {
	m := &mapper{rv: newRowView(p, row)}
	inv := Invoice{Source: kind, Direction: DirectionUnknown, Kind: DocKindUnknown}

	vals, code, complete := m.typeCode()
	inv.TypeCode = code
	rule := (*TypeRule)(nil)
	if complete {
		rule = matchRule(p.TypeRules, vals)
	}
	if rule != nil && rule.Skip {
		return Invoice{}, true
	}
	if rule == nil {
		m.warn(WarnTypeUnmapped, "tür kodu eşleme tablosunda yok (%s); yön ve tür bilinmiyor", code)
	} else {
		inv.Direction = rule.Direction
		inv.Kind = rule.Kind
		inv.DocumentClass = rule.DocumentClass
	}

	inv.SourceID = m.str(FieldSourceID, true)
	inv.GUID = m.str(FieldGUID, false)
	inv.DocNo = m.str(FieldDocNo, false)
	inv.InternalNo = m.str(FieldInternalNo, false)
	inv.Date = m.date(FieldDate, true)
	inv.DocDate = m.date(FieldDocDate, false)
	inv.Cancelled = m.boolean(FieldCancelled)
	inv.CounterpartyCode = m.str(FieldCounterpartyCode, false)
	inv.CounterpartyTitle = m.str(FieldCounterpartyTitle, false)
	inv.CounterpartyTaxID = m.str(FieldCounterpartyTaxID, false)
	inv.Currency = m.str(FieldCurrency, false)
	inv.ETTN = m.str(FieldETTN, false)
	inv.Totals = Totals{
		Gross:         m.amount(FieldGross),
		Discounts:     m.amount(FieldDiscounts),
		VAT:           m.amount(FieldVAT),
		AdditionalTax: m.amount(FieldAdditionalTax),
		Net:           m.amount(FieldNet),
		Amount:        m.amount(FieldAmount),
		Subtotal:      m.amount(FieldSubtotal),
	}
	m.checkTaxBreakdown()

	inv.Raw = rawCopy(p, row)
	inv.Warnings = m.warnings
	return inv, false
}

// checkTaxBreakdown, oran bazında KDV kolonlarında değer varsa uyarır.
// Sıra ↔ oran eşlemesi firma parametresinden okunmalı ve doğrulanmadı
// (15 §2.2); tahmin edilmez, değerler ham satırda kalır.
func (m *mapper) checkTaxBreakdown() {
	prefix := strings.ToLower(m.rv.profile.TaxBreakdownPrefix)
	if prefix == "" {
		return
	}
	found := false
	for k, v := range m.rv.byLower {
		suffix, ok := strings.CutPrefix(k, prefix)
		if !ok || suffix == "" {
			continue
		}
		if _, err := strconv.Atoi(suffix); err != nil {
			continue
		}
		if d, err := toDecimal(v); err == nil && !d.IsZero() {
			found = true
			break
		}
	}
	if found {
		m.warn(WarnTaxRateUnmapped, "oran bazında KDV alanları dolu ama sıra ↔ oran eşlemesi doğrulanmadı; KDV toplamı yazılmadı, değerler ham veride")
	}
}

// rawCopy satırın kopyasıdır; DropColumns (ör. kredi kartı no) çıkarılır.
func rawCopy(p *Profile, row Row) map[string]any {
	drop := map[string]bool{}
	for _, c := range p.DropColumns {
		drop[strings.ToLower(c)] = true
	}
	out := make(map[string]any, len(row))
	for k, v := range row {
		if drop[strings.ToLower(k)] {
			continue
		}
		if b, ok := v.([]byte); ok {
			v = append([]byte(nil), b...)
		}
		out[k] = v
	}
	return out
}
