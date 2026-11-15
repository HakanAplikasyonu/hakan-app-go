package banking

import "strings"

// keywordRule is one row of 21 §4.2. Patterns are matched on the normalized
// description word by word (never as substrings): "POS" must not match
// "POSTA", "ASKI" must not match "MASKI". A multi-word pattern must appear as
// consecutive words. Turkish suffixes are accepted on the last word
// ("KOMISYONU", "FAIZI", "KIRASI"), see wordMatches.
type keywordRule struct {
	Patterns []string
	Dir      Direction // empty: either direction
	Kind     Kind      // empty with Transfer: the transfer type from the text
	Detail   Detail
	Transfer bool
	// ByDir picks the detail by direction (CEK, SENET, "yöne göre").
	ByDir map[Direction]Detail
	// KindByDir picks the kind by direction (EFEKTIF).
	KindByDir map[Direction]Kind
}

// keywordRules: order matters, the first match wins. More specific patterns
// come before the generic word they contain (POS KOMISYON before KOMISYON,
// UCRET ODEMESI before UCRET, EFT UCRETI before EFT).
var keywordRules = []keywordRule{
	{Patterns: []string{"BSMV"}, Kind: KindUcretKomisyonFaiz, Detail: DetailBSMV},
	{Patterns: []string{"STOPAJ", "GV KESINTI", "GELIR VERGISI KESINTISI"}, Kind: KindUcretKomisyonFaiz, Detail: DetailWithholdingTax},
	{Patterns: []string{"POS KOMISYON", "UYE ISYERI KOMISYON"}, Kind: KindUyeIsyeri, Detail: DetailPOSCommission},
	{Patterns: []string{"KAR PAYI"}, Dir: DirIn, Kind: KindUcretKomisyonFaiz, Detail: DetailInterestIncome},
	{Patterns: []string{"FAIZ"}, Dir: DirIn, Kind: KindUcretKomisyonFaiz, Detail: DetailInterestIncome},
	{Patterns: []string{"KMH FAIZ", "FAIZ", "TEMERRUT"}, Dir: DirOut, Kind: KindUcretKomisyonFaiz, Detail: DetailInterestExpense},
	{Patterns: []string{"EFT UCRETI", "HAVALE UCRETI", "KOMISYON"}, Kind: KindUcretKomisyonFaiz, Detail: DetailCommission},
	{Patterns: []string{"MAAS", "UCRET ODEMESI", "PERSONEL"}, Transfer: true, Detail: DetailSalaryPayment},
	{Patterns: []string{"HESAP ISLETIM", "MASRAF", "UCRET", "KART AIDATI", "SMS UCRETI"}, Kind: KindUcretKomisyonFaiz, Detail: DetailBankFee},
	{Patterns: []string{"POS", "UYE ISYERI", "BLOKE COZUM", "GUN SONU"}, Dir: DirIn, Kind: KindUyeIsyeri, Detail: DetailPOSSettlement},
	{Patterns: []string{"SGK", "SSK", "SOSYAL GUVENLIK", "PRIM BORCU"}, Kind: KindSGK, Detail: DetailSGKPayment},
	{Patterns: []string{"VERGI", "GIB", "GELIR IDARESI", "KDV", "MUHTASAR", "GECICI VERGI", "MTV", "DAMGA"}, Kind: KindVergi, Detail: DetailTaxPayment},
	{Patterns: []string{"KIRA"}, Transfer: true, Detail: DetailRentPayment},
	{Patterns: []string{"ELEKTRIK", "DOGALGAZ", "SU IDARESI", "ISKI", "ASKI", "IGDAS", "TELEKOM", "TURKCELL", "VODAFONE", "INTERNET"}, Kind: KindKurumFatura, Detail: DetailUtilityPayment},
	{Patterns: []string{"HGS", "OGS"}, Kind: KindHGSOGS, Detail: DetailVehicleToll},
	{Patterns: []string{"SIGORTA", "KASKO", "POLICE"}, Kind: KindSigorta, Detail: DetailInsurancePayment},
	{Patterns: []string{"KREDI KARTI ODEME", "KK ODEME", "KART BORCU"}, Kind: KindKKOdemesi, Detail: DetailOwnTransfer},
	{Patterns: []string{"NAKIT AVANS"}, Kind: KindKKNakitAvans, Detail: DetailUnknown},
	{Patterns: []string{"KREDI TAKSIT", "TAKSIT TAHSIL"}, Kind: KindKrediOdemesi, Detail: DetailLoanPrincipal},
	{Patterns: []string{"KREDI KULLANDIRIM", "KREDI ACILIS"}, Kind: KindKrediKullanim, Detail: DetailLoanDisbursement},
	{Patterns: []string{"CEK"}, Kind: KindCek, ByDir: map[Direction]Detail{DirIn: DetailCustomerCollection, DirOut: DetailSupplierPayment}},
	{Patterns: []string{"SENET"}, Kind: KindSenet, ByDir: map[Direction]Detail{DirIn: DetailCustomerCollection, DirOut: DetailSupplierPayment}},
	{Patterns: []string{"DBS"}, Kind: KindDBS, Detail: DetailSupplierPayment},
	{Patterns: []string{"DOVIZ ALIS"}, Kind: KindDovizAlim, Detail: DetailFXTrade},
	{Patterns: []string{"DOVIZ SATIS"}, Kind: KindDovizSatim, Detail: DetailFXTrade},
	{Patterns: []string{"EFEKTIF"}, Detail: DetailFXTrade, KindByDir: map[Direction]Kind{DirOut: KindDovizAlim, DirIn: KindDovizSatim}},
	{Patterns: []string{"ATM", "NAKIT CEKIM"}, Dir: DirOut, Kind: KindParaCekme, Detail: DetailCashWithdrawal},
	{Patterns: []string{"ATM", "NAKIT YATIR"}, Dir: DirIn, Kind: KindParaYatirma, Detail: DetailCashDeposit},
	{Patterns: []string{"IDDAA", "MILLI PIYANGO", "SANS OYUN"}, Kind: KindSansOyunu, Detail: DetailPrivate},
	{Patterns: []string{"FAST"}, Kind: KindFAST, Transfer: true},
	{Patterns: []string{"EFT"}, Kind: KindEFT, Transfer: true},
	{Patterns: []string{"HAVALE", "VIRMAN"}, Kind: KindHavale, Transfer: true},
}

// suffixes are Turkish inflections accepted after a pattern's last word.
// Locative forms starting with T (POSTA) are left out on purpose.
var suffixes = []string{"", "I", "U", "SI", "SU", "NI", "NU", "YI", "YU", "IN", "UN", "NIN", "NUN",
	"A", "E", "YA", "YE", "DA", "DE", "DAN", "DEN", "LAR", "LER", "LARI", "LERI", "SINI", "SUNU"}

func wordMatches(word, pattern string, last bool) bool {
	if !last {
		return word == pattern
	}
	if !strings.HasPrefix(word, pattern) {
		return false
	}
	rest := word[len(pattern):]
	for _, s := range suffixes {
		if rest == s {
			return true
		}
	}
	return false
}

// containsPhrase reports whether pattern's words appear consecutively in words.
func containsPhrase(words []string, pattern string) bool {
	p := strings.Fields(pattern)
	if len(p) == 0 {
		return false
	}
	for i := 0; i+len(p) <= len(words); i++ {
		ok := true
		for j := range p {
			if !wordMatches(words[i+j], p[j], j == len(p)-1) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// transferKind picks HAVALE / EFT / FAST from the text; an EFT MT940 code
// wins. Without a hint the movement is a HAVALE (bank-internal transfer).
func transferKind(words []string, mt940 string) Kind {
	switch {
	case mt940 == "EFT":
		return KindEFT
	case containsPhrase(words, "FAST"):
		return KindFAST
	case containsPhrase(words, "EFT"):
		return KindEFT
	}
	return KindHavale
}
