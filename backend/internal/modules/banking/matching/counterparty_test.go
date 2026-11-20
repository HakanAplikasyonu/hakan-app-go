package matching

import "testing"

func TestCounterpartyFromDescription(t *testing.T) {
	for desc, want := range map[string]string{
		"FAST GELEN GONDEREN: X YAZILIM TEKNOLOJILERI A.S. EKIM HIZMET BEDELI": "X YAZILIM TEKNOLOJILERI A.S.",
		"EFT GIDEN ALICI: KALEM KIRTASIYE LTD STI FATURA KRT2026000001207":     "KALEM KIRTASIYE LTD STI",
		"HAVALE GELEN DENIZ YILDIRIM":                                          "DENIZ YILDIRIM",
		"havale gelen  deniz   yıldırım kira":                                  "DENIZ YILDIRIM",
		"FAST GELEN GONDEREN: AHMET KAYA 12345 NOLU SIPARIS":                   "AHMET KAYA",
		// No marker, a single word or only a note: no name.
		"FAST TOPLU ODEME":                "",
		"EFT UCRETI":                      "",
		"GIB VERGI ODEMESI KDV 2026/09":   "",
		"FAST GELEN":                      "",
		"HAVALE GELEN MEHMET":             "",
		"FAST GELEN FATURA ATL2026000101": "",
		"":                                "",
	} {
		if got := CounterpartyFromDescription(desc); got != want {
			t.Errorf("%q: got %q, want %q", desc, got, want)
		}
	}
}
