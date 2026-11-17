package banking

import "testing"

func TestNormalizeDescription(t *testing.T) {
	cases := map[string]string{
		"İstanbul'da şube":       "ISTANBUL DA SUBE",
		"ığdır, ÇEŞME / ödeme..": "IGDIR CESME ODEME",
		"  kâr   payı ":          "KAR PAYI",
		"EFT-ÜCRETİ(BSMV)":       "EFT UCRETI BSMV",
	}
	for in, want := range cases {
		if got := NormalizeDescription(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeIBAN(t *testing.T) {
	if got := NormalizeIBAN("tr33 0006 1005 1978 6457 8413 26"); got != "TR330006100519786457841326" {
		t.Errorf("valid: %q", got)
	}
	for _, bad := range []string{"TR330006100519786457841327", "TR33 **** **** 1326", "TR3300061005", ""} {
		if got := NormalizeIBAN(bad); got != "" {
			t.Errorf("%q should be rejected, got %q", bad, got)
		}
	}
}

func TestSplitTaxID(t *testing.T) {
	if v, c := SplitTaxID("920 000 0035"); v != "9200000035" || c != "" {
		t.Errorf("vkn: %q %q", v, c)
	}
	if v, c := SplitTaxID("10000000146"); v != "" || c != "10000000146" {
		t.Errorf("tckn: %q %q", v, c)
	}
	if v, c := SplitTaxID("0000000000"); v != "" || c != "" {
		t.Errorf("filler: %q %q", v, c)
	}
}

func TestNormalizeCurrency(t *testing.T) {
	for in, want := range map[string]string{"TL": "TRY", "ytl": "TRY", "usd": "USD", "": "TRY"} {
		if got := NormalizeCurrency(in); got != want {
			t.Errorf("%q → %q", in, got)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"X Yazılım Teknolojileri A.Ş.":           "X YAZILIM TEKNOLOJILERI",
		"X YAZILIM TEKNOLOJILERI AS":             "X YAZILIM TEKNOLOJILERI",
		"Kalem Kırtasiye San. ve Tic. Ltd. Şti.": "KALEM KIRTASIYE",
		"Oğuz Yılmaz":                            "OGUZ YILMAZ",
		"  ":                                     "",
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Fatalf("%q → %q, want %q", in, got, want)
		}
	}
}
