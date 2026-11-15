package excelformula

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var day = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

func batch() ae.Batch {
	return ae.Batch{
		Period:   "2026-10",
		Taxpayer: ae.Party{Title: "AAA Anonim Şirketi", TaxID: "1288331521"},
		Entries: []ae.Entry{
			{ID: "e1", No: "1", Date: day, Type: ae.EntryMahsup, Description: "Satış faturası", Lines: []ae.Line{
				{Account: "120.01", Debit: d("1200"), Source: ae.SourceRef{Kind: "invoice", ID: "6a2c1d2e", URL: "https://app.example/i/6a2c1d2e"}},
				{Account: "600.01", Credit: d("1000")},
				{Account: "391.20", Credit: d("200")},
			}},
			{ID: "e2", No: "2", Date: day.AddDate(0, 0, 1), Type: ae.EntryMahsup, Description: "Alış faturası", Lines: []ae.Line{
				{Account: "153.01", Debit: d("500.55")},
				{Account: "191.20", Debit: d("100.11")},
				{Account: "320.01", Credit: d("600.66")},
			}},
			{ID: "e3", No: "3", Date: day.AddDate(0, 0, 2), Type: ae.EntryMahsup, Description: "Tahsilat (dengesiz)", Lines: []ae.Line{
				{Account: "102.01", Debit: d("100")},
				{Account: "120.01", Credit: d("99")},
			}},
		},
		VATSummary: []ae.VATRow{
			{Direction: "outgoing", Rate: d("20"), Base: d("1000"), VAT: d("200")},
			{Direction: "incoming", Rate: d("20"), Base: d("500.55"), VAT: d("100.11")},
		},
		Warnings:    []ae.WarningRow{{Code: "TAX_TOTAL_MISMATCH", Severity: "warning", Entity: "invoice", Message: "KDV toplamı tutmuyor"}},
		GeneratedAt: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
	}
}

func export(t *testing.T, b ae.Batch) ([]byte, ae.Result) {
	t.Helper()
	drv, err := ae.Open(Name, ae.Config{})
	if err != nil {
		t.Fatal(err)
	}
	w := ae.NewMemoryWriter()
	res, err := drv.Export(context.Background(), b, w)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("files: %+v", res)
	}
	data, _ := w.File(res.Files[0].Name)
	return data, res
}

func TestWorkbookStructureAndFormulas(t *testing.T) {
	data, res := export(t, batch())
	if res.Files[0].Name != "kontrol_2026-10.xlsx" || len(res.EntryIDs) != 3 || ae.HasBlocker(res.Issues) {
		t.Fatalf("result: %+v", res)
	}
	// The unbalanced entry is reported but does not block this workbook.
	found := false
	for _, i := range res.Issues {
		found = found || (i.Code == ae.IssueUnbalanced && i.EntryNo == "3")
	}
	if !found {
		t.Fatalf("issues: %+v", res.Issues)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	want := []string{SheetEntries, SheetSummary, SheetTrial, SheetVAT, SheetWarnings, SheetSettings}
	if got := f.GetSheetList(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("sheets: %v", got)
	}
	// Totals are formulas, not values.
	for _, c := range []struct{ sheet, cell, contains string }{
		{SheetEntries, "F5", "SUMIFS("},
		{SheetEntries, "H5", "F5-G5"},
		{SheetSummary, "E2", "SUMIFS("},
		{SheetSummary, "H2", "IF(ABS(G2)<"},
		{SheetTrial, "C2", "SUMIFS("},
		{SheetVAT, "C6", `"391*"`},
	} {
		fx, err := f.GetCellFormula(c.sheet, c.cell)
		if err != nil || !strings.Contains(fx, c.contains) {
			t.Errorf("%s!%s formula = %q (%v), want %q", c.sheet, c.cell, fx, err, c.contains)
		}
	}
	// Amounts are numeric cells with #,##0.00.
	typ, _ := f.GetCellType(SheetEntries, "F2")
	if typ != excelize.CellTypeNumber && typ != excelize.CellTypeUnset {
		t.Errorf("F2 type = %v", typ)
	}
	if v, _ := f.GetCellValue(SheetEntries, "F2", excelize.Options{RawCellValue: true}); v != "1200" {
		t.Errorf("F2 raw = %q", v)
	}
	sid, _ := f.GetCellStyle(SheetEntries, "F2")
	st, _ := f.GetStyle(sid)
	if st.CustomNumFmt == nil || *st.CustomNumFmt != "#,##0.00" {
		t.Errorf("F2 number format = %+v", st.CustomNumFmt)
	}
	// Red conditional format on the entry total difference.
	cf, err := f.GetConditionalFormats(SheetEntries)
	if err != nil || len(cf) == 0 {
		t.Fatalf("conditional formats: %v %v", cf, err)
	}
	if link, target, _ := f.GetCellHyperLink(SheetEntries, "I2"); !link || target != "https://app.example/i/6a2c1d2e" {
		t.Errorf("source link: %v %q", link, target)
	}
}

// excelize's own calculator agrees with the expected totals.
func TestFormulasCalculateWithExcelize(t *testing.T) {
	data, _ := export(t, batch())
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	calc := func(sheet, c string) string {
		v, err := f.CalcCellValue(sheet, c, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("%s!%s: %v", sheet, c, err)
		}
		return v
	}
	checks := map[[2]string]string{
		{SheetSummary, "E2"}: "1200", {SheetSummary, "F2"}: "1200", {SheetSummary, "H2"}: "Dengeli",
		{SheetSummary, "E3"}: "600.66", {SheetSummary, "H3"}: "Dengeli",
		{SheetSummary, "G4"}: "1", {SheetSummary, "H4"}: "DENGESİZ",
		{SheetVAT, "B6"}: "200", {SheetVAT, "C6"}: "200", {SheetVAT, "D6"}: "0",
		{SheetVAT, "B7"}: "100.11", {SheetVAT, "C7"}: "100.11", {SheetVAT, "D7"}: "0",
		{SheetVAT, "C9"}: "99.89", {SheetVAT, "C10"}: "0",
	}
	for k, want := range checks {
		if got := calc(k[0], k[1]); got != want {
			t.Errorf("%s!%s = %q, want %q", k[0], k[1], got, want)
		}
	}
}

// docs/02 §5 test 6: LibreOffice opens the file and calculates the formulas.
// LibreOffice re-saves the workbook as xlsx with the values it computed as the
// cells' cached results; those are read back and checked.
func TestFormulasCalculateInLibreOffice(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("LibreOffice not installed")
	}
	data, _ := export(t, batch())
	dir := t.TempDir()
	in := filepath.Join(dir, "kontrol.xlsx")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "lo")
	cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "profile"),
		"--convert-to", "xlsx:Calc MS Excel 2007 XML", "--outdir", outDir, in)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("soffice: %v\n%s", err, out)
	}
	f, err := excelize.OpenFile(filepath.Join(outDir, "kontrol.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	cached := func(sheet, c string) string {
		v, err := f.GetCellValue(sheet, c, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("%s!%s: %v", sheet, c, err)
		}
		return v
	}
	checks := map[[2]string]string{
		{SheetSummary, "E2"}: "1200", {SheetSummary, "H2"}: "Dengeli",
		{SheetSummary, "E3"}: "600.66", {SheetSummary, "H3"}: "Dengeli",
		{SheetSummary, "G4"}: "1", {SheetSummary, "H4"}: "DENGESİZ",
		{SheetEntries, "H5"}: "0", {SheetEntries, "H12"}: "1",
		{SheetVAT, "B6"}: "200", {SheetVAT, "C6"}: "200", {SheetVAT, "D6"}: "0",
		{SheetVAT, "B7"}: "100.11", {SheetVAT, "C7"}: "100.11", {SheetVAT, "D7"}: "0",
		{SheetVAT, "C9"}: "99.89",
	}
	for k, want := range checks {
		if got := cached(k[0], k[1]); !sameNumber(got, want) {
			t.Errorf("LibreOffice %s!%s = %q, want %q", k[0], k[1], got, want)
		}
	}
	// The formulas survive the round trip (the accountant can keep editing).
	if fx, _ := f.GetCellFormula(SheetSummary, "E2"); !strings.Contains(strings.ToUpper(fx), "SUMIFS") {
		t.Errorf("formula lost: %q", fx)
	}
}

// sameNumber compares numeric strings ignoring float noise ("600.66000000000008").
func sameNumber(got, want string) bool {
	if got == want {
		return true
	}
	g, err1 := decimal.NewFromString(got)
	w, err2 := decimal.NewFromString(want)
	return err1 == nil && err2 == nil && g.Sub(w).Abs().LessThan(d("0.000001"))
}

func TestDoubleBookingExclusionAndEmpty(t *testing.T) {
	b := batch()
	b.Settings.InvoicesReachLucaDirectly = true
	_, res := export(t, b)
	if len(res.EntryIDs) != 2 {
		t.Fatalf("invoice entry not excluded: %v", res.EntryIDs)
	}
	b.Entries = nil
	drv, _ := ae.Open(Name, ae.Config{})
	res, err := drv.Export(context.Background(), b, ae.NewMemoryWriter())
	if err != nil || len(res.Files) != 0 {
		t.Fatalf("empty batch: %+v %v", res, err)
	}
	if info := drv.Info(); !info.Verified || info.Name != Name {
		t.Fatalf("info: %+v", info)
	}
}
