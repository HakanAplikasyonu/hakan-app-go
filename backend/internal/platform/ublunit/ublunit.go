// Package ublunit names UBL / UN/ECE Rec. 20 unit codes in Turkish (#378).
// It is the one mapping every surface uses: the API returns `unit_name`
// next to `unit_code` and web and mobile show the name.
package ublunit

import "strings"

var names = map[string]string{
	// Adet ve ambalaj
	"C62": "adet", "NIU": "adet", "EA": "adet", "H87": "adet", "PCE": "adet",
	"PR": "çift", "DZN": "düzine", "SET": "set", "PA": "paket", "PK": "paket",
	"BX": "kutu", "CT": "koli", "CS": "kasa", "BG": "torba", "SA": "çuval",
	"BO": "şişe", "CA": "teneke", "RO": "rulo", "PF": "palet", "TU": "tüp",
	// Ağırlık
	"KGM": "kg", "GRM": "g", "MGM": "mg", "TNE": "ton",
	// Hacim
	"LTR": "lt", "MLT": "ml", "CLT": "cl", "MTQ": "m³", "CMQ": "cm³",
	// Uzunluk ve alan
	"MTR": "m", "CMT": "cm", "MMT": "mm", "KTM": "km", "MTK": "m²", "CMK": "cm²",
	// Zaman
	"SEC": "saniye", "MIN": "dakika", "HUR": "saat", "DAY": "gün", "WEE": "hafta",
	"MON": "ay", "ANN": "yıl",
	// Enerji
	"KWH": "kWh", "KWT": "kW", "MWH": "MWh", "SM3": "Sm³",
}

// Name is the Turkish name of code ("C62" → "adet"); an unknown code is
// returned as given, an empty one as "".
func Name(code string) string {
	c := strings.ToUpper(strings.TrimSpace(code))
	if n, ok := names[c]; ok {
		return n
	}
	return strings.TrimSpace(code)
}
