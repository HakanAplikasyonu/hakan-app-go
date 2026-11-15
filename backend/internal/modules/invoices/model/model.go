// Package model is the common data model for invoices (kaynak paket `01`
// §4.4–4.6): what every adapter (UBL, programs, Excel, OCR) produces and what
// the controls, review queue and accounting export read. Field names follow
// the Easytrade API names used in `01`; amounts are decimal, never float.
package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// Direction of an invoice from the taxpayer's point of view.
type Direction string

const (
	DirectionOutgoing Direction = "outgoing" // satış
	DirectionIncoming Direction = "incoming" // alış
)

// Invoice types (Easytrade enum, `01` §4.4). Not extended: unknown UBL values
// map to SATIS / ALIS with UNMAPPED_TYPE_CODE.
const (
	TypeSatis        = "SATIS"
	TypeAlis         = "ALIS"
	TypeIade         = "IADE"
	TypeTevkifat     = "TEVKIFAT"
	TypeIstisna      = "ISTISNA"
	TypeOzelMatrah   = "OZELMATRAH"
	TypeIhracKayitli = "IHRACKAYITLI"
	TypeSGK          = "SGK"
)

// Profiles (`01` §4.4). Nil profile means paper / unknown.
const (
	ProfileTemel          = "TEMELFATURA"
	ProfileTicari         = "TICARIFATURA"
	ProfileEArsiv         = "EARSIVFATURA"
	ProfileIhracat        = "IHRACAT"
	ProfileYolcuBeraber   = "YOLCUBERABERFATURA"
	ProfileOzel           = "OZELFATURA"
	ProfileKamu           = "KAMU"
	ProfileIlacTibbiCihaz = "ILAC_TIBBICIHAZ"
)

// Capture methods (`01` §1.2 rule 1).
const (
	CaptureAPI         = "api"
	CaptureExcelImport = "excel_import"
	CaptureOCR         = "ocr"
	CaptureManual      = "manual"
	CaptureQR          = "qr"
	CaptureBankMatch   = "bank_match"
	CaptureUBLUpload   = "ubl_upload"
)

// Tax summary sources (`01` §4.6).
const (
	TaxSummaryFromItems          = "items"
	TaxSummaryFromSource         = "source_summary"
	TaxSummaryFromHeaderEstimate = "header_estimate"
)

// Line types (`01` §4.5).
const (
	LineGoods   = "goods"
	LineService = "service"
	LineUnknown = "unknown"
)

// Party is the counterparty as written on the document (denormalised onto
// invoice.customer_* and used to find or create the taxpayer's own customer
// record; D1b: never linked to another taxpayer).
type Party struct {
	TaxNumber      string `json:"tax_number,omitempty"`      // VKN, 10 digits
	IdentityNumber string `json:"identity_number,omitempty"` // TCKN, 11 digits
	Name           string `json:"name,omitempty"`
	FirstName      string `json:"first_name,omitempty"`
	LastName       string `json:"last_name,omitempty"`
	LegalType      string `json:"customer_legal_type,omitempty"` // individual | corporate
	TaxOffice      string `json:"tax_office,omitempty"`
	Address        string `json:"address,omitempty"`
	City           string `json:"city,omitempty"`
	District       string `json:"district,omitempty"`
	PostalCode     string `json:"postal_code,omitempty"`
	Country        string `json:"country,omitempty"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
	// IsDefaultRetail marks the generic end-consumer id (TCKN 11111111111).
	IsDefaultRetail bool `json:"is_default_retail_customer,omitempty"`
}

// Amounts are the header totals (`01` §4.4 amounts.*), all positive.
type Amounts struct {
	LineExtensionAmount        decimal.Decimal `json:"line_extension_amount"`
	AllowanceTotalAmount       decimal.Decimal `json:"allowance_total_amount"`
	TaxExclusiveAmount         decimal.Decimal `json:"tax_exclusive_amount"`
	TaxAmount                  decimal.Decimal `json:"tax_amount"` // KDV only
	TaxInclusiveAmount         decimal.Decimal `json:"tax_inclusive_amount"`
	WithholdingAmount          decimal.Decimal `json:"withholding_amount"`
	ExciseAmount               decimal.Decimal `json:"excise_amount"`
	CommunicationsTaxAmount    decimal.Decimal `json:"communications_tax_amount"`
	IncomeTaxWithholdingAmount decimal.Decimal `json:"income_tax_withholding_amount"`
	PayableAmount              decimal.Decimal `json:"payable_amount"`
	PayableAmountTRY           decimal.Decimal `json:"payable_amount_try"`
}

// Invoice is `01` §4.4.
type Invoice struct {
	SourceSystem  string `json:"source_system"`
	SourceID      string `json:"source_id"`
	CaptureMethod string `json:"capture_method"`

	UUID          string    `json:"uuid,omitempty"` // ETTN, lower-case canonical form
	InvoiceNumber string    `json:"invoice_number"`
	Direction     Direction `json:"direction"`
	Type          string    `json:"type"`
	TypeCodeRaw   string    `json:"invoice_type_code_raw,omitempty"`
	Profile       *string   `json:"profile"`
	ProfileRaw    string    `json:"profile_raw,omitempty"`
	DocumentClass string    `json:"document_class"`

	IssueDate      time.Time  `json:"issue_date"` // date only, UTC midnight
	IssueTime      string     `json:"issue_time,omitempty"`
	PaymentDueDate *time.Time `json:"payment_due_date,omitempty"`
	Period         string     `json:"period"` // YYYY-MM

	Counterparty Party `json:"customer"`

	Currency     string           `json:"currency"`
	ExchangeRate *decimal.Decimal `json:"exchange_rate,omitempty"`
	Amounts      Amounts          `json:"amounts"`

	IncomeTaxWithholdingRate *decimal.Decimal `json:"income_tax_withholding_rate,omitempty"`
	GlobalTaxExemptionCode   string           `json:"global_tax_exemption_code,omitempty"`
	OriginalInvoiceNumber    string           `json:"original_invoice_number,omitempty"`
	OriginalInvoiceIssueDate *time.Time       `json:"original_invoice_issue_date,omitempty"`
	OrderReference           map[string]any   `json:"order_reference,omitempty"`
	Notes                    string           `json:"notes,omitempty"`

	HasItems         bool   `json:"has_items"`
	TaxSummarySource string `json:"tax_summary_source"`
	ContentHash      string `json:"content_hash"` // sha256 hex of the source bytes

	Items        []InvoiceItem  `json:"items"`
	TaxSummaries []TaxSummary   `json:"tax_summary"`
	RawPayload   map[string]any `json:"raw_payload,omitempty"`
}

// InvoiceItem is `01` §4.5.
type InvoiceItem struct {
	LineNo      int    `json:"line_no"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ProductCode string `json:"product_code,omitempty"`

	Quantity             decimal.Decimal `json:"quantity"`
	UnitCode             string          `json:"unit_code,omitempty"`
	UnitPrice            decimal.Decimal `json:"unit_price"`
	LineExtensionAmount  decimal.Decimal `json:"line_extension_amount"` // quantity × unit price
	OwnAllowanceAmount   decimal.Decimal `json:"own_allowance_amount"`
	AllowanceTotalAmount decimal.Decimal `json:"allowance_total_amount"`
	TaxExclusiveAmount   decimal.Decimal `json:"tax_exclusive_amount"` // KDV matrahı
	TaxRate              decimal.Decimal `json:"tax_rate"`
	TotalTaxAmount       decimal.Decimal `json:"total_tax_amount"` // KDV
	TaxInclusiveAmount   decimal.Decimal `json:"tax_inclusive_amount"`

	WithholdingCode   string          `json:"withholding_code,omitempty"`
	WithholdingRate   decimal.Decimal `json:"withholding_rate"` // percent: 5/10 → 50
	WithholdingAmount decimal.Decimal `json:"withholding_amount"`
	TaxExemptionCode  string          `json:"tax_exemption_code,omitempty"`

	SpecialTaxBaseCode   string           `json:"special_tax_base_code,omitempty"`
	SpecialTaxBaseAmount *decimal.Decimal `json:"special_tax_base_amount,omitempty"`
	ExportRegisteredCode string           `json:"export_registered_code,omitempty"`
	GTIPCode             string           `json:"gtip_code,omitempty"`

	ExciseRate                 decimal.Decimal `json:"excise_rate"`
	ExciseAmount               decimal.Decimal `json:"excise_amount"`
	CommunicationsTaxRate      decimal.Decimal `json:"communications_tax_rate"`
	CommunicationsTaxAmount    decimal.Decimal `json:"communications_tax_amount"`
	IncomeTaxWithholdingAmount decimal.Decimal `json:"income_tax_withholding_amount"`

	LineType      string         `json:"line_type"`
	TaxBreakdowns []TaxBreakdown `json:"tax_breakdowns"`
}

// TaxBreakdown is one raw UBL tax subtotal, kept losslessly (amounts as the
// lexical strings from the XML).
type TaxBreakdown struct {
	Kind          string `json:"kind"` // tax | withholding
	TaxTypeCode   string `json:"tax_type_code"`
	Name          string `json:"name,omitempty"`
	TaxableAmount string `json:"taxable_amount,omitempty"`
	TaxAmount     string `json:"tax_amount"`
	Percent       string `json:"percent,omitempty"`
	ExemptionCode string `json:"exemption_code,omitempty"`
	Exemption     string `json:"exemption,omitempty"`
	Currency      string `json:"currency,omitempty"`
}

// TaxSummary is `01` §4.6 (one row per rate / withholding / exemption).
type TaxSummary struct {
	TaxRate          decimal.Decimal `json:"tax_rate"`
	Base             decimal.Decimal `json:"base"`
	VAT              decimal.Decimal `json:"vat"`
	Withholding      decimal.Decimal `json:"withholding"`
	WithholdingCode  string          `json:"withholding_code,omitempty"`
	TaxExemptionCode string          `json:"tax_exemption_code,omitempty"`
}
