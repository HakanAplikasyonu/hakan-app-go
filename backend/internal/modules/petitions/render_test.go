package petitions

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

var base = Data{
	Taxpayer: Taxpayer{Title: "Alfa Gıda A.Ş.", TaxID: "1234567890", TaxIDType: "VKN", TaxOffice: "KADIKÖY"},
	Office:   Office{Name: "Örnek SMMM"},
	Signer:   Signer{Name: "Ayşe Yılmaz", SMMMRegistryNo: "34-12345"},
}

func sample(t Template) map[string]string {
	f := map[string]string{}
	for _, fl := range t.Fields {
		switch fl.Type {
		case FieldDate:
			f[fl.Key] = "2026-11-16"
		case FieldTextarea:
			f[fl.Key] = "Birinci paragraf.\nİkinci paragraf."
		default:
			f[fl.Key] = "1"
		}
	}
	return f
}

// Every template renders with sample fields, is marked draft and prints.
func TestEveryTemplateRendersAsDraft(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	keys := map[string]bool{}
	for _, tpl := range Templates() {
		if keys[tpl.Key] {
			t.Fatalf("duplicate key %s", tpl.Key)
		}
		keys[tpl.Key] = true
		r, _, err := Render(tpl.Key, sample(tpl), base, now)
		if err != nil {
			t.Fatalf("%s: %v", tpl.Key, err)
		}
		if !r.Draft || r.DraftNotice != DraftNotice {
			t.Errorf("%s: not marked as draft (legal texts are drafts until reviewed)", tpl.Key)
		}
		if r.Recipient == "" || r.Body == "" || r.Date != "16.11.2026" || r.Petitioner != "Alfa Gıda A.Ş." {
			t.Errorf("%s: incomplete %+v", tpl.Key, r)
		}
		if !strings.Contains(r.Signer, "34-12345") || !strings.Contains(r.Signer, "Örnek SMMM") {
			t.Errorf("%s: signer %q", tpl.Key, r.Signer)
		}
		pdf, err := PDF(r)
		if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) || len(pdf) < 1000 {
			t.Errorf("%s: pdf %d bytes, %v", tpl.Key, len(pdf), err)
		}
	}
}

func TestFillsTaxpayerFields(t *testing.T) {
	r, _, err := Render("vd_borc_durum_yazisi", map[string]string{"amac": "  ihale   komisyonuna "}, base, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1234567890 VKN", "Alfa Gıda A.Ş.", "ihale komisyonuna"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("body %q lacks %q", r.Body, want)
		}
	}
	if r.Recipient != "KADIKÖY VERGİ DAİRESİ MÜDÜRLÜĞÜNE" {
		t.Errorf("recipient %q", r.Recipient)
	}
	noOffice := base
	noOffice.Taxpayer.TaxOffice = ""
	r, _, _ = Render("vd_borc_durum_yazisi", map[string]string{"amac": "x"}, noOffice, time.Now())
	if !strings.HasPrefix(r.Recipient, "....") {
		t.Errorf("missing tax office should print dots: %q", r.Recipient)
	}
}

func TestFieldValidation(t *testing.T) {
	cases := map[string]struct {
		key    string
		fields map[string]string
	}{
		"unknown template": {"yok", nil},
		"missing required": {"vd_borc_durum_yazisi", map[string]string{}},
		"unknown field":    {"vd_borc_durum_yazisi", map[string]string{"amac": "x", "ekstra": "y"}},
		"too long":         {"vd_mukellefiyet_yazisi", map[string]string{"adet": "12345"}},
		"control char":     {"vd_borc_durum_yazisi", map[string]string{"amac": "a\x00b"}},
	}
	for name, tc := range cases {
		if _, _, err := Render(tc.key, tc.fields, base, time.Now()); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	// Template syntax in user input is printed, not executed.
	r, _, err := Render("genel_dilekce", map[string]string{"muhatap": "X", "konu": "{{.Signer.Name}}", "metin": "{{.Taxpayer.TaxID}}"}, base, time.Now())
	if err != nil || r.Subject != "{{.Signer.Name}}" || r.Body != "{{.Taxpayer.TaxID}}" {
		t.Fatalf("user input was executed as a template: %+v %v", r, err)
	}
}
