package taxid

import (
	"errors"
	"testing"
)

func TestValidTCKN(t *testing.T) {
	for _, s := range []string{"10000000146", "11111111110"} {
		if !ValidTCKN(s) {
			t.Errorf("ValidTCKN(%q) = false, want true", s)
		}
	}
	for _, s := range []string{
		"10000000147", // 11. hane yanlış
		"10000000156", // 10. hane yanlış
		"01111111110", // ilk hane 0
		"1000000014",  // 10 hane
		"1000000014a", // rakam dışı
	} {
		if ValidTCKN(s) {
			t.Errorf("ValidTCKN(%q) = true, want false", s)
		}
	}
}

func TestValidVKN(t *testing.T) {
	// 1234567890 yaygın kullanılan geçerli test VKN'sidir. Diğerleri
	// algoritmadan türetildi. Yanlış olanlar sağlama hanesi bir artırılarak
	// üretilir.
	valid := []string{"1234567890", "0000000001", "1111111114"}
	for _, s := range valid {
		if !ValidVKN(s) {
			t.Errorf("ValidVKN(%q) = false, want true", s)
		}
		last := s[9] - '0'
		bad := s[:9] + string(rune('0'+(last+1)%10))
		if ValidVKN(bad) {
			t.Errorf("ValidVKN(%q) = true, want false", bad)
		}
	}
	// Her 9 haneli önek için tam olarak bir sağlama hanesi geçerli olmalı.
	for _, prefix := range []string{"123456789", "987654321", "000000001", "470000000"} {
		n := 0
		for c := '0'; c <= '9'; c++ {
			if ValidVKN(prefix + string(c)) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("prefix %s: %d geçerli sağlama hanesi, want 1", prefix, n)
		}
	}
	for _, s := range []string{"123456789", "12345678901", "12345678a0"} {
		if ValidVKN(s) {
			t.Errorf("ValidVKN(%q) = true, want false", s)
		}
	}
}

func TestParse(t *testing.T) {
	n, typ, err := Parse(" 100 000 001-46 ")
	if err != nil || n != "10000000146" || typ != TCKN {
		t.Fatalf("Parse TCKN = %q %q %v", n, typ, err)
	}
	n, typ, err = Parse("1234567890")
	if err != nil || n != "1234567890" || typ != VKN {
		t.Fatalf("Parse VKN = %q %q %v", n, typ, err)
	}
	if _, _, err := Parse("123"); !errors.Is(err, ErrFormat) {
		t.Fatalf("short: %v", err)
	}
	if _, _, err := Parse("12345678ab"); !errors.Is(err, ErrFormat) {
		t.Fatalf("letters: %v", err)
	}
	if _, _, err := Parse("10000000147"); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad tckn checksum: %v", err)
	}
	if _, _, err := Parse("1234567891"); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad vkn checksum: %v", err)
	}
}
