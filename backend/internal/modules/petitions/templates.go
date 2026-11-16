// Package petitions produces dilekçe (petitions) from templates (issue #56,
// T16 "dilekçematik"): office and mükellef fields are filled in from the
// engagement, the user adds the case-specific fields, and the result is
// previewed and printed as PDF.
//
// Templates live in code so that every change to a legal text goes through
// review. None has had a legal review yet: every template is Draft, and the
// preview and PDF carry DraftNotice. Templates are fixed petitions to
// request documents or to notify; the product does not give legal advice
// and does not file anything (`06` §4 ilke 7).
package petitions

// DraftNotice is printed on every petition whose template has not had a
// legal review.
const DraftNotice = "TASLAK – Bu metin hukuki kontrolden geçmemiştir. Kullanmadan önce gözden geçiriniz."

// Field types.
const (
	FieldText     = "text"
	FieldTextarea = "textarea"
	FieldDate     = "date"
)

// Field is a value the user enters for one petition.
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
	MaxLen   int    `json:"max_length"`
}

// Template is one petition.
type Template struct {
	Key       string  `json:"key"`
	Title     string  `json:"title"`
	Category  string  `json:"category"`
	Recipient string  `json:"recipient"` // addressee line, may use fields
	Subject   string  `json:"subject"`   // "Konu:" line, may use fields
	Body      string  `json:"-"`         // text/template paragraphs
	Fields    []Field `json:"fields"`
	// Draft is true until a lawyer reviewed the text.
	Draft bool `json:"draft"`
	// Version changes whenever the text changes; stored with each petition.
	Version int `json:"version"`
}

// Categories.
const (
	CategoryTaxOffice     = "vergi_dairesi"
	CategoryTradeRegistry = "ticaret_sicil"
	CategoryGeneral       = "genel"
)

// Template data (text/template):
//
//	.Taxpayer.Title .Taxpayer.TaxID .Taxpayer.TaxIDType .Taxpayer.TaxOffice
//	.Office.Name
//	.Signer.Name .Signer.SMMMRegistryNo
//	.Date (02.01.2006)
//	.F.<field key>
var templates = []Template{
	{
		Key: "vd_borc_durum_yazisi", Title: "Borç durum (borcu yoktur) yazısı talebi",
		Category: CategoryTaxOffice, Draft: true, Version: 1,
		Recipient: "{{.Taxpayer.TaxOffice | orDots}} VERGİ DAİRESİ MÜDÜRLÜĞÜNE",
		Subject:   "Borç durum yazısı talebi",
		Body:      `Dairenizin {{.Taxpayer.TaxID}} {{.Taxpayer.TaxIDType}} numaralı mükellefi {{.Taxpayer.Title}} adına, {{.F.amac}} sunulmak üzere vadesi geçmiş vergi borcu bulunup bulunmadığına ilişkin borç durum yazısının tarafımıza verilmesini arz ederim.`,
		Fields: []Field{
			{Key: "amac", Label: "Yazının sunulacağı yer / amaç", Type: FieldText, Required: true, Hint: "ör. ilgili kuruma", MaxLen: 200},
		},
	},
	{
		Key: "vd_mukellefiyet_yazisi", Title: "Mükellefiyet yazısı talebi",
		Category: CategoryTaxOffice, Draft: true, Version: 1,
		Recipient: "{{.Taxpayer.TaxOffice | orDots}} VERGİ DAİRESİ MÜDÜRLÜĞÜNE",
		Subject:   "Mükellefiyet yazısı talebi",
		Body:      `Dairenizin {{.Taxpayer.TaxID}} {{.Taxpayer.TaxIDType}} numaralı mükellefi {{.Taxpayer.Title}} adına, mükellefiyet durumunu gösterir yazının {{.F.adet}} adet olarak tarafımıza verilmesini arz ederim.`,
		Fields: []Field{
			{Key: "adet", Label: "Adet", Type: FieldText, Required: true, Hint: "ör. 1", MaxLen: 3},
		},
	},
	{
		Key: "ts_faaliyet_belgesi", Title: "Faaliyet belgesi talebi",
		Category: CategoryTradeRegistry, Draft: true, Version: 1,
		Recipient: "{{.F.sicil_mudurlugu}} TİCARET SİCİLİ MÜDÜRLÜĞÜNE",
		Subject:   "Faaliyet belgesi talebi",
		Body:      `Müdürlüğünüzün {{.F.sicil_no}} sicil numarasında kayıtlı {{.Taxpayer.Title}} (VKN/TCKN: {{.Taxpayer.TaxID}}) için faaliyet belgesi düzenlenmesini arz ederim.`,
		Fields: []Field{
			{Key: "sicil_mudurlugu", Label: "Ticaret sicili müdürlüğü", Type: FieldText, Required: true, Hint: "ör. İSTANBUL", MaxLen: 100},
			{Key: "sicil_no", Label: "Ticaret sicil no", Type: FieldText, Required: true, MaxLen: 40},
		},
	},
	{
		Key: "genel_dilekce", Title: "Genel dilekçe",
		Category: CategoryGeneral, Draft: true, Version: 1,
		Recipient: "{{.F.muhatap}}",
		Subject:   "{{.F.konu}}",
		Body:      `{{.F.metin}}`,
		Fields: []Field{
			{Key: "muhatap", Label: "Muhatap (kurum)", Type: FieldText, Required: true, Hint: "ör. KADIKÖY VERGİ DAİRESİ MÜDÜRLÜĞÜNE", MaxLen: 200},
			{Key: "konu", Label: "Konu", Type: FieldText, Required: true, MaxLen: 200},
			{Key: "metin", Label: "Metin", Type: FieldTextarea, Required: true, MaxLen: 4000},
		},
	},
}

// Templates returns all templates.
func Templates() []Template { return append([]Template(nil), templates...) }

// Lookup finds a template by key.
func Lookup(key string) (Template, bool) {
	for _, t := range templates {
		if t.Key == key {
			return t, true
		}
	}
	return Template{}, false
}
