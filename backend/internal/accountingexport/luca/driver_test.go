package luca

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var day = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

func entry(no string, date time.Time, lines ...ae.Line) ae.Entry {
	return ae.Entry{ID: "id-" + no, No: no, Date: date, Type: ae.EntryMahsup, Description: "fiş " + no, Lines: lines}
}

func dr(acc, amt string) ae.Line { return ae.Line{Account: acc, Debit: d(amt)} }
func cr(acc, amt string) ae.Line { return ae.Line{Account: acc, Credit: d(amt)} }

// open opens the earlier "unverified-2" layout these tests were written
// for; the default layout is covered in verified_test.go.
func open(t *testing.T) ae.Driver {
	t.Helper()
	drv, err := ae.Open(Name, ae.Config{Template: "unverified-2"})
	if err != nil {
		t.Fatal(err)
	}
	return drv
}

func run(t *testing.T, drv ae.Driver, b ae.Batch) (ae.Result, *ae.MemoryWriter) {
	t.Helper()
	w := ae.NewMemoryWriter()
	res, err := drv.Export(context.Background(), b, w)
	if err != nil {
		t.Fatal(err)
	}
	return res, w
}

// rows reads a produced file's first sheet (raw cell values).
func rows(t *testing.T, w *ae.MemoryWriter, name string) [][]string {
	t.Helper()
	data, ok := w.File(name)
	if !ok {
		t.Fatalf("no file %s (have %v)", name, w.Names())
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	out, err := f.GetRows(f.GetSheetList()[0], excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInfoUnverified(t *testing.T) {
	info := open(t).Info()
	if info.Name != Name || info.TemplateVer != "unverified-2" || info.Verified ||
		info.Limits.MaxEntriesPerFile != 50 || info.Limits.MaxLinesPerEntry != 400 {
		t.Fatalf("info: %+v", info)
	}
	if !slices.Contains(Versions(), DefaultTemplate) || !slices.Contains(Versions(), "unverified-1") || !slices.Contains(Versions(), "unverified-2") {
		t.Fatalf("versions: %v", Versions())
	}
	if _, err := ae.Open(Name, ae.Config{Template: "nope"}); err == nil {
		t.Fatal("unknown template version opened")
	}
}

// docs/02 §5 test 1: dengeli 3 fişlik batch → Luca dosyası; Fiş No + Tarih gruplaması.
func TestThreeEntriesGrouping(t *testing.T) {
	b := ae.Batch{Period: "2026-10", Entries: []ae.Entry{
		entry("1", day, dr("770.01", "100"), cr("320.01", "100")),
		entry("2", day, dr("153", "50.5"), dr("191.20", "10.1"), cr("320.02", "60.6")),
		entry("3", day.AddDate(0, 0, 1), dr("102", "30"), cr("120", "30")),
	}}
	res, w := run(t, open(t), b)
	if len(res.Files) != 1 || res.Files[0].Name != "luca_fis_2026-10.xlsx" || res.Files[0].Entries != 3 || len(res.EntryIDs) != 3 {
		t.Fatalf("result: %+v", res)
	}
	// The unverified template is always flagged.
	if len(res.Issues) != 1 || res.Issues[0].Code != ae.IssueTemplateUnverified || res.Issues[0].Severity != ae.SeverityWarning {
		t.Fatalf("issues: %+v", res.Issues)
	}
	got := rows(t, w, "luca_fis_2026-10.xlsx")
	if fmt.Sprint(got[0]) != "[Fiş No Fiş Tarihi Hesap Kodu Borç Alacak Evrak No Evrak Tarihi Açıklama]" {
		t.Fatalf("header: %v", got[0])
	}
	if len(got) != 1+7 {
		t.Fatalf("rows: %d", len(got))
	}
	// Each voucher's rows share Fiş No + Fiş Tarihi and balance on their own.
	type key struct{ no, date string }
	sums := map[key][2]decimal.Decimal{}
	var order []key
	for _, r := range got[1:] {
		k := key{r[0], r[1]}
		if _, ok := sums[k]; !ok {
			order = append(order, k)
		}
		s := sums[k]
		s[0], s[1] = s[0].Add(d(r[3])), s[1].Add(d(r[4]))
		sums[k] = s
	}
	if len(order) != 3 || order[0].no != "1" || order[2].no != "3" || order[0].date == order[2].date {
		t.Fatalf("grouping: %v", order)
	}
	for k, s := range sums {
		if !s[0].Equal(s[1]) {
			t.Errorf("voucher %v unbalanced: %v", k, s)
		}
	}
	if got[3][2] != "153" || got[3][3] != "50.5" || got[1][4] != "0" {
		t.Fatalf("amounts: %v %v", got[3], got[1])
	}
}

// docs/02 §5 test 2: 120 fişlik batch → 3 dosya (50 / 50 / 20).
func TestHundredTwentyEntriesThreeFiles(t *testing.T) {
	var es []ae.Entry
	for i := range 120 {
		es = append(es, entry(fmt.Sprint(i+1), day, dr("770", "1"), cr("102", "1")))
	}
	res, w := run(t, open(t), ae.Batch{Period: "2026-10", Entries: es})
	if len(res.Files) != 3 {
		t.Fatalf("files: %+v", res.Files)
	}
	want := []int{50, 50, 20}
	for i, f := range res.Files {
		if f.Entries != want[i] || f.Name != fmt.Sprintf("luca_fis_2026-10_%d.xlsx", i+1) {
			t.Errorf("file %d: %+v", i, f)
		}
		if n := len(rows(t, w, f.Name)) - 1; n != want[i]*2 {
			t.Errorf("file %d rows = %d", i, n)
		}
	}
	if len(res.EntryIDs) != 120 {
		t.Fatalf("entry ids: %d", len(res.EntryIDs))
	}
}

// docs/02 §5 test 3: 450 satırlık mahsup → dengeli iki fiş; bölünemeyen → blocker.
func TestLongMahsupSplit(t *testing.T) {
	var lines []ae.Line
	for i := range 225 {
		amt := fmt.Sprint(i + 1)
		lines = append(lines, dr("770", amt), cr("102", amt))
	}
	res, w := run(t, open(t), ae.Batch{Entries: []ae.Entry{entry("7", day, lines...)}})
	if len(res.Files) != 1 || res.Files[0].Entries != 2 || len(res.EntryIDs) != 1 {
		t.Fatalf("split: %+v", res)
	}
	got := rows(t, w, res.Files[0].Name)[1:]
	counts := map[string]int{}
	bal := map[string]decimal.Decimal{}
	for _, r := range got {
		counts[r[0]]++
		bal[r[0]] = bal[r[0]].Add(d(r[3])).Sub(d(r[4]))
	}
	if counts["7-1"] != 400 || counts["7-2"] != 50 || !bal["7-1"].IsZero() || !bal["7-2"].IsZero() {
		t.Fatalf("pieces: %v %v", counts, bal)
	}

	var bad []ae.Line
	for range 449 {
		bad = append(bad, dr("770", "1"))
	}
	bad = append(bad, cr("102", "449"))
	res, w = run(t, open(t), ae.Batch{Entries: []ae.Entry{entry("8", day, bad...)}})
	if len(res.Files) != 0 || !ae.HasBlocker(res.Issues) || len(w.Names()) != 0 {
		t.Fatalf("unsplittable exported: %+v", res)
	}
	found := false
	for _, i := range res.Issues {
		found = found || (i.Code == ae.IssueSplitImpossible && i.EntryNo == "8")
	}
	if !found {
		t.Fatalf("issues: %+v", res.Issues)
	}
	// Açılış has no line limit in Luca: the same 450 lines stay one voucher.
	acilis := entry("9", day, lines...)
	acilis.Type = ae.EntryAcilis
	res, _ = run(t, open(t), ae.Batch{Entries: []ae.Entry{acilis}})
	if len(res.Files) != 1 || res.Files[0].Entries != 1 {
		t.Fatalf("açılış: %+v", res)
	}
}

func TestUnknownAccountAndUnbalancedBlock(t *testing.T) {
	b := ae.Batch{
		Entries:    []ae.Entry{entry("1", day, dr("770.99", "10"), cr("320", "10")), entry("2", day, dr("770", "10"), cr("320", "9"))},
		TargetPlan: ae.AccountPlan{"770": true, "320": true},
		Settings:   ae.Settings{BlockOnUnknownAccount: true},
	}
	res, _ := run(t, open(t), b)
	codes := map[string]bool{}
	for _, i := range res.Issues {
		codes[i.Code] = codes[i.Code] || i.Severity == ae.SeverityBlocker
	}
	if len(res.Files) != 0 || !codes[ae.IssueAccountNotInPlan] || !codes[ae.IssueUnbalanced] {
		t.Fatalf("docs/02 §5 test 4: %+v", res)
	}
}

// Kabul: şablon geldiğinde yalnız YAML değişerek uyum sağlanıyor. A
// "verified" template with other headers, order, an extra description column,
// text dates and comma decimals changes the file without touching code.
func TestTemplateChangeIsYAMLOnly(t *testing.T) {
	fsys := fstest.MapFS{"templates/fis_v2.yaml": {Data: []byte(`driver: luca
template: fis
version: "v2"
verified: true
file: { format: xlsx, sheet: "Fis", header_row: 2 }
limits: { max_entries_per_file: 2, max_lines_per_mahsup: 3 }
columns:
  - { key: entry_date,  header: "FİŞ TARİHİ", required: true, format: "02/01/2006" }
  - { key: entry_no,    header: "FİŞ NO",     required: true }
  - { key: entry_type,  header: "FİŞ TÜRÜ" }
  - { key: account,     header: "HESAP KODU", required: true }
  - { key: description, header: "AÇIKLAMA" }
  - { key: debit,       header: "BORÇ",       required: true, decimal: "," }
  - { key: credit,      header: "ALACAK",     required: true, decimal: "," }
entry_types: { mahsup: "M", acilis: "A", kapanis: "K" }
`)}}
	tpl, err := LoadTemplate(fsys, "v2")
	if err != nil {
		t.Fatal(err)
	}
	drv, err := New(tpl)
	if err != nil {
		t.Fatal(err)
	}
	if info := drv.Info(); !info.Verified || info.TemplateVer != "v2" {
		t.Fatalf("info: %+v", info)
	}
	b := ae.Batch{Entries: []ae.Entry{
		entry("1", day, dr("770", "1234.5"), cr("320", "1234.5")),
		entry("2", day, dr("770", "1"), dr("771", "1"), cr("320", "2"), dr("153", "3"), cr("320", "3")), // 5 lines > 3 → 2 vouchers
	}}
	res, w := run(t, drv, b)
	if len(res.Issues) != 0 || len(res.Files) != 2 { // 3 vouchers, 2 per file
		t.Fatalf("result: %+v", res)
	}
	got := rows(t, w, res.Files[0].Name)
	if len(got) < 3 || fmt.Sprint(got[1]) != "[FİŞ TARİHİ FİŞ NO FİŞ TÜRÜ HESAP KODU AÇIKLAMA BORÇ ALACAK]" {
		t.Fatalf("header row 2: %v", got)
	}
	if fmt.Sprint(got[2]) != "[15/10/2026 1 M 770 fiş 1 1234,50 0,00]" {
		t.Fatalf("first row: %v", got[2])
	}
	data, _ := w.File(res.Files[0].Name)
	f, _ := excelize.OpenReader(bytes.NewReader(data))
	if f.GetSheetList()[0] != "Fis" {
		t.Fatalf("sheet: %v", f.GetSheetList())
	}
	_ = f.Close()

	bad := fstest.MapFS{"templates/fis_bad.yaml": {Data: []byte("driver: luca\ntemplate: fis\nversion: bad\nfile: {format: xlsx, header_row: 1}\ncolumns: [{key: entry_no, header: A}, {key: colour, header: B}]\n")}}
	tb, err := LoadTemplate(bad, "bad")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(tb); err == nil {
		t.Fatal("unknown column key accepted")
	}
}

// #466: every line carries its source document: Evrak No / Evrak Tarihi and
// the invoice ETTN in Açıklama; a line without a document keeps its plain
// description.
func TestSourceDocumentColumns(t *testing.T) {
	docDate := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	inv := ae.SourceRef{Kind: "invoice_item", ID: "41", DocumentKey: "3f2a0c1e-0000-4000-8000-000000000001", ETTN: "3f2a0c1e-0000-4000-8000-000000000001"}
	l1 := dr("153", "100")
	l1.Description, l1.DocNo, l1.DocDate, l1.Source = "Mal alışı", "ABC2026000000123", &docDate, inv
	l2 := cr("320.01", "100")
	l2.DocNo, l2.DocDate, l2.Source = "ABC2026000000123", &docDate, inv
	plain := entry("2", day, dr("770", "5"), cr("102", "5"))
	res, w := run(t, open(t), ae.Batch{Period: "2026-10", Entries: []ae.Entry{entry("1", day, l1, l2), plain}})
	got := rows(t, w, res.Files[0].Name)
	head := got[0]
	col := func(name string) int {
		for i, h := range head {
			if h == name {
				return i
			}
		}
		t.Fatalf("no column %q in %v", name, head)
		return -1
	}
	no, date, desc := col("Evrak No"), col("Evrak Tarihi"), col("Açıklama")
	if got[1][no] != "ABC2026000000123" || got[1][date] == "" {
		t.Fatalf("document columns: %v", got[1])
	}
	if got[1][desc] != "Mal alışı ETTN:3f2a0c1e-0000-4000-8000-000000000001" {
		t.Fatalf("line description: %q", got[1][desc])
	}
	// No line description: the voucher's description, then the ETTN.
	if got[2][desc] != "fiş 1 ETTN:3f2a0c1e-0000-4000-8000-000000000001" {
		t.Fatalf("voucher description: %q", got[2][desc])
	}
	// No document: no ETTN suffix, no Evrak No.
	if got[3][desc] != "fiş 2" || (len(got[3]) > no && got[3][no] != "") {
		t.Fatalf("plain line: %v", got[3])
	}
	if res.EntryFiles["id-1"] != res.Files[0].Name || res.EntryFiles["id-2"] != res.Files[0].Name {
		t.Fatalf("entry files: %v", res.EntryFiles)
	}
}

func TestSourceDescriptionPattern(t *testing.T) {
	cases := []struct{ pattern, desc, ettn, want string }{
		{"", "Kira", "e1", "Kira ETTN:e1"},
		{"{description} ETTN:{ettn}", "", "e1", "ETTN:e1"},
		{"{description} ETTN:{ettn}", "Kira", "", "Kira"},
		{"{ettn} / {description}", "Kira", "e1", "e1 / Kira"},
		{"{description} [{unknown}]", "Kira", "e1", "Kira [{unknown}]"},
		{"{description} ETTN:{ettn}", "", "", ""},
	}
	for _, c := range cases {
		if got := sourceDescription(c.pattern, c.desc, c.ettn); got != c.want {
			t.Errorf("sourceDescription(%q, %q, %q) = %q, want %q", c.pattern, c.desc, c.ettn, got, c.want)
		}
	}
}

// The older template version stays loadable for exports made with it.
func TestPreviousTemplateStillOpens(t *testing.T) {
	drv, err := ae.Open(Name, ae.Config{Template: "unverified-1"})
	if err != nil {
		t.Fatal(err)
	}
	if drv.Info().TemplateVer != "unverified-1" || drv.Info().Verified {
		t.Fatalf("info: %+v", drv.Info())
	}
}
