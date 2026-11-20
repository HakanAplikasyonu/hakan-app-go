package customers

import (
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

func TestParty(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       map[string]string
		vkn, tck string
		typ, err string
	}{
		{map[string]string{"name": "Atlas A.Ş.", "tax_id": "123 456 7890", "type": "Müşteri (120)"}, "1234567890", "", "customer", ""},
		{map[string]string{"name": "Ayşe Yılmaz", "tax_id": "12345678901", "type": "Tedarikçi"}, "", "12345678901", "supplier", ""},
		{map[string]string{"name": "Beta Ltd", "tax_id": "1234567890"}, "1234567890", "", "both", ""},
		{map[string]string{"name": "Beta Ltd", "tax_id": "1234567890", "type": "Her ikisi"}, "1234567890", "", "both", ""},
		{map[string]string{"name": "", "tax_id": "1234567890"}, "", "", "", "unvan boş"},
		{map[string]string{"name": "X", "tax_id": "12345"}, "", "", "", "VKN 10, TCKN 11 haneli olmalı"},
		{map[string]string{"name": "X", "tax_id": "12345A7890"}, "", "", "", "VKN 10, TCKN 11 haneli olmalı"},
		{map[string]string{"name": "X", "tax_id": "1234567890", "type": "personel"}, "", "", "", errType.Error()},
	}
	for _, c := range cases {
		cp, typ, reason := party(c.in)
		if reason != c.err {
			t.Fatalf("%v: reason %q, want %q", c.in, reason, c.err)
		}
		if reason != "" {
			continue
		}
		if cp.TaxNumber != c.vkn || cp.IdentityNumber != c.tck || typ != c.typ {
			t.Fatalf("%v: got %q %q %q", c.in, cp.TaxNumber, cp.IdentityNumber, typ)
		}
	}
}

func TestHeuristic(t *testing.T) {
	t.Parallel()
	l := parse.Layout{HeaderRow: 1, Columns: []parse.Column{
		{Letter: "A", Header: "Ünvan"}, {Letter: "B", Header: "VKN/TCKN"}, {Letter: "C", Header: "Cari Türü"},
		{Letter: "D", Header: "Vergi Dairesi"}, {Letter: "E", Header: "Not"},
	}}
	m := Target{}.Heuristic(l)
	want := map[string]string{"A": "name", "B": "tax_id", "C": "type", "D": "tax_office", "E": ""}
	for _, c := range m.Columns {
		if c.Field != want[c.Column] {
			t.Fatalf("column %s (%s): %q, want %q", c.Column, c.Header, c.Field, want[c.Column])
		}
	}
}
