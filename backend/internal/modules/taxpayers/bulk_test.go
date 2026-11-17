package taxpayers

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

const listCSV = `Mükellef listesi (Kasım),,,,
Ünvan;VKN / TCKN;Vergi Dairesi;E-posta
Alfa Gıda A.Ş.;123 456 78 90;Kadıköy;muhasebe@alfa.test
Selin Yılmaz;10000000146;Çankaya;SELIN@example.test
Bozuk Vkn Ltd.;1234567891;Beşiktaş;b@example.test
E-postasız A.Ş.;1111111114;Şişli;
Tekrar A.Ş.;1234567890;Kadıköy;tekrar@example.test
;;;
TOPLAM;;;
`

func TestParseTaxpayerListCSV(t *testing.T) {
	rows, err := ParseTaxpayerList("liste.csv", []byte(strings.ReplaceAll(listCSV, ",", ";")))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5 (blank and total rows skipped): %+v", len(rows), rows)
	}
	alfa, selin, bozuk, epostasiz, tekrar := rows[0], rows[1], rows[2], rows[3], rows[4]
	if !alfa.Valid() || alfa.TaxID != "1234567890" || alfa.Kind != KindLegal || alfa.Row != 3 || alfa.TaxOffice != "Kadıköy" {
		t.Errorf("alfa = %+v", alfa)
	}
	if !selin.Valid() || selin.Kind != KindIndividual || selin.ContactEmail != "selin@example.test" {
		t.Errorf("selin = %+v", selin)
	}
	if bozuk.Valid() || bozuk.Errors[0].Field != "tax_id" || bozuk.Errors[0].Code != RowErrInvalid {
		t.Errorf("bad check digit = %+v", bozuk)
	}
	// Ofis Türkçe gerekçe görür; Go hata zinciri ("invalid request: tax_id: …") değil (#397).
	if got, want := bozuk.Errors[0].Message, "VKN/TCKN geçersiz: sağlama hanesi tutmuyor"; got != want {
		t.Errorf("bad check digit message = %q, want %q", got, want)
	}
	if epostasiz.Valid() || epostasiz.Errors[0].Field != "contact_email" || epostasiz.Errors[0].Code != RowErrRequired {
		t.Errorf("no email = %+v", epostasiz)
	}
	if tekrar.Valid() || tekrar.Errors[0].Code != RowErrDuplicate || !strings.Contains(tekrar.Errors[0].Message, "3.") {
		t.Errorf("duplicate = %+v", tekrar)
	}
}

func TestParseTaxpayerListXLSX(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	data := [][]any{
		{"VKN", "Firma Adı", "Email", "Tür"},
		{"1234567890", "Alfa Gıda A.Ş.", "a@alfa.test", "Tüzel"},
		{"10000000146", "Selin Yılmaz", "s@example.test", "Gerçek"},
	}
	for i, row := range data {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	rows, err := ParseTaxpayerList("liste.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].Valid() || !rows[1].Valid() || rows[1].Kind != KindIndividual || rows[0].Row != 2 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestParseTaxpayerListErrors(t *testing.T) {
	if _, err := ParseTaxpayerList("x.csv", []byte("Ad;Telefon\nA;1\n")); !errors.Is(err, ErrImportNoHeader) {
		t.Errorf("no header: %v", err)
	}
	var b strings.Builder
	b.WriteString("VKN;Unvan;E-posta\n")
	for i := 0; i <= MaxImportRows; i++ {
		b.WriteString("1234567890;A;a@b.test\n")
	}
	if _, err := ParseTaxpayerList("x.csv", []byte(b.String())); !errors.Is(err, ErrImportTooLarge) {
		t.Errorf("too large: %v", err)
	}
}

func TestValidationMessageTurkish(t *testing.T) {
	cases := []struct {
		in   CreateInput
		want string
	}{
		{CreateInput{Kind: KindLegal, TaxID: "123", Title: "A"}, "VKN 10, TCKN 11 haneli olmalı"},
		{CreateInput{Kind: KindLegal, TaxID: "1234567891", Title: "A"}, "VKN/TCKN geçersiz: sağlama hanesi tutmuyor"},
		{CreateInput{Kind: KindLegal, TaxID: "10000000146", Title: "A"}, "tüzel kişi için 10 haneli VKN gerekli"},
		{CreateInput{Kind: KindLegal, TaxID: "1234567890", Title: strings.Repeat("a", maxTitleLen+1)}, "unvan boş ya da 255 karakterden uzun"},
		{CreateInput{Kind: KindLegal, TaxID: "1234567890", Title: "A", TaxOffice: strings.Repeat("a", maxTaxOfficeLen+1)}, "vergi dairesi en çok 128 karakter olabilir"},
	}
	for _, c := range cases {
		_, _, err := Validate(c.in)
		if err == nil {
			t.Fatalf("Validate(%+v) = nil", c.in)
		}
		if !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("Validate(%+v) = %v, not ErrInvalidRequest", c.in, err)
		}
		if got := validationMessage(err); got != c.want {
			t.Errorf("validationMessage(%v) = %q, want %q", err, got, c.want)
		}
	}
}
