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
