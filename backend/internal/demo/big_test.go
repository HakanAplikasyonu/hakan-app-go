package demo

import (
	"strings"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxid"
)

// Büyük demo kimlikleri sağlamayı geçer, sabit önekte kalır, tekrar etmez
// ve her çalışmada aynıdır.
func TestBigIdentities(t *testing.T) {
	seen := map[string]bool{}
	for _, tp := range Taxpayers {
		seen[tp.TaxID] = true
	}
	for o := range BigOffices {
		a, b := GenerateTaxpayers(o, 40), GenerateTaxpayers(o, 40)
		for i, tp := range a {
			if tp.TaxID != b[i].TaxID || tp.Title != b[i].Title || tp.Size != b[i].Size {
				t.Fatalf("office %d taxpayer %d is not deterministic", o, i)
			}
			switch tp.Kind {
			case "legal":
				if !taxid.ValidVKN(tp.TaxID) || !strings.HasPrefix(tp.TaxID, BigVKNPrefix) {
					t.Errorf("VKN %s", tp.TaxID)
				}
			case "individual":
				if !taxid.ValidTCKN(tp.TaxID) || !strings.HasPrefix(tp.TaxID, BigTCKNPrefix) {
					t.Errorf("TCKN %s", tp.TaxID)
				}
			default:
				t.Errorf("kind %q", tp.Kind)
			}
			if seen[tp.TaxID] {
				t.Errorf("duplicate tax id %s", tp.TaxID)
			}
			seen[tp.TaxID] = true
			if !strings.HasSuffix(tp.Email, ".example.com") || !strings.HasPrefix(tp.Phone, "0555 000 ") {
				t.Errorf("contact %s %s", tp.Email, tp.Phone)
			}
			if e := EmployeeTCKN(o, i, 3); !taxid.ValidTCKN(e) || seen[e] {
				t.Errorf("employee TCKN %s", e)
			}
		}
	}
	for _, c := range GenerateCounterparties(240) {
		if !taxid.ValidVKN(c.TaxID) || seen[c.TaxID] {
			t.Errorf("counterparty VKN %s", c.TaxID)
		}
		seen[c.TaxID] = true
		if len(c.IBAN) != 26 || !strings.HasPrefix(c.IBAN, "TR") {
			t.Errorf("IBAN %s", c.IBAN)
		}
	}
	for _, o := range BigOffices[1:] {
		for _, m := range append(o.Members, o.TaxpayerUser) {
			if !strings.HasSuffix(m.Email, ".test") {
				t.Errorf("email %s", m.Email)
			}
		}
	}
}
