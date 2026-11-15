// Package catalog lists the warning codes of
// docs/kaynak/musavir-entegrasyon-paketi/05-uyari-kodlari.md (§3.1–§3.4)
// plus UBL_SCHEMATRON_FAIL (docs/01 §1.1) and ACCOUNT_NOT_IN_TARGET_PLAN
// (docs/02). Producers write codes, never texts; the panel renders the text.
//
// Severities are NOT here: the suggested level of every code is seeded into
// the warning_codes table and overridden per office / engagement in
// warning_rule_configs, so a level changes by configuration, not by a
// release. catalog_test.go keeps this list, the seed and 05 in step.
package catalog

// Severity of a warning (05 §2).
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityBlocker Severity = "blocker"
)

// Valid reports whether s is a known severity.
func (s Severity) Valid() bool {
	return s == SeverityInfo || s == SeverityWarning || s == SeverityBlocker
}

// Status of a warning (05 §1).
type Status string

const (
	StatusOpen         Status = "open"
	StatusAcknowledged Status = "acknowledged"
	StatusResolved     Status = "resolved"
	StatusAutoResolved Status = "auto_resolved"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusAcknowledged, StatusResolved, StatusAutoResolved:
		return true
	}
	return false
}

// Entity types a warning can point at (05 §1). AnyEntity means "herhangi".
const (
	EntityInvoice              = "invoice"
	EntityInvoiceItem          = "invoice_item"
	EntityCustomer             = "customer"
	EntityExpense              = "expense"
	EntityFinancialTransaction = "financial_transaction"
	EntityRetailSale           = "retail_sale"
	EntityBankTransaction      = "bank_transaction"
	EntityEntegratorDocument   = "entegrator_document"
	EntityPeriod               = "period"
	AnyEntity                  = "*"
)

// EntityTypes is every concrete entity type.
var EntityTypes = []string{
	EntityInvoice, EntityInvoiceItem, EntityCustomer, EntityExpense,
	EntityFinancialTransaction, EntityRetailSale, EntityBankTransaction,
	EntityEntegratorDocument, EntityPeriod,
}

// Code is one catalog entry.
type Code struct {
	Code string
	// Section of 05 (or "01"/"02" for the two added codes).
	Section string
	// EntityTypes the code may be raised on; AnyEntity allows all.
	EntityTypes []string
}

// Codes, in 05 order.
var Codes = []Code{
	// §3.1 Fatura ve UBL
	{"PENDING_RESPONSE", "3.1", []string{EntityInvoice}},
	{"REJECTED", "3.1", []string{EntityInvoice}},
	{"RETURNED", "3.1", []string{EntityInvoice}},
	{"DELIVERY_ERROR", "3.1", []string{EntityInvoice}},
	{"STATUS_UNMAPPED", "3.1", []string{EntityInvoice}},
	{"STATUS_UNVERIFIED", "3.1", []string{EntityInvoice}},
	{"EARCHIVE_REPORT_ERROR", "3.1", []string{EntityInvoice}},
	{"DOC_KIND_MISMATCH", "3.1", []string{EntityInvoice}},
	{"UUID_MISMATCH", "3.1", []string{EntityEntegratorDocument}},
	{"PARTY_MISMATCH", "3.1", []string{EntityInvoice}},
	{"UNMAPPED_TYPE_CODE", "3.1", []string{EntityInvoice}},
	{"UNMAPPED_PROFILE", "3.1", []string{EntityInvoice}},
	{"UNKNOWN_TAX_CODE", "3.1", []string{EntityInvoice}},
	{"LINE_TAXABLE_MISMATCH", "3.1", []string{EntityInvoiceItem}},
	{"TAX_TOTAL_MISMATCH", "3.1", []string{EntityInvoice}},
	{"BASE_TOTAL_MISMATCH", "3.1", []string{EntityInvoice}},
	{"PAYABLE_MISMATCH", "3.1", []string{EntityInvoice}},
	{"HEADER_ESTIMATE", "3.1", []string{EntityInvoice}},
	{"UBL_MISSING", "3.1", []string{EntityEntegratorDocument}},
	{"MISSING_AT_SOURCE", "3.1", []string{EntityEntegratorDocument}},
	{"UNPROCESSED_INBOX", "3.1", []string{EntityInvoice}},
	// §3.2 Genel veri kalitesi
	{"POSSIBLE_DUPLICATE", "3.2", []string{EntityInvoice}},
	{"INVALID_TAX_ID", "3.2", []string{EntityInvoice, EntityCustomer}},
	{"DELETED_AT_SOURCE", "3.2", []string{AnyEntity}},
	{"LOCKED_PERIOD_CHANGE", "3.2", []string{AnyEntity}},
	{"VAT_REPORT_MISMATCH", "3.2", []string{EntityPeriod}},
	{"OUT_OF_PERIOD", "3.2", []string{AnyEntity}},
	{"UNAPPROVED_EXPENSE", "3.2", []string{EntityExpense}},
	{"RETAIL_ALREADY_INVOICED", "3.2", []string{EntityRetailSale}},
	// §3.3 Müşavir talebinden gelen kontroller
	{"FIXED_ASSET_CANDIDATE", "3.3", []string{EntityInvoiceItem}},
	{"UNUSUAL_RECORD_TYPE", "3.3", []string{EntityInvoiceItem}},
	{"UNUSUAL_AMOUNT", "3.3", []string{EntityInvoice}},
	{"NEW_PARTY_HIGH_AMOUNT", "3.3", []string{EntityInvoice}},
	{"PLATE_NOT_REGISTERED", "3.3", []string{EntityExpense}},
	// §3.4 Banka (21 §6.3)
	{"BNK_UNINVOICED_COLLECTION", "3.4", bank},
	{"BNK_UNDOCUMENTED_PAYMENT", "3.4", bank},
	{"BNK_FEES_NOT_BOOKED", "3.4", bank},
	{"BNK_INTEREST", "3.4", bank},
	{"BNK_TAX_PAID", "3.4", bank},
	{"BNK_RENT_WITHHOLDING", "3.4", bank},
	{"BNK_SALARY_VS_PAYROLL", "3.4", bank},
	{"BNK_PAYMENT_METHOD_MISMATCH", "3.4", bank},
	{"BNK_CASH_LIMIT", "3.4", bank},
	{"BNK_POS_VS_SALES", "3.4", bank},
	{"BNK_BALANCE_MISMATCH", "3.4", bank},
	{"BNK_GAP", "3.4", bank},
	{"BNK_NON_BUSINESS", "3.4", bank},
	{"BNK_UNCLASSIFIED", "3.4", bank},
	// Added by HAKAN docs.
	{"UBL_SCHEMATRON_FAIL", "01", []string{EntityInvoice}},
	{"ACCOUNT_NOT_IN_TARGET_PLAN", "02", []string{AnyEntity}},
}

var bank = []string{EntityBankTransaction, EntityPeriod}

var byCode = func() map[string]Code {
	m := make(map[string]Code, len(Codes))
	for _, c := range Codes {
		m[c.Code] = c
	}
	return m
}()

// Lookup returns a catalog entry.
func Lookup(code string) (Code, bool) {
	c, ok := byCode[code]
	return c, ok
}

// Allows reports whether code may be raised on entityType.
func (c Code) Allows(entityType string) bool {
	for _, t := range c.EntityTypes {
		if t == AnyEntity || t == entityType {
			return true
		}
	}
	return false
}

// ValidEntityType reports whether t is a concrete entity type.
func ValidEntityType(t string) bool {
	for _, e := range EntityTypes {
		if e == t {
			return true
		}
	}
	return false
}
