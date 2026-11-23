package accountingexport

import (
	"context"
	"io"
	"time"

	"github.com/shopspring/decimal"
)

// EntryType is the common voucher type; each driver maps it to its program's
// value (DriverInfo.EntryTypes).
type EntryType string

const (
	EntryMahsup  EntryType = "mahsup"
	EntryAcilis  EntryType = "acilis"
	EntryKapanis EntryType = "kapanis"
)

// Limits are a program's import limits. Zero means unlimited.
type Limits struct {
	MaxEntriesPerFile int `json:"max_entries_per_file" yaml:"max_entries_per_file"`
	// MaxLinesPerEntry applies to mahsup entries (Luca: 400). Açılış and
	// kapanış entries have no line limit in Luca.
	MaxLinesPerEntry int `json:"max_lines_per_mahsup" yaml:"max_lines_per_mahsup"`
}

// DriverInfo describes a driver.
type DriverInfo struct {
	Name        string               `json:"name"`
	TemplateVer string               `json:"template_version"`
	Limits      Limits               `json:"limits"`
	EntryTypes  map[EntryType]string `json:"entry_types"`
	FileTypes   []string             `json:"file_types"`
	// Verified: the template was checked against a real file of the program.
	// The UI shows "şablon doğrulanmadı" while it is false.
	Verified bool `json:"verified"`
}

// Config opens a driver. Template selects a template version ("" = the
// driver's default); Options are driver specific.
type Config struct {
	Template string
	Options  map[string]string
}

// Batch is what a driver exports: approved JournalDrafts plus context.
type Batch struct {
	ID       string
	Period   string // YYYY-MM
	Taxpayer Party
	Office   Party
	Entries  []Entry
	// TargetPlan is the office's account plan for the target program (#30).
	// Nil means "no plan loaded": the account check is skipped.
	TargetPlan AccountPlan
	Settings   Settings
	// VATSummary is the portal's own KDV summary for the period (excel_formula
	// shows it next to the computed one).
	VATSummary []VATRow
	// Warnings are open data warnings to list next to the output.
	Warnings    []WarningRow
	GeneratedAt time.Time
	GeneratedBy string
}

// Party names a taxpayer or office in file headers.
type Party struct {
	Title string
	TaxID string
}

// Settings carry per-taxpayer and per-office choices.
type Settings struct {
	// BlockOnUnknownAccount turns ACCOUNT_NOT_IN_TARGET_PLAN into a blocker
	// (docs/02 §5 test 4, "ayara bağlı").
	BlockOnUnknownAccount bool
	// InvoicesReachLucaDirectly: invoices already reach the program through a
	// TÜRMOB preferred integrator (docs/02 §4); invoice-sourced entries are
	// left out unless IncludeInvoiceEntries is set.
	InvoicesReachLucaDirectly bool
	IncludeInvoiceEntries     bool
}

// AccountPlan is the set of account codes the target program knows.
type AccountPlan map[string]bool

// Has reports whether code is in the plan.
func (p AccountPlan) Has(code string) bool { return p[code] }

// Entry is one voucher (fiş).
type Entry struct {
	ID           string
	No           string
	Date         time.Time
	Type         EntryType
	Description  string
	DocumentType string
	Lines        []Line
}

// Line is one account movement of an entry.
type Line struct {
	Account       string
	Description   string
	Debit         decimal.Decimal
	Credit        decimal.Decimal
	Quantity      *decimal.Decimal
	Currency      string
	ExchangeRate  *decimal.Decimal
	ForeignAmount *decimal.Decimal
	DocNo         string
	DocDate       *time.Time
	// TaxID is the counterparty VKN / TCKN of the source document (Luca:
	// TC/Vergi No); "" when the line has none.
	TaxID      string
	Source     SourceRef
	Confidence *decimal.Decimal
}

// SourceRef links a line to its source document.
type SourceRef struct {
	Kind string // invoice, invoice_item, bank_transaction, receipt …
	ID   string // journal_draft_lines.source_id (invoice id, bank transaction id …)
	URL  string // portal link that opens the document
	// DocumentKey identifies the source document across exports (#466): the
	// invoice ETTN (lower case), "" when the line has no document. Two lines
	// with the same key come from the same invoice.
	DocumentKey string
	// ETTN is the invoice ETTN when known; drivers write it into the file
	// (Luca: Açıklama) so the program shows which document a line came from.
	ETTN string
}

// VATRow is one rate of a KDV summary.
type VATRow struct {
	Direction string // outgoing | incoming
	Rate      decimal.Decimal
	Base      decimal.Decimal
	VAT       decimal.Decimal
}

// WarningRow is an open data warning shown with the output.
type WarningRow struct {
	Code     string
	Severity string
	Entity   string
	Message  string
	Link     string
}

// Severities of an Issue.
const (
	SeverityBlocker = "blocker"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Issue is a validation finding. A blocker stops the export.
type Issue struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	EntryNo  string         `json:"entry_no,omitempty"`
	Line     int            `json:"line,omitempty"` // 1-based, 0 = whole entry
	Params   map[string]any `json:"params,omitempty"`
}

// Issue codes produced here and by the drivers.
const (
	IssueUnbalanced           = "JOURNAL_UNBALANCED"
	IssueEmptyEntry           = "JOURNAL_EMPTY"
	IssueAccountNotInPlan     = "ACCOUNT_NOT_IN_TARGET_PLAN"
	IssueSplitImpossible      = "JOURNAL_SPLIT_IMPOSSIBLE"
	IssueTemplateUnverified   = "TEMPLATE_UNVERIFIED"
	IssueUnsupportedType      = "ENTRY_TYPE_UNSUPPORTED"
	IssueInvoiceEntryExcluded = "INVOICE_ENTRY_EXCLUDED"
	// Double booking guard (#466). Both leave the entry out of the files
	// until the accountant confirms the document for this export.
	//
	// IssueDocumentAlreadyExported: the same document (ETTN) is already in a
	// live export file of the same program.
	IssueDocumentAlreadyExported = "DOCUMENT_ALREADY_EXPORTED"
	// IssueProgramPullsDocument: the target program fetches this e-invoice
	// itself from the integrator (Luca: İşNet NetteFatura, TÜRMOB e-Belge,
	// GİB e-Belge), per office / taxpayer setting.
	IssueProgramPullsDocument = "PROGRAM_PULLS_DOCUMENT"
)

// HasBlocker reports whether any issue blocks the export.
func HasBlocker(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityBlocker {
			return true
		}
	}
	return false
}

// FileWriter receives the files a driver produces.
type FileWriter interface {
	Create(name, contentType string) (io.WriteCloser, error)
}

// File is one produced file.
type File struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Entries     int    `json:"entries"`
}

// Result is what Export returns.
type Result struct {
	Files  []File  `json:"files"`
	Issues []Issue `json:"issues"`
	// EntryIDs are the entries that went into the files (after exclusions);
	// export_batch_item rows are written for these.
	EntryIDs []string `json:"entry_ids"`
	// EntryFiles maps an entry ID to the file it went into (the first one
	// when a split entry spans files): the document → file trail (#466).
	EntryFiles map[string]string `json:"-"`
}

// AddEntryFile records that entry id went into file name (first file wins).
func (r *Result) AddEntryFile(id, name string) {
	if r.EntryFiles == nil {
		r.EntryFiles = map[string]string{}
	}
	if _, ok := r.EntryFiles[id]; !ok {
		r.EntryFiles[id] = name
	}
}

// Driver is a target program.
type Driver interface {
	Info() DriverInfo
	Validate(ctx context.Context, b Batch) []Issue
	Export(ctx context.Context, b Batch, w FileWriter) (Result, error)
}
