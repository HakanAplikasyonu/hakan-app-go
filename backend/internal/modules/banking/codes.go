package banking

// Bank transaction code tables (21 §4.1), kept as data. A code either decides
// fully (Kind and Detail known) or only hints the Kind ("açıklamaya göre",
// "karşı tarafa göre", "anahtar kelimeye bırakılır"); a hint lets the later
// steps pick the detail. Codes missing from a table are logged by the caller
// and fall through to the next step.

// codeRule maps one code. ByDir, when set, overrides Kind/Detail per direction.
type codeRule struct {
	Kind   Kind
	Detail Detail
	// Transfer: the kind is a transfer whose type (HAVALE/EFT/FAST) comes
	// from the description, detail from the counterparty.
	Transfer bool
	// ByDir picks kind/detail by direction (A in, B out).
	ByDir map[Direction]codeRule
	// Keyword: BSMV in the description turns commission into bsmv (MT940 COM).
	BSMVInDescription bool
	// InterestInDescription: FAIZ in the description → interest_income (Ziraat 300Y).
	InterestInDescription bool
	Note                  string
}

func byDir(in, out codeRule) map[Direction]codeRule {
	return map[Direction]codeRule{DirIn: in, DirOut: out}
}

// mt940Codes: the full list is only in the Fibabanka document; Halkbank and
// Akbank give examples (21 §4.1). CCP is ⚠️: its meaning is not documented,
// so it stays DIGER until a real statement confirms it.
var mt940Codes = map[string]codeRule{
	"TRF": {Transfer: true, Note: "MT940 TRF"},
	"COM": {Kind: KindUcretKomisyonFaiz, Detail: DetailCommission, BSMVInDescription: true, Note: "MT940 COM"},
	"CHG": {Kind: KindUcretKomisyonFaiz, Detail: DetailBankFee, Note: "MT940 CHG"},
	"INT": {ByDir: byDir(
		codeRule{Kind: KindUcretKomisyonFaiz, Detail: DetailInterestIncome},
		codeRule{Kind: KindUcretKomisyonFaiz, Detail: DetailInterestExpense}), Note: "MT940 INT"},
	"LDP": {Kind: KindKrediOdemesi, Detail: DetailLoanPrincipal, Note: "MT940 LDP"},
	"CHK": {ByDir: byDir(
		codeRule{Kind: KindCek, Detail: DetailCustomerCollection},
		codeRule{Kind: KindCek, Detail: DetailSupplierPayment}), Note: "MT940 CHK"},
	"BOE": {ByDir: byDir(
		codeRule{Kind: KindSenet, Detail: DetailCustomerCollection},
		codeRule{Kind: KindSenet, Detail: DetailSupplierPayment}), Note: "MT940 BOE"},
	"PRO": {ByDir: byDir(
		codeRule{Kind: KindSenet, Detail: DetailCustomerCollection},
		codeRule{Kind: KindSenet, Detail: DetailSupplierPayment}), Note: "MT940 PRO"},
	"BND": {ByDir: byDir(
		codeRule{Kind: KindYatirimdanAktarim, Detail: DetailOwnTransfer},
		codeRule{Kind: KindYatirimaAktarim, Detail: DetailOwnTransfer}), Note: "MT940 BND"},
	"EXP": {Kind: KindYabanciParaHavale, Detail: DetailCustomerCollection, Note: "MT940 EXP"},
	"IMC": {Kind: KindYabanciParaHavale, Detail: DetailSupplierPayment, Note: "MT940 IMC"},
	"IMP": {Kind: KindYabanciParaHavale, Detail: DetailSupplierPayment, Note: "MT940 IMP"},
	"FEX": {ByDir: byDir(
		codeRule{Kind: KindDovizSatim, Detail: DetailFXTrade},
		codeRule{Kind: KindDovizAlim, Detail: DetailFXTrade}), Note: "MT940 FEX"},
	"MSC": {Kind: KindDiger, Note: "MT940 MSC"},
	"EFT": {Kind: KindEFT, Note: "MT940 EFT"},
	"VRM": {Kind: KindHavale, Note: "MT940 VRM"},
	"CCP": {Kind: KindDiger, Note: "MT940 CCP (⚠️ anlamı doğrulanmadı)"},
}

// bankCodes: per source_system tables of the bank's own code (bank_txn_code).
var bankCodes = map[string]map[string]codeRule{
	// Ziraat Bankası IslemTipi.
	"bank_ziraat": {
		"211A": {Kind: KindHavale, Note: "Ziraat 211A"},
		"211M": {Transfer: true, Note: "Ziraat 211M"},
		"212T": {Transfer: true, Note: "Ziraat 212T"},
		"212C": {ByDir: byDir(
			codeRule{Kind: KindCek, Detail: DetailCustomerCollection},
			codeRule{Kind: KindCek, Detail: DetailSupplierPayment}), Note: "Ziraat 212C"},
		"212X": {Kind: KindDiger, Note: "Ziraat 212X"},
		"214A": {Kind: KindYabanciParaHavale, Note: "Ziraat 214A"},
		"218F": {Kind: KindUcretKomisyonFaiz, Detail: DetailBankFee, Note: "Ziraat 218F"},
		"221K": {ByDir: byDir(
			codeRule{Kind: KindKrediKullanim, Detail: DetailLoanDisbursement},
			codeRule{Kind: KindKrediOdemesi, Detail: DetailLoanPrincipal}), Note: "Ziraat 221K"},
		"300D": {ByDir: byDir(
			codeRule{Kind: KindDovizSatim, Detail: DetailFXTrade},
			codeRule{Kind: KindDovizAlim, Detail: DetailFXTrade}), Note: "Ziraat 300D"},
		"300K": {Kind: KindUcretKomisyonFaiz, Detail: DetailCommission, Note: "Ziraat 300K"},
		"300V": {Kind: KindDiger, Note: "Ziraat 300V"},
		"300Y": {Kind: KindDiger, InterestInDescription: true, Note: "Ziraat 300Y"},
		"400I": {Kind: KindYabanciParaHavale, Detail: DetailCustomerCollection, Note: "Ziraat 400I"},
	},
	// Vakıf Katılım TransactionType. Kuveyt Türk uses the same core banking
	// system but the table is ⚠️ there, so it is not applied to bank_kuveyt_turk.
	"bank_vakif_katilim": {
		"1":  {Kind: KindEFT, Note: "Vakıf Katılım 1 gelen EFT"},
		"2":  {Kind: KindEFT, Note: "Vakıf Katılım 2 muhabirden gelen EFT"},
		"3":  {Kind: KindYabanciParaHavale, Note: "Vakıf Katılım 3 gelen SWIFT"},
		"4":  {Kind: KindHavale, Note: "Vakıf Katılım 4 havale"},
		"5":  {Kind: KindParaYatirma, Detail: DetailCashDeposit, Note: "Vakıf Katılım 5 nakit yatırma"},
		"6":  {Kind: KindHavale, Note: "Vakıf Katılım 6 gelen PTT havale"},
		"7":  {Kind: KindDiger, Note: "Vakıf Katılım 7 bağış tahsilatı"},
		"8":  {Kind: KindEFT, Note: "Vakıf Katılım 8 giden EFT"},
		"9":  {Kind: KindParaCekme, Detail: DetailCashWithdrawal, Note: "Vakıf Katılım 9 nakit çekme"},
		"10": {Kind: KindYabanciParaHavale, Note: "Vakıf Katılım 10 giden SWIFT"},
		"11": {Kind: KindEFT, Note: "Vakıf Katılım 11 muhabire giden EFT"},
	},
	// TEB ISLEM_TUR: only the two codes whose meaning is known (21 §4.1).
	"bank_teb": {
		"22":   {Kind: KindHavale, Note: "TEB 22 şubeler arası TL havale"},
		"1207": {Kind: KindDBS, Detail: DetailSupplierPayment, Note: "TEB 1207 DBS tahsilatı"},
	},
}

// paymentPurposes: EFT payment purpose codes; only Vakıfbank returns them as
// a field (21 §3.3). M03 is only a tax-payment candidate.
var paymentPurposes = map[string]struct {
	Kind       Kind // empty: keep the transfer kind
	Detail     Detail
	Confidence float64
}{
	"M03": {Kind: KindVergi, Detail: DetailTaxPayment, Confidence: 0.80},
	"M04": {Detail: DetailRentPayment, Confidence: 0.95},
	"M05": {Detail: DetailRentPayment, Confidence: 0.95},
	"M06": {Detail: DetailRentPayment, Confidence: 0.95},
	"M07": {Detail: DetailSalaryPayment, Confidence: 0.95},
	"M08": {Kind: KindKKOdemesi, Detail: DetailOwnTransfer, Confidence: 0.95},
}
