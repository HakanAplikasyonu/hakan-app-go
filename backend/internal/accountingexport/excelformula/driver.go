// Package excelformula is the "excel_formula" accounting export driver
// (issue #28, docs/02 §3.4): a control and working workbook the accountant can
// check and correct in Excel or LibreOffice. Totals are formulas (SUMIFS),
// never precomputed values, so edits recalculate.
//
// Import for its side effect:
//
//	import _ ".../accountingexport/excelformula"
package excelformula

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

// Name is the driver name.
const Name = "excel_formula"

// Sheet names (docs/02 §3.4).
const (
	SheetEntries  = "Fişler"
	SheetSummary  = "Fiş Özeti"
	SheetTrial    = "Mizan (dönem)"
	SheetVAT      = "KDV"
	SheetWarnings = "Uyarılar"
	SheetSettings = "Ayarlar"
)

// Tolerance is the balance tolerance written to Ayarlar and used by the
// formulas (docs/02 §3.4: 0,005).
const Tolerance = "0.005"

// ContentType of the produced workbook.
const ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func init() {
	ae.Register(Name, func(ae.Config) (ae.Driver, error) { return driver{}, nil })
}

type driver struct{}

func (driver) Info() ae.DriverInfo {
	return ae.DriverInfo{
		Name: Name, TemplateVer: "1", Verified: true, FileTypes: []string{"xlsx"},
		EntryTypes: map[ae.EntryType]string{ae.EntryMahsup: "Mahsup", ae.EntryAcilis: "Açılış", ae.EntryKapanis: "Kapanış"},
	}
}

// Validate runs the common checks but never blocks: this workbook exists to
// show problems (an unbalanced entry is painted red), not to feed a program.
func (driver) Validate(_ context.Context, b ae.Batch) []ae.Issue {
	issues := ae.CommonIssues(b)
	for i := range issues {
		if issues[i].Severity == ae.SeverityBlocker {
			issues[i].Severity = ae.SeverityWarning
		}
	}
	return issues
}

func (d driver) Export(ctx context.Context, b ae.Batch, w ae.FileWriter) (ae.Result, error) {
	entries, excluded := ae.SelectEntries(b)
	res := ae.Result{Issues: append(d.Validate(ctx, b), excluded...)}
	if len(entries) == 0 {
		return res, nil
	}
	f, err := build(b, entries)
	if err != nil {
		return res, err
	}
	defer func() { _ = f.Close() }()
	buf, err := f.WriteToBuffer()
	if err != nil {
		return res, err
	}
	name := "kontrol.xlsx"
	if b.Period != "" {
		name = fmt.Sprintf("kontrol_%s.xlsx", b.Period)
	}
	file, err := ae.WriteFile(w, name, ContentType, buf.Bytes(), len(entries))
	if err != nil {
		return res, err
	}
	res.Files = []ae.File{file}
	for _, e := range entries {
		res.EntryIDs = append(res.EntryIDs, e.ID)
	}
	return res, nil
}

type styles struct {
	header, money, moneyBold, date, bold, red, pct int
}

func newStyles(f *excelize.File) (styles, error) {
	var s styles
	var err error
	moneyFmt := "#,##0.00"
	if s.header, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2E8F0"}}}); err != nil {
		return s, err
	}
	if s.money, err = f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt}); err != nil {
		return s, err
	}
	if s.moneyBold, err = f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt, Font: &excelize.Font{Bold: true}}); err != nil {
		return s, err
	}
	dateFmt := "dd.mm.yyyy"
	if s.date, err = f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt}); err != nil {
		return s, err
	}
	if s.bold, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err != nil {
		return s, err
	}
	pctFmt := "0.00"
	if s.pct, err = f.NewStyle(&excelize.Style{CustomNumFmt: &pctFmt}); err != nil {
		return s, err
	}
	s.red, err = f.NewConditionalStyle(&excelize.Style{Font: &excelize.Font{Color: "9C0006", Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFC7CE"}}})
	return s, err
}

// tol is the absolute reference to the tolerance cell.
var tol = fmt.Sprintf("'%s'!$B$2", SheetSettings)

func build(b ae.Batch, entries []ae.Entry) (*excelize.File, error) {
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", SheetEntries); err != nil {
		return nil, err
	}
	for _, s := range []string{SheetSummary, SheetTrial, SheetVAT, SheetWarnings, SheetSettings} {
		if _, err := f.NewSheet(s); err != nil {
			return nil, err
		}
	}
	st, err := newStyles(f)
	if err != nil {
		return nil, err
	}
	if err := writeSettings(f, st, b, entries); err != nil {
		return nil, err
	}
	layout, err := writeEntries(f, st, entries)
	if err != nil {
		return nil, err
	}
	steps := []func() error{
		func() error { return writeSummary(f, st, layout, entries) },
		func() error { return writeTrial(f, st, layout, entries) },
		func() error { return writeVAT(f, st, layout, b) },
		func() error { return writeWarnings(f, st, b) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	f.SetActiveSheet(0)
	return f, nil
}

func cell(col string, row int) string { return fmt.Sprintf("%s%d", col, row) }

func header(f *excelize.File, st styles, sheet string, cols []string, widths []float64) error {
	for i, h := range cols {
		c, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr(sheet, c, h); err != nil {
			return err
		}
		if i < len(widths) {
			col, _ := excelize.ColumnNumberToName(i + 1)
			if err := f.SetColWidth(sheet, col, col, widths[i]); err != nil {
				return err
			}
		}
	}
	last, _ := excelize.CoordinatesToCellName(len(cols), 1)
	if err := f.SetCellStyle(sheet, "A1", last, st.header); err != nil {
		return err
	}
	return f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}

func money(f *excelize.File, st styles, sheet, c string, d decimal.Decimal) error {
	if err := f.SetCellFloat(sheet, c, d.InexactFloat64(), -1, 64); err != nil {
		return err
	}
	return f.SetCellStyle(sheet, c, c, st.money)
}

func formula(f *excelize.File, sheet, c, fx string, style int) error {
	if err := f.SetCellFormula(sheet, c, fx); err != nil {
		return err
	}
	return f.SetCellStyle(sheet, c, c, style)
}

// entriesLayout tells the other sheets where the line rows of Fişler are.
type entriesLayout struct{ first, last int }

// Fişler columns: A Fiş No, B Tarih, C Fiş Tipi, D Hesap Kodu, E Açıklama,
// F Borç, G Alacak, H Fark, I Kaynak Belge, J Güven, K (fiş anahtarı, toplam satırı).
func writeEntries(f *excelize.File, st styles, entries []ae.Entry) (entriesLayout, error) {
	s := SheetEntries
	if err := header(f, st, s, []string{"Fiş No", "Tarih", "Fiş Tipi", "Hesap Kodu", "Açıklama", "Borç", "Alacak", "Fark", "Kaynak Belge", "Güven", "Fiş"},
		[]float64{12, 12, 10, 16, 40, 16, 16, 14, 30, 8, 8}); err != nil {
		return entriesLayout{}, err
	}
	row := 2
	type block struct {
		no    string
		total int
	}
	var blocks []block
	for _, e := range entries {
		for _, l := range e.Lines {
			vals := map[string]string{"A": e.No, "C": entryTypeLabel(e.Type), "D": l.Account, "E": firstNonEmpty(l.Description, e.Description)}
			for col, v := range vals {
				if err := f.SetCellStr(s, cell(col, row), v); err != nil {
					return entriesLayout{}, err
				}
			}
			if err := f.SetCellValue(s, cell("B", row), e.Date); err != nil {
				return entriesLayout{}, err
			}
			if err := f.SetCellStyle(s, cell("B", row), cell("B", row), st.date); err != nil {
				return entriesLayout{}, err
			}
			if err := money(f, st, s, cell("F", row), l.Debit); err != nil {
				return entriesLayout{}, err
			}
			if err := money(f, st, s, cell("G", row), l.Credit); err != nil {
				return entriesLayout{}, err
			}
			if err := sourceCell(f, s, cell("I", row), l.Source); err != nil {
				return entriesLayout{}, err
			}
			if l.Confidence != nil {
				if err := f.SetCellFloat(s, cell("J", row), l.Confidence.InexactFloat64(), -1, 64); err != nil {
					return entriesLayout{}, err
				}
			}
			row++
		}
		blocks = append(blocks, block{no: e.No, total: row})
		row++ // the total row
	}
	last := row - 1
	layout := entriesLayout{first: 2, last: last}
	rng := func(col string) string { return fmt.Sprintf("$%s$2:$%s$%d", col, col, last) }
	for _, bl := range blocks {
		r := bl.total
		if err := f.SetCellStr(s, cell("K", r), bl.no); err != nil {
			return entriesLayout{}, err
		}
		if err := f.SetCellStr(s, cell("E", r), "Fiş "+bl.no+" toplamı"); err != nil {
			return entriesLayout{}, err
		}
		if err := f.SetCellStyle(s, cell("E", r), cell("E", r), st.bold); err != nil {
			return entriesLayout{}, err
		}
		if err := formula(f, s, cell("F", r), fmt.Sprintf("SUMIFS(%s,%s,K%d)", rng("F"), rng("A"), r), st.moneyBold); err != nil {
			return entriesLayout{}, err
		}
		if err := formula(f, s, cell("G", r), fmt.Sprintf("SUMIFS(%s,%s,K%d)", rng("G"), rng("A"), r), st.moneyBold); err != nil {
			return entriesLayout{}, err
		}
		if err := formula(f, s, cell("H", r), fmt.Sprintf("F%d-G%d", r, r), st.moneyBold); err != nil {
			return entriesLayout{}, err
		}
		// Red when |fark| ≥ tolerance.
		if err := f.SetConditionalFormat(s, cell("H", r), []excelize.ConditionalFormatOptions{{
			Type: "formula", Criteria: "=", Format: &st.red, Value: fmt.Sprintf("ABS($H$%d)>=%s", r, tol),
		}}); err != nil {
			return entriesLayout{}, err
		}
	}
	return layout, f.SetColVisible(s, "K", false)
}

func sourceCell(f *excelize.File, sheet, c string, src ae.SourceRef) error {
	if src.ID == "" {
		return nil
	}
	label := src.Kind + ": " + src.ID
	if err := f.SetCellStr(sheet, c, label); err != nil {
		return err
	}
	if src.URL != "" {
		return f.SetCellHyperLink(sheet, c, src.URL, "External")
	}
	return nil
}

// Fiş Özeti: A Fiş No, B Tarih, C Fiş Tipi, D Açıklama, E Borç, F Alacak, G Fark, H Durum.
func writeSummary(f *excelize.File, st styles, layout entriesLayout, entries []ae.Entry) error {
	s := SheetSummary
	if err := header(f, st, s, []string{"Fiş No", "Tarih", "Fiş Tipi", "Açıklama", "Borç", "Alacak", "Fark", "Durum"},
		[]float64{12, 12, 10, 40, 16, 16, 14, 12}); err != nil {
		return err
	}
	ref := func(col string) string {
		return fmt.Sprintf("'%s'!$%s$%d:$%s$%d", SheetEntries, col, layout.first, col, layout.last)
	}
	for i, e := range entries {
		r := i + 2
		if err := f.SetCellStr(s, cell("A", r), e.No); err != nil {
			return err
		}
		if err := f.SetCellValue(s, cell("B", r), e.Date); err != nil {
			return err
		}
		if err := f.SetCellStyle(s, cell("B", r), cell("B", r), st.date); err != nil {
			return err
		}
		if err := f.SetCellStr(s, cell("C", r), entryTypeLabel(e.Type)); err != nil {
			return err
		}
		if err := f.SetCellStr(s, cell("D", r), e.Description); err != nil {
			return err
		}
		if err := formula(f, s, cell("E", r), fmt.Sprintf("SUMIFS(%s,%s,A%d)", ref("F"), ref("A"), r), st.money); err != nil {
			return err
		}
		if err := formula(f, s, cell("F", r), fmt.Sprintf("SUMIFS(%s,%s,A%d)", ref("G"), ref("A"), r), st.money); err != nil {
			return err
		}
		if err := formula(f, s, cell("G", r), fmt.Sprintf("E%d-F%d", r, r), st.money); err != nil {
			return err
		}
		if err := formula(f, s, cell("H", r), fmt.Sprintf(`IF(ABS(G%d)<%s,"Dengeli","DENGESİZ")`, r, tol), 0); err != nil {
			return err
		}
	}
	if len(entries) == 0 {
		return nil
	}
	last := len(entries) + 1
	return f.SetConditionalFormat(s, fmt.Sprintf("G2:H%d", last), []excelize.ConditionalFormatOptions{{
		Type: "formula", Criteria: "=", Format: &st.red, Value: fmt.Sprintf("ABS($G2)>=%s", tol),
	}})
}

// Mizan: A Hesap Kodu, B Ana Hesap, C Borç, D Alacak, E Bakiye; below, ana
// hesap (ilk 3 hane) ara toplamları.
func writeTrial(f *excelize.File, st styles, layout entriesLayout, entries []ae.Entry) error {
	s := SheetTrial
	if err := header(f, st, s, []string{"Hesap Kodu", "Ana Hesap", "Borç", "Alacak", "Bakiye"}, []float64{18, 12, 16, 16, 16}); err != nil {
		return err
	}
	set := map[string]bool{}
	for _, e := range entries {
		for _, l := range e.Lines {
			set[l.Account] = true
		}
	}
	accounts := make([]string, 0, len(set))
	for a := range set {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)
	ref := func(col string) string {
		return fmt.Sprintf("'%s'!$%s$%d:$%s$%d", SheetEntries, col, layout.first, col, layout.last)
	}
	mains := map[string]bool{}
	for i, a := range accounts {
		r := i + 2
		main := mainAccount(a)
		mains[main] = true
		if err := f.SetCellStr(s, cell("A", r), a); err != nil {
			return err
		}
		if err := f.SetCellStr(s, cell("B", r), main); err != nil {
			return err
		}
		if err := formula(f, s, cell("C", r), fmt.Sprintf("SUMIFS(%s,%s,A%d)", ref("F"), ref("D"), r), st.money); err != nil {
			return err
		}
		if err := formula(f, s, cell("D", r), fmt.Sprintf("SUMIFS(%s,%s,A%d)", ref("G"), ref("D"), r), st.money); err != nil {
			return err
		}
		if err := formula(f, s, cell("E", r), fmt.Sprintf("C%d-D%d", r, r), st.money); err != nil {
			return err
		}
	}
	detailLast := len(accounts) + 1
	r := detailLast + 2
	if err := f.SetCellStr(s, cell("A", r), "Ana hesap ara toplamları"); err != nil {
		return err
	}
	if err := f.SetCellStyle(s, cell("A", r), cell("A", r), st.bold); err != nil {
		return err
	}
	mainList := make([]string, 0, len(mains))
	for m := range mains {
		mainList = append(mainList, m)
	}
	sort.Strings(mainList)
	rng := func(col string) string { return fmt.Sprintf("$%s$2:$%s$%d", col, col, detailLast) }
	for _, m := range mainList {
		r++
		if err := f.SetCellStr(s, cell("B", r), m); err != nil {
			return err
		}
		if err := formula(f, s, cell("C", r), fmt.Sprintf("SUMIFS(%s,%s,B%d)", rng("C"), rng("B"), r), st.moneyBold); err != nil {
			return err
		}
		if err := formula(f, s, cell("D", r), fmt.Sprintf("SUMIFS(%s,%s,B%d)", rng("D"), rng("B"), r), st.moneyBold); err != nil {
			return err
		}
		if err := formula(f, s, cell("E", r), fmt.Sprintf("C%d-D%d", r, r), st.moneyBold); err != nil {
			return err
		}
	}
	r += 2
	if err := f.SetCellStr(s, cell("A", r), "Genel toplam"); err != nil {
		return err
	}
	for _, col := range []string{"C", "D", "E"} {
		if err := formula(f, s, cell(col, r), fmt.Sprintf("SUM(%s)", rng(col)), st.moneyBold); err != nil {
			return err
		}
	}
	return nil
}

// KDV: the portal's own summary per rate next to what the entries post to 391
// (hesaplanan) and 191 (indirilecek), with differences and ödenecek /
// devreden KDV.
func writeVAT(f *excelize.File, st styles, layout entriesLayout, b ae.Batch) error {
	s := SheetVAT
	if err := header(f, st, s, []string{"Yön", "Oran (%)", "Matrah (portal)", "KDV (portal)"}, []float64{26, 10, 18, 18}); err != nil {
		return err
	}
	rows := append([]ae.VATRow(nil), b.VATSummary...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Direction != rows[j].Direction {
			return rows[i].Direction > rows[j].Direction // outgoing first
		}
		return rows[i].Rate.LessThan(rows[j].Rate)
	})
	for i, v := range rows {
		r := i + 2
		if err := f.SetCellStr(s, cell("A", r), v.Direction); err != nil {
			return err
		}
		if err := f.SetCellFloat(s, cell("B", r), v.Rate.InexactFloat64(), -1, 64); err != nil {
			return err
		}
		if err := f.SetCellStyle(s, cell("B", r), cell("B", r), st.pct); err != nil {
			return err
		}
		if err := money(f, st, s, cell("C", r), v.Base); err != nil {
			return err
		}
		if err := money(f, st, s, cell("D", r), v.VAT); err != nil {
			return err
		}
	}
	last := max(len(rows)+1, 2)
	portal := func(dir string) string {
		return fmt.Sprintf(`SUMIFS($D$2:$D$%d,$A$2:$A$%d,"%s")`, last, last, dir)
	}
	ent := func(col, prefix string) string {
		return fmt.Sprintf(`SUMIFS('%s'!$%s$%d:$%s$%d,'%s'!$D$%d:$D$%d,"%s*")`,
			SheetEntries, col, layout.first, col, layout.last, SheetEntries, layout.first, layout.last, prefix)
	}
	r := last + 2
	if err := f.SetCellStr(s, cell("A", r), "Karşılaştırma"); err != nil {
		return err
	}
	for i, h := range []string{"Portal", "Fişler", "Fark"} {
		c, _ := excelize.CoordinatesToCellName(i+2, r)
		if err := f.SetCellStr(s, c, h); err != nil {
			return err
		}
	}
	if err := f.SetCellStyle(s, cell("A", r), cell("D", r), st.header); err != nil {
		return err
	}
	type cmp struct{ label, portal, entries string }
	rowsCmp := []cmp{
		{"Hesaplanan KDV (391)", portal("outgoing"), ent("G", "391") + "-" + ent("F", "391")},
		{"İndirilecek KDV (191)", portal("incoming"), ent("F", "191") + "-" + ent("G", "191")},
	}
	first := r + 1
	for i, c := range rowsCmp {
		rr := first + i
		if err := f.SetCellStr(s, cell("A", rr), c.label); err != nil {
			return err
		}
		if err := formula(f, s, cell("B", rr), c.portal, st.money); err != nil {
			return err
		}
		if err := formula(f, s, cell("C", rr), c.entries, st.money); err != nil {
			return err
		}
		if err := formula(f, s, cell("D", rr), fmt.Sprintf("C%d-B%d", rr, rr), st.money); err != nil {
			return err
		}
	}
	if err := f.SetConditionalFormat(s, fmt.Sprintf("D%d:D%d", first, first+1), []excelize.ConditionalFormatOptions{{
		Type: "formula", Criteria: "=", Format: &st.red, Value: fmt.Sprintf("ABS($D%d)>=%s", first, tol),
	}}); err != nil {
		return err
	}
	out, in := first, first+1
	pay := first + 3
	labels := []struct{ label, fx string }{
		{"Ödenecek KDV", fmt.Sprintf("MAX(0,C%d-C%d)", out, in)},
		{"Devreden KDV", fmt.Sprintf("MAX(0,C%d-C%d)", in, out)},
	}
	for i, l := range labels {
		rr := pay + i
		if err := f.SetCellStr(s, cell("A", rr), l.label); err != nil {
			return err
		}
		if err := formula(f, s, cell("C", rr), l.fx, st.moneyBold); err != nil {
			return err
		}
	}
	return nil
}

func writeWarnings(f *excelize.File, st styles, b ae.Batch) error {
	s := SheetWarnings
	if err := header(f, st, s, []string{"Kod", "Seviye", "Varlık", "Açıklama", "Bağlantı"}, []float64{28, 10, 18, 50, 40}); err != nil {
		return err
	}
	for i, w := range b.Warnings {
		r := i + 2
		for col, v := range map[string]string{"A": w.Code, "B": w.Severity, "C": w.Entity, "D": w.Message, "E": w.Link} {
			if err := f.SetCellStr(s, cell(col, r), v); err != nil {
				return err
			}
		}
		if w.Link != "" {
			if err := f.SetCellHyperLink(s, cell("E", r), w.Link, "External"); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeSettings(f *excelize.File, st styles, b ae.Batch, entries []ae.Entry) error {
	s := SheetSettings
	if err := header(f, st, s, []string{"Ayar", "Değer"}, []float64{24, 40}); err != nil {
		return err
	}
	if err := f.SetCellStr(s, "A2", "Denge toleransı"); err != nil {
		return err
	}
	if err := f.SetCellFloat(s, "B2", decimal.RequireFromString(Tolerance).InexactFloat64(), -1, 64); err != nil {
		return err
	}
	var debit, credit decimal.Decimal
	for _, e := range entries {
		d, c := e.Totals()
		debit, credit = debit.Add(d), credit.Add(c)
	}
	rows := [][2]string{
		{"Dönem", b.Period},
		{"Mükellef", strings.TrimSpace(b.Taxpayer.Title)},
		{"VKN / TCKN", b.Taxpayer.TaxID},
		{"Ofis", b.Office.Title},
		{"Sürücü / şablon", Name + " v1"},
		{"Üretim zamanı", b.GeneratedAt.Format("2006-01-02 15:04:05 MST")},
		{"Üreten", b.GeneratedBy},
		{"Fiş sayısı", fmt.Sprint(len(entries))},
		{"Toplam borç", debit.StringFixed(2)},
		{"Toplam alacak", credit.StringFixed(2)},
	}
	for i, kv := range rows {
		r := i + 3
		if err := f.SetCellStr(s, cell("A", r), kv[0]); err != nil {
			return err
		}
		if err := f.SetCellStr(s, cell("B", r), kv[1]); err != nil {
			return err
		}
	}
	return nil
}

func entryTypeLabel(t ae.EntryType) string {
	switch t {
	case ae.EntryAcilis:
		return "Açılış"
	case ae.EntryKapanis:
		return "Kapanış"
	}
	return "Mahsup"
}

// mainAccount is the first three characters of a code (ana hesap).
func mainAccount(code string) string {
	r := []rune(code)
	if len(r) <= 3 {
		return code
	}
	return string(r[:3])
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
