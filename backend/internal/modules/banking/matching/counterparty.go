package matching

import (
	"strings"
	"unicode"
)

// Markers banks put before the other side's name in a description.
var partyMarkers = []string{
	"GONDEREN:", "GÖNDEREN:", "ALICI:", "LEHDAR:", "LEHTAR:", "AMIR:", "AMİR:", "BORCLU:", "BORÇLU:",
	"HAVALE GELEN ", "HAVALE GIDEN ", "HAVALE GİDEN ", "EFT GELEN ", "EFT GIDEN ", "EFT GİDEN ",
	"FAST GELEN ", "FAST GIDEN ", "FAST GİDEN ",
}

// Company suffixes end a name; the words after them are the payment note.
var partySuffixes = map[string]bool{
	"A.S.": true, "A.Ş.": true, "AS": true, "AŞ": true, "A.S": true, "A.Ş": true,
	"STI": true, "STI.": true, "ŞTİ": true, "ŞTİ.": true, "ŞTI": true, "ŞTI.": true,
}

// Words that are never part of a name: the note starts here.
var partyStops = map[string]bool{
	"FATURA": true, "FATURASI": true, "ODEME": true, "ÖDEME": true, "ODEMESI": true, "ÖDEMESİ": true,
	"BEDELI": true, "BEDELİ": true, "TAHSILAT": true, "TAHSİLAT": true, "ACIKLAMA": true, "AÇIKLAMA": true,
	"ACIKLAMA:": true, "AÇIKLAMA:": true, "IBAN": true, "IBAN:": true, "NO": true, "NO:": true,
	"KIRA": true, "KİRA": true, "KIRASI": true, "KİRASI": true, "AVANS": true, "HAVALE": true, "EFT": true, "FAST": true,
	"GONDEREN:": true, "GÖNDEREN:": true, "ALICI:": true,
}

const maxPartyWords = 5

// CounterpartyFromDescription reads the other side's name out of a bank
// description when the bank sent none ("FAST GELEN GONDEREN: X YAZILIM A.S.
// EKIM HIZMET BEDELI" → "X YAZILIM A.S."). It is a hint for the screen, not
// a matching key: only descriptions with a known marker give a name, and a
// name of a single word is not trusted (#565).
func CounterpartyFromDescription(desc string) string {
	up := strings.ToUpper(strings.Join(strings.Fields(desc), " "))
	at := -1
	for _, m := range partyMarkers {
		if i := strings.Index(up, m); i >= 0 && i+len(m) > at {
			at = i + len(m)
		}
	}
	if at < 0 {
		return ""
	}
	var name []string
	for _, w := range strings.Fields(up[at:]) {
		if partyStops[w] || hasDigit(w) {
			break
		}
		name = append(name, w)
		if partySuffixes[w] || len(name) == maxPartyWords {
			break
		}
	}
	if len(name) < 2 {
		return ""
	}
	return strings.Join(name, " ")
}

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
