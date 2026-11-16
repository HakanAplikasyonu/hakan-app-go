package mask

import (
	"strings"
	"testing"
)

// Serbest metindeki adlar, kayıtlı ad listesi olmadan da maskelenir (#169).
func TestMaskFreeTextNames(t *testing.T) {
	cases := []struct {
		in     string
		hidden []string
		kept   []string
	}{
		{
			in:     "Ödeme Ayşe Yılmaz tarafından yapıldı.",
			hidden: []string{"Ayşe", "Yılmaz"}, kept: []string{"Ödeme", "tarafından yapıldı."},
		},
		{
			in:     "MEHMET ALİ KAYA adına kesilen fatura",
			hidden: []string{"MEHMET", "KAYA"}, kept: []string{"adına kesilen fatura"},
		},
		{
			in:     "Sayın Kerimoğlu, ekteki dekontu inceleyiniz.",
			hidden: []string{"Kerimoğlu"}, kept: []string{"Sayın", "ekteki dekontu"},
		},
		{
			in:     "Muhasebeden Tarkan Bey aradı.",
			hidden: []string{"Tarkan"}, kept: []string{"Bey aradı", "Muhasebeden"},
		},
		{
			in:     "Ad Soyad: Zarife Kocabaş\nTutar: 1.250,00",
			hidden: []string{"Zarife", "Kocabaş"}, kept: []string{"Ad Soyad:", "Tutar: 1.250,00"},
		},
		{
			in:     "Satıcı: Kuzey Yıldızı Gıda San. ve Tic. Ltd. Şti. (Kadıköy)",
			hidden: []string{"Kuzey", "Gıda", "Ltd"}, kept: []string{"Satıcı:", "(Kadıköy)"},
		},
		{
			in:     "MİGROS TİCARET A.Ş.\nTOPLAM *125,00",
			hidden: []string{"MİGROS", "TİCARET"}, kept: []string{"TOPLAM *125,00"},
		},
		{
			in:     "İletişim: ayse.yilmaz@ornek.com.tr, 0532 123 45 67 veya +90 (212) 555-12-34",
			hidden: []string{"ayse.yilmaz", "0532", "555-12-34"}, kept: []string{"İletişim:", "veya"},
		},
	}
	for _, c := range cases {
		m := New()
		out := m.Mask(c.in)
		for _, h := range c.hidden {
			if strings.Contains(out, h) {
				t.Errorf("%q leaked in %q", h, out)
			}
		}
		for _, k := range c.kept {
			if !strings.Contains(out, k) {
				t.Errorf("%q lost in %q", k, out)
			}
		}
		if got := m.Unmask(out); got != c.in {
			t.Errorf("round trip:\n got %q\nwant %q", got, c.in)
		}
	}
}

// Sıradan cümleler ve tutarlar maskelenmez.
func TestMaskLeavesOrdinaryText(t *testing.T) {
	for _, in := range []string{
		"Toplam KDV 20 oranında hesaplandı.",
		"Fatura No: ABC2026000000123, Tarih: 05.11.2026",
		"Deniz yolu taşımacılığı gideri",
		"Başlangıç bakiyesi ve kasa sayımı",
		"Kartaş ve baş harfler",
	} {
		m := New()
		if out := m.Mask(in); out != in {
			t.Errorf("masked ordinary text:\n in  %q\n out %q", in, out)
		}
	}
}

func TestFindReturnsSpansWithTokens(t *testing.T) {
	m := New()
	s := "VKN 1234567890 Ayşe Yılmaz"
	spans := m.Find(s)
	if len(spans) != 2 {
		t.Fatalf("spans = %+v", spans)
	}
	if s[spans[0].Start:spans[0].End] != "1234567890" || spans[0].Token != "[VKN_1]" {
		t.Fatalf("first span = %+v", spans[0])
	}
	if s[spans[1].Start:spans[1].End] != "Ayşe Yılmaz" || spans[1].Kind != KindName {
		t.Fatalf("second span = %+v", spans[1])
	}
	// Same masker, same value -> same token.
	if out := m.Mask("ayşe yılmaz değil, AYŞE YILMAZ"); !strings.HasSuffix(out, "[AD_1]") {
		t.Fatalf("got %q", out)
	}
}
