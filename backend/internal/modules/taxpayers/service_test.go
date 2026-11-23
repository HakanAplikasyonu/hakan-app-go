package taxpayers

import (
	"errors"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
)

func TestValidate(t *testing.T) {
	v, typ, err := Validate(CreateInput{Kind: " Legal ", TaxID: "123 456 7890", Title: "  Örnek   A.Ş. ", TaxOffice: " Kadıköy "})
	if err != nil {
		t.Fatal(err)
	}
	if v.Kind != KindLegal || v.TaxID != "1234567890" || v.Title != "Örnek A.Ş." || v.TaxOffice != "Kadıköy" || typ != taxid.VKN {
		t.Fatalf("normalized = %+v %s", v, typ)
	}
	if _, typ, err := Validate(CreateInput{Kind: KindIndividual, TaxID: "10000000146", Title: "Ayşe"}); err != nil || typ != taxid.TCKN {
		t.Fatalf("individual tckn: %v %s", err, typ)
	}
	if _, _, err := Validate(CreateInput{Kind: KindIndividual, TaxID: "1234567890", Title: "Şahıs"}); err != nil {
		t.Fatalf("individual vkn: %v", err)
	}
	for name, in := range map[string]CreateInput{
		"legal with tckn": {Kind: KindLegal, TaxID: "10000000146", Title: "x"},
		"bad kind":        {Kind: "company", TaxID: "1234567890", Title: "x"},
		"bad checksum":    {Kind: KindLegal, TaxID: "1234567891", Title: "x"},
		"too short":       {Kind: KindLegal, TaxID: "123456789", Title: "x"},
		"empty title":     {Kind: KindLegal, TaxID: "1234567890", Title: "   "},
	} {
		if _, _, err := Validate(in); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v, want ErrInvalidRequest", name, err)
		}
	}
}

func TestValidateCertificateExtras(t *testing.T) {
	v, _, err := Validate(CreateInput{Kind: "legal", TaxID: "1234567890", Title: "Örnek A.Ş.",
		TaxOffice: "Kadıköy", Address: "  Moda Cad.  No:1 ", ActivityCode: "471101"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Address != "Moda Cad. No:1" || v.ActivityCode != "47.11.01" {
		t.Fatalf("got %+v", v)
	}
	for _, bad := range []string{"4711", "47.11.0A", "4711011"} {
		if _, _, err := Validate(CreateInput{Kind: "legal", TaxID: "1234567890", Title: "Örnek A.Ş.", ActivityCode: bad}); err == nil {
			t.Errorf("activity code %q accepted", bad)
		}
	}
	if c, ok := normalizeActivityCode("47.11.01"); !ok || c != "47.11.01" {
		t.Fatalf("dotted code: %q %v", c, ok)
	}
}

func TestTaxCertificateAIEnabled(t *testing.T) {
	cases := map[string]bool{"": true, `{}`: true, `{"tax_certificate_ai":true}`: true, `{"tax_certificate_ai":false}`: false}
	for in, want := range cases {
		if got := TaxCertificateAIEnabled([]byte(in)); got != want {
			t.Errorf("%s: %v", in, got)
		}
	}
}

func TestSniffCertificate(t *testing.T) {
	if f, ok := SniffCertificate([]byte("%PDF-1.4\n")); !ok || !f.PDF {
		t.Fatalf("pdf: %+v %v", f, ok)
	}
	heic := append([]byte{0, 0, 0, 24}, []byte("ftypheic0000")...)
	if f, ok := SniffCertificate(heic); !ok || !f.HEIC {
		t.Fatalf("heic: %+v %v", f, ok)
	}
	if _, ok := SniffCertificate([]byte("merhaba")); ok {
		t.Fatal("text accepted")
	}
}
