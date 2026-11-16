package accountingexport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/shopspring/decimal"
)

// fakeDriver writes one CSV-ish file per group of entries.
type fakeDriver struct{ cfg Config }

func (f fakeDriver) Info() DriverInfo {
	return DriverInfo{Name: "fake", TemplateVer: "test-1", Limits: Limits{MaxEntriesPerFile: 2}, Verified: true}
}

func (f fakeDriver) Validate(_ context.Context, b Batch) []Issue { return CommonIssues(b) }

func (f fakeDriver) Export(ctx context.Context, b Batch, w FileWriter) (Result, error) {
	issues := f.Validate(ctx, b)
	if HasBlocker(issues) {
		return Result{Issues: issues}, nil
	}
	var res Result
	for i, group := range SplitIntoFiles(b.Entries, f.Info().Limits.MaxEntriesPerFile) {
		var sb strings.Builder
		for _, e := range group {
			fmt.Fprintf(&sb, "%s;%s\n", e.No, e.Date.Format("2006-01-02"))
			res.EntryIDs = append(res.EntryIDs, e.ID)
		}
		file, err := WriteFile(w, fmt.Sprintf("fake-%d.txt", i+1), "text/plain", []byte(sb.String()), len(group))
		if err != nil {
			return res, err
		}
		res.Files = append(res.Files, file)
	}
	return res, nil
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func entry(no string, lines ...Line) Entry {
	return Entry{ID: "id-" + no, No: no, Date: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC), Type: EntryMahsup, Lines: lines}
}

func dr(acc, amt string) Line { return Line{Account: acc, Debit: d(amt)} }
func cr(acc, amt string) Line { return Line{Account: acc, Credit: d(amt)} }

// Kabul: örnek sahte sürücü kayıt edilip çağrılabiliyor.
func TestFakeDriverRegisterOpenExport(t *testing.T) {
	Register("fake", func(cfg Config) (Driver, error) { return fakeDriver{cfg: cfg}, nil })
	t.Cleanup(func() { unregister("fake") })

	found := false
	for _, n := range Drivers() {
		found = found || n == "fake"
	}
	if !found {
		t.Fatalf("drivers = %v", Drivers())
	}
	drv, err := Open("fake", Config{Template: "test-1"})
	if err != nil {
		t.Fatal(err)
	}
	if drv.Info().Name != "fake" || drv.(fakeDriver).cfg.Template != "test-1" {
		t.Fatalf("info/config: %+v", drv.Info())
	}
	b := Batch{Entries: []Entry{
		entry("1", dr("770", "100"), cr("320", "100")),
		entry("2", dr("153", "50"), cr("320", "50")),
		entry("3", dr("100", "10"), cr("600", "10")),
	}}
	w := NewMemoryWriter()
	res, err := drv.Export(context.Background(), b, w)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 2 || res.Files[0].Entries != 2 || res.Files[1].Entries != 1 || len(res.EntryIDs) != 3 {
		t.Fatalf("result: %+v", res)
	}
	if got, _ := w.File("fake-2.txt"); string(got) != "3;2026-10-31\n" || len(res.Files[1].SHA256) != 64 {
		t.Fatalf("file: %q %+v", got, res.Files[1])
	}

	if _, err := Open("nope", Config{}); !errors.Is(err, ErrUnknownDriver) {
		t.Fatalf("unknown: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate Register did not panic")
		}
	}()
	Register("fake", func(Config) (Driver, error) { return fakeDriver{}, nil })
}

func TestCommonIssues(t *testing.T) {
	b := Batch{
		Entries: []Entry{
			entry("1", dr("770.01", "100"), cr("320.01", "100")),
			entry("2", dr("770.01", "100"), cr("320.01", "99.99")),
			entry("3"),
		},
		TargetPlan: AccountPlan{"770.01": true},
	}
	codes := map[string]Issue{}
	for _, i := range CommonIssues(b) {
		codes[i.Code+"/"+i.EntryNo] = i
	}
	if i := codes[IssueUnbalanced+"/2"]; i.Severity != SeverityBlocker || i.Params["difference"] != "0.01" {
		t.Errorf("unbalanced: %+v", i)
	}
	if i := codes[IssueEmptyEntry+"/3"]; i.Severity != SeverityBlocker {
		t.Errorf("empty: %+v", codes)
	}
	if i := codes[IssueAccountNotInPlan+"/1"]; i.Severity != SeverityWarning || i.Params["account"] != "320.01" || i.Line != 2 {
		t.Errorf("plan: %+v", i)
	}
	b.Settings.BlockOnUnknownAccount = true
	b.Entries = b.Entries[:1]
	if !HasBlocker(CommonIssues(b)) {
		t.Error("unknown account should block when the setting is on")
	}
	b.TargetPlan = nil
	if len(CommonIssues(b)) != 0 {
		t.Error("no plan: account check must be skipped")
	}
}

func TestSelectEntriesDoubleBooking(t *testing.T) {
	inv := entry("1", Line{Account: "600", Credit: d("10"), Source: SourceRef{Kind: "invoice", ID: "ettn"}}, dr("120", "10"))
	bank := entry("2", dr("102", "5"), cr("120", "5"))
	b := Batch{Entries: []Entry{inv, bank}, Settings: Settings{InvoicesReachLucaDirectly: true}}
	got, issues := SelectEntries(b)
	if len(got) != 1 || got[0].No != "2" || len(issues) != 1 || issues[0].Code != IssueInvoiceEntryExcluded {
		t.Fatalf("excluded: %v %v", got, issues)
	}
	b.Settings.IncludeInvoiceEntries = true
	if got, _ := SelectEntries(b); len(got) != 2 {
		t.Fatal("accountant chose to include invoice entries")
	}
	// #319: lines booked per invoice item are invoice entries too.
	item := entry("3", Line{Account: "153", Debit: d("10"), Source: SourceRef{Kind: "invoice_item", ID: "42"}}, cr("320", "10"))
	b = Batch{Entries: []Entry{item, bank}, Settings: Settings{InvoicesReachLucaDirectly: true}}
	if got, _ := SelectEntries(b); len(got) != 1 || got[0].No != "2" {
		t.Fatalf("invoice_item entry must be excluded: %v", got)
	}
}

// docs/02 §5 test 2: 120 fiş → 50 / 50 / 20.
func TestSplitIntoFiles(t *testing.T) {
	var es []Entry
	for i := range 120 {
		es = append(es, entry(fmt.Sprint(i+1), dr("1", "1"), cr("2", "1")))
	}
	groups := SplitIntoFiles(es, 50)
	if len(groups) != 3 || len(groups[0]) != 50 || len(groups[1]) != 50 || len(groups[2]) != 20 {
		t.Fatalf("groups: %d", len(groups))
	}
	if SplitIntoFiles(nil, 50) != nil || len(SplitIntoFiles(es, 0)) != 1 {
		t.Fatal("edge cases")
	}
}

// docs/02 §5 test 3: 450 satırlık mahsup → dengeli iki fiş; bölünemeyen örnek.
func TestSplitEntry(t *testing.T) {
	var lines []Line
	for i := range 225 { // 225 debit/credit pairs = 450 lines
		lines = append(lines, dr("770", fmt.Sprint(i+1)), cr("102", fmt.Sprint(i+1)))
	}
	e := entry("M7", lines...)
	pieces, ok := SplitEntry(e, 400)
	if !ok || len(pieces) != 2 || len(pieces[0].Lines) != 400 || len(pieces[1].Lines) != 50 {
		t.Fatalf("split: ok=%v pieces=%d", ok, len(pieces))
	}
	for _, p := range pieces {
		if !p.Balanced() || len(p.Lines) > 400 {
			t.Fatalf("piece %s unbalanced or too long", p.No)
		}
	}
	if pieces[0].No != "M7-1" || pieces[1].No != "M7-2" {
		t.Fatalf("numbers: %s %s", pieces[0].No, pieces[1].No)
	}
	// One 449-line debit block against a single credit: no balanced cut fits 400.
	var bad []Line
	for range 449 {
		bad = append(bad, dr("770", "1"))
	}
	bad = append(bad, cr("102", "449"))
	if _, ok := SplitEntry(entry("M8", bad...), 400); ok {
		t.Fatal("unsplittable entry split")
	}
	if p, ok := SplitEntry(e, 0); !ok || len(p) != 1 {
		t.Fatal("no limit")
	}
}

func TestLoadTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"ok.yaml": {Data: []byte(`driver: x
template: fis
version: "unverified-1"
verified: false
file: { format: xlsx, sheet: "", header_row: 1 }
limits: { max_entries_per_file: 50, max_lines_per_mahsup: 400 }
columns:
  - { key: entry_no, header: "Fiş No", required: true }
  - { key: debit, header: "Borç", required: true }
entry_types: { mahsup: "M" }
`)},
		"dup.yaml":     {Data: []byte("driver: x\ntemplate: f\nversion: v\nfile: {header_row: 1}\ncolumns: [{key: a, header: A}, {key: a, header: B}]\n")},
		"unknown.yaml": {Data: []byte("driver: x\ntemplate: f\nversion: v\nfile: {header_row: 1}\ncolumns: [{key: a, header: A}]\ncolour: red\n")},
	}
	tpl, err := LoadTemplate(fsys, "ok.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if tpl.Limits.MaxEntriesPerFile != 50 || tpl.Limits.MaxLinesPerEntry != 400 || tpl.Columns[0].Header != "Fiş No" || tpl.EntryTypes[EntryMahsup] != "M" {
		t.Fatalf("template: %+v", tpl)
	}
	for _, bad := range []string{"dup.yaml", "unknown.yaml", "missing.yaml"} {
		if _, err := LoadTemplate(fsys, bad); !errors.Is(err, ErrTemplate) {
			t.Errorf("%s: %v", bad, err)
		}
	}
}
