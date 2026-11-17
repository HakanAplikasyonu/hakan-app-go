package build

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/mapping"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

func TestRecordsNormaliseAndCheckByKind(t *testing.T) {
	sh := parse.Sheet{Name: "Ekstre", Rows: [][]string{
		{"İşlem Tarihi", "Açıklama", "Borç", "Alacak", "Bakiye"},
		{"01.10.2026", "EFT GELEN", "", "1.250,00", "11.250,00"},
		{"02.10.2026", "KİRA", "-3.000,00", "", "8.250,00"},
		{"xx", "HATALI", "10,00", "", ""},
		{"03.10.2026", "", "", "5,00", ""},
	}}
	l, ok := parse.DetectLayout(sh)
	if !ok {
		t.Fatal("layout")
	}
	mp := mapping.Mapping{DataType: mapping.BankTransactions, Columns: []mapping.ColumnMap{
		{Column: "A", Field: "date"}, {Column: "B", Field: "description"}, {Column: "C", Field: "debit"},
		{Column: "D", Field: "credit"}, {Column: "E", Field: "balance"},
	}}
	rows := Records(sh, l, mp, Options{Period: "2026-10"})
	if len(rows) != 4 {
		t.Fatalf("rows %d", len(rows))
	}
	r := rows[0]
	if r.Result != ResultOK || r.Record["date"] != "2026-10-01" || r.Record["credit"] != "1250.00" || r.Record["balance"] != "11250.00" {
		t.Fatalf("row 1 %+v", r)
	}
	// A minus is normal in a statement: no warning.
	if rows[1].Result != ResultOK || rows[1].Record["debit"] != "-3000.00" {
		t.Fatalf("row 2 %+v", rows[1])
	}
	if rows[2].Result != ResultError || rows[2].Messages[0].Code != CodeBadDate {
		t.Fatalf("row 3 %+v", rows[2])
	}
	if rows[3].Result != ResultError || rows[3].Messages[0].Code != CodeRequired {
		t.Fatalf("row 4 %+v", rows[3])
	}
	if s := Summarize(rows); s.OK != 2 || s.Errors != 2 {
		t.Fatalf("summary %+v", s)
	}
}

func TestRecordsTotalAndPeriod(t *testing.T) {
	sh := parse.Sheet{Name: "Masraf", Rows: [][]string{
		{"Tarih", "Firma", "Tutar", "Fiş Sayısı"},
		{"03.10.2026", "Opet", "1.200,00", "3"},
		{"04.11.2026", "Lokanta", "550,00", "1"},
	}}
	l, _ := parse.DetectLayout(sh)
	mp := mapping.Mapping{DataType: mapping.Expenses, Columns: []mapping.ColumnMap{
		{Column: "A", Field: "date"}, {Column: "B", Field: "vendor_name"}, {Column: "C", Field: "total"},
	}}
	rows := Records(sh, l, mp, Options{Period: "2026-10"})
	if rows[1].Result != ResultWarning || rows[1].Messages[0].Code != CodeOutOfPeriod {
		t.Fatalf("out of period %+v", rows[1])
	}
	if s := Summarize(rows); s.Total != "1750.00" || s.Warnings != 1 {
		t.Fatalf("summary %+v", s)
	}
}
