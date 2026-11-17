package target

import (
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

func TestCSVKeepsRowNumbers(t *testing.T) {
	data, lines, err := CSV([][]string{{"Hesap IBAN", "TR1"}}, []Column{{"date", "Tarih"}, {"total", "Toplam"}},
		[]Row{{RowNo: 7, Values: map[string]string{"date": "2026-10-01", "total": "12.50"}}, {RowNo: 9, Values: map[string]string{"date": "2026-10-02"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := "Hesap IBAN;TR1\nTarih;Toplam\n2026-10-01;12.50\n2026-10-02;\n"
	if string(data) != want || lines[3] != 7 || lines[4] != 9 {
		t.Fatalf("%q %v", data, lines)
	}
	if CSVName(`C:\\x\\Ekim masraf.xlsx`) != "Ekim masraf.csv" || CSVName("") != "excel-ice-aktarma.csv" {
		t.Fatal(CSVName(`C:\\x\\Ekim masraf.xlsx`))
	}
}

func TestMatchHeadersFirstColumnWins(t *testing.T) {
	l := parse.Layout{HeaderRow: 1, Columns: []parse.Column{{Letter: "A", Header: "Tarih"}, {Letter: "B", Header: "Tarih 2"}, {Letter: "C", Header: "Tutar"}}}
	mp := MatchHeaders("expenses", l, func(h string) string {
		switch {
		case strings.HasPrefix(h, "Tarih"):
			return "date"
		case h == "Tutar":
			return "total"
		}
		return ""
	})
	if mp.Columns[0].Field != "date" || mp.Columns[1].Field != "" || mp.Columns[2].Field != "total" {
		t.Fatalf("%+v", mp.Columns)
	}
}
