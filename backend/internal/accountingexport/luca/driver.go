// Package luca is the "luca" accounting export driver (issue #29, docs/02 §1,
// §3): Luca "Excel Veri Aktarımı" muhasebe fişi files.
//
// The layout (headers, order, date and decimal format, voucher type values)
// is data in templates/fis_<version>.yaml, never code. The default
// "verified-1" follows the header row of Luca's own fiş Excel (15 columns,
// Fiş No … TC/Vergi No); the older "unverified-1" / "unverified-2" layouts
// stay selectable by version.
//
// Rules from the Luca help pages: rows with the same Fiş No and Fiş Tarihi
// form one voucher; at most 50 vouchers per upload; at most 400 lines per
// mahsup voucher. Bigger batches become several files, longer mahsup vouchers
// become several balanced vouchers.
package luca

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

// Name is the driver name.
const Name = "luca"

// DefaultTemplate is used when Config.Template is empty.
const DefaultTemplate = "verified-1"

// ContentType of the produced files.
const ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

//go:embed templates/*.yaml
var templates embed.FS

func init() {
	ae.Register(Name, func(cfg ae.Config) (ae.Driver, error) {
		v := cfg.Template
		if v == "" {
			v = DefaultTemplate
		}
		t, err := LoadTemplate(templates, v)
		if err != nil {
			return nil, err
		}
		return New(t)
	})
}

// LoadTemplate reads templates/fis_<version>.yaml from fsys.
func LoadTemplate(fsys fs.FS, version string) (ae.Template, error) {
	return ae.LoadTemplate(fsys, fmt.Sprintf("templates/fis_%s.yaml", version))
}

// Versions lists the embedded template versions.
func Versions() []string {
	matches, _ := fs.Glob(templates, "templates/fis_*.yaml")
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(m, "templates/fis_"), ".yaml"))
	}
	sort.Strings(out)
	return out
}

// Column keys a template may use.
var knownKeys = map[string]bool{
	"entry_no": true, "entry_date": true, "account": true, "debit": true, "credit": true,
	"description": true, "entry_description": true, "entry_type": true, "document_type": true,
	"doc_no": true, "doc_date": true, "quantity": true, "currency": true, "exchange_rate": true,
	"foreign_amount": true, "source_id": true, "ettn": true, "description_source": true, "tax_id": true,
}

// New builds a driver from a template (tests, or a template loaded elsewhere).
func New(t ae.Template) (ae.Driver, error) {
	if t.Driver != Name {
		return nil, fmt.Errorf("%w: template is for driver %q", ae.ErrTemplate, t.Driver)
	}
	if t.File.Format != "xlsx" {
		return nil, fmt.Errorf("%w: file format %q is not supported yet", ae.ErrTemplate, t.File.Format)
	}
	keys := map[string]bool{}
	for _, c := range t.Columns {
		if !knownKeys[c.Key] {
			return nil, fmt.Errorf("%w: unknown column key %q", ae.ErrTemplate, c.Key)
		}
		keys[c.Key] = true
	}
	for _, k := range []string{"entry_no", "entry_date", "account", "debit", "credit"} {
		if !keys[k] {
			return nil, fmt.Errorf("%w: column %q is required by Luca", ae.ErrTemplate, k)
		}
	}
	return &driver{t: t}, nil
}

type driver struct{ t ae.Template }

func (d *driver) Info() ae.DriverInfo {
	return ae.DriverInfo{
		Name: Name, TemplateVer: d.t.Version, Limits: d.t.Limits, EntryTypes: d.t.EntryTypes,
		FileTypes: []string{d.t.File.Format}, Verified: d.t.Verified,
	}
}

// Validate adds the Luca rules to the common checks: the voucher type must be
// in the template, and a mahsup voucher longer than the limit must split into
// balanced vouchers (otherwise blocker).
func (d *driver) Validate(_ context.Context, b ae.Batch) []ae.Issue {
	issues := ae.CommonIssues(b)
	if !d.t.Verified {
		issues = append(issues, ae.Issue{Code: ae.IssueTemplateUnverified, Severity: ae.SeverityWarning,
			Params: map[string]any{"driver": Name, "template": d.t.Version}})
	}
	entries, _ := ae.SelectEntries(b)
	for _, e := range entries {
		if _, ok := d.t.EntryTypes[e.Type]; !ok {
			issues = append(issues, ae.Issue{Code: ae.IssueUnsupportedType, Severity: ae.SeverityBlocker, EntryNo: e.No,
				Params: map[string]any{"entry_type": string(e.Type)}})
			continue
		}
		if e.Type == ae.EntryMahsup && d.t.Limits.MaxLinesPerEntry > 0 && len(e.Lines) > d.t.Limits.MaxLinesPerEntry {
			if _, ok := ae.SplitEntry(e, d.t.Limits.MaxLinesPerEntry); !ok {
				issues = append(issues, ae.Issue{Code: ae.IssueSplitImpossible, Severity: ae.SeverityBlocker, EntryNo: e.No,
					Params: map[string]any{"lines": len(e.Lines), "max_lines": d.t.Limits.MaxLinesPerEntry}})
			}
		}
	}
	return issues
}

func (d *driver) Export(ctx context.Context, b ae.Batch, w ae.FileWriter) (ae.Result, error) {
	issues := d.Validate(ctx, b)
	res := ae.Result{Issues: issues}
	if ae.HasBlocker(issues) {
		return res, nil
	}
	entries, _ := ae.SelectEntries(b)
	var vouchers []ae.Entry
	for _, e := range entries {
		pieces := []ae.Entry{e}
		if e.Type == ae.EntryMahsup {
			pieces, _ = ae.SplitEntry(e, d.t.Limits.MaxLinesPerEntry) // checked in Validate
		}
		vouchers = append(vouchers, pieces...)
		res.EntryIDs = append(res.EntryIDs, e.ID)
	}
	groups := ae.SplitIntoFiles(vouchers, d.t.Limits.MaxEntriesPerFile)
	for i, g := range groups {
		data, err := d.workbook(g)
		if err != nil {
			return res, err
		}
		name := fileName(b.Period, i+1, len(groups))
		f, err := ae.WriteFile(w, name, ContentType, data, len(g))
		if err != nil {
			return res, err
		}
		res.Files = append(res.Files, f)
		for _, v := range g {
			res.AddEntryFile(v.ID, f.Name)
		}
	}
	return res, nil
}

func fileName(period string, n, total int) string {
	base := "luca_fis"
	if period != "" {
		base += "_" + period
	}
	if total > 1 {
		return fmt.Sprintf("%s_%d.xlsx", base, n)
	}
	return base + ".xlsx"
}

func (d *driver) workbook(vouchers []ae.Entry) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := "Sheet1"
	if d.t.File.Sheet != "" {
		if err := f.SetSheetName(sheet, d.t.File.Sheet); err != nil {
			return nil, err
		}
		sheet = d.t.File.Sheet
	}
	moneyFmt := "0.00"
	money, err := f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt})
	if err != nil {
		return nil, err
	}
	dateFmt := "dd.mm.yyyy"
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt})
	if err != nil {
		return nil, err
	}
	hr := d.t.File.HeaderRow
	for i, c := range d.t.Columns {
		ref, _ := excelize.CoordinatesToCellName(i+1, hr)
		if err := f.SetCellStr(sheet, ref, c.Header); err != nil {
			return nil, err
		}
	}
	row := hr + 1
	for _, e := range vouchers {
		for _, l := range e.Lines {
			for i, c := range d.t.Columns {
				ref, _ := excelize.CoordinatesToCellName(i+1, row)
				if err := d.writeCell(f, sheet, ref, c, e, l, money, dateStyle); err != nil {
					return nil, err
				}
			}
			row++
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *driver) writeCell(f *excelize.File, sheet, ref string, c ae.Column, e ae.Entry, l ae.Line, money, dateStyle int) error {
	switch c.Key {
	case "entry_no":
		return f.SetCellStr(sheet, ref, e.No)
	case "entry_date":
		return writeDate(f, sheet, ref, c, e.Date, dateStyle)
	case "doc_date":
		if l.DocDate == nil {
			return nil
		}
		return writeDate(f, sheet, ref, c, *l.DocDate, dateStyle)
	case "account":
		return f.SetCellStr(sheet, ref, l.Account)
	case "debit":
		return writeAmount(f, sheet, ref, c, l.Debit, money)
	case "credit":
		return writeAmount(f, sheet, ref, c, l.Credit, money)
	case "quantity", "exchange_rate", "foreign_amount":
		v := map[string]*decimal.Decimal{"quantity": l.Quantity, "exchange_rate": l.ExchangeRate, "foreign_amount": l.ForeignAmount}[c.Key]
		if v == nil {
			return nil
		}
		return writeAmount(f, sheet, ref, c, *v, money)
	}
	if c.Key == "description_source" {
		val := sourceDescription(c.Pattern, firstNonEmpty(l.Description, e.Description), l.Source.ETTN)
		if val == "" {
			return nil
		}
		return f.SetCellStr(sheet, ref, val)
	}
	val := map[string]string{
		"description":       firstNonEmpty(l.Description, e.Description),
		"ettn":              l.Source.ETTN,
		"entry_description": e.Description,
		"entry_type":        d.t.EntryTypes[e.Type],
		"document_type":     e.DocumentType,
		"doc_no":            l.DocNo,
		"currency":          l.Currency,
		"source_id":         l.Source.ID,
		"tax_id":            l.TaxID,
	}[c.Key]
	if val == "" {
		return nil
	}
	return f.SetCellStr(sheet, ref, val)
}

// writeDate writes an Excel date cell, or text in Column.Format when the
// template asks for one (Go layout, e.g. "02.01.2006").
func writeDate(f *excelize.File, sheet, ref string, c ae.Column, t time.Time, style int) error {
	if c.Format != "" {
		return f.SetCellStr(sheet, ref, t.Format(c.Format))
	}
	if err := f.SetCellValue(sheet, ref, t); err != nil {
		return err
	}
	return f.SetCellStyle(sheet, ref, ref, style)
}

// writeAmount writes a numeric cell, or text with the template's decimal
// separator when Column.Decimal is set.
func writeAmount(f *excelize.File, sheet, ref string, c ae.Column, v decimal.Decimal, style int) error {
	if c.Decimal != "" {
		return f.SetCellStr(sheet, ref, strings.Replace(v.StringFixed(2), ".", c.Decimal, 1))
	}
	if err := f.SetCellFloat(sheet, ref, v.InexactFloat64(), -1, 64); err != nil {
		return err
	}
	return f.SetCellStyle(sheet, ref, ref, style)
}

// sourceDescription fills a description_source pattern. Placeholders:
// {description}, {ettn}. A segment whose placeholder is empty is dropped
// with the literal text that leads it ("{description} ETTN:{ettn}" with no
// ETTN gives just the description). Empty pattern: "{description} ETTN:{ettn}".
func sourceDescription(pattern, description, ettn string) string {
	if pattern == "" {
		pattern = "{description} ETTN:{ettn}"
	}
	vals := map[string]string{"description": description, "ettn": ettn}
	var out strings.Builder
	rest := pattern
	for rest != "" {
		open := strings.Index(rest, "{")
		if open < 0 {
			out.WriteString(rest)
			break
		}
		end := strings.Index(rest[open:], "}")
		if end < 0 {
			out.WriteString(rest)
			break
		}
		lead, key := rest[:open], rest[open+1:open+end]
		rest = rest[open+end+1:]
		v, known := vals[key]
		if !known {
			out.WriteString(lead + "{" + key + "}")
			continue
		}
		if v == "" {
			continue
		}
		if out.Len() == 0 {
			lead = strings.TrimLeft(lead, " ")
		}
		out.WriteString(lead + v)
	}
	return strings.TrimSpace(out.String())
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
