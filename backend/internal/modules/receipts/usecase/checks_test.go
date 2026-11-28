package usecase

import "testing"

// Fiş yapay zekâ kontrolleri: alıcı mükellef mi?
func TestBuyerMatch(t *testing.T) {
	cases := []struct {
		name, buyerID, buyerName, taxID, title, want string
	}{
		{"vkn aynı", "1234567890", "", "1234567890", "Deniz Gıda Ltd. Şti.", "match"},
		{"vkn farklı", "9999999999", "Deniz Gıda", "1234567890", "Deniz Gıda Ltd. Şti.", "mismatch"},
		{"vkn okunmuş, mükellefin yok", "9999999999", "", "", "Deniz", ""},
		{"alıcı yok", "", "", "1234567890", "Deniz Gıda", ""},
		{"ad ortak kelime", "", "DENİZ GIDA SAN. TİC. LTD. ŞTİ.", "1234567890", "Deniz Gıda Ltd. Şti.", "match"},
		{"ad büyük İ/ı", "", "IŞIK İNŞAAT", "1", "Işık İnşaat A.Ş.", "match"},
		{"ad başka", "", "Yıldız Market A.Ş.", "1234567890", "Deniz Gıda Ltd. Şti.", "mismatch"},
		{"yalnız şirket ekleri", "", "LTD. ŞTİ.", "1234567890", "Deniz Gıda Ltd. Şti.", ""},
	}
	for _, c := range cases {
		if got := BuyerMatch(c.buyerID, c.buyerName, c.taxID, c.title); got != c.want {
			t.Errorf("%s: BuyerMatch = %q, want %q", c.name, got, c.want)
		}
	}
}
