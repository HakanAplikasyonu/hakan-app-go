package zirve

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	ae "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var day = time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

func dr(acc, amt string) ae.Line { return ae.Line{Account: acc, Debit: d(amt)} }
func cr(acc, amt string) ae.Line { return ae.Line{Account: acc, Credit: d(amt)} }

func open(t *testing.T) ae.Driver {
	t.Helper()
	drv, err := ae.Open(Name, ae.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return drv
}

func TestInfoUnverified(t *testing.T) {
	info := open(t).Info()
	if info.Name != "zirve" || info.Verified || info.TemplateVer != DefaultTemplate ||
		info.Limits.MaxEntriesPerFile != 0 || info.Limits.MaxLinesPerEntry != 0 {
		t.Fatalf("info: %+v", info)
	}
	if v := Versions(); len(v) != 1 || v[0] != DefaultTemplate {
		t.Fatalf("versions: %v", v)
	}
}

// docs/02 §5: a balanced batch becomes one Zirve file in the template's
// column order; the unverified template is reported as a warning.
func TestExport(t *testing.T) {
	doc := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	sale := ae.Entry{ID: "a", No: "1", Date: day, Type: ae.EntryMahsup, Description: "Satış faturası", Lines: []ae.Line{
		{Account: "120.01.001", Debit: d("1200"), Description: "Yıldız Market", DocNo: "ATL2026000000101", DocDate: &doc},
		cr("600.01", "1000"), cr("391.01", "200"),
	}}
	// 600 lines: no per-voucher limit, nothing is split.
	big := ae.Entry{ID: "b", No: "2", Date: day, Type: ae.EntryMahsup, Description: "Toplu"}
	for i := 0; i < 300; i++ {
		big.Lines = append(big.Lines, dr("770.01", "1"), cr("102.01", "1"))
	}
	w := ae.NewMemoryWriter()
	res, err := open(t).Export(context.Background(), ae.Batch{Period: "2026-10", Entries: []ae.Entry{sale, big}}, w)
	if err != nil {
		t.Fatal(err)
	}
	if ae.HasBlocker(res.Issues) || len(res.Files) != 1 || res.Files[0].Name != "zirve_fis_2026-10.xlsx" {
		t.Fatalf("result: %+v", res)
	}
	warned := false
	for _, is := range res.Issues {
		warned = warned || is.Code == ae.IssueTemplateUnverified
	}
	if !warned {
		t.Fatal("unverified template must warn")
	}
	data, _ := w.File("zirve_fis_2026-10.xlsx")
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, _ := f.GetRows(f.GetSheetList()[0])
	head := []string{"Fiş No", "Fiş Tarihi", "Fiş Türü", "Fiş Açıklaması", "Hesap Kodu", "Açıklama", "Evrak No", "Evrak Tarihi", "Borç", "Alacak"}
	for i, h := range head {
		if rows[0][i] != h {
			t.Fatalf("header %d: %q want %q", i, rows[0][i], h)
		}
	}
	if len(rows) != 1+3+600 {
		t.Fatalf("rows: %d", len(rows))
	}
	r := rows[1]
	if r[0] != "1" || r[1] != "15.10.2026" || r[2] != "Mahsup" || r[3] != "Satış faturası" || r[4] != "120.01.001" ||
		r[5] != "Yıldız Market" || r[6] != "ATL2026000000101" || r[7] != "14.10.2026" || r[8] != "1200.00" {
		t.Fatalf("first line: %v", r)
	}
	// Second line: description falls back to the voucher's, credit side.
	if rows[2][5] != "Satış faturası" || rows[2][9] != "1000.00" {
		t.Fatalf("second line: %v", rows[2])
	}
}

func TestUnbalancedIsBlocked(t *testing.T) {
	e := ae.Entry{ID: "x", No: "9", Date: day, Type: ae.EntryMahsup, Lines: []ae.Line{dr("770", "10"), cr("102", "9")}}
	res, err := open(t).Export(context.Background(), ae.Batch{Entries: []ae.Entry{e}}, ae.NewMemoryWriter())
	if err != nil || !ae.HasBlocker(res.Issues) || len(res.Files) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}
