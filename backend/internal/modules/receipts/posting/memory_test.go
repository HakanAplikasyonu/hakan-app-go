package posting

import "testing"

// #780: hesap önerisi sırası — aynı satıcının son hesabı, satıcı adındaki
// anahtar kelime, mükellefin son kullandığı hesap, fiş türü / genel.
func TestSuggestWithTaxpayerMemory(t *testing.T) {
	cats := Merge(DefaultCategories, nil, nil)
	vendor := &Memory{CategoryCode: "yemek", AccountCode: "770.05.001"}
	tp := &Memory{CategoryCode: "genel", AccountCode: "770.99.001"}

	s, ok := SuggestWith(cats, "okc_receipt", "Demo Market", "", vendor, tp, nil)
	if !ok || s.AccountCode != "770.05.001" || s.Source != SourceVendorMemory {
		t.Fatalf("vendor memory first: %+v", s)
	}
	s, ok = SuggestWith(cats, "okc_receipt", "OPET Petrolcülük", "", nil, tp, nil)
	if !ok || s.CategoryCode != "akaryakit" || s.Source == SourceTaxpayerMemory {
		t.Fatalf("a keyword match beats the taxpayer memory: %+v", s)
	}
	s, ok = SuggestWith(cats, "okc_receipt", "Demo Market", "", nil, tp, nil)
	if !ok || s.AccountCode != "770.99.001" || s.Source != SourceTaxpayerMemory || s.CategoryCode != "genel" {
		t.Fatalf("taxpayer memory: %+v", s)
	}
	s, ok = SuggestWith(cats, "okc_receipt", "Demo Market", "", nil, nil, nil)
	if !ok || s.Source == SourceTaxpayerMemory {
		t.Fatalf("no memory: category suggestion expected, got %+v", s)
	}
}
