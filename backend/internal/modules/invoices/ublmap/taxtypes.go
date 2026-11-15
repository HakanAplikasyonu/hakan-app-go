package ublmap

// Tax type code → common-model field (`31` §5.3). This is the single place to
// change when the GİB "Vergi Türü Kodları" table is verified or moves to a DB
// table (tax_type_code). Codes not listed here but valid in the GİB code list
// stay in tax_breakdowns as "other"; codes not in the GİB list at all raise
// UNKNOWN_TAX_CODE.
//
// ⚠️ Only VAT (0015) is confirmed by the `31` spec. The ÖTV / ÖİV / stopaj
// assignments below follow GİB's published code names and must be checked
// against the current UBL-TR Kod Listeleri before relying on them for KDV1.
const (
	fieldVAT                  = "vat"
	fieldExcise               = "excise"
	fieldCommunicationsTax    = "communications_tax"
	fieldIncomeTaxWithholding = "income_tax_withholding"
	fieldOther                = "other"
)

var taxTypeField = map[string]string{
	"0015": fieldVAT,
	// ÖTV listeleri
	"0071": fieldExcise,
	"0073": fieldExcise,
	"0074": fieldExcise,
	"0075": fieldExcise,
	"0076": fieldExcise,
	"0077": fieldExcise,
	"9077": fieldExcise,
	// Özel iletişim vergisi
	"4080": fieldCommunicationsTax,
	"4081": fieldCommunicationsTax,
	// Gelir / kurumlar vergisi stopajı
	"0003": fieldIncomeTaxWithholding,
	"0011": fieldIncomeTaxWithholding,
}

func taxField(code string) string {
	if f, ok := taxTypeField[code]; ok {
		return f
	}
	return fieldOther
}
