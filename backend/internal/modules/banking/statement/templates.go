package statement

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking"
)

// Statement templates (issue #383). A bank's "Excel'e aktar" or PDF output
// is recognised by its header row: the fingerprint is the SHA-256 of the
// ordered, normalized header names (the excelimport fingerprint idea, 04
// §4.2). A template names the field of every header. The big Turkish banks
// ship as built-in templates below; an office saves its own for any other
// layout after the analysis step suggests the columns (heuristics or AI).
// When neither a template nor the header synonyms find the table, the
// upload answers ErrLayoutUnknown and the panel offers the analysis.

// Field is the meaning of one statement column.
type Field string

const (
	FieldNone              Field = ""
	FieldDate              Field = "date"
	FieldValueDate         Field = "value_date"
	FieldTime              Field = "time"
	FieldDescription       Field = "description"
	FieldAmount            Field = "amount"
	FieldDebit             Field = "debit"
	FieldCredit            Field = "credit"
	FieldBalance           Field = "balance"
	FieldReceipt           Field = "receipt"
	FieldReference         Field = "reference"
	FieldChannel           Field = "channel"
	FieldCounterparty      Field = "counterparty"
	FieldCounterpartyIBAN  Field = "counterparty_iban"
	FieldCounterpartyTaxID Field = "counterparty_tax_id"
)

var fieldColumns = map[Field]column{
	FieldDate: colDate, FieldValueDate: colValueDate, FieldTime: colTime, FieldDescription: colDescription,
	FieldAmount: colAmount, FieldDebit: colDebit, FieldCredit: colCredit, FieldBalance: colBalance,
	FieldReceipt: colReceipt, FieldReference: colReference, FieldChannel: colChannel,
	FieldCounterparty: colCounterparty, FieldCounterpartyIBAN: colCounterpartyIBAN, FieldCounterpartyTaxID: colCounterpartyTaxID,
}

// Fields lists the fields in a stable order (the panel's picker).
var Fields = []Field{
	FieldDate, FieldTime, FieldValueDate, FieldDescription, FieldAmount, FieldDebit, FieldCredit, FieldBalance,
	FieldReceipt, FieldReference, FieldChannel, FieldCounterparty, FieldCounterpartyIBAN, FieldCounterpartyTaxID,
}

// Valid reports whether f is a known field (FieldNone included: ignored column).
func (f Field) Valid() bool {
	if f == FieldNone {
		return true
	}
	_, ok := fieldColumns[f]
	return ok
}

// Template sources.
const (
	SourceBuiltin  = "builtin"
	SourceOffice   = "office"
	SourceSynonyms = "synonyms"
)

// TemplateColumn is one header cell and its field.
type TemplateColumn struct {
	Header string `json:"header"`
	Field  Field  `json:"field"`
}

// Template is a known statement layout.
type Template struct {
	// ID is the bank_statement_templates row (office templates only).
	ID     int64
	BankID string
	Name   string
	// Format limits the template to excel, csv or pdf; empty: any table.
	Format  string
	Columns []TemplateColumn
	Source  string
}

// Fingerprint of the template's header row.
func (t Template) Fingerprint() string {
	hs := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		hs[i] = c.Header
	}
	return Fingerprint(hs)
}

// TemplateMatch says how a statement's columns were found.
type TemplateMatch struct {
	Source      string `json:"source"`
	Name        string `json:"name,omitempty"`
	BankID      string `json:"bank_id,omitempty"`
	Fingerprint string `json:"fingerprint"`
	ID          int64  `json:"-"`
}

// ErrLayoutUnknown: no template and no header synonyms found the movement
// table. The panel analyses the file and the accountant saves a template.
var ErrLayoutUnknown = errors.New("statement: the column layout is not known; define a template")

// ErrInvalidTemplate: a template without date and amount columns, an
// unknown field or a field used twice.
var ErrInvalidTemplate = errors.New("statement: invalid template")

// Fingerprint hashes the ordered, normalized, non-empty header names.
func Fingerprint(headers []string) string {
	parts := make([]string, 0, len(headers))
	for _, h := range headers {
		if n := banking.NormalizeDescription(h); n != "" {
			parts = append(parts, n)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// Validate checks a template's columns.
func (t Template) Validate() error {
	seen := map[Field]bool{}
	headers := 0
	for i, c := range t.Columns {
		if strings.TrimSpace(c.Header) == "" || banking.NormalizeDescription(c.Header) == "" {
			return fmt.Errorf("%w: column %d has no header", ErrInvalidTemplate, i+1)
		}
		headers++
		if !c.Field.Valid() {
			return fmt.Errorf("%w: unknown field %q", ErrInvalidTemplate, c.Field)
		}
		if c.Field != FieldNone && seen[c.Field] {
			return fmt.Errorf("%w: field %q is used twice", ErrInvalidTemplate, c.Field)
		}
		seen[c.Field] = true
	}
	if headers < 2 {
		return fmt.Errorf("%w: at least two columns are needed", ErrInvalidTemplate)
	}
	hasAmount := seen[FieldAmount] || (seen[FieldDebit] && seen[FieldCredit])
	if !seen[FieldDate] || !hasAmount {
		return fmt.Errorf("%w: date and amount (or debit and credit) columns are required", ErrInvalidTemplate)
	}
	switch t.Format {
	case "", "excel", "csv", "pdf":
	default:
		return fmt.Errorf("%w: format %q", ErrInvalidTemplate, t.Format)
	}
	return nil
}

// matchTemplate maps a header row to columns when its fingerprint is a
// template's. Office templates come first, then the built-in ones.
func matchTemplate(row []string, format string, tpls []Template) (map[column]int, *TemplateMatch) {
	var idx []int
	var headers []string
	for j, c := range row {
		if banking.NormalizeDescription(c) != "" {
			idx = append(idx, j)
			headers = append(headers, c)
		}
	}
	if len(headers) < 2 {
		return nil, nil
	}
	fp := Fingerprint(headers)
	for _, set := range [][]Template{tpls, builtinTemplates} {
		for _, t := range set {
			if t.Format != "" && t.Format != format {
				continue
			}
			if len(t.Columns) != len(headers) || t.Fingerprint() != fp {
				continue
			}
			cols := map[column]int{}
			for k, c := range t.Columns {
				if c.Field == FieldNone {
					continue
				}
				cols[fieldColumns[c.Field]] = idx[k]
			}
			src := t.Source
			if src == "" {
				src = SourceBuiltin
			}
			return cols, &TemplateMatch{Source: src, Name: t.Name, BankID: t.BankID, Fingerprint: fp, ID: t.ID}
		}
	}
	return nil, nil
}

// BuiltinTemplates returns the start templates (a copy).
func BuiltinTemplates() []Template {
	return append([]Template(nil), builtinTemplates...)
}

func cols(pairs ...string) []TemplateColumn {
	out := make([]TemplateColumn, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, TemplateColumn{Header: pairs[i], Field: Field(pairs[i+1])})
	}
	return out
}

// builtinTemplates: the "Excel'e aktar" / PDF header rows of the big
// Turkish banks' internet branch (corporate) as documented in their help
// pages and seen in sample files. ⚠️ Until masked real statements arrive
// (docs/00 M6) they are reference layouts; a bank that changes its header
// simply falls back to the synonyms or the analysis step, and the office
// saves the new layout as its own template.
var builtinTemplates = []Template{
	{BankID: "ziraat", Name: "Ziraat Bankası hesap hareketleri", Columns: cols(
		"Tarih", "date", "Fiş No", "receipt", "Açıklama", "description", "İşlem Tutarı", "amount", "Bakiye", "balance")},
	{BankID: "isbank", Name: "Türkiye İş Bankası hesap hareketleri", Columns: cols(
		"Tarih/Saat", "date", "Kanal", "channel", "Açıklama", "description", "İşlem Tutarı", "amount", "Bakiye", "balance", "Ek Bilgi", "")},
	{BankID: "garanti", Name: "Garanti BBVA hesap hareketleri", Columns: cols(
		"Tarih", "date", "Açıklama", "description", "Etiket", "", "Tutar", "amount", "Bakiye", "balance", "Dekont No", "receipt")},
	{BankID: "yapikredi", Name: "Yapı Kredi hesap hareketleri", Columns: cols(
		"Tarih", "date", "Saat", "time", "İşlem", "", "Kanal", "channel", "Açıklama", "description", "İşlem Tutarı", "amount", "Bakiye", "balance")},
	{BankID: "akbank", Name: "Akbank hesap hareketleri", Columns: cols(
		"Tarih", "date", "Saat", "time", "Tutar", "amount", "Bakiye", "balance", "Açıklama", "description", "Fiş/Dekont No", "receipt")},
	{BankID: "vakifbank", Name: "VakıfBank hesap hareketleri", Columns: cols(
		"Tarih", "date", "Saat", "time", "İşlem No", "reference", "Tutar", "amount", "Bakiye", "balance", "İşlem Adı", "channel", "Açıklama", "description")},
	{BankID: "halkbank", Name: "Halkbank hesap ekstresi", Columns: cols(
		"İşlem Tarihi", "date", "Valör", "value_date", "Açıklama", "description", "Borç", "debit", "Alacak", "credit", "Bakiye", "balance", "Referans", "reference")},
	{BankID: "qnb", Name: "QNB hesap hareketleri", Columns: cols(
		"İşlem Tarihi", "date", "Valör Tarihi", "value_date", "Açıklama", "description", "Karşı Taraf IBAN", "counterparty_iban", "İşlem Tutarı", "amount", "Bakiye", "balance", "Dekont No", "receipt")},
	// Enpara.com is QNB's digital bank: same IBAN bank code (00111).
	{BankID: "qnb", Name: "Enpara.com hesap hareketleri", Columns: cols(
		"Tarih", "date", "Hareket Tipi", "", "Açıklama", "description", "İşlem Tutarı", "amount", "Bakiye", "balance")},
	{BankID: "denizbank", Name: "DenizBank hesap hareketleri", Columns: cols(
		"Tarih", "date", "Açıklama", "description", "Tutar", "amount", "Bakiye", "balance", "Referans No", "reference", "Karşı Taraf", "counterparty")},
	{BankID: "teb", Name: "TEB hesap ekstresi", Columns: cols(
		"Tarih", "date", "Valör", "value_date", "Açıklama", "description", "Borç", "debit", "Alacak", "credit", "Bakiye", "balance", "Dekont No", "receipt")},
	{BankID: "kuveyt_turk", Name: "Kuveyt Türk hesap hareketleri", Columns: cols(
		"Tarih", "date", "Açıklama", "description", "Tutar", "amount", "Bakiye", "balance", "İşlem Kanalı", "channel", "Fiş No", "receipt", "Gönderen/Alıcı VKN", "counterparty_tax_id")},
}

// bankNames finds the bank from the header area's text when the file names
// no IBAN (normalized text, longest names first within a bank).
var bankNames = []struct{ text, bank string }{
	{"ENPARA", "qnb"},
	{"QNB", "qnb"},
	{"FINANSBANK", "qnb"},
	{"ZIRAAT KATILIM", "ziraat_katilim"},
	{"VAKIF KATILIM", "vakif_katilim"},
	{"ZIRAAT", "ziraat"},
	{"IS BANKASI", "isbank"},
	{"ISBANK", "isbank"},
	{"GARANTI", "garanti"},
	{"YAPI KREDI", "yapikredi"},
	{"YAPI VE KREDI", "yapikredi"},
	{"AKBANK", "akbank"},
	{"VAKIFBANK", "vakifbank"},
	{"VAKIFLAR BANKASI", "vakifbank"},
	{"HALKBANK", "halkbank"},
	{"HALK BANKASI", "halkbank"},
	{"DENIZBANK", "denizbank"},
	{"TURK EKONOMI BANKASI", "teb"},
	{"KUVEYT TURK", "kuveyt_turk"},
}

// bankFromText names the bank written in the header rows ("" unknown).
func bankFromText(rows [][]string, upTo int) string {
	for i := 0; i < len(rows) && i < upTo; i++ {
		n := " " + banking.NormalizeDescription(strings.Join(rows[i], " ")) + " "
		for _, b := range bankNames {
			if strings.Contains(n, " "+b.text+" ") || strings.Contains(n, " "+b.text) {
				return b.bank
			}
		}
		if strings.Contains(n, " TEB ") {
			return "teb"
		}
	}
	return ""
}
