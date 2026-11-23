// Package documents routes the files the desktop agent picks up from the
// watched folders (#780 e: Luca has no API). The müşavir exports the bordro
// dökümü and the mizan from Luca, or drops a foreign service invoice PDF,
// into a watched folder; the agent uploads it here. The file is recognised
// by its content, matched to a taxpayer by the VKN / TCKN in the file, the
// watched folder's engagement or the folder name, and handed to the import
// the office would use by hand (payroll import, account plan / trial
// balance import, foreign invoice intake). A file whose kind or taxpayer is
// unclear waits as an "eşleştir" item the office resolves.
package documents

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"

	accountplan "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/payroll"
)

// Kinds of an agent document.
const (
	KindUnknown        = "unknown"
	KindPayroll        = "payroll"       // Luca (or Zirve / Logo) bordro dökümü
	KindTrialBalance   = "trial_balance" // mizan Excel'i
	KindAccountPlan    = "account_plan"  // hesap planı dökümü (tutarsız)
	KindForeignInvoice = "foreign_invoice"
	// KindReceipt: a fiş / fatura / Z raporu photo; read by the same receipt
	// capture as the mobile app and the WhatsApp extension.
	KindReceipt = "receipt"
)

// ValidKind reports whether k is a kind the office may choose.
func ValidKind(k string) bool {
	switch k {
	case KindPayroll, KindTrialBalance, KindAccountPlan, KindForeignInvoice, KindReceipt:
		return true
	}
	return false
}

// Classified is what the content says about a file.
type Classified struct {
	Kind string
	// Period "YYYY-MM" read from the file ("" when it does not say).
	Period string
	// SourceSystem of a payroll dump (luca, zirve, logo, generic).
	SourceSystem string
	// TaxIDs are the 10 (VKN) or 11 (TCKN) digit numbers in the title rows
	// and the file name.
	TaxIDs []string
}

// minAccounts: fewer parsed codes than this is not a plan or a mizan.
const minAccounts = 3

// Classify recognises a file by its content. It never fails: an
// unrecognised file is KindUnknown.
func Classify(name string, data []byte) Classified {
	out := Classified{Kind: KindUnknown}
	if len(data) == 0 {
		return out
	}
	nameIDs := TaxIDsIn(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		out.Kind, out.TaxIDs = KindForeignInvoice, nameIDs
		return out
	}
	if Photo(data) {
		out.Kind, out.TaxIDs = KindReceipt, nameIDs
		return out
	}
	if !spreadsheet(name, data) {
		return out
	}
	// Bordro dökümü first: it needs a name, a gross and a net column, which
	// a mizan never has.
	if p, err := payroll.ReadFile(name, data); err == nil && len(payroll.Missing(p.Mapping)) == 0 {
		if lines, _, rowPeriod := payroll.Lines(p); len(lines) > 0 {
			out.Kind, out.SourceSystem = KindPayroll, p.SourceSystem
			out.Period = p.Period
			if out.Period == "" {
				out.Period = rowPeriod
			}
			head := p.Sheet.Rows
			if p.Layout.HeaderRow-1 < len(head) && p.Layout.HeaderRow > 0 {
				head = head[:p.Layout.HeaderRow-1]
			}
			out.TaxIDs = unique(append(rowsTaxIDs(head), nameIDs...))
			return out
		}
	}
	if p, err := accountplan.Parse(name, data); err == nil && p.HeaderRow > 0 && len(p.Accounts) >= minAccounts {
		out.Kind = p.Kind
		if p.Kind == KindTrialBalance {
			out.Period = p.Period
		}
		out.TaxIDs = unique(append(titleTaxIDs(name, data, p.HeaderRow), nameIDs...))
		return out
	}
	return out
}

// Photo reports whether data is a JPEG, PNG, WebP or HEIC photo (a fiş or a
// Z raporu shot with a phone or scanned to an image).
func Photo(data []byte) bool {
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1":
			return true
		}
	}
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/webp":
		return true
	}
	return false
}

// spreadsheet: an xlsx (zip), a csv or a legacy xls by name or magic.
func spreadsheet(name string, data []byte) bool {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".xls", ".csv", ".txt":
		return true
	}
	return false
}

// titleTaxIDs reads the rows above the header of the first sheet.
func titleTaxIDs(name string, data []byte, header int) []string {
	_, sheets, err := parse.Read(name, data)
	if err != nil || len(sheets) == 0 {
		return nil
	}
	rows := sheets[0].Rows
	if header-1 < len(rows) {
		rows = rows[:header-1]
	}
	return rowsTaxIDs(rows)
}

func rowsTaxIDs(rows [][]string) []string {
	var out []string
	for i, r := range rows {
		if i >= 30 {
			break
		}
		out = append(out, TaxIDsIn(strings.Join(r, " "))...)
	}
	return out
}

// TaxIDsIn returns the 10 or 11 digit numbers in s (VKN / TCKN candidates).
func TaxIDsIn(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
		if len(f) == 10 || len(f) == 11 {
			out = append(out, f)
		}
	}
	return unique(out)
}

func unique(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
