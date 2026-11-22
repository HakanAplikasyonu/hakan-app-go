package extract

import "testing"

// Yabancı fatura cevabı (#780 A2): geçerli alanlar normalleşir, emin
// olunmayan ve biçimi bozuk alanlar düşer.
func TestParseForeignInvoice(t *testing.T) {
	got, err := ParseForeignInvoice(`Yanıt: {"fields":{
		"vendor":{"value":"Google Ireland  Ltd.","confidence":0.95},
		"country":{"value":"ie","confidence":0.9},
		"invoice_no":{"value":"5123456789","confidence":0.8},
		"invoice_date":{"value":"03.10.2026","confidence":0.9},
		"amount":{"value":"1.250,40","confidence":0.9},
		"currency":{"value":"usd","confidence":0.9},
		"service_kind":{"value":"reklam","confidence":0.7}}}`)
	if err != nil {
		t.Fatal(err)
	}
	want := ForeignInvoice{Vendor: "Google Ireland Ltd.", Country: "IE", InvoiceNo: "5123456789",
		InvoiceDate: "2026-10-03", Amount: "1250.40", Currency: "USD", ServiceKind: "reklam"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	got, err = ParseForeignInvoice(`{"fields":{"vendor":{"value":"AWS","confidence":0.3},"country":{"value":"USA","confidence":0.9},"service_kind":{"value":"yemek","confidence":0.9}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Empty() || got.ServiceKind != "" {
		t.Fatalf("unsure / invalid fields kept: %+v", got)
	}
	if _, err := ParseForeignInvoice("okunamadı"); err == nil {
		t.Fatal("no JSON accepted")
	}
}
