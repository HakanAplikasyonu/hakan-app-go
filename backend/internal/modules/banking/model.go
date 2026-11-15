// Package banking holds the bank transaction model and the classification
// pipeline (issue #20, docs/kaynak/musavir-entegrasyon-paketi/21-banka-veri-modeli.md).
//
// Every channel (bank web service, MT940, Excel, PDF; #19) writes the same
// Transaction through Ingest; the control layer (#21) only reads the tables.
package banking

import "time"

// Kind is the central transaction type: the ÖHVPS (TR.OHVPS.DataCode.IslemTuru)
// list, used verbatim as the naming standard (21 §3.1).
type Kind string

const (
	KindHavale            Kind = "HAVALE"
	KindEFT               Kind = "EFT"
	KindFAST              Kind = "FAST"
	KindParaYatirma       Kind = "PARA_YATIRMA"
	KindParaCekme         Kind = "PARA_CEKME"
	KindYabanciParaHavale Kind = "YABANCI_PARA_HAVALE"
	KindYatirimaAktarim   Kind = "YATIRIM_HESABINA_AKTARIM"
	KindYatirimdanAktarim Kind = "YATIRIM_HESABINDAN_AKTARIM"
	KindKurumFatura       Kind = "KURUM_FATURA_ODEMESI"
	KindCek               Kind = "CEK"
	KindSenet             Kind = "SENET"
	KindSigorta           Kind = "SIGORTA_ODEMESI"
	KindUcretKomisyonFaiz Kind = "UCRET_KOMISYON_FAIZ"
	KindSGK               Kind = "SGK_ODEMESI"
	KindVergi             Kind = "VERGI_ODEMESI"
	KindDovizAlim         Kind = "DOVIZ_ALIM"
	KindDovizSatim        Kind = "DOVIZ_SATIM"
	KindKrediOdemesi      Kind = "KREDI_ODEMESI"
	KindKrediKullanim     Kind = "KREDI_KULLANIM"
	KindKKOdemesi         Kind = "KK_ODEMESI"
	KindKKNakitAvans      Kind = "KK_NAKIT_AVANS"
	KindSansOyunu         Kind = "SANS_OYUNU"
	KindUyeIsyeri         Kind = "UYE_ISYERI_ISLEMLERI"
	KindHGSOGS            Kind = "HGS_OGS_ISLEMLERI"
	KindDBS               Kind = "DOGRUDAN_BORCLANDIRMA_SISTEMI"
	KindDiger             Kind = "DIGER"
)

// Detail splits a Kind into what the declaration needs (21 §3.2).
type Detail string

const (
	DetailCustomerCollection Detail = "customer_collection"
	DetailSupplierPayment    Detail = "supplier_payment"
	DetailOwnTransfer        Detail = "own_account_transfer"
	DetailBankFee            Detail = "bank_fee"
	DetailCommission         Detail = "commission"
	DetailBSMV               Detail = "bsmv"
	DetailInterestExpense    Detail = "interest_expense"
	DetailInterestIncome     Detail = "interest_income"
	DetailWithholdingTax     Detail = "withholding_tax"
	DetailLoanPrincipal      Detail = "loan_principal"
	DetailLoanDisbursement   Detail = "loan_disbursement"
	DetailTaxPayment         Detail = "tax_payment"
	DetailSGKPayment         Detail = "sgk_payment"
	DetailSalaryPayment      Detail = "salary_payment"
	DetailRentPayment        Detail = "rent_payment"
	DetailUtilityPayment     Detail = "utility_payment"
	DetailPOSSettlement      Detail = "pos_settlement"
	DetailPOSCommission      Detail = "pos_commission"
	DetailInsurancePayment   Detail = "insurance_payment"
	DetailVehicleToll        Detail = "vehicle_toll"
	DetailFXTrade            Detail = "fx_trade"
	DetailCashDeposit        Detail = "cash_deposit"
	DetailCashWithdrawal     Detail = "cash_withdrawal"
	DetailPrivate            Detail = "private_or_non_business"
	DetailUnknown            Detail = "unknown"
)

// Source says which step of the pipeline decided (21 §2 kind_source).
type Source string

const (
	SourceBankCode       Source = "bank_code"
	SourceMT940Code      Source = "mt940_code"
	SourcePaymentPurpose Source = "payment_purpose"
	SourceCounterparty   Source = "counterparty"
	SourceKeyword        Source = "keyword"
	SourceCompanyRule    Source = "company_rule"
	SourceAI             Source = "ai"
	SourceManual         Source = "manual"
)

// Direction: A money in (alacak), B money out (borç), as banks label it.
type Direction string

const (
	DirIn  Direction = "A"
	DirOut Direction = "B"
)

// Capture methods in source priority order (21 §5): api > mt940 > excel > pdf.
const (
	CaptureAPI   = "api"
	CaptureMT940 = "mt940"
	CaptureExcel = "excel_import"
	CapturePDF   = "pdf_import"
)

// CapturePriority: higher wins as the primary source.
func CapturePriority(method string) int {
	switch method {
	case CaptureAPI:
		return 4
	case CaptureMT940:
		return 3
	case CaptureExcel:
		return 2
	case CapturePDF:
		return 1
	}
	return 0
}

// ReviewThreshold: classifications below it are shown as "kontrol et" (21 §4).
const ReviewThreshold = 0.80

// AIConfidenceCap: an AI suggestion never claims more (21 §4 step 7).
const AIConfidenceCap = 0.70

// Transaction is one movement as a channel adapter delivers it. Amounts are
// decimal strings, always positive; the sign lives in Direction (21 §2.1).
type Transaction struct {
	SourceSystem    string // bank_<id> or bank_file
	SourceID        string // the bank's unique key; empty → hash (20 §7.3)
	CaptureMethod   string
	BookedAt        time.Time // Europe/Istanbul; 00:00 when the bank gives no time
	HasTime         bool
	BankSequence    *int32
	TransactionDate time.Time
	ValueDate       *time.Time
	Direction       Direction
	Amount          string
	Currency        string
	BalanceAfter    *string
	BalanceBefore   *string

	Description      string
	DescriptionExtra string
	BankReference    string
	ReceiptNumber    string
	BankTxnCode      string
	MT940Code        string
	Channel          string

	CounterpartyName     string
	CounterpartyIBAN     string
	CounterpartyTaxID    string // VKN or TCKN, normalized on ingest
	CounterpartyBankCode string
	EFTQueryNumber       string
	PaymentPurpose       string
	IsCancelled          bool

	Raw map[string]any
}

// Classification is the pipeline's answer for one movement.
type Classification struct {
	Kind       Kind
	Detail     Detail
	Source     Source // empty: nothing decided
	Confidence float64
	// Reason names the evidence (G12): the code, rule or word that matched.
	Reason string
	// RuleID is set when a bank_rules row decided (its hits are counted).
	RuleID int64
}

// NeedsReview reports whether the accountant must check it (21 §4).
func (c Classification) NeedsReview() bool { return c.Confidence < ReviewThreshold }
