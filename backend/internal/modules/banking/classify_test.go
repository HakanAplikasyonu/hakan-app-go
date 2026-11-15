package banking

import (
	"context"
	"testing"
)

// Cases from 21-banka-veri-modeli.md (§3.3, §4, §4.1, §4.2): the issue's
// acceptance is that these examples classify as the document says.
const (
	ownIBAN      = "TR330006100519786457841326"
	supplierIBAN = "TR320010009999901234567890"
)

func ctxFixture() Context {
	return Context{
		OwnIBANs: map[string]bool{ownIBAN: true},
		Rules: []Rule{
			{ID: 7, MatchType: "description", MatchValue: "XYZ LTD", Dir: DirOut, Kind: KindEFT, Detail: DetailSupplierPayment},
		},
		Counterparties: []Counterparty{
			{CustomerID: 1, Type: "supplier", TaxID: "9200000035"},
			{CustomerID: 2, Type: "customer", TaxID: "9100000021"},
			{CustomerID: 3, Type: "employee", TaxID: "10000000146"},
			{CustomerID: 4, Type: "supplier", IBANs: []string{"TR32 0010 0099 9990 1234 5678 90"}},
		},
	}
}

func TestClassifyDocumentExamples(t *testing.T) {
	cases := []struct {
		name   string
		txn    Transaction
		kind   Kind
		detail Detail
		source Source
		conf   float64
	}{
		// §4 step 1
		{"own account transfer", Transaction{Direction: DirOut, CounterpartyIBAN: "tr33 0006 1005 1978 6457 8413 26", Description: "EFT GONDERIM"},
			KindEFT, DetailOwnTransfer, SourceCounterparty, 1.00},
		// §4 step 2 examples: Ziraat 218F → masraf, Fibabanka LDP → kredi anapara
		{"ziraat 218F", Transaction{SourceSystem: "bank_ziraat", BankTxnCode: "218F", Direction: DirOut, Description: "SERBEST MASRAF"},
			KindUcretKomisyonFaiz, DetailBankFee, SourceBankCode, 0.95},
		{"mt940 LDP", Transaction{MT940Code: "LDP", Direction: DirOut, Description: "KREDI ODEMESI"},
			KindKrediOdemesi, DetailLoanPrincipal, SourceMT940Code, 0.95},
		{"mt940 COM with BSMV", Transaction{MT940Code: "COM", Direction: DirOut, Description: "EFT Komisyonu BSMV"},
			KindUcretKomisyonFaiz, DetailBSMV, SourceMT940Code, 0.95},
		{"mt940 INT in", Transaction{MT940Code: "INT", Direction: DirIn},
			KindUcretKomisyonFaiz, DetailInterestIncome, SourceMT940Code, 0.95},
		{"mt940 INT out", Transaction{MT940Code: "INT", Direction: DirOut},
			KindUcretKomisyonFaiz, DetailInterestExpense, SourceMT940Code, 0.95},
		{"ziraat 221K in", Transaction{SourceSystem: "bank_ziraat", BankTxnCode: "221K", Direction: DirIn},
			KindKrediKullanim, DetailLoanDisbursement, SourceBankCode, 0.95},
		{"ziraat 300Y with faiz", Transaction{SourceSystem: "bank_ziraat", BankTxnCode: "300Y", Direction: DirIn, Description: "Vadeli faiz tahakkuku"},
			KindUcretKomisyonFaiz, DetailInterestIncome, SourceBankCode, 0.95},
		{"vakif katilim 9 cash", Transaction{SourceSystem: "bank_vakif_katilim", BankTxnCode: "9", Direction: DirOut},
			KindParaCekme, DetailCashWithdrawal, SourceBankCode, 0.95},
		// §3.3 / step 3: M05 → işyeri kirası
		{"payment purpose M05", Transaction{SourceSystem: "bank_vakifbank", PaymentPurpose: "M05", Direction: DirOut, Description: "EFT ODEME"},
			KindEFT, DetailRentPayment, SourcePaymentPurpose, 0.95},
		{"payment purpose M07", Transaction{PaymentPurpose: "M07", Direction: DirOut, Description: "FAST"},
			KindFAST, DetailSalaryPayment, SourcePaymentPurpose, 0.95},
		// step 4: "XYZ LTD" → tedarikçi ödemesi
		{"company rule", Transaction{Direction: DirOut, Description: "XYZ LTD STI ODEME"},
			KindEFT, DetailSupplierPayment, SourceCompanyRule, 0.90},
		// step 5: customer.type=supplier → supplier_payment
		{"supplier by VKN", Transaction{Direction: DirOut, CounterpartyTaxID: "9200000035", Description: "HAVALE"},
			KindHavale, DetailSupplierPayment, SourceCounterparty, 0.90},
		{"supplier by IBAN", Transaction{Direction: DirOut, CounterpartyIBAN: supplierIBAN, Description: "EFT"},
			KindEFT, DetailSupplierPayment, SourceCounterparty, 0.90},
		{"customer collection", Transaction{Direction: DirIn, CounterpartyTaxID: "9100000021", Description: "FAST GELEN"},
			KindFAST, DetailCustomerCollection, SourceCounterparty, 0.90},
		{"employee salary", Transaction{Direction: DirOut, CounterpartyTaxID: "10000000146", Description: "EFT"},
			KindEFT, DetailSalaryPayment, SourceCounterparty, 0.90},
		// step 6: §4.2 keyword table
		{"BSMV", Transaction{Direction: DirOut, Description: "BSMV tahsilatı"},
			KindUcretKomisyonFaiz, DetailBSMV, SourceKeyword, 0.80},
		{"stopaj", Transaction{Direction: DirOut, Description: "Gelir vergisi kesintisi"},
			KindUcretKomisyonFaiz, DetailWithholdingTax, SourceKeyword, 0.80},
		{"faiz in", Transaction{Direction: DirIn, Description: "Mevduat faizi"},
			KindUcretKomisyonFaiz, DetailInterestIncome, SourceKeyword, 0.80},
		{"kmh faiz out", Transaction{Direction: DirOut, Description: "KMH faiz tahsili"},
			KindUcretKomisyonFaiz, DetailInterestExpense, SourceKeyword, 0.80},
		{"kar payi", Transaction{Direction: DirIn, Description: "Kâr payı"},
			KindUcretKomisyonFaiz, DetailInterestIncome, SourceKeyword, 0.80},
		{"komisyon", Transaction{Direction: DirOut, Description: "Teminat mektubu komisyonu"},
			KindUcretKomisyonFaiz, DetailCommission, SourceKeyword, 0.80},
		{"hesap isletim", Transaction{Direction: DirOut, Description: "Hesap işletim ücreti"},
			KindUcretKomisyonFaiz, DetailBankFee, SourceKeyword, 0.80},
		{"pos settlement", Transaction{Direction: DirIn, Description: "POS gün sonu"},
			KindUyeIsyeri, DetailPOSSettlement, SourceKeyword, 0.80},
		{"pos komisyon", Transaction{Direction: DirOut, Description: "Üye işyeri komisyonu"},
			KindUyeIsyeri, DetailPOSCommission, SourceKeyword, 0.80},
		{"sgk", Transaction{Direction: DirOut, Description: "SGK prim ödemesi"},
			KindSGK, DetailSGKPayment, SourceKeyword, 0.80},
		{"vergi", Transaction{Direction: DirOut, Description: "GİB KDV ödemesi"},
			KindVergi, DetailTaxPayment, SourceKeyword, 0.80},
		{"maas", Transaction{Direction: DirOut, Description: "EFT maaş ödemesi"},
			KindEFT, DetailSalaryPayment, SourceKeyword, 0.80},
		{"kira", Transaction{Direction: DirOut, Description: "Ekim kirası"},
			KindHavale, DetailRentPayment, SourceKeyword, 0.80},
		{"utility", Transaction{Direction: DirOut, Description: "İGDAŞ doğalgaz faturası"},
			KindKurumFatura, DetailUtilityPayment, SourceKeyword, 0.80},
		{"hgs", Transaction{Direction: DirOut, Description: "HGS yükleme"},
			KindHGSOGS, DetailVehicleToll, SourceKeyword, 0.80},
		{"kasko", Transaction{Direction: DirOut, Description: "Kasko poliçesi"},
			KindSigorta, DetailInsurancePayment, SourceKeyword, 0.80},
		{"kredi karti", Transaction{Direction: DirOut, Description: "Kredi kartı ödemesi"},
			KindKKOdemesi, DetailOwnTransfer, SourceKeyword, 0.80},
		{"kredi taksit", Transaction{Direction: DirOut, Description: "Kredi taksit tahsilatı"},
			KindKrediOdemesi, DetailLoanPrincipal, SourceKeyword, 0.80},
		{"cek in", Transaction{Direction: DirIn, Description: "Çek tahsilatı"},
			KindCek, DetailCustomerCollection, SourceKeyword, 0.80},
		{"dbs", Transaction{Direction: DirOut, Description: "DBS ödeme"},
			KindDBS, DetailSupplierPayment, SourceKeyword, 0.80},
		{"doviz alis", Transaction{Direction: DirOut, Description: "Döviz alış"},
			KindDovizAlim, DetailFXTrade, SourceKeyword, 0.80},
		{"atm out", Transaction{Direction: DirOut, Description: "ATM para çekme"},
			KindParaCekme, DetailCashWithdrawal, SourceKeyword, 0.80},
		{"atm in", Transaction{Direction: DirIn, Description: "ATM yatırma"},
			KindParaYatirma, DetailCashDeposit, SourceKeyword, 0.80},
		{"iddaa", Transaction{Direction: DirOut, Description: "İddaa"},
			KindSansOyunu, DetailPrivate, SourceKeyword, 0.80},
		// transfers with no counterparty: direction guess, below threshold
		{"fast unknown counterparty", Transaction{Direction: DirIn, Description: "FAST gelen"},
			KindFAST, DetailCustomerCollection, SourceKeyword, 0.60},
		// §4.2: word based, not substring
		{"POSTA is not POS", Transaction{Direction: DirIn, Description: "POSTA çeki"},
			KindCek, DetailCustomerCollection, SourceKeyword, 0.80},
		{"MASKI is not ASKI", Transaction{Direction: DirOut, Description: "MASKI"},
			KindDiger, DetailUnknown, "", 0},
		// nothing matches
		{"unknown", Transaction{Direction: DirOut, Description: "XQZ"},
			KindDiger, DetailUnknown, "", 0},
	}
	var cl Classifier
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cl.Classify(context.Background(), tc.txn, ctxFixture())
			if got.Kind != tc.kind || got.Detail != tc.detail || got.Source != tc.source || got.Confidence != tc.conf {
				t.Errorf("got %s/%s/%s/%.2f (%s), want %s/%s/%s/%.2f",
					got.Kind, got.Detail, got.Source, got.Confidence, got.Reason, tc.kind, tc.detail, tc.source, tc.conf)
			}
		})
	}
}

type fakeAI struct{ conf float64 }

func (f fakeAI) SuggestKind(_ context.Context, in MaskedTransaction) (Kind, Detail, float64, error) {
	for _, w := range []string{"TR33", "9200000035"} {
		if contains(in.Description, w) || contains(in.CounterpartyName, w) {
			panic("unmasked data sent to AI")
		}
	}
	return KindDiger, DetailPrivate, f.conf, nil
}

func contains(s, sub string) bool { return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestAIIsLastAndCapped(t *testing.T) {
	c := ctxFixture()
	c.AI = fakeAI{conf: 0.99}
	var cl Classifier
	got := cl.Classify(context.Background(), Transaction{Direction: DirOut, Description: "XQZ"}, c)
	if got.Source != SourceAI || got.Confidence != AIConfidenceCap || !got.NeedsReview() {
		t.Fatalf("ai: %+v", got)
	}
	// A keyword hit never reaches the AI.
	got = cl.Classify(context.Background(), Transaction{Direction: DirOut, Description: "BSMV"}, c)
	if got.Source != SourceKeyword {
		t.Fatalf("keyword should win over ai: %+v", got)
	}
}

func TestRuleIDIsReported(t *testing.T) {
	var cl Classifier
	got := cl.Classify(context.Background(), Transaction{Direction: DirOut, Description: "XYZ LTD"}, ctxFixture())
	if got.RuleID != 7 {
		t.Fatalf("rule id: %+v", got)
	}
	// Rule is direction-bound.
	got = cl.Classify(context.Background(), Transaction{Direction: DirIn, Description: "XYZ LTD"}, ctxFixture())
	if got.Source == SourceCompanyRule {
		t.Fatalf("rule must not apply to incoming: %+v", got)
	}
}
