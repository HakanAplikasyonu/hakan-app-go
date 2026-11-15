package model

// Warning codes produced while building the common model (`05` §3). The text
// shown to the accountant is rendered from the code; severity comes from
// configuration (`05` §2), not from here.
const (
	WarnPartyMismatch       = "PARTY_MISMATCH"
	WarnUnmappedTypeCode    = "UNMAPPED_TYPE_CODE"
	WarnUnmappedProfile     = "UNMAPPED_PROFILE"
	WarnUnknownTaxCode      = "UNKNOWN_TAX_CODE"
	WarnLineTaxableMismatch = "LINE_TAXABLE_MISMATCH"
	WarnTaxTotalMismatch    = "TAX_TOTAL_MISMATCH"
	WarnBaseTotalMismatch   = "BASE_TOTAL_MISMATCH"
	WarnPayableMismatch     = "PAYABLE_MISMATCH"
	WarnHeaderEstimate      = "HEADER_ESTIMATE"
	WarnInvalidTaxID        = "INVALID_TAX_ID"
	WarnPossibleDuplicate   = "POSSIBLE_DUPLICATE"
	WarnUBLSchematronFail   = "UBL_SCHEMATRON_FAIL"
)

// Entity types a warning attaches to (`05` §1).
const (
	EntityInvoice     = "invoice"
	EntityInvoiceItem = "invoice_item"
)

// Warning is a data_warning row before it is stored.
type Warning struct {
	Code       string         `json:"code"`
	EntityType string         `json:"entity_type"`
	LineNo     int            `json:"line_no,omitempty"` // for invoice_item
	Params     map[string]any `json:"params,omitempty"`
}
