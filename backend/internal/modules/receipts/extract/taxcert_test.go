package extract

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/provider"
)

// A fake model answer with every field read (vergi levhası of a company).
const certAllAnswer = `{"fields":{
"tax_id":{"value":"123 456 7890","confidence":0.97},
"title":{"value":"ÖRNEK  GIDA TİCARET A.Ş.","confidence":0.95},
"tax_office":{"value":"Kadıköy Vergi Dairesi","confidence":0.92},
"address":{"value":"Caferağa Mah. Moda Cad. No:12  Kadıköy/İSTANBUL","confidence":0.9},
"activity_code":{"value":"471101","confidence":0.88}}}`

func TestParseTaxCertificateAllFields(t *testing.T) {
	c, err := ParseTaxCertificate("İşte: " + certAllAnswer)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		CertTaxID: "1234567890", CertTitle: "ÖRNEK GIDA TİCARET A.Ş.", CertTaxOffice: "Kadıköy",
		CertAddress: "Caferağa Mah. Moda Cad. No:12 Kadıköy/İSTANBUL", CertActivityCode: "47.11.01",
	}
	for f, v := range want {
		got := c.Fields[f]
		if got.Value != v || got.Check || got.Problem != "" {
			t.Errorf("%s = %+v, want %q without check", f, got, v)
		}
	}
	if c.TaxIDType != "vkn" || c.Kind != "legal" {
		t.Fatalf("type %q kind %q", c.TaxIDType, c.Kind)
	}
	if len(c.CheckFields()) != 0 {
		t.Fatalf("check fields %v", c.CheckFields())
	}
}

func TestParseTaxCertificateMissingFieldsAreChecked(t *testing.T) {
	c, err := ParseTaxCertificate(`{"fields":{
"tax_id":{"value":"10000000146","confidence":0.95},
"title":{"value":"AYŞE YILMAZ","confidence":0.6},
"tax_office":{"value":"","confidence":0},
"activity_code":{"value":"4711","confidence":0.9}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.CheckFields(); !reflect.DeepEqual(got, []string{CertTitle, CertTaxOffice, CertAddress, CertActivityCode}) {
		t.Fatalf("check fields %v", got)
	}
	if p := c.Fields[CertTaxOffice].Problem; p != CertMissing {
		t.Errorf("tax_office problem %q", p)
	}
	if p := c.Fields[CertAddress].Problem; p != CertMissing {
		t.Errorf("address problem %q", p)
	}
	if f := c.Fields[CertTitle]; f.Problem != CertLowConfidence || f.Value != "AYŞE YILMAZ" {
		t.Errorf("title %+v", f)
	}
	if f := c.Fields[CertActivityCode]; f.Problem != CertInvalid || f.Value != "" {
		t.Errorf("activity_code %+v", f)
	}
	// A TCKN is always a gerçek kişi.
	if c.TaxIDType != "tckn" || c.Kind != "individual" {
		t.Fatalf("type %q kind %q", c.TaxIDType, c.Kind)
	}
}

func TestParseTaxCertificateInvalidVKN(t *testing.T) {
	c, err := ParseTaxCertificate(`{"fields":{"tax_id":{"value":"1234567891","confidence":0.99},
"title":{"value":"Deneme Ltd. Şti.","confidence":0.9}}}`)
	if err != nil {
		t.Fatal(err)
	}
	id := c.Fields[CertTaxID]
	// The digits stay so the office can fix the wrong one; the check flag is on.
	if id.Value != "1234567891" || !id.Check || id.Problem != CertInvalid {
		t.Fatalf("tax_id %+v", id)
	}
	if c.TaxIDType != "" || c.Kind != "" {
		t.Fatalf("type %q kind %q", c.TaxIDType, c.Kind)
	}
	if _, err := ParseTaxCertificate("okunamadı"); err == nil {
		t.Fatal("answer without JSON accepted")
	}
}

func TestReadTaxCertificateUsesReceiptAI(t *testing.T) {
	ai := &fakeAI{answers: []string{certAllAnswer}}
	c, err := New(ai).ReadTaxCertificate(context.Background(), CertRequest{
		OrganizationID: 7, UserID: 3, Text: "VERGİ LEVHASI ... VERGİ KİMLİK NO 1234567890",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Model != "claude-x" || c.Fields[CertTaxID].Value != "1234567890" {
		t.Fatalf("result %+v", c)
	}
	call := ai.calls[0]
	if call.Purpose != "tax_certificate_ocr" || call.TaxpayerID != nil || call.OrganizationID != 7 {
		t.Fatalf("call %+v", call)
	}
	// A PDF goes as its text, never as an image block.
	for _, b := range call.Messages[0].Content {
		if b.Type == provider.BlockImage {
			t.Fatal("PDF text sent as image")
		}
	}
	if !strings.Contains(call.Messages[0].Content[0].Text, "1234567890") {
		t.Fatal("PDF text missing")
	}

	if _, err := New(nil).ReadTaxCertificate(context.Background(), CertRequest{}); !errors.Is(err, ErrAIUnavailable) {
		t.Fatalf("no AI: %v", err)
	}
}

func TestPDFTextRejectsNonPDF(t *testing.T) {
	if _, err := PDFText([]byte("not a pdf")); err == nil {
		t.Fatal("non-PDF accepted")
	}
}
