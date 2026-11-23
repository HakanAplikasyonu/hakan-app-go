package statement

import "strings"

// bankByCode maps the 5-digit bank code in an IBAN (positions 5–9) to the
// bank id used in cash_boxes.bank_id and source_system "bank_<id>" (23 §2).
// Codes are the BDDK / TCMB EFT participant codes.
var bankByCode = map[string]string{
	"00010": "ziraat",
	"00012": "halkbank",
	"00015": "vakifbank",
	"00032": "teb",
	"00046": "akbank",
	"00059": "sekerbank",
	"00062": "garanti",
	"00064": "isbank",
	"00067": "yapikredi",
	"00099": "ing",
	"00103": "fibabanka",
	"00111": "qnb",
	"00123": "hsbc",
	"00134": "denizbank",
	"00135": "anadolubank",
	"00146": "odeabank",
	"00203": "albaraka",
	"00205": "kuveyt_turk",
	"00206": "turkiye_finans",
	"00209": "ziraat_katilim",
	"00210": "vakif_katilim",
	"00211": "emlak_katilim",
}

// BankFromIBAN returns the bank id of a Turkish IBAN, "" when unknown.
func BankFromIBAN(iban string) string {
	if len(iban) != 26 || !strings.HasPrefix(iban, "TR") {
		return ""
	}
	return bankByCode[iban[4:9]]
}

// KnownBank reports whether id is one of the bank ids above.
func KnownBank(id string) bool {
	for _, b := range bankByCode {
		if b == id {
			return true
		}
	}
	return false
}
