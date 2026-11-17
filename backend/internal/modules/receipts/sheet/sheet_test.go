package sheet

import "testing"

func TestWriteReadRoundTrip(t *testing.T) {
	rows := []Row{
		{"date": "14.10.2026", "vendor_name": "Opet", "vendor_tax_number": "1234567890", "total": "1200", "tax_amount": "200", "status": "Bekliyor"},
		{"date": "15.10.2026", "vendor_name": "Köşe Lokantası", "total": "350.5"},
	}
	data, err := Write("Masraflar", ExpenseColumns, rows)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read("m.xlsx", data, ExpenseColumns)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Line != 2 || got[0].Row["vendor_name"] != "Opet" || got[1].Row["total"] != "350.5" {
		t.Fatalf("rows %+v", got)
	}
	if a, ok := Amount(got[0].Row["total"]); !ok || a.String() != "1200" {
		t.Fatalf("amount %v", a)
	}
	// Durum sütunu ve sayı yazımı anahtarı değiştirmez.
	again, _ := Read("m.xlsx", data, ExpenseColumns)
	rows[0]["status"] = "Onaylı"
	rows[0]["total"] = "1200.00"
	data2, _ := Write("Masraflar", ExpenseColumns, rows)
	third, _ := Read("m.xlsx", data2, ExpenseColumns)
	if again[0].Key != third[0].Key || got[0].Key == got[1].Key {
		t.Fatal("row key must ignore status and differ between rows")
	}
}

func TestReadCSVWithAliasesAndTitleRows(t *testing.T) {
	csv := "Ekim masrafları\n\nFiş Tarihi;Firma;Tutar;KDV;Ödeme Şekli\n01.10.2026;ABC Kırtasiye;1.200;200;Kredi Kartı\n;;;;\nToplam;;1.200;200;\n"
	got, err := Read("m.csv", []byte(csv), ExpenseColumns)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Row["vendor_name"] != "ABC Kırtasiye" {
		t.Fatalf("rows %+v", got)
	}
	if a, _ := Amount(got[0].Row["total"]); a.String() != "1200" {
		t.Fatalf("TR thousands: %v", a)
	}
	if m, ok := PaymentMethod(got[0].Row["payment_method"]); !ok || m != "credit_card" {
		t.Fatalf("payment %q", m)
	}
	if _, err := Read("x.csv", []byte("a;b\n1;2\n"), ExpenseColumns); err != ErrNoHeader {
		t.Fatalf("no header: %v", err)
	}
}
