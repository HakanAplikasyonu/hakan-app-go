package banking

import (
	"encoding/json"
	"strings"
)

// SettingStatementTracking is the office setting (organizations.settings)
// of the missing statement follow-up (Oğuz'un geri bildirimi): the monthly
// "ekstre geldi / eksik" status of every account, the office to-do list
// "Eksik ekstreler" and the "Listeye ekle" suggestion of a statement upload
// whose account is not on the list. Missing or true: on. The account list
// itself is never hidden.
const SettingStatementTracking = "bank_statement_tracking"

// StatementTrackingEnabled reads the setting from the office settings JSON.
func StatementTrackingEnabled(settings []byte) bool {
	var m map[string]any
	if len(settings) == 0 || json.Unmarshal(settings, &m) != nil {
		return true
	}
	on, ok := m[SettingStatementTracking].(bool)
	return !ok || on
}

// bankNames are the display names of the bank ids (cash_boxes.bank_id),
// the same wording as the office panel (office.bank.banks.*).
var bankNames = map[string]string{
	"ziraat":         "Ziraat Bankası",
	"halkbank":       "Halkbank",
	"vakifbank":      "VakıfBank",
	"teb":            "TEB",
	"akbank":         "Akbank",
	"sekerbank":      "Şekerbank",
	"garanti":        "Garanti BBVA",
	"isbank":         "İş Bankası",
	"yapikredi":      "Yapı Kredi",
	"ing":            "ING",
	"fibabanka":      "Fibabanka",
	"qnb":            "QNB",
	"hsbc":           "HSBC",
	"denizbank":      "DenizBank",
	"anadolubank":    "Anadolubank",
	"odeabank":       "Odeabank",
	"albaraka":       "Albaraka Türk",
	"kuveyt_turk":    "Kuveyt Türk",
	"turkiye_finans": "Türkiye Finans",
	"ziraat_katilim": "Ziraat Katılım",
	"vakif_katilim":  "Vakıf Katılım",
	"emlak_katilim":  "Emlak Katılım",
}

// BankName is the display name of a bank id; "" when the id is empty and
// the id itself when it is unknown.
func BankName(id string) string {
	id = strings.TrimSpace(id)
	if n, ok := bankNames[id]; ok {
		return n
	}
	return id
}
