package statement

import (
	"context"
	"errors"
	"testing"
)

// Bilinmeyen düzende bir CSV: başlıklar eşanlamlılarda ve sezgide yok;
// devreden satırı, bozuk bir tutar ve bir toplam satırı var.
const previewCSV = "Gün;Metin;Para;Son Durum\n" +
	"DEVREDEN;;;1.000,00\n" +
	"01.10.2026;FAST GELEN YILDIZ MARKET;500,00;1.500,00\n" +
	"02.10.2026;HESAP İŞLETİM ÜCRETİ;on iki elli;1.487,50\n" +
	"03.10.2026;EFT GİDEN KIRA;-400,00;1.087,50\n" +
	"Toplam;;87,50;\n" +
	"Not: bu satır okunmaz;;;\n"

var previewColumns = []TemplateColumn{
	{Header: "Gün", Field: FieldDate},
	{Header: "Metin", Field: FieldDescription},
	{Header: "Para", Field: FieldAmount},
	{Header: "Son Durum", Field: FieldBalance},
}

func TestAnalyzeReturnsPreviewRows(t *testing.T) {
	a, err := AnalyzeFile(context.Background(), "ekstre.csv", []byte(previewCSV), nil, nil, ColumnRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if a.HeaderRow != 1 || len(a.Columns) != 4 {
		t.Fatalf("header %d columns %+v", a.HeaderRow, a.Columns)
	}
	// Bütün satırlar (başlığın altı, boş olmayan) önizlemede; hücreler kolon sırasında.
	if len(a.Rows) != 6 {
		t.Fatalf("rows %d: %+v", len(a.Rows), a.Rows)
	}
	if a.Rows[1].Row != 3 || a.Rows[1].Cells[1] != "FAST GELEN YILDIZ MARKET" || a.Rows[1].Cells[2] != "500,00" {
		t.Fatalf("row 3: %+v", a.Rows[1])
	}
}

func TestPreviewRowKinds(t *testing.T) {
	text, _ := Decode([]byte(previewCSV))
	rows, err := ReadCSV(text)
	if err != nil {
		t.Fatal(err)
	}
	o := Overrides{Columns: previewColumns}
	tpl, err := o.template("csv")
	if err != nil {
		t.Fatal(err)
	}
	cs := []ColumnSuggestion{{Index: 0}, {Index: 1}, {Index: 2}, {Index: 3}}
	got := previewRows(rows, 0, "csv", cs, tpl)
	want := []string{RowKindError, RowKindMovement, RowKindError, RowKindMovement, RowKindSkip, RowKindSkip}
	if len(got) != len(want) {
		t.Fatalf("rows %d: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].Kind != w {
			t.Errorf("row %d kind %q, want %q (%s)", got[i].Row, got[i].Kind, w, got[i].Reason)
		}
	}
	// Eşleme yokken satırlar sınıflanmaz.
	for _, p := range previewRows(rows, 0, "csv", cs, Template{}) {
		if p.Kind != "" {
			t.Fatalf("no mapping, kind %q", p.Kind)
		}
	}
}

func TestParseWithOverrides(t *testing.T) {
	// Devreden satırı dışarıda, bozuk tutar düzeltilir.
	o := Overrides{
		Columns:  previewColumns,
		SkipRows: []int{2},
		Edits:    []CellEdit{{Row: 4, Column: 2, Value: "-12,50"}},
	}
	st, err := parseWithOverrides("csv", []byte(previewCSV), nil, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Lines) != 3 || len(st.Errors) != 0 {
		t.Fatalf("lines %d errors %+v", len(st.Lines), st.Errors)
	}
	if st.Lines[1].Amount != "12.5" || st.Lines[1].Description != "HESAP İŞLETİM ÜCRETİ" {
		t.Fatalf("edited row: %+v", st.Lines[1].Transaction)
	}
	if g := CheckRunning(st.Lines); g != nil {
		t.Fatalf("running balance: %s", g.Reason)
	}
	// Düzeltme yapılmazsa bozuk satır hata olarak kalır, öteki satırlar okunur.
	st, err = parseWithOverrides("csv", []byte(previewCSV), nil, Overrides{Columns: previewColumns, SkipRows: []int{2}})
	if err != nil || len(st.Lines) != 2 || len(st.Errors) != 1 || st.Errors[0].Row != 4 {
		t.Fatalf("without edit: %d lines, errors %+v, %v", len(st.Lines), st.Errors, err)
	}
	// Aynı dosyanın kendi düzeniyle (eşlemesiz) okunamadığı görülür.
	if _, _, err := Parse("ekstre.csv", []byte(previewCSV)); !errors.Is(err, ErrLayoutUnknown) {
		t.Fatalf("unknown layout without a mapping: %v", err)
	}
}

func TestInvalidOverrides(t *testing.T) {
	cases := map[string]Overrides{
		"no date column": {Columns: []TemplateColumn{
			{Header: "Gün", Field: FieldNone}, {Header: "Metin", Field: FieldDescription},
			{Header: "Para", Field: FieldAmount}, {Header: "Son Durum", Field: FieldBalance}}},
		"edit on the header": {Columns: previewColumns, Edits: []CellEdit{{Row: 1, Column: 0, Value: "Tarih"}}},
		"skip outside":       {Columns: previewColumns, SkipRows: []int{99}},
	}
	for name, o := range cases {
		if _, err := parseWithOverrides("csv", []byte(previewCSV), nil, o); !errors.Is(err, ErrInvalidOverrides) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Başlıkları dosyada olmayan eşleme: düzen bulunamaz.
	other := []TemplateColumn{{Header: "Tarih", Field: FieldDate}, {Header: "Tutar", Field: FieldAmount}}
	if _, err := parseWithOverrides("csv", []byte(previewCSV), nil, Overrides{Columns: other}); !errors.Is(err, ErrLayoutUnknown) {
		t.Errorf("foreign header: %v", err)
	}
}
