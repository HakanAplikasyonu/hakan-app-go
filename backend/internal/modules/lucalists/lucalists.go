// Package lucalists is "Luca'ya aktar (Excel)" for the taxpayer's lists
// (Oğuz'un geri bildirimi): the period's incoming and outgoing invoices,
// bank movements and Z reports as a Luca fiş file, like the Masraflar
// screen's file (#852).
//
// The file is the Luca driver's default (verified-1) fiş template: one
// header row (Fiş No … TC/Vergi No), one fiş per document, each VAT rate on
// its own line. A fiş whose debit and credit do not balance is left out and
// reported. An account the app does not know (no approved code, no
// suggestion) is written with the usual main account and the line's Detay
// Açıklama says "(hesap kodu kontrol edin)"; those lines are reported too.
//
// Double export guard: the same one as the expenses' (#852) and invoices'
// (#466). Every document that goes out is recorded in
// expense_export_documents (000245: source_kind + source_uuid); a document
// already exported (by this file or the journal export) stays out of the
// next file unless the export re-includes it.
package lucalists

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport/luca"
)

// List is one of the taxpayer's lists.
type List string

// The lists that export to Luca.
const (
	IncomingInvoices List = "incoming-invoices"
	OutgoingInvoices List = "outgoing-invoices"
	BankTransactions List = "bank-transactions"
	ZReports         List = "z-reports"
)

// Lists are the valid lists, in screen order.
var Lists = []List{IncomingInvoices, OutgoingInvoices, BankTransactions, ZReports}

// Valid reports whether l is a known list.
func (l List) Valid() bool {
	for _, x := range Lists {
		if l == x {
			return true
		}
	}
	return false
}

// filePrefix names the file: gelen_fatura_luca_fis_2026-10.xlsx.
func (l List) filePrefix() string {
	switch l {
	case IncomingInvoices:
		return "gelen_fatura_"
	case OutgoingInvoices:
		return "giden_fatura_"
	case BankTransactions:
		return "banka_"
	default:
		return "z_raporu_"
	}
}

// SourceKind is expense_export_documents.source_kind of the list's documents.
func (l List) SourceKind() string {
	switch l {
	case IncomingInvoices, OutgoingInvoices:
		return "invoice"
	case BankTransactions:
		return "bank_transaction"
	default:
		return "retail_sale"
	}
}

// CheckNote marks a line whose account the app does not know: the usual
// main account is written and the accountant checks it in Luca.
const CheckNote = "(hesap kodu kontrol edin)"

// Why a line is marked CheckNote (Row.Reason in Report.Check).
const (
	CheckItemAccount         = "ITEM_ACCOUNT"
	CheckCounterpartyAccount = "COUNTERPARTY_ACCOUNT"
	CheckVATAccount          = "VAT_ACCOUNT"
	CheckBankAccount         = "BANK_ACCOUNT"
	CheckCounterAccount      = "COUNTER_ACCOUNT"
	CheckMainAccount         = "MAIN_ACCOUNT"
)

// Why a document is left out (Row.Reason in Report.Skipped). Invoices also
// use the invoice posting's refusal codes (FOREIGN_CURRENCY, RETURN_INVOICE …).
const (
	SkipUnbalanced      = "UNBALANCED"
	SkipNoDate          = "NO_DATE"
	SkipNoAmount        = "NO_AMOUNT"
	SkipForeignCurrency = "FOREIGN_CURRENCY"
	SkipSameAccount     = "SAME_ACCOUNT"
	SkipZBreakdown      = "Z_BREAKDOWN"
)

// ErrNothingToExport: the period has no document that can go to Luca.
var ErrNothingToExport = errors.New("lucalists: nothing to export in this period")

// ErrInvalid: a bad request (list, period) or a ledger without account codes.
var ErrInvalid = errors.New("lucalists: invalid request")

// Line is one fiş line of a document. Check is set when the account is a
// fallback the accountant has to check (a Check* code).
type Line struct {
	Account     string
	Description string
	Debit       decimal.Decimal
	Credit      decimal.Decimal
	Check       string
}

// Doc is one document of a list, ready for its fiş.
type Doc struct {
	// Key is the double export guard key: an invoice's ETTN (else
	// "inv:<uuid>"), "bank:<uuid>", "z:<uuid>".
	Key          string
	UUID         uuid.UUID
	No           string
	Date         time.Time // fiş date
	DocDate      time.Time // Evrak Tarihi
	TaxID        string
	Description  string
	DocumentType string
	SourceKind   string // journal line source kind (invoice, bank_transaction, receipt)
	SourceID     string
	Lines        []Line
}

// Row is one document in the report shown after the download.
type Row struct {
	DocumentNo string `json:"document_no"`
	Date       string `json:"date,omitempty"`
	Reason     string `json:"reason"`
	Account    string `json:"account,omitempty"`
}

// Report says what went into the file and what did not.
type Report struct {
	Exported int `json:"exported"`
	// Check: lines written with a fallback account ("kontrol edin").
	Check []Row `json:"check"`
	// Skipped: documents left out (unbalanced, foreign currency …).
	Skipped []Row `json:"skipped"`
	// AlreadyExported: documents that already went to Luca and were not
	// re-included.
	AlreadyExported []Row `json:"already_exported"`
}

func rowOf(d Doc, reason, account string) Row {
	no := d.No
	if no == "" {
		// A bank movement has no document number: its description.
		no = d.Description
		if r := []rune(no); len(r) > 60 {
			no = string(r[:60])
		}
	}
	r := Row{DocumentNo: no, Reason: reason, Account: account}
	if !d.Date.IsZero() {
		r.Date = d.Date.Format(time.DateOnly)
	}
	return r
}

// isMain reports whether code is a bare main account (no sub account).
func isMain(code string) bool {
	code = strings.TrimSpace(code)
	return code != "" && !strings.ContainsAny(code, ".-_ ")
}

// Entries turns documents into Luca vouchers: one fiş per document, Fiş
// No 1…n in date order (then document number). Zero lines are dropped. A
// document whose debit and credit do not balance (or are zero) is left out
// and reported as UNBALANCED. Lines with a fallback account get CheckNote
// and a Check row. It returns the vouchers, the documents that went in (in
// voucher order) and the report rows.
func Entries(docs []Doc) (entries []ae.Entry, kept []Doc, check, skipped []Row) {
	xs := append([]Doc(nil), docs...)
	sort.SliceStable(xs, func(i, j int) bool {
		if !xs[i].Date.Equal(xs[j].Date) {
			return xs[i].Date.Before(xs[j].Date)
		}
		return xs[i].No < xs[j].No
	})
	for _, d := range xs {
		var debit, credit decimal.Decimal
		var lines []Line
		for _, l := range d.Lines {
			l.Debit, l.Credit = l.Debit.Round(2), l.Credit.Round(2)
			if l.Debit.IsZero() && l.Credit.IsZero() {
				continue
			}
			debit, credit = debit.Add(l.Debit), credit.Add(l.Credit)
			lines = append(lines, l)
		}
		if !debit.Equal(credit) || !debit.IsPositive() {
			skipped = append(skipped, rowOf(d, SkipUnbalanced, ""))
			continue
		}
		docDate := d.DocDate
		if docDate.IsZero() {
			docDate = d.Date
		}
		e := ae.Entry{ID: d.Key, No: strconv.Itoa(len(entries) + 1), Date: d.Date, Type: ae.EntryMahsup,
			Description: d.Description, DocumentType: d.DocumentType}
		seen := map[string]bool{}
		for _, l := range lines {
			desc := l.Description
			if l.Check != "" {
				desc = strings.TrimSpace(desc + " " + CheckNote)
				if !seen[l.Account+"|"+l.Check] {
					seen[l.Account+"|"+l.Check] = true
					check = append(check, rowOf(d, l.Check, l.Account))
				}
			}
			dd := docDate
			e.Lines = append(e.Lines, ae.Line{Account: l.Account, Description: desc, Debit: l.Debit, Credit: l.Credit,
				DocNo: d.No, DocDate: &dd, TaxID: d.TaxID, Source: ae.SourceRef{Kind: d.SourceKind, ID: d.SourceID, DocumentKey: d.Key}})
		}
		entries = append(entries, e)
		kept = append(kept, d)
	}
	return entries, kept, check, skipped
}

// Guard leaves out the documents already exported (prior holds their
// keys) unless this export re-includes them: one by one (confirm, document
// keys) or all of them. It returns the documents that go, the re-included
// keys and the ones that stayed out.
func Guard(docs []Doc, prior map[string]bool, confirm []string, all bool) ([]Doc, map[string]bool, []Doc) {
	ok := map[string]bool{}
	for _, c := range confirm {
		ok[strings.ToLower(strings.TrimSpace(c))] = true
	}
	out := docs[:0:0]
	confirmed := map[string]bool{}
	var left []Doc
	for _, d := range docs {
		if prior[d.Key] {
			if !all && !ok[d.Key] {
				left = append(left, d)
				continue
			}
			confirmed[d.Key] = true
		}
		out = append(out, d)
	}
	return out, confirmed, left
}

// File is the Luca file of one export.
type File struct {
	Name        string
	ContentType string
	Data        []byte
	// EntryFiles: entry ID (document key) → file it went into.
	EntryFiles map[string]string
}

// WriteFile writes the vouchers with the Luca driver's default (verified)
// fiş template — the same header row as the journal export and the
// expenses' file. More than one file (Luca: at most 50 vouchers per
// upload) comes as a zip.
func WriteFile(list List, period string, entries []ae.Entry) (File, error) {
	drv, err := ae.Open(luca.Name, ae.Config{})
	if err != nil {
		return File{}, err
	}
	mw := ae.NewMemoryWriter()
	res, err := drv.Export(context.Background(), ae.Batch{Period: period, Entries: entries}, mw)
	if err != nil {
		return File{}, err
	}
	if ae.HasBlocker(res.Issues) {
		return File{}, fmt.Errorf("%w: the Luca file cannot be written (%s)", ErrInvalid, res.Issues[0].Code)
	}
	prefix := list.filePrefix()
	files := map[string]string{}
	for id, f := range res.EntryFiles {
		files[id] = prefix + f
	}
	if len(res.Files) == 1 {
		data, _ := mw.File(res.Files[0].Name)
		return File{Name: prefix + res.Files[0].Name, ContentType: luca.ContentType, Data: data, EntryFiles: files}, nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range res.Files {
		data, _ := mw.File(f.Name)
		w, err := zw.Create(prefix + f.Name)
		if err != nil {
			return File{}, err
		}
		if _, err := w.Write(data); err != nil {
			return File{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return File{}, err
	}
	name := prefix + "luca_fis"
	if period != "" {
		name += "_" + period
	}
	return File{Name: name + ".zip", ContentType: "application/zip", Data: buf.Bytes(), EntryFiles: files}, nil
}
