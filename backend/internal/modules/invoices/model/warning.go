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

// DefaultSeverity is the fallback level per code when the warning catalog
// (#13: warning_codes / warning_rule_configs) has no row for it. It mirrors
// the catalog seed.
var DefaultSeverity = map[string]string{
	WarnPartyMismatch:       "blocker",
	WarnUnmappedTypeCode:    "warning",
	WarnUnmappedProfile:     "info",
	WarnUnknownTaxCode:      "warning",
	WarnLineTaxableMismatch: "warning",
	WarnTaxTotalMismatch:    "warning",
	WarnBaseTotalMismatch:   "warning",
	WarnPayableMismatch:     "warning",
	WarnHeaderEstimate:      "warning",
	WarnInvalidTaxID:        "warning",
	WarnPossibleDuplicate:   "warning",
	WarnUBLSchematronFail:   "blocker",
}

// Severity returns the level for code, "warning" when unknown.
func Severity(code string) string {
	if s, ok := DefaultSeverity[code]; ok {
		return s
	}
	return "warning"
}
