package banking

import (
	"context"
	"fmt"
	"strings"
)

// Counterparty is a customer record of the taxpayer (step 5).
type Counterparty struct {
	CustomerID int64
	// Type: customer | supplier | both | employee (customers.type).
	Type  string
	TaxID string // VKN or TCKN
	IBANs []string
}

// Rule is a learned bank_rules row (step 4).
type Rule struct {
	ID         int64
	MatchType  string // counterparty_iban | counterparty_tax_id | description
	MatchValue string
	Dir        Direction // empty: either direction
	Kind       Kind
	Detail     Detail
}

// Suggester is the AI fallback (step 7). It receives a masked view only:
// no IBAN, VKN or TCKN (21 §4). Its confidence is capped at AIConfidenceCap.
type Suggester interface {
	SuggestKind(ctx context.Context, in MaskedTransaction) (Kind, Detail, float64, error)
}

// MaskedTransaction is what may leave the system for the AI step.
type MaskedTransaction struct {
	Direction        Direction
	Description      string
	CounterpartyName string
	MT940Code        string
	BankTxnCode      string
}

// Context is everything about the taxpayer the pipeline needs.
type Context struct {
	OwnIBANs       map[string]bool
	Rules          []Rule
	Counterparties []Counterparty
	AI             Suggester
}

// Classifier runs the 21 §4 pipeline. The first certain result wins; AI only
// at the end and only as a suggestion.
type Classifier struct{}

// hint is a partial answer from a code table: the kind is known, the detail
// is left to the later steps.
type hint struct {
	kind     Kind
	transfer bool
	source   Source
	reason   string
}

// Classify classifies one movement. txn.Description may be raw; it is
// normalized here.
func (Classifier) Classify(ctx context.Context, txn Transaction, c Context) Classification {
	words := strings.Fields(NormalizeDescription(txn.Description + " " + txn.DescriptionExtra))
	nameWords := strings.Fields(NormalizeDescription(txn.CounterpartyName))
	dir := txn.Direction
	iban := NormalizeIBAN(txn.CounterpartyIBAN)
	vkn, tckn := SplitTaxID(txn.CounterpartyTaxID)
	taxID := vkn + tckn
	mt940 := strings.ToUpper(strings.TrimSpace(txn.MT940Code))

	var h *hint
	resolveKind := func(k Kind, transfer bool) Kind {
		if transfer || k == "" {
			if h != nil && h.kind != "" && h.kind != KindDiger && !h.transfer {
				return h.kind
			}
			return transferKind(words, mt940)
		}
		return k
	}

	// 1. Counterparty IBAN is one of the taxpayer's own accounts.
	if iban != "" && c.OwnIBANs[iban] {
		return Classification{Kind: resolveKind("", true), Detail: DetailOwnTransfer, Source: SourceCounterparty,
			Confidence: 1.00, Reason: "own IBAN " + maskIBAN(iban)}
	}

	// 2. Bank code tables: MT940 first, then the bank's own code.
	if r, ok := mt940Codes[mt940]; ok && mt940 != "" {
		if res, final := applyCode(r, dir, words, SourceMT940Code); final {
			return res
		}
		h = &hint{kind: r.Kind, transfer: r.Transfer, source: SourceMT940Code, reason: r.Note}
	}
	if table, ok := bankCodes[txn.SourceSystem]; ok {
		code := strings.ToUpper(strings.TrimSpace(txn.BankTxnCode))
		if r, ok := table[code]; ok && code != "" {
			if res, final := applyCode(r, dir, words, SourceBankCode); final {
				return res
			}
			if h == nil || h.kind == "" || h.kind == KindDiger {
				h = &hint{kind: r.Kind, transfer: r.Transfer, source: SourceBankCode, reason: r.Note}
			}
		}
	}

	// 3. Payment purpose (only when the bank returns it).
	if p, ok := paymentPurposes[strings.ToUpper(strings.TrimSpace(txn.PaymentPurpose))]; ok {
		k := p.Kind
		if k == "" {
			k = resolveKind("", true)
		}
		return Classification{Kind: k, Detail: p.Detail, Source: SourcePaymentPurpose, Confidence: p.Confidence,
			Reason: "ödeme amacı " + strings.ToUpper(txn.PaymentPurpose)}
	}

	// 4. Learned taxpayer rules.
	for _, r := range c.Rules {
		if r.Dir != "" && r.Dir != dir {
			continue
		}
		hit := false
		switch r.MatchType {
		case "counterparty_iban":
			hit = iban != "" && iban == r.MatchValue
		case "counterparty_tax_id":
			hit = taxID != "" && taxID == r.MatchValue
		case "description":
			// Learned rules may name the counterparty, so its name counts here
			// (not in the keyword step, where a name like "X SIGORTA" would mislead).
			hit = containsPhrase(words, r.MatchValue) || containsPhrase(nameWords, r.MatchValue)
		}
		if hit {
			return Classification{Kind: r.Kind, Detail: r.Detail, Source: SourceCompanyRule, Confidence: 0.90,
				Reason: fmt.Sprintf("kural %s=%s", r.MatchType, maskValue(r.MatchType, r.MatchValue)), RuleID: r.ID}
		}
	}

	// 5. Counterparty is a known customer / supplier / employee.
	if cp, ok := findCounterparty(c.Counterparties, taxID, iban); ok {
		if d := detailFromCustomerType(cp.Type, dir); d != "" {
			return Classification{Kind: resolveKind("", true), Detail: d, Source: SourceCounterparty, Confidence: 0.90,
				Reason: "cari " + cp.Type}
		}
	}

	// 6. Keywords.
	for _, r := range keywordRules {
		if r.Dir != "" && r.Dir != dir {
			continue
		}
		matched := ""
		for _, p := range r.Patterns {
			if containsPhrase(words, p) {
				matched = p
				break
			}
		}
		if matched == "" {
			continue
		}
		kind := r.Kind
		if k, ok := r.KindByDir[dir]; ok {
			kind = k
		}
		if r.Transfer && r.Kind == "" {
			// "(transfer türü)": HAVALE / EFT / FAST from the code hint or the text.
			kind = resolveKind("", true)
		}
		detail := r.Detail
		if d, ok := r.ByDir[dir]; ok {
			detail = d
		}
		conf := 0.80
		if detail == "" {
			// "karşı tarafa göre" with no known counterparty: a guess by
			// direction, below the review threshold.
			detail = guessByDirection(dir)
			conf = 0.60
		}
		return Classification{Kind: kind, Detail: detail, Source: SourceKeyword, Confidence: conf, Reason: "kelime " + matched}
	}

	// A code hint without detail: keep the kind, guess the detail.
	if h != nil && h.kind != KindDiger {
		k := resolveKind(h.kind, h.transfer || h.kind == "")
		d := DetailUnknown
		if isTransfer(k) {
			d = guessByDirection(dir)
		}
		return Classification{Kind: k, Detail: d, Source: h.source, Confidence: 0.60, Reason: h.reason}
	}

	// 7. AI suggestion, masked and capped.
	if c.AI != nil {
		k, d, conf, err := c.AI.SuggestKind(ctx, MaskedTransaction{
			Direction: dir, Description: strings.Join(words, " "), CounterpartyName: txn.CounterpartyName,
			MT940Code: mt940, BankTxnCode: txn.BankTxnCode,
		})
		if err == nil && validKind(k) && validDetail(d) {
			if conf > AIConfidenceCap {
				conf = AIConfidenceCap
			}
			return Classification{Kind: k, Detail: d, Source: SourceAI, Confidence: conf, Reason: "AI önerisi"}
		}
	}

	res := Classification{Kind: KindDiger, Detail: DetailUnknown}
	if h != nil {
		res.Source, res.Reason = h.source, h.reason
	}
	return res
}

func applyCode(r codeRule, dir Direction, words []string, src Source) (Classification, bool) {
	if d, ok := r.ByDir[dir]; ok {
		return Classification{Kind: d.Kind, Detail: d.Detail, Source: src, Confidence: 0.95, Reason: r.Note}, true
	}
	if r.InterestInDescription && containsPhrase(words, "FAIZ") {
		return Classification{Kind: KindUcretKomisyonFaiz, Detail: DetailInterestIncome, Source: src, Confidence: 0.95, Reason: r.Note + " + FAIZ"}, true
	}
	if r.Detail == "" || r.Transfer {
		return Classification{}, false
	}
	detail := r.Detail
	reason := r.Note
	if r.BSMVInDescription && containsPhrase(words, "BSMV") {
		detail = DetailBSMV
		reason += " + BSMV"
	}
	return Classification{Kind: r.Kind, Detail: detail, Source: src, Confidence: 0.95, Reason: reason}, true
}

func findCounterparty(cps []Counterparty, taxID, iban string) (Counterparty, bool) {
	for _, cp := range cps {
		if taxID != "" && cp.TaxID == taxID {
			return cp, true
		}
	}
	if iban == "" {
		return Counterparty{}, false
	}
	for _, cp := range cps {
		for _, i := range cp.IBANs {
			if NormalizeIBAN(i) == iban {
				return cp, true
			}
		}
	}
	return Counterparty{}, false
}

func detailFromCustomerType(typ string, dir Direction) Detail {
	switch {
	case typ == "employee" && dir == DirOut:
		return DetailSalaryPayment
	case (typ == "supplier" || typ == "both") && dir == DirOut:
		return DetailSupplierPayment
	case (typ == "customer" || typ == "both") && dir == DirIn:
		return DetailCustomerCollection
	}
	return ""
}

func guessByDirection(dir Direction) Detail {
	if dir == DirIn {
		return DetailCustomerCollection
	}
	return DetailSupplierPayment
}

func isTransfer(k Kind) bool { return k == KindHavale || k == KindEFT || k == KindFAST }

var allKinds = map[Kind]bool{}
var allDetails = map[Detail]bool{}

func init() {
	for _, k := range []Kind{KindHavale, KindEFT, KindFAST, KindParaYatirma, KindParaCekme, KindYabanciParaHavale,
		KindYatirimaAktarim, KindYatirimdanAktarim, KindKurumFatura, KindCek, KindSenet, KindSigorta,
		KindUcretKomisyonFaiz, KindSGK, KindVergi, KindDovizAlim, KindDovizSatim, KindKrediOdemesi,
		KindKrediKullanim, KindKKOdemesi, KindKKNakitAvans, KindSansOyunu, KindUyeIsyeri, KindHGSOGS, KindDBS, KindDiger} {
		allKinds[k] = true
	}
	for _, d := range []Detail{DetailCustomerCollection, DetailSupplierPayment, DetailOwnTransfer, DetailBankFee,
		DetailCommission, DetailBSMV, DetailInterestExpense, DetailInterestIncome, DetailWithholdingTax,
		DetailLoanPrincipal, DetailLoanDisbursement, DetailTaxPayment, DetailSGKPayment, DetailSalaryPayment,
		DetailRentPayment, DetailUtilityPayment, DetailPOSSettlement, DetailPOSCommission, DetailInsurancePayment,
		DetailVehicleToll, DetailFXTrade, DetailCashDeposit, DetailCashWithdrawal, DetailPrivate, DetailUnknown} {
		allDetails[d] = true
	}
}

func validKind(k Kind) bool     { return allKinds[k] }
func validDetail(d Detail) bool { return allDetails[d] }

// ValidKind / ValidDetail check manual corrections.
func ValidKind(k Kind) bool     { return validKind(k) }
func ValidDetail(d Detail) bool { return validDetail(d) }

func maskIBAN(iban string) string {
	if len(iban) < 8 {
		return "****"
	}
	return iban[:4] + "…" + iban[len(iban)-4:]
}

func maskValue(matchType, v string) string {
	switch matchType {
	case "counterparty_iban":
		return maskIBAN(v)
	case "counterparty_tax_id":
		if len(v) > 4 {
			return strings.Repeat("*", len(v)-4) + v[len(v)-4:]
		}
	}
	return v
}
