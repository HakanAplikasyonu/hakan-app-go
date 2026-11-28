package banking

import "testing"

// Descriptions are made up; they only follow the shape of bank texts.
func TestDetectMarketplace(t *testing.T) {
	for _, c := range []struct {
		desc    string
		key     string // "" = no channel
		keyword string
		temlik  bool
	}{
		{"EFT GELEN TRENDYOL HAKEDIS ODEMESI", "trendyol", "TRENDYOL", false},
		{"fast gelen trendyol satis bedeli", "trendyol", "TRENDYOL", false},
		{"Trendyol'dan gelen ödeme", "trendyol", "TRENDYOL", false},
		{"DSM GRUP DANISMANLIK ILETISIM VE SATIS TIC AS", "trendyol", "DSM GRUP", false},
		{"HAVALE DSM 2026-10 HAKEDIS", "trendyol", "DSM", false},
		{"D-MARKET ELEKTRONIK HIZMETLER VE TIC AS ODEME", "hepsiburada", "D-MARKET", false},
		{"D.MARKET ELEKTRONİK HAKEDİŞ", "hepsiburada", "D-MARKET", false},
		{"hepsiburada.com satış tahsilatı", "hepsiburada", "HEPSİBURADA", false},
		{"HEPSİBURADA SATICI ÖDEMESİ", "hepsiburada", "HEPSİBURADA", false},
		{"hepsıburada odeme", "hepsiburada", "HEPSİBURADA", false},
		{"AMAZON TURKEY PERAKENDE HIZMETLERI", "amazon", "AMAZON", false},
		{"N11.COM HAKEDIS", "n11", "N11", false},
		{"DOĞUŞ PLANET ELEKTRONİK TİC", "n11", "DOĞUŞ PLANET", false},
		{"ÇİÇEKSEPETİ İNTERNET HİZMETLERİ A.Ş.", "ciceksepeti", "ÇİÇEKSEPETİ", false},
		{"ciceksepeti odeme", "ciceksepeti", "ÇİÇEKSEPETİ", false},
		{"Çiçek Sepeti hakediş", "ciceksepeti", "ÇİÇEK SEPETİ", false},
		{"PAZARAMA SATIS ODEMESI", "pazarama", "PAZARAMA", false},
		{"TEMLİK DSM GRUP ALACAK ODEMESI", "trendyol", "DSM GRUP", true},
		{"temlik hepsiburada alacağı", "hepsiburada", "HEPSİBURADA", true},
		// Word boundaries: no channel inside another word.
		{"DSMX LOJISTIK ODEME", "", "", false},
		{"TRENDYOLCU LTD", "", "", false},
		{"N110 FATURA", "", "", false},
		{"TEMLIK ODEMESI XYZ LTD", "", "", false},
		{"EFT GELEN ABC GIDA SAN TIC AS FATURA", "", "", false},
		{"", "", "", false},
	} {
		hit, ok := DetectMarketplace(c.desc)
		if c.key == "" {
			if ok {
				t.Errorf("%q: got %s (%s), want none", c.desc, hit.Marketplace.Key, hit.Keyword)
			}
			continue
		}
		if !ok || hit.Marketplace.Key != c.key || hit.Keyword != c.keyword || hit.Temlik != c.temlik {
			t.Errorf("%q: got %v %s %q temlik=%v, want %s %q temlik=%v",
				c.desc, ok, hit.Marketplace.Key, hit.Keyword, hit.Temlik, c.key, c.keyword, c.temlik)
		}
	}
}

func TestMarketplaceNameMatches(t *testing.T) {
	trendyol := Marketplaces[0]
	hb := Marketplaces[1]
	for _, c := range []struct {
		m    Marketplace
		name string
		want bool
	}{
		{trendyol, "TRENDYOL", true},
		{trendyol, "Trendyol Satış", true},
		{trendyol, "DSM GRUP DANIŞMANLIK İLETİŞİM VE SATIŞ TİC. A.Ş.", true},
		{trendyol, "HEPSİBURADA", false},
		{hb, "D-MARKET ELEKTRONİK HİZMETLER", true},
		{hb, "Hepsiburada", true},
		{hb, "ABC GIDA", false},
	} {
		if got := c.m.NameMatches(c.name); got != c.want {
			t.Errorf("%s.NameMatches(%q) = %v", c.m.Key, c.name, got)
		}
	}
}

// Every pattern must survive normalization (no empty pattern that would
// match nothing, no duplicate key).
func TestMarketplaceTable(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Marketplaces {
		if m.Key == "" || m.Name == "" || seen[m.Key] {
			t.Errorf("bad row %+v", m)
		}
		seen[m.Key] = true
		for _, p := range m.Patterns {
			if NormalizeDescription(p) == "" {
				t.Errorf("%s: pattern %q is empty after normalization", m.Key, p)
			}
			if hit, ok := DetectMarketplace(p); !ok || hit.Marketplace.Key != m.Key {
				t.Errorf("%s: pattern %q is shadowed by %s", m.Key, p, hit.Marketplace.Key)
			}
		}
	}
}
